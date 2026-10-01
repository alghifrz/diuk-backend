package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	automationColumns = `id, business_id, name, description, status, trigger_type, trigger_config, conditions, action_type, action_config, created_by, updated_by, created_at, updated_at`
	eventColumns      = `id, business_id, event_type, subject_type, subject_id, idempotency_key, payload, occurred_at, created_at`
	runColumns        = `id, business_id, automation_id, event_id, execution_key, subject_type, subject_id, status, scheduled_at, started_at, completed_at, failed_at, attempt, error_code, error_message, created_at, updated_at`
)

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateAutomation(ctx context.Context, item Automation) (*Automation, error) {
	const query = `
		INSERT INTO automations (
			id, business_id, name, description, status, trigger_type, trigger_config, conditions,
			action_type, action_config, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING ` + automationColumns
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	if item.Status == "" {
		item.Status = StatusInactive
	}
	return scanAutomation(r.db.QueryRow(ctx, query,
		item.ID, item.BusinessID, item.Name, item.Description, item.Status, item.TriggerType,
		item.TriggerConfig, item.Conditions, item.ActionType, item.ActionConfig, item.CreatedBy, item.UpdatedBy,
	))
}

func (r *Repository) GetAutomation(ctx context.Context, businessID, automationID uuid.UUID) (*Automation, error) {
	const query = `SELECT ` + automationColumns + ` FROM automations WHERE id = $1 AND business_id = $2`
	item, err := scanAutomation(r.db.QueryRow(ctx, query, automationID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) ListAutomations(ctx context.Context, businessID uuid.UUID, status string) ([]Automation, error) {
	query := `SELECT ` + automationColumns + ` FROM automations WHERE business_id = $1`
	args := []any{businessID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]Automation, 0)
	for rows.Next() {
		item, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ListActiveByTrigger(ctx context.Context, businessID uuid.UUID, triggerType string) ([]Automation, error) {
	const query = `
		SELECT ` + automationColumns + `
		FROM automations
		WHERE business_id = $1 AND status = $2 AND trigger_type = $3
		ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, businessID, StatusActive, triggerType)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]Automation, 0)
	for rows.Next() {
		item, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ListActiveScheduled(ctx context.Context) ([]Automation, error) {
	const query = `
		SELECT ` + automationColumns + `
		FROM automations
		WHERE status = $1 AND trigger_type IN ($2, $3)
		ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, StatusActive, TriggerScheduled, TriggerConversationInactive)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]Automation, 0)
	for rows.Next() {
		item, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateAutomation(ctx context.Context, item Automation) (*Automation, error) {
	const query = `
		UPDATE automations
		SET name = $3, description = $4, status = $5, trigger_type = $6, trigger_config = $7,
		    conditions = $8, action_type = $9, action_config = $10, updated_by = $11
		WHERE id = $1 AND business_id = $2
		RETURNING ` + automationColumns
	out, err := scanAutomation(r.db.QueryRow(ctx, query,
		item.ID, item.BusinessID, item.Name, item.Description, item.Status, item.TriggerType,
		item.TriggerConfig, item.Conditions, item.ActionType, item.ActionConfig, item.UpdatedBy,
	))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

func (r *Repository) InsertEvent(ctx context.Context, event Event) (*Event, bool, error) {
	const query = `
		INSERT INTO automation_events (
			id, business_id, event_type, subject_type, subject_id, idempotency_key, payload, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (business_id, event_type, idempotency_key) DO NOTHING
		RETURNING ` + eventColumns
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return nil, false, err
	}
	var subjectType *string
	if strings.TrimSpace(event.SubjectType) != "" {
		subjectType = &event.SubjectType
	}
	var subjectID *uuid.UUID
	if event.SubjectID != uuid.Nil {
		subjectID = &event.SubjectID
	}
	out, err := scanEvent(r.db.QueryRow(ctx, query,
		event.ID, event.BusinessID, event.Type, subjectType, subjectID, event.IdempotencyKey, payload, event.OccurredAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrEventNotFound) {
			existing, getErr := r.getEventByKey(ctx, event.BusinessID, event.Type, event.IdempotencyKey)
			return existing, false, getErr
		}
		return nil, false, err
	}
	return out, true, nil
}

func (r *Repository) getEventByKey(ctx context.Context, businessID uuid.UUID, eventType, key string) (*Event, error) {
	const query = `SELECT ` + eventColumns + ` FROM automation_events WHERE business_id = $1 AND event_type = $2 AND idempotency_key = $3`
	return scanEvent(r.db.QueryRow(ctx, query, businessID, eventType, key))
}

func (r *Repository) GetEvent(ctx context.Context, businessID, eventID uuid.UUID) (*Event, error) {
	const query = `SELECT ` + eventColumns + ` FROM automation_events WHERE id = $1 AND business_id = $2`
	item, err := scanEvent(r.db.QueryRow(ctx, query, eventID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) InsertRun(ctx context.Context, run Run) (*Run, bool, error) {
	const query = `
		INSERT INTO automation_runs (
			id, business_id, automation_id, event_id, execution_key, subject_type, subject_id,
			status, scheduled_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + runColumns
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = RunPending
	}
	var subjectType *string
	if strings.TrimSpace(run.SubjectType) != "" {
		subjectType = &run.SubjectType
	}
	var subjectID *uuid.UUID
	if run.SubjectID != uuid.Nil {
		subjectID = &run.SubjectID
	}
	out, err := scanRun(r.db.QueryRow(ctx, query,
		run.ID, run.BusinessID, run.AutomationID, run.EventID, run.ExecutionKey, subjectType, subjectID, run.Status, run.ScheduledAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			existing, getErr := r.findExistingRun(ctx, run)
			return existing, false, getErr
		}
		return nil, false, err
	}
	return out, true, nil
}

func (r *Repository) findExistingRun(ctx context.Context, run Run) (*Run, error) {
	if run.EventID != nil {
		const query = `SELECT ` + runColumns + ` FROM automation_runs WHERE automation_id = $1 AND event_id = $2`
		item, err := scanRun(r.db.QueryRow(ctx, query, run.AutomationID, *run.EventID))
		if err == nil {
			return item, nil
		}
	}
	if run.ExecutionKey != nil {
		const query = `SELECT ` + runColumns + ` FROM automation_runs WHERE automation_id = $1 AND execution_key = $2`
		return scanRun(r.db.QueryRow(ctx, query, run.AutomationID, *run.ExecutionKey))
	}
	return nil, ErrRunNotFound
}

func (r *Repository) GetRun(ctx context.Context, businessID, runID uuid.UUID) (*Run, error) {
	const query = `SELECT ` + runColumns + ` FROM automation_runs WHERE id = $1 AND business_id = $2`
	item, err := scanRun(r.db.QueryRow(ctx, query, runID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) ListRuns(ctx context.Context, businessID, automationID uuid.UUID, limit int) ([]Run, error) {
	if limit < 1 {
		limit = 50
	}
	const query = `
		SELECT ` + runColumns + `
		FROM automation_runs
		WHERE business_id = $1 AND automation_id = $2
		ORDER BY created_at DESC
		LIMIT $3`
	rows, err := r.db.Query(ctx, query, businessID, automationID, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]Run, 0)
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ClaimRun(ctx context.Context, businessID, runID uuid.UUID) (*Run, error) {
	const query = `
		UPDATE automation_runs
		SET status = 'RUNNING', started_at = COALESCE(started_at, now()), attempt = attempt + 1
		WHERE id = $1 AND business_id = $2 AND status = 'PENDING'
		RETURNING ` + runColumns
	item, err := scanRun(r.db.QueryRow(ctx, query, runID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrRunNotFound) {
			return nil, ErrRunNotClaimed
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) MarkRunStatus(ctx context.Context, businessID, runID uuid.UUID, status string, errorCode, errorMessage *string) (*Run, error) {
	const query = `
		UPDATE automation_runs
		SET status = $3,
		    completed_at = CASE WHEN $3 = 'COMPLETED' THEN now() ELSE completed_at END,
		    failed_at = CASE WHEN $3 = 'FAILED' THEN now() ELSE failed_at END,
		    error_code = $4,
		    error_message = $5
		WHERE id = $1 AND business_id = $2
		RETURNING ` + runColumns
	item, err := scanRun(r.db.QueryRow(ctx, query, runID, businessID, status, errorCode, errorMessage))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	return item, nil
}

func (r *Repository) CancelPendingRuns(ctx context.Context, businessID, automationID uuid.UUID) error {
	const query = `
		UPDATE automation_runs
		SET status = 'CANCELLED'
		WHERE business_id = $1 AND automation_id = $2 AND status = 'PENDING'`
	_, err := r.db.Exec(ctx, query, businessID, automationID)
	return err
}

func (r *Repository) HasRunForKey(ctx context.Context, automationID uuid.UUID, executionKey string) (bool, error) {
	const query = `SELECT 1 FROM automation_runs WHERE automation_id = $1 AND execution_key = $2 LIMIT 1`
	var n int
	err := r.db.QueryRow(ctx, query, automationID, executionKey).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) ListDueReservations(ctx context.Context, businessID uuid.UUID, dueBefore, lookback time.Time, limit int) ([]ReservationCandidate, error) {
	if limit < 1 {
		limit = 50
	}
	const query = `
		SELECT id, business_id, customer_id, status, party_size, start_at
		FROM reservations
		WHERE business_id = $1
		  AND status = 'CONFIRMED'
		  AND start_at <= $2
		  AND start_at > $3
		ORDER BY start_at ASC
		LIMIT $4`
	rows, err := r.db.Query(ctx, query, businessID, dueBefore, lookback, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]ReservationCandidate, 0)
	for rows.Next() {
		var item ReservationCandidate
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CustomerID, &item.Status, &item.PartySize, &item.StartAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListInactiveConversations(ctx context.Context, businessID uuid.UUID, inactiveSince time.Time, limit int) ([]ConversationCandidate, error) {
	if limit < 1 {
		limit = 50
	}
	const query = `
		SELECT c.id, c.business_id, c.customer_id, c.customer_identity_id, c.status, c.channel, last_in.last_inbound
		FROM conversations c
		JOIN LATERAL (
			SELECT MAX(m.created_at) AS last_inbound
			FROM messages m
			WHERE m.business_id = c.business_id
			  AND m.conversation_id = c.id
			  AND m.direction = 'INBOUND'
			  AND m.sender_type = 'CUSTOMER'
		) last_in ON last_in.last_inbound IS NOT NULL
		WHERE c.business_id = $1
		  AND c.status = 'OPEN'
		  AND last_in.last_inbound <= $2
		ORDER BY last_in.last_inbound ASC
		LIMIT $3`
	rows, err := r.db.Query(ctx, query, businessID, inactiveSince, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]ConversationCandidate, 0)
	for rows.Next() {
		var item ConversationCandidate
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CustomerID, &item.IdentityID, &item.Status, &item.Channel, &item.LastInboundAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) LastInboundCustomerAt(ctx context.Context, businessID, conversationID uuid.UUID) (*time.Time, error) {
	const query = `
		SELECT MAX(created_at)
		FROM messages
		WHERE business_id = $1
		  AND conversation_id = $2
		  AND direction = 'INBOUND'
		  AND sender_type = 'CUSTOMER'`
	var at *time.Time
	if err := r.db.QueryRow(ctx, query, businessID, conversationID).Scan(&at); err != nil {
		return nil, err
	}
	return at, nil
}

func (r *Repository) TrySchedulerLock(ctx context.Context) (bool, func(), error) {
	var locked bool
	if err := r.db.QueryRow(ctx, `SELECT pg_try_advisory_lock(872315)`).Scan(&locked); err != nil {
		return false, func() {}, err
	}
	unlock := func() {
		_, _ = r.db.Exec(context.Background(), `SELECT pg_advisory_unlock(872315)`)
	}
	if !locked {
		return false, func() {}, nil
	}
	return true, unlock, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAutomation(row rowScanner) (*Automation, error) {
	var item Automation
	if err := row.Scan(
		&item.ID, &item.BusinessID, &item.Name, &item.Description, &item.Status, &item.TriggerType,
		&item.TriggerConfig, &item.Conditions, &item.ActionType, &item.ActionConfig, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func scanEvent(row rowScanner) (*Event, error) {
	var (
		item        Event
		subjectType *string
		subjectID   *uuid.UUID
		payload     []byte
	)
	if err := row.Scan(
		&item.ID, &item.BusinessID, &item.Type, &subjectType, &subjectID, &item.IdempotencyKey, &payload, &item.OccurredAt, &item.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	if subjectType != nil {
		item.SubjectType = *subjectType
	}
	if subjectID != nil {
		item.SubjectID = *subjectID
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &item.Payload)
	}
	if item.Payload == nil {
		item.Payload = map[string]any{}
	}
	return &item, nil
}

func scanRun(row rowScanner) (*Run, error) {
	var (
		item        Run
		subjectType *string
		subjectID   *uuid.UUID
	)
	if err := row.Scan(
		&item.ID, &item.BusinessID, &item.AutomationID, &item.EventID, &item.ExecutionKey, &subjectType, &subjectID,
		&item.Status, &item.ScheduledAt, &item.StartedAt, &item.CompletedAt, &item.FailedAt, &item.Attempt,
		&item.ErrorCode, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	if subjectType != nil {
		item.SubjectType = *subjectType
	}
	if subjectID != nil {
		item.SubjectID = *subjectID
	}
	return &item, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
