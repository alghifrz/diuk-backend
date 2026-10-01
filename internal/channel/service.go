package channel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/channel/provider"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

type MetaConfig struct {
	AccessToken    string
	AppSecret      string
	VerifyToken    string
	APIVersion     string
	BaseURL        string
	Timeout        time.Duration
	MaxWebhookBody int64
}

type Service struct {
	log           *slog.Logger
	store         Store
	customers     CustomerAPI
	conversations ConversationAPI
	ai            AIService
	trigger       ReplyTrigger
	sender        provider.MessagingProvider
	meta          MetaConfig
	metrics       DeliveryMetrics
	meter         usage.Meter
}

type DeliveryMetrics interface {
	ObserveDelivery(channel, operation, result string, duration time.Duration)
}

func NewService(
	log *slog.Logger,
	store Store,
	customers CustomerAPI,
	conversations ConversationAPI,
	ai AIService,
	sender provider.MessagingProvider,
	meta MetaConfig,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if meta.Timeout <= 0 {
		meta.Timeout = 10 * time.Second
	}
	if meta.APIVersion == "" {
		meta.APIVersion = "v21.0"
	}
	if meta.BaseURL == "" {
		meta.BaseURL = "https://graph.facebook.com"
	}
	s := &Service{
		log:           log,
		store:         store,
		customers:     customers,
		conversations: conversations,
		ai:            ai,
		sender:        sender,
		meta:          meta,
	}
	s.trigger = AsyncTrigger{Service: s, Timeout: 90 * time.Second}
	return s
}

func (s *Service) SetTrigger(trigger ReplyTrigger) {
	s.trigger = trigger
}

func (s *Service) SetMetrics(metrics DeliveryMetrics) {
	s.metrics = metrics
}

func (s *Service) SetUsageMeter(meter usage.Meter) {
	s.meter = meter
}

func (s *Service) ListChannels(ctx context.Context, businessID uuid.UUID) ([]BusinessChannel, error) {
	return s.store.List(ctx, businessID)
}

func (s *Service) GetChannel(ctx context.Context, businessID, channelID uuid.UUID) (*BusinessChannel, error) {
	return s.store.Get(ctx, businessID, channelID)
}

func (s *Service) CreateChannel(ctx context.Context, businessID uuid.UUID, in CreateChannelInput) (*BusinessChannel, error) {
	ch, err := parseChannel(in.Channel)
	if err != nil {
		return nil, err
	}
	status, err := parseChannelStatus(in.Status)
	if err != nil {
		return nil, err
	}
	account, err := normalizeExternalRef(in.ExternalAccountID, "external_account_id")
	if err != nil {
		return nil, err
	}
	phone, err := normalizeExternalRef(in.ExternalPhoneNumberID, "external_phone_number_id")
	if err != nil {
		return nil, err
	}
	if err := requirePhoneNumberID(ch, phone); err != nil {
		return nil, err
	}
	if s.meter != nil && ch == ChannelWhatsApp {
		if err := s.meter.CheckResource(ctx, businessID, usage.FeatureWhatsAppChannels); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) || errors.Is(err, usage.ErrFeatureDisabled) || errors.Is(err, usage.ErrNotEntitled) || errors.Is(err, usage.ErrPlanInactive) {
				return nil, ErrQuotaExceeded
			}
			return nil, err
		}
	}
	return s.store.Create(ctx, BusinessChannel{
		BusinessID:            businessID,
		Channel:               ch,
		Status:                status,
		DisplayName:           normalizeOptionalName(in.DisplayName),
		ExternalAccountID:     account,
		ExternalPhoneNumberID: phone,
	})
}

func (s *Service) UpdateChannel(ctx context.Context, businessID, channelID uuid.UUID, in UpdateChannelInput) (*BusinessChannel, error) {
	current, err := s.store.Get(ctx, businessID, channelID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.DisplayName != nil {
		next.DisplayName = normalizeOptionalName(in.DisplayName)
	}
	if in.ExternalAccountID != nil {
		account, err := normalizeExternalRef(in.ExternalAccountID, "external_account_id")
		if err != nil {
			return nil, err
		}
		next.ExternalAccountID = account
	}
	if in.ExternalPhoneNumberID != nil {
		phone, err := normalizeExternalRef(in.ExternalPhoneNumberID, "external_phone_number_id")
		if err != nil {
			return nil, err
		}
		next.ExternalPhoneNumberID = phone
	}
	if in.Status != nil {
		status, err := parseChannelStatus(*in.Status)
		if err != nil {
			return nil, err
		}
		next.Status = status
	}
	if err := requirePhoneNumberID(next.Channel, next.ExternalPhoneNumberID); err != nil && next.Status == StatusActive {
		return nil, err
	}
	return s.store.Update(ctx, businessID, next)
}

func (s *Service) DeleteChannel(ctx context.Context, businessID, channelID uuid.UUID) (*BusinessChannel, error) {
	current, err := s.store.Get(ctx, businessID, channelID)
	if err != nil {
		return nil, err
	}
	current.Status = StatusInactive
	return s.store.Update(ctx, businessID, *current)
}

func (s *Service) ResolveByPhoneNumberID(ctx context.Context, phoneNumberID string) (*BusinessChannel, error) {
	phoneNumberID = strings.TrimSpace(phoneNumberID)
	if phoneNumberID == "" {
		return nil, ErrInvalidWebhook
	}
	item, err := s.store.FindByPhoneNumberID(ctx, ChannelWhatsApp, phoneNumberID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrChannelNotFound
	}
	if item.Status != StatusActive {
		return nil, ErrChannelDisabled
	}
	return item, nil
}

func (s *Service) HandleIncoming(ctx context.Context, msg IncomingMessage) (*HandleResult, error) {
	if msg.MessageType != MessageTypeText {
		s.log.Info("channel unsupported inbound",
			"channel", msg.Channel,
			"event_type", msg.MessageType,
			"external_message_id", msg.ExternalMessageID,
		)
		return &HandleResult{Unsupported: true}, nil
	}
	if strings.TrimSpace(msg.ExternalAccountID) == "" || strings.TrimSpace(msg.ExternalIdentityID) == "" || strings.TrimSpace(msg.ExternalMessageID) == "" {
		return nil, ErrInvalidWebhook
	}
	cfg, err := s.ResolveByPhoneNumberID(ctx, msg.ExternalAccountID)
	if err != nil {
		return nil, err
	}

	identity, err := s.resolveIdentity(ctx, cfg.BusinessID, msg)
	if err != nil {
		return nil, err
	}
	conv, err := s.resolveConversation(ctx, cfg.BusinessID, identity, msg.Channel)
	if err != nil {
		return nil, err
	}

	content := msg.Text
	createdMsg, created, err := s.conversations.CreateMessage(ctx, cfg.BusinessID, conv.ID, conversation.CreateMessageInput{
		Direction:   string(conversation.DirectionInbound),
		SenderType:  string(conversation.SenderCustomer),
		MessageType: string(conversation.MessageTypeText),
		Content:     &content,
		ExternalID:  &msg.ExternalMessageID,
		Status:      string(conversation.MessageReceived),
	})
	if err != nil {
		return nil, err
	}
	result := &HandleResult{
		BusinessID:     cfg.BusinessID,
		ConversationID: conv.ID,
		MessageID:      createdMsg.ID,
		Created:        created,
		Duplicate:      !created,
	}
	s.log.Info("channel inbound handled",
		"channel", msg.Channel,
		"business_id", cfg.BusinessID,
		"conversation_id", conv.ID,
		"message_id", createdMsg.ID,
		"external_message_id", msg.ExternalMessageID,
		"duplicate", !created,
	)
	if created && s.trigger != nil {
		latest := conv
		if refreshed, err := s.conversations.GetConversation(ctx, cfg.BusinessID, conv.ID); err == nil && refreshed != nil {
			latest = refreshed
		}
		if conversation.AllowsAIGeneration(latest.HandlingState) {
			s.trigger.Trigger(ctx, cfg.BusinessID, conv.ID, createdMsg.ID)
		}
	}
	return result, nil
}

func (s *Service) HandleStatus(ctx context.Context, update StatusUpdate) error {
	if strings.TrimSpace(update.ExternalAccountID) == "" || strings.TrimSpace(update.ExternalMessageID) == "" {
		return ErrInvalidWebhook
	}
	cfg, err := s.ResolveByPhoneNumberID(ctx, update.ExternalAccountID)
	if err != nil {
		return err
	}
	msg, err := s.conversations.FindMessageByExternalID(ctx, cfg.BusinessID, update.ExternalMessageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}
	_, _, err = s.conversations.ApplyDeliveryStatus(ctx, cfg.BusinessID, msg.ID, update.Status, nil)
	if err != nil {
		return err
	}
	s.log.Info("channel status applied",
		"channel", update.Channel,
		"business_id", cfg.BusinessID,
		"message_id", msg.ID,
		"external_message_id", update.ExternalMessageID,
		"status", update.Status,
	)
	return nil
}

func (s *Service) NotifyInboundRead(ctx context.Context, businessID, conversationID uuid.UUID, externalIDs []string) error {
	if s.sender == nil || strings.TrimSpace(s.meta.AccessToken) == "" {
		return nil
	}
	latest := ""
	for i := len(externalIDs) - 1; i >= 0; i-- {
		if id := strings.TrimSpace(externalIDs[i]); id != "" {
			latest = id
			break
		}
	}
	if latest == "" {
		return nil
	}
	conv, err := s.conversations.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return err
	}
	if conv.Channel != conversation.ChannelWhatsApp {
		return nil
	}
	cfg, err := s.store.FindActive(ctx, businessID, string(conv.Channel))
	if err != nil || cfg == nil || cfg.Status != StatusActive || cfg.ExternalPhoneNumberID == nil {
		return nil
	}
	if err := s.sender.MarkRead(ctx, provider.MarkReadInput{
		PhoneNumberID: *cfg.ExternalPhoneNumberID,
		MessageID:     latest,
		AccessToken:   s.meta.AccessToken,
		BaseURL:       s.meta.BaseURL,
		APIVersion:    s.meta.APIVersion,
	}); err != nil {
		s.log.Warn("whatsapp mark read failed",
			"error", sanitizeProviderError(err),
			"conversation_id", conversationID,
		)
	}
	return nil
}

func (s *Service) DeliverMessage(ctx context.Context, businessID, messageID uuid.UUID) error {
	msg, err := s.conversations.ClaimForDelivery(ctx, businessID, messageID, 2*time.Minute)
	if err != nil {
		if errors.Is(err, conversation.ErrMessageNotFound) {
			return ErrMessageNotFound
		}
		return err
	}
	if msg == nil {
		return nil
	}
	if msg.MessageType == conversation.MessageTypeInternalNote || msg.Status == conversation.MessageInternal {
		return nil
	}
	kind, templateName, templateLang, templateParams := campaignSendMeta(msg.Metadata)
	if msg.Direction != conversation.DirectionOutbound || msg.MessageType != conversation.MessageTypeText {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrUnsupportedMessage
	}
	if kind != "TEMPLATE" && msg.Content == nil {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrUnsupportedMessage
	}

	conv, err := s.conversations.GetConversation(ctx, businessID, msg.ConversationID)
	if err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			return ErrConversationNotFound
		}
		return err
	}
	cfg, err := s.store.FindActive(ctx, businessID, string(conv.Channel))
	if err != nil {
		return err
	}
	if cfg == nil {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrChannelNotFound
	}
	if cfg.Status != StatusActive {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrChannelDisabled
	}
	if s.sender == nil || strings.TrimSpace(s.meta.AccessToken) == "" {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrProviderUnavailable
	}
	to := ""
	if conv.Identity != nil {
		to = conv.Identity.ExternalID
	}
	if to == "" {
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return invalid("conversation identity is missing")
	}
	phoneID := ""
	if cfg.ExternalPhoneNumberID != nil {
		phoneID = *cfg.ExternalPhoneNumberID
	}

	sendStarted := time.Now()
	var sent provider.SendResult
	if kind == "TEMPLATE" {
		sent, err = s.sender.SendTemplate(ctx, provider.SendTemplateInput{
			PhoneNumberID: phoneID, To: to, AccessToken: s.meta.AccessToken,
			BaseURL: s.meta.BaseURL, APIVersion: s.meta.APIVersion,
			TemplateName: templateName, Language: templateLang, Parameters: templateParams,
		})
	} else {
		sent, err = s.sender.SendText(ctx, provider.SendTextInput{
			PhoneNumberID: phoneID, To: to, Body: *msg.Content,
			AccessToken: s.meta.AccessToken, BaseURL: s.meta.BaseURL, APIVersion: s.meta.APIVersion,
		})
	}
	if s.metrics != nil {
		result := "success"
		if err != nil {
			result = "failure"
		}
		s.metrics.ObserveDelivery(strings.ToLower(string(conv.Channel)), "send", result, time.Since(sendStarted))
	}
	if err != nil {
		s.log.Error("channel delivery failed",
			"channel", conv.Channel,
			"business_id", businessID,
			"conversation_id", conv.ID,
			"message_id", msg.ID,
			"error", sanitizeProviderError(err),
		)
		if after, ok := retryAfterFrom(err); ok {
			_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageQueued), nil)
			return RateLimitedError{After: after}
		}
		if isTransientSendError(err) {
			_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageQueued), nil)
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return ErrProviderUnavailable
			}
			return ErrProviderUnavailable
		}
		_, _, _ = s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageFailed), nil)
		return ErrProviderRejected
	}
	providerID := sent.ProviderMessageID
	if _, _, err := s.conversations.ApplyDeliveryStatus(ctx, businessID, msg.ID, string(conversation.MessageSent), &providerID); err != nil {
		return err
	}
	s.log.Info("channel delivery sent",
		"channel", conv.Channel,
		"business_id", businessID,
		"conversation_id", conv.ID,
		"message_id", msg.ID,
		"external_message_id", sent.ProviderMessageID,
		"status", "SENT",
	)
	return nil
}

func (s *Service) GenerateAndDeliver(ctx context.Context, businessID, conversationID uuid.UUID) error {
	if s.ai == nil {
		return nil
	}
	result, err := s.ai.GenerateReply(ctx, businessID, ai.GenerateReplyInput{
		ConversationID:  conversationID,
		PersistIncoming: false,
	})
	if err != nil {
		if errors.Is(err, ai.ErrHumanHandling) {
			return nil
		}
		s.log.Error("channel ai generation failed",
			"business_id", businessID,
			"conversation_id", conversationID,
			"error", err.Error(),
		)
		return err
	}
	return s.DeliverMessage(ctx, businessID, result.MessageID)
}

func (s *Service) resolveIdentity(ctx context.Context, businessID uuid.UUID, msg IncomingMessage) (*customer.Identity, error) {
	name := strings.TrimSpace(msg.CustomerName)
	if name == "" {
		name = strings.TrimSpace(msg.ExternalIdentityID)
	}
	identity, _, err := s.customers.EnsureIdentity(ctx, businessID, customer.EnsureIdentityInput{
		Channel:     msg.Channel,
		ExternalID:  msg.ExternalIdentityID,
		Name:        name,
		Phone:       optional(strings.TrimSpace(msg.CustomerPhone)),
		DisplayName: optional(name),
	})
	return identity, err
}

func (s *Service) resolveConversation(ctx context.Context, businessID uuid.UUID, identity *customer.Identity, channelName string) (*conversation.Conversation, error) {
	latest, err := s.conversations.LatestConversation(ctx, businessID, identity.CustomerID, channelName, &identity.ID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		return latest, nil
	}
	ident := identity.ID.String()
	created, _, err := s.conversations.CreateConversation(ctx, businessID, conversation.CreateConversationInput{
		CustomerID:         identity.CustomerID.String(),
		CustomerIdentityID: &ident,
		Channel:            channelName,
	})
	return created, err
}

func optional(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func sanitizeProviderError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type retryAfterer interface {
	RetryAfter() time.Duration
}

func retryAfterFrom(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	if errors.Is(err, ErrRateLimited) {
		var ra retryAfterer
		if errors.As(err, &ra) {
			return ra.RetryAfter(), true
		}
		return 0, true
	}
	var ra retryAfterer
	if errors.As(err, &ra) && ra.RetryAfter() > 0 {
		return ra.RetryAfter(), true
	}
	return 0, false
}

func isTransientSendError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrRateLimited) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "http 5") || strings.Contains(msg, "unavailable") || strings.Contains(msg, "rate limited")
}

func campaignSendMeta(raw json.RawMessage) (kind, name, language string, params []string) {
	if len(raw) == 0 {
		return "", "", "", nil
	}
	var meta struct {
		MessageKind        string   `json:"message_kind"`
		TemplateName       string   `json:"template_name"`
		TemplateLanguage   string   `json:"template_language"`
		TemplateParameters []string `json:"template_parameters"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", "", "", nil
	}
	return strings.ToUpper(strings.TrimSpace(meta.MessageKind)), strings.TrimSpace(meta.TemplateName), strings.TrimSpace(meta.TemplateLanguage), meta.TemplateParameters
}
