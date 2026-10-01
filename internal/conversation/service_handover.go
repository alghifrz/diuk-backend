package conversation

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) ListHumanInbox(ctx context.Context, businessID uuid.UUID, stateRaw, limitRaw, offsetRaw string) ([]HumanInboxItem, error) {
	states, err := parseHumanInboxStates(stateRaw)
	if err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListHumanInbox(ctx, businessID, states, limit, offset)
}

func (s *Service) RequestHumanHandover(ctx context.Context, businessID, conversationID uuid.UUID, reasonRaw string) (*Conversation, error) {
	reason, err := parseHandoverReason(reasonRaw)
	if err != nil {
		return nil, err
	}
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	state := normalizeHandling(current.HandlingState)
	if state == HandlingHumanRequested || state == HandlingHumanActive {
		return current, nil
	}
	if !canTransition(state, HandlingHumanRequested) {
		return nil, ErrInvalidHandlingTransition
	}
	now := time.Now().UTC()
	next := *current
	next.HandlingState = HandlingHumanRequested
	next.HandoverReason = &reason
	next.HandoverRequestedAt = &now
	next.ResolvedAt = nil
	updated, err := s.store.UpdateConversation(ctx, businessID, next)
	if err != nil {
		return nil, err
	}
	s.emitHandling(ctx, "human_requested", updated)
	return updated, nil
}

func (s *Service) TakeoverConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	state := normalizeHandling(current.HandlingState)
	if state == HandlingHumanActive {
		return current, nil
	}
	if !canTransition(state, HandlingHumanActive) {
		return nil, ErrInvalidHandlingTransition
	}
	now := time.Now().UTC()
	next := *current
	next.HandlingState = HandlingHumanActive
	if next.HumanStartedAt == nil {
		next.HumanStartedAt = &now
	}
	updated, err := s.store.UpdateConversation(ctx, businessID, next)
	if err != nil {
		return nil, err
	}
	s.emitHandling(ctx, "human_started", updated)
	return updated, nil
}

func (s *Service) ResolveConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	state := normalizeHandling(current.HandlingState)
	if state == HandlingResolved {
		return current, nil
	}
	if !canTransition(state, HandlingResolved) {
		return nil, ErrInvalidHandlingTransition
	}
	now := time.Now().UTC()
	next := *current
	next.HandlingState = HandlingResolved
	next.ResolvedAt = &now
	updated, err := s.store.UpdateConversation(ctx, businessID, next)
	if err != nil {
		return nil, err
	}
	s.emitHandling(ctx, "human_resolved", updated)
	return updated, nil
}

func (s *Service) ResumeAI(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	state := normalizeHandling(current.HandlingState)
	if state == HandlingAIActive {
		return current, nil
	}
	if !canTransition(state, HandlingAIActive) {
		return nil, ErrInvalidHandlingTransition
	}
	next := *current
	next.HandlingState = HandlingAIActive
	updated, err := s.store.UpdateConversation(ctx, businessID, next)
	if err != nil {
		return nil, err
	}
	s.emitHandling(ctx, "ai_resumed", updated)
	return updated, nil
}

func (s *Service) Reply(ctx context.Context, businessID, conversationID, userID uuid.UUID, content string) (*Message, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, invalid("content is required")
	}
	current, err := s.store.GetConversation(ctx, businessID, conversationID)
	if err != nil {
		return nil, err
	}
	state := normalizeHandling(current.HandlingState)
	if state == HandlingAIActive || state == HandlingResolved {
		return nil, ErrInvalidHandlingTransition
	}
	if state == HandlingHumanRequested {
		if _, err := s.TakeoverConversation(ctx, businessID, conversationID); err != nil {
			return nil, err
		}
	}
	meta, _ := json.Marshal(map[string]any{
		"source":  "human_reply",
		"user_id": userID.String(),
	})
	msg, created, err := s.CreateMessage(ctx, businessID, conversationID, CreateMessageInput{
		Direction:   string(DirectionOutbound),
		SenderType:  string(SenderUser),
		MessageType: string(MessageTypeText),
		Content:     &trimmed,
		Status:      string(MessageQueued),
		Metadata:    meta,
	})
	if err != nil {
		return nil, err
	}
	_ = created
	return msg, nil
}

func (s *Service) CreateNote(ctx context.Context, businessID, conversationID, userID uuid.UUID, content string) (*Message, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, invalid("content is required")
	}
	if _, err := s.store.GetConversation(ctx, businessID, conversationID); err != nil {
		return nil, err
	}
	meta, _ := json.Marshal(map[string]any{
		"source":  "internal_note",
		"user_id": userID.String(),
	})
	note := trimmed
	return s.store.CreateMessage(ctx, Message{
		BusinessID:     businessID,
		ConversationID: conversationID,
		Direction:      DirectionOutbound,
		SenderType:     SenderUser,
		MessageType:    MessageTypeInternalNote,
		Content:        &note,
		Status:         MessageInternal,
		Metadata:       meta,
	})
}

func (s *Service) emitHandling(ctx context.Context, kind string, conv *Conversation) {
	if s.events == nil || conv == nil {
		return
	}
	var err error
	switch kind {
	case "human_requested":
		err = s.events.ConversationHumanRequested(ctx, conv)
	case "human_started":
		err = s.events.ConversationHumanStarted(ctx, conv)
	case "human_resolved":
		err = s.events.ConversationHumanResolved(ctx, conv)
	case "ai_resumed":
		err = s.events.ConversationAIResumed(ctx, conv)
	}
	if err != nil {
		slog.Default().Warn("automation conversation handling event failed", "event", kind, "error", err)
	}
}
