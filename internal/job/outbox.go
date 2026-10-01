package job

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Outbox writes PostgreSQL first, then best-effort Redis. A publisher drains
// PENDING rows so a Redis outage cannot silently drop persisted work.
type Outbox struct {
	log         *slog.Logger
	store       OutboxStore
	queue       Queue
	maxAttempts int
}

func NewOutbox(log *slog.Logger, store OutboxStore, queue Queue, maxAttempts int) *Outbox {
	if log == nil {
		log = slog.Default()
	}
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	return &Outbox{log: log, store: store, queue: queue, maxAttempts: maxAttempts}
}

func (o *Outbox) EnqueueAIGenerate(ctx context.Context, businessID, conversationID, messageID uuid.UUID) error {
	j, err := NewAIGenerate(businessID, conversationID, messageID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueDelivery(ctx context.Context, businessID, messageID uuid.UUID) error {
	j, err := NewMessageDelivery(businessID, messageID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueKnowledgeIngest(ctx context.Context, businessID, documentID uuid.UUID) error {
	j, err := NewKnowledgeIngest(businessID, documentID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueAutomationEvaluate(ctx context.Context, businessID, eventID uuid.UUID) error {
	j, err := NewAutomationEvaluate(businessID, eventID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueAutomationRun(ctx context.Context, businessID, runID uuid.UUID) error {
	j, err := NewAutomationRun(businessID, runID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueCampaignStart(ctx context.Context, businessID, runID uuid.UUID) error {
	j, err := NewCampaignStart(businessID, runID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueCampaignDispatch(ctx context.Context, businessID, runID uuid.UUID) error {
	j, err := NewCampaignDispatch(businessID, runID, o.maxAttempts)
	if err != nil {
		return err
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueCampaignDispatchAfter(ctx context.Context, businessID, runID uuid.UUID, after time.Duration) error {
	j, err := NewCampaignDispatch(businessID, runID, o.maxAttempts)
	if err != nil {
		return err
	}
	if after > 0 {
		j.AvailableAt = time.Now().UTC().Add(after)
	}
	return o.EnqueueJob(ctx, j)
}

func (o *Outbox) EnqueueJob(ctx context.Context, j Job) error {
	if err := o.store.Insert(ctx, OutboxRow{
		ID: j.ID, JobType: j.Type, Payload: j.Payload, Status: OutboxPending,
		Attempts: j.Attempt, MaxAttempts: j.MaxAttempts, AvailableAt: j.AvailableAt, CreatedAt: j.CreatedAt,
	}); err != nil {
		return err
	}
	if o.queue == nil {
		return nil
	}
	if err := o.queue.Enqueue(ctx, j); err != nil {
		o.log.Error("redis enqueue failed; outbox remains pending",
			"job_id", j.ID, "job_type", j.Type, "error", err.Error())
		return nil
	}
	if err := o.store.MarkPublished(ctx, j.ID); err != nil {
		o.log.Error("outbox publish mark failed", "job_id", j.ID, "error", err.Error())
	}
	return nil
}
