package conversation

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type EventSink interface {
	ConversationCreated(ctx context.Context, conv *Conversation) error
	ConversationHumanRequested(ctx context.Context, conv *Conversation) error
	ConversationHumanStarted(ctx context.Context, conv *Conversation) error
	ConversationHumanResolved(ctx context.Context, conv *Conversation) error
	ConversationAIResumed(ctx context.Context, conv *Conversation) error
}

type UserReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*tenant.User, error)
}

type MembershipReader interface {
	GetActiveAccess(ctx context.Context, userID, businessID uuid.UUID) (*tenant.Access, error)
}

// Service applies conversation and message rules. The business id must come from tenant context.
type Service struct {
	store      Store
	users      UserReader
	members    MembershipReader
	events     EventSink
	onQueued      func(ctx context.Context, businessID, messageID uuid.UUID) error
	onDelivery    func(ctx context.Context, businessID, messageID uuid.UUID, status MessageStatus) error
	onInboundRead func(ctx context.Context, businessID, conversationID uuid.UUID, externalIDs []string) error
}

func (s *Service) SetEvents(events EventSink) {
	s.events = events
}

func (s *Service) SetOnQueuedOutbound(hook func(ctx context.Context, businessID, messageID uuid.UUID) error) {
	s.onQueued = hook
}

func (s *Service) SetOnDeliveryStatus(hook func(ctx context.Context, businessID, messageID uuid.UUID, status MessageStatus) error) {
	s.onDelivery = hook
}

func (s *Service) SetOnInboundRead(hook func(ctx context.Context, businessID, conversationID uuid.UUID, externalIDs []string) error) {
	s.onInboundRead = hook
}

func NewService(store Store, users UserReader, members MembershipReader) *Service {
	return &Service{store: store, users: users, members: members}
}

func (s *Service) ListConversations(ctx context.Context, businessID uuid.UUID, statusRaw, channelRaw, assignedRaw, customerRaw, search, unreadRaw, limitRaw, offsetRaw string) ([]Conversation, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	channel, err := parseChannelFilter(channelRaw)
	if err != nil {
		return nil, err
	}
	assignedUserID, err := parseOptionalUUID(assignedRaw)
	if err != nil {
		return nil, invalid("assigned_user_id is invalid")
	}
	customerID, err := parseOptionalUUID(customerRaw)
	if err != nil {
		return nil, invalid("customer_id is invalid")
	}
	unreadOnly, err := parseUnread(unreadRaw)
	if err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListConversations(ctx, businessID, ListParams{
		Status:         status,
		Channel:        channel,
		AssignedUserID: assignedUserID,
		CustomerID:     customerID,
		Search:         likePattern(search),
		UnreadOnly:     unreadOnly,
		Limit:          limit,
		Offset:         offset,
	})
}

func (s *Service) GetConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	return s.store.GetConversation(ctx, businessID, conversationID)
}

func (s *Service) CreateConversation(ctx context.Context, businessID uuid.UUID, in CreateConversationInput) (*Conversation, bool, error) {
	customerID, err := uuid.Parse(strings.TrimSpace(in.CustomerID))
	if err != nil {
		return nil, false, invalid("customer_id is invalid")
	}
	channel, err := parseChannel(in.Channel)
	if err != nil {
		return nil, false, err
	}
	var identityID *uuid.UUID
	if in.CustomerIdentityID != nil && strings.TrimSpace(*in.CustomerIdentityID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*in.CustomerIdentityID))
		if err != nil {
			return nil, false, invalid("customer_identity_id is invalid")
		}
		identityID = &parsed
	}

	customer, err := s.store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return nil, false, err
	}
	if customer.Status != string(tenant.StatusActive) {
		return nil, false, ErrCustomerInactive
	}
	if identityID != nil {
		identity, err := s.store.GetIdentity(ctx, businessID, *identityID)
		if err != nil {
			return nil, false, err
		}
		if identity.Status != string(tenant.StatusActive) {
			return nil, false, ErrIdentityNotFound
		}
		if identity.CustomerID != customerID {
			return nil, false, ErrIdentityMismatch
		}
		if identity.Channel != channel {
			return nil, false, ErrChannelMismatch
		}
	}

	existing, err := s.store.FindOpenConversation(ctx, businessID, customerID, channel, identityID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	created, err := s.store.CreateConversation(ctx, Conversation{
		BusinessID:         businessID,
		CustomerID:         customerID,
		Channel:            channel,
		CustomerIdentityID: identityID,
		Status:             StatusOpen,
		HandlingState:      HandlingAIActive,
	})
	if errors.Is(err, ErrOpenConversationExists) {
		existing, findErr := s.store.FindOpenConversation(ctx, businessID, customerID, channel, identityID)
		if findErr != nil {
			return nil, false, findErr
		}
		if existing != nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	if err != nil {
		return nil, false, err
	}
	if s.events != nil && !in.SuppressAutomation {
		if pubErr := s.events.ConversationCreated(ctx, created); pubErr != nil {
			slog.Default().Warn("automation conversation.created failed", "error", pubErr)
		}
	}
	return created, true, nil
}

func (s *Service) UpdateConversation(ctx context.Context, businessID, conversationID uuid.UUID, in UpdateConversationInput) (*Conversation, error) {
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.Status != nil {
		next.Status = *in.Status
	}
	if in.ClearAssignedUser {
		next.AssignedUserID = nil
		next.AssignedUser = nil
	} else if in.AssignedUserID != nil {
		if err := s.validateAssignee(ctx, businessID, *in.AssignedUserID); err != nil {
			return nil, err
		}
		next.AssignedUserID = in.AssignedUserID
	}
	return s.store.UpdateConversation(ctx, businessID, next)
}

func (s *Service) MarkRead(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	var (
		out         *Conversation
		externalIDs []string
	)
	err := s.store.RunInTx(ctx, func(store Store) error {
		if _, err := store.GetConversation(ctx, businessID, conversationID); err != nil {
			return err
		}
		updated, err := store.MarkRead(ctx, businessID, conversationID)
		if err != nil {
			return err
		}
		ids, err := store.MarkInboundRead(ctx, businessID, conversationID)
		if err != nil {
			return err
		}
		out = updated
		externalIDs = ids
		return nil
	})
	if err == nil && s.onInboundRead != nil && len(externalIDs) > 0 {
		if hookErr := s.onInboundRead(ctx, businessID, conversationID, externalIDs); hookErr != nil {
			slog.Default().Warn("inbound read receipt failed", "error", hookErr, "conversation_id", conversationID)
		}
	}
	return out, err
}

func (s *Service) ListMessages(ctx context.Context, businessID, conversationID uuid.UUID, limitRaw, offsetRaw, beforeRaw, afterRaw string) ([]Message, error) {
	if _, err := s.store.GetConversation(ctx, businessID, conversationID); err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	before, err := parseOptionalTime(beforeRaw)
	if err != nil {
		return nil, err
	}
	after, err := parseOptionalTime(afterRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListMessages(ctx, businessID, conversationID, MessageListParams{
		Limit:  limit,
		Offset: offset,
		Before: before,
		After:  after,
	})
}

func (s *Service) ListRecentMessages(ctx context.Context, businessID, conversationID uuid.UUID, limit int) ([]Message, error) {
	if _, err := s.store.GetConversation(ctx, businessID, conversationID); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return s.store.ListRecentMessages(ctx, businessID, conversationID, limit)
}

func (s *Service) LatestConversation(ctx context.Context, businessID, customerID uuid.UUID, channelRaw string, identityID *uuid.UUID) (*Conversation, error) {
	channel, err := parseChannel(channelRaw)
	if err != nil {
		return nil, err
	}
	return s.store.FindLatestConversation(ctx, businessID, customerID, channel, identityID)
}

func (s *Service) GetMessage(ctx context.Context, businessID, messageID uuid.UUID) (*Message, error) {
	return s.store.GetMessage(ctx, businessID, messageID)
}

func (s *Service) FindMessageByExternalID(ctx context.Context, businessID uuid.UUID, externalID string) (*Message, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return nil, invalid("external_id is required")
	}
	return s.store.FindMessageByExternalID(ctx, businessID, externalID)
}

func (s *Service) ClaimForDelivery(ctx context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*Message, error) {
	return s.store.ClaimForDelivery(ctx, businessID, messageID, staleAfter)
}

func (s *Service) ApplyDeliveryStatus(ctx context.Context, businessID, messageID uuid.UUID, statusRaw string, externalID *string) (*Message, bool, error) {
	status, err := parseMessageStatus(statusRaw)
	if err != nil || status == "" {
		return nil, false, ErrInvalidMessageStatus
	}
	current, err := s.store.GetMessage(ctx, businessID, messageID)
	if err != nil {
		return nil, false, err
	}
	next, ok := nextDeliveryStatus(current.Status, status)
	if !ok {
		return current, false, nil
	}
	normalized, err := normalizeExternalID(externalID)
	if err != nil {
		return nil, false, err
	}
	updated, err := s.store.UpdateMessageDelivery(ctx, businessID, messageID, next, normalized)
	if err == nil && updated != nil && s.onDelivery != nil {
		_ = s.onDelivery(ctx, businessID, updated.ID, updated.Status)
	}
	return updated, true, err
}

func nextDeliveryStatus(current, incoming MessageStatus) (MessageStatus, bool) {
	if incoming == current {
		return current, false
	}
	if current == MessageFailed || current == MessageRead {
		return current, false
	}
	switch incoming {
	case MessageFailed:
		if current == MessageQueued || current == MessageSending || current == MessageSent {
			return MessageFailed, true
		}
		return current, false
	case MessageQueued:
		if current == MessageSending {
			return MessageQueued, true
		}
		return current, false
	case MessageSending:
		if current == MessageQueued {
			return MessageSending, true
		}
		return current, false
	case MessageSent:
		if current == MessageQueued || current == MessageSending {
			return MessageSent, true
		}
		return current, false
	case MessageDelivered:
		if current == MessageQueued || current == MessageSending || current == MessageSent {
			return MessageDelivered, true
		}
		return current, false
	case MessageRead:
		if current == MessageReceived || current == MessageQueued || current == MessageSending || current == MessageSent || current == MessageDelivered {
			return MessageRead, true
		}
		return current, false
	default:
		return current, false
	}
}

func (s *Service) CreateMessage(ctx context.Context, businessID, conversationID uuid.UUID, in CreateMessageInput) (*Message, bool, error) {
	direction, err := parseDirection(in.Direction)
	if err != nil {
		return nil, false, err
	}
	sender, err := parseSenderType(in.SenderType)
	if err != nil {
		return nil, false, err
	}
	if sender == SenderCampaign && !in.AllowCampaign {
		return nil, false, invalid("campaign messages cannot be created through this endpoint")
	}
	if err := validateDirectionSender(direction, sender); err != nil {
		return nil, false, err
	}
	messageType, err := parseMessageType(in.MessageType)
	if err != nil {
		return nil, false, err
	}
	if messageType == MessageTypeInternalNote {
		return nil, false, invalid("internal notes must use the notes endpoint")
	}
	status, err := parseMessageStatus(in.Status)
	if err != nil {
		return nil, false, err
	}
	if status == "" {
		status = defaultMessageStatus(direction)
	}
	content, err := normalizeContent(in.Content, messageType)
	if err != nil {
		return nil, false, err
	}
	externalID, err := normalizeExternalID(in.ExternalID)
	if err != nil {
		return nil, false, err
	}
	metadata, err := normalizeMetadata(in.Metadata)
	if err != nil {
		return nil, false, err
	}

	var (
		out     *Message
		created bool
	)
	err = s.store.RunInTx(ctx, func(store Store) error {
		conv, err := store.LockConversation(ctx, businessID, conversationID)
		if err != nil {
			return err
		}
		if externalID != nil {
			existing, err := store.GetMessageByExternalID(ctx, businessID, conversationID, *externalID)
			if err != nil {
				return err
			}
			if existing != nil {
				out = existing
				created = false
				return nil
			}
		}

		msg, err := store.CreateMessage(ctx, Message{
			BusinessID:     businessID,
			ConversationID: conv.ID,
			Direction:      direction,
			SenderType:     sender,
			MessageType:    messageType,
			Content:        content,
			ExternalID:     externalID,
			Status:         status,
			Metadata:       metadata,
		})
		if err != nil {
			return err
		}

		unreadDelta := 0
		if direction == DirectionInbound && sender == SenderCustomer {
			unreadDelta = 1
		}
		if err := store.ApplyMessageSideEffects(ctx, businessID, conv.ID, msg.CreatedAt, unreadDelta, conv.Status == StatusClosed); err != nil {
			return err
		}
		out = msg
		created = true
		return nil
	})
	if errors.Is(err, ErrDuplicateExternalID) && externalID != nil {
		existing, findErr := s.store.GetMessageByExternalID(ctx, businessID, conversationID, *externalID)
		if findErr != nil {
			return nil, false, findErr
		}
		if existing != nil {
			return existing, false, nil
		}
	}
	if created && out != nil && s.onQueued != nil && out.Direction == DirectionOutbound && out.Status == MessageQueued {
		if qerr := s.onQueued(ctx, businessID, out.ID); qerr != nil {
			return out, created, qerr
		}
	}
	return out, created, err
}

func (s *Service) validateAssignee(ctx context.Context, businessID, userID uuid.UUID) error {
	if s.users == nil {
		return ErrAssignedUserNotFound
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) {
			return ErrAssignedUserNotFound
		}
		return err
	}
	if user.Status != tenant.StatusActive {
		return ErrAssignedUserInactive
	}
	if s.members == nil {
		return ErrAssignedUserNotMember
	}
	if _, err := s.members.GetActiveAccess(ctx, user.ID, businessID); err != nil {
		if errors.Is(err, tenant.ErrNotFound) {
			return ErrAssignedUserNotMember
		}
		return err
	}
	return nil
}
