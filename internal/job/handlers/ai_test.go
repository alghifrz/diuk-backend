package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/job"
)

func TestAIHandlerIdempotentAndSuccess(t *testing.T) {
	biz, convID, inbound := uuid.New(), uuid.New(), uuid.New()
	outbound := uuid.New()
	store := job.NewMemoryStore()
	gen := &stubAI{id: outbound}
	msgs := &stubMessages{msg: &conversation.Message{
		ID: inbound, BusinessID: biz, ConversationID: convID,
		Direction: conversation.DirectionInbound, Status: conversation.MessageReceived,
	}}
	h := NewAIHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), gen, msgs, store)
	j, _ := job.NewAIGenerate(biz, convID, inbound, 3)
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("first: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("calls = %d", gen.calls)
	}
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("dup: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("duplicate generated again, calls=%d", gen.calls)
	}
}

func TestAIHandlerMissingMessage(t *testing.T) {
	h := NewAIHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubAI{}, &stubMessages{err: conversation.ErrMessageNotFound}, job.NewMemoryStore())
	j, _ := job.NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), j); !errors.Is(err, job.ErrPermanent) && !errors.Is(err, conversation.ErrMessageNotFound) {
		t.Fatalf("missing = %v", err)
	}
}

func TestAIHandlerHumanHandlingNoop(t *testing.T) {
	biz, convID, inbound := uuid.New(), uuid.New(), uuid.New()
	store := job.NewMemoryStore()
	gen := &stubAI{err: ai.ErrHumanHandling}
	msgs := &stubMessages{msg: &conversation.Message{
		ID: inbound, BusinessID: biz, ConversationID: convID,
		Direction: conversation.DirectionInbound, Status: conversation.MessageReceived,
	}}
	h := NewAIHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), gen, msgs, store)
	j, _ := job.NewAIGenerate(biz, convID, inbound, 3)
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("human handling must no-op: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("calls = %d", gen.calls)
	}
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("completed generation must stay complete: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("stale job regenerated after human takeover, calls=%d", gen.calls)
	}
}

func TestAIHandlerQuotaExceededNoRetry(t *testing.T) {
	biz, convID, inbound := uuid.New(), uuid.New(), uuid.New()
	store := job.NewMemoryStore()
	gen := &stubAI{err: ai.ErrQuotaExceeded}
	msgs := &stubMessages{msg: &conversation.Message{
		ID: inbound, BusinessID: biz, ConversationID: convID,
		Direction: conversation.DirectionInbound, Status: conversation.MessageReceived,
	}}
	h := NewAIHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), gen, msgs, store)
	j, _ := job.NewAIGenerate(biz, convID, inbound, 3)
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("quota must complete without retry: %v", err)
	}
	if err := h.Handle(context.Background(), j); err != nil {
		t.Fatalf("completed quota job retried: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("quota job regenerated, calls=%d", gen.calls)
	}
}

func TestAIHandlerTransient(t *testing.T) {
	biz, convID, inbound := uuid.New(), uuid.New(), uuid.New()
	h := NewAIHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubAI{err: ai.ErrProvider}, &stubMessages{msg: &conversation.Message{
		ID: inbound, BusinessID: biz, Direction: conversation.DirectionInbound,
	}}, job.NewMemoryStore())
	j, _ := job.NewAIGenerate(biz, convID, inbound, 3)
	if err := h.Handle(context.Background(), j); !errors.Is(err, ai.ErrProvider) {
		t.Fatalf("transient = %v", err)
	}
}

func TestDeliveryHandlerDelegates(t *testing.T) {
	d := &stubDeliver{}
	h := NewDeliveryHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), d)
	j, _ := job.NewMessageDelivery(uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), j); err != nil || d.calls != 1 {
		t.Fatalf("deliver = calls=%d err=%v", d.calls, err)
	}
}

type stubAI struct {
	calls int
	id    uuid.UUID
	err   error
}

func (s *stubAI) GenerateReply(context.Context, uuid.UUID, ai.GenerateReplyInput) (*ai.GenerateReplyResult, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.id == uuid.Nil {
		s.id = uuid.New()
	}
	return &ai.GenerateReplyResult{MessageID: s.id, Reply: "ok"}, nil
}

type stubMessages struct {
	msg *conversation.Message
	err error
}

func (s *stubMessages) GetMessage(context.Context, uuid.UUID, uuid.UUID) (*conversation.Message, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.msg, nil
}

type stubDeliver struct{ calls int }

func (s *stubDeliver) DeliverMessage(context.Context, uuid.UUID, uuid.UUID) error {
	s.calls++
	return nil
}
