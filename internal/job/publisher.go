package job

import (
	"context"
	"log/slog"
	"time"
)

// Publisher copies PENDING outbox rows onto Redis. It is the recovery path
// when enqueue failed after the PostgreSQL write.
type Publisher struct {
	log      *slog.Logger
	store    OutboxStore
	queue    Queue
	interval time.Duration
	batch    int
}

func NewPublisher(log *slog.Logger, store OutboxStore, queue Queue, interval time.Duration) *Publisher {
	if log == nil {
		log = slog.Default()
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Publisher{log: log, store: store, queue: queue, interval: interval, batch: 50}
}

func (p *Publisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if err := p.Flush(ctx); err != nil && ctx.Err() == nil {
			p.log.Error("outbox flush failed", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (p *Publisher) Flush(ctx context.Context) error {
	if p.queue == nil {
		return nil
	}
	rows, err := p.store.ListDue(ctx, p.batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		j := Job{
			ID: row.ID, Type: row.JobType, Payload: row.Payload,
			Attempt: row.Attempts, MaxAttempts: row.MaxAttempts,
			CreatedAt: row.CreatedAt, AvailableAt: row.AvailableAt,
		}
		if err := p.queue.Enqueue(ctx, j); err != nil {
			p.log.Error("outbox redis publish failed", "job_id", row.ID, "job_type", row.JobType, "error", err.Error())
			continue
		}
		if err := p.store.MarkPublished(ctx, row.ID); err != nil {
			p.log.Error("outbox mark published failed", "job_id", row.ID, "error", err.Error())
		}
	}
	return nil
}
