package channel

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// QueueTrigger persists work to the job outbox (and Redis) so workers run
// GenerateReply. This is the production ReplyTrigger for JOB 13.
type QueueTrigger struct {
	Enqueue JobEnqueuer
	Log     *slog.Logger
}

func (t QueueTrigger) Trigger(ctx context.Context, businessID, conversationID, inboundMessageID uuid.UUID) {
	if t.Enqueue == nil {
		return
	}
	if err := t.Enqueue.EnqueueAIGenerate(ctx, businessID, conversationID, inboundMessageID); err != nil && t.Log != nil {
		t.Log.Error("enqueue ai generate failed",
			"business_id", businessID,
			"conversation_id", conversationID,
			"message_id", inboundMessageID,
			"error", err.Error(),
		)
	}
}

// AsyncTrigger is a last-resort in-process fallback when no queue is wired.
// Production wiring uses QueueTrigger.
type AsyncTrigger struct {
	Service *Service
	Timeout time.Duration
}

func (t AsyncTrigger) Trigger(ctx context.Context, businessID, conversationID, inboundMessageID uuid.UUID) {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	svc := t.Service
	_ = inboundMessageID
	go func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		_ = svc.GenerateAndDeliver(bg, businessID, conversationID)
	}()
}

// SyncTrigger runs generation and delivery in the caller goroutine. Tests use
// this so they do not need Redis.
type SyncTrigger struct {
	Service *Service
}

func (t SyncTrigger) Trigger(ctx context.Context, businessID, conversationID, inboundMessageID uuid.UUID) {
	_ = inboundMessageID
	_ = t.Service.GenerateAndDeliver(ctx, businessID, conversationID)
}
