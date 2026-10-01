package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const outboxColumns = `id, job_type, payload, status, attempts, max_attempts, available_at, created_at, processed_at, last_error`

type OutboxStore interface {
	Insert(ctx context.Context, row OutboxRow) error
	ListDue(ctx context.Context, limit int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error
	Reschedule(ctx context.Context, id uuid.UUID, availableAt time.Time, lastError string) error
	ClaimAIGeneration(ctx context.Context, businessID, conversationID, inboundID uuid.UUID) (claimed bool, alreadyDone bool, err error)
	CompleteAIGeneration(ctx context.Context, inboundID, outboundID uuid.UUID) error
	RecordAIGenerationMeta(ctx context.Context, inboundID uuid.UUID, provider, model string) error
	RecordAIGenerationFailure(ctx context.Context, inboundID uuid.UUID) error
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, row OutboxRow) error {
	const query = `
		INSERT INTO job_outbox (id, job_type, payload, status, attempts, max_attempts, available_at, created_at)
		VALUES ($1, $2, $3, 'PENDING', $4, $5, $6, $7)`
	_, err := r.db.Exec(ctx, query, row.ID, row.JobType, []byte(row.Payload), row.Attempts, row.MaxAttempts, row.AvailableAt, row.CreatedAt)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) ListDue(ctx context.Context, limit int) ([]OutboxRow, error) {
	if limit < 1 {
		limit = 50
	}
	const query = `
		SELECT ` + outboxColumns + `
		FROM job_outbox
		WHERE status = 'PENDING' AND available_at <= now()
		ORDER BY available_at ASC, created_at ASC
		LIMIT $1`
	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]OutboxRow, 0)
	for rows.Next() {
		item, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	const query = `
		UPDATE job_outbox
		SET status = 'PUBLISHED', processed_at = now(), last_error = NULL
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	const query = `
		UPDATE job_outbox
		SET status = 'FAILED', processed_at = now(), last_error = $2, attempts = attempts + 1
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id, lastError)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) Reschedule(ctx context.Context, id uuid.UUID, availableAt time.Time, lastError string) error {
	const query = `
		UPDATE job_outbox
		SET status = 'PENDING', available_at = $2, last_error = $3, attempts = attempts + 1, processed_at = NULL
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id, availableAt, lastError)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) ClaimAIGeneration(ctx context.Context, businessID, conversationID, inboundID uuid.UUID) (bool, bool, error) {
	const query = `
		INSERT INTO ai_generations (inbound_message_id, business_id, conversation_id, status, started_at)
		VALUES ($1, $2, $3, 'PENDING', now())
		ON CONFLICT (inbound_message_id) DO UPDATE
		SET inbound_message_id = ai_generations.inbound_message_id
		WHERE ai_generations.outbound_message_id IS NULL
		RETURNING outbound_message_id`
	var outbound *uuid.UUID
	err := r.db.QueryRow(ctx, query, inboundID, businessID, conversationID).Scan(&outbound)
	if errorsIsNoRows(err) {
		return false, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("database: %w", err)
	}
	if outbound != nil {
		return false, true, nil
	}
	return true, false, nil
}

func (r *Repository) CompleteAIGeneration(ctx context.Context, inboundID, outboundID uuid.UUID) error {
	status := "SUCCESS"
	var outbound any
	if outboundID == uuid.Nil {
		status = "SKIPPED"
		outbound = nil
	} else {
		outbound = outboundID
	}
	const query = `
		UPDATE ai_generations
		SET outbound_message_id = $2,
		    completed_at = now(),
		    status = $3,
		    latency_ms = CASE
		        WHEN started_at IS NOT NULL THEN GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (now() - started_at)) * 1000))::int
		        ELSE latency_ms
		    END
		WHERE inbound_message_id = $1`
	_, err := r.db.Exec(ctx, query, inboundID, outbound, status)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) RecordAIGenerationMeta(ctx context.Context, inboundID uuid.UUID, provider, model string) error {
	const query = `
		UPDATE ai_generations
		SET provider = COALESCE(NULLIF($2, ''), provider),
		    model = COALESCE(NULLIF($3, ''), model)
		WHERE inbound_message_id = $1`
	_, err := r.db.Exec(ctx, query, inboundID, provider, model)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) RecordAIGenerationFailure(ctx context.Context, inboundID uuid.UUID) error {
	const query = `
		UPDATE ai_generations
		SET status = 'FAILED',
		    completed_at = now(),
		    latency_ms = CASE
		        WHEN started_at IS NOT NULL THEN GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (now() - started_at)) * 1000))::int
		        ELSE latency_ms
		    END
		WHERE inbound_message_id = $1 AND outbound_message_id IS NULL`
	_, err := r.db.Exec(ctx, query, inboundID)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func errorsIsNoRows(err error) bool {
	return err != nil && err == pgx.ErrNoRows
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOutbox(row scanner) (OutboxRow, error) {
	var item OutboxRow
	var payload []byte
	if err := row.Scan(
		&item.ID, &item.JobType, &payload, &item.Status, &item.Attempts, &item.MaxAttempts,
		&item.AvailableAt, &item.CreatedAt, &item.ProcessedAt, &item.LastError,
	); err != nil {
		return OutboxRow{}, fmt.Errorf("database: %w", err)
	}
	item.Payload = json.RawMessage(payload)
	return item, nil
}
