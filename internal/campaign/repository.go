package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const campaignColumns = `id, business_id, name, description, status, channel, message_kind, template_name, template_language, template_parameters, audience_type, audience_definition, message_body, scheduled_at, started_at, paused_at, completed_at, cancelled_at, created_by, updated_by, created_at, updated_at`
const runColumns = `id, business_id, campaign_id, status, scheduled_at, started_at, completed_at, paused_at, cancelled_at, total_recipients, queued_count, sent_count, delivered_count, read_count, failed_count, skipped_count, created_at, updated_at`
const recipientColumns = `id, business_id, campaign_run_id, customer_id, channel, external_recipient_id, status, message_id, queued_at, sent_at, delivered_at, read_at, failed_at, skipped_at, failure_code, failure_reason, created_at, updated_at`

type Store interface {
	ListCampaigns(ctx context.Context, businessID uuid.UUID, status string, limit, offset int) ([]Campaign, error)
	GetCampaign(ctx context.Context, businessID, campaignID uuid.UUID) (*Campaign, error)
	CreateCampaign(ctx context.Context, c Campaign) (*Campaign, error)
	UpdateCampaign(ctx context.Context, c Campaign) (*Campaign, error)

	CreateRun(ctx context.Context, run Run) (*Run, bool, error)
	GetRun(ctx context.Context, businessID, campaignID, runID uuid.UUID) (*Run, error)
	GetRunByID(ctx context.Context, businessID, runID uuid.UUID) (*Run, error)
	GetActiveRun(ctx context.Context, businessID, campaignID uuid.UUID) (*Run, error)
	ListRuns(ctx context.Context, businessID, campaignID uuid.UUID, limit, offset int) ([]Run, error)
	UpdateRun(ctx context.Context, run Run) (*Run, error)
	RefreshRunCounts(ctx context.Context, businessID, runID uuid.UUID) (*Run, error)

	SnapshotRecipients(ctx context.Context, businessID, runID uuid.UUID, channel string, def AudienceDefinition) (int, error)
	PreviewAudience(ctx context.Context, businessID uuid.UUID, channel string, def AudienceDefinition) (AudiencePreview, error)
	CustomerSendable(ctx context.Context, businessID, customerID uuid.UUID, channel string) (optIn bool, identityID *uuid.UUID, externalID string, err error)

	ListRecipients(ctx context.Context, businessID, runID uuid.UUID, status string, limit, offset int) ([]Recipient, error)
	ClaimPendingRecipients(ctx context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error)
	ListQueuedWithoutMessage(ctx context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error)
	GetRecipient(ctx context.Context, businessID, recipientID uuid.UUID) (*Recipient, error)
	UpdateRecipient(ctx context.Context, rec Recipient) (*Recipient, error)
	FindRecipientByMessage(ctx context.Context, businessID, messageID uuid.UUID) (*Recipient, error)
	FindRecipientMessage(ctx context.Context, businessID, recipientID uuid.UUID) (*uuid.UUID, error)
	SkipPendingRecipients(ctx context.Context, businessID, runID uuid.UUID, code, reason string) (int, error)
	PendingRecipientCount(ctx context.Context, businessID, runID uuid.UUID) (int, error)

	ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error)
	TrySchedulerLock(ctx context.Context) (bool, func(), error)
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListCampaigns(ctx context.Context, businessID uuid.UUID, status string, limit, offset int) ([]Campaign, error) {
	query := `SELECT ` + campaignColumns + ` FROM campaigns WHERE business_id = $1`
	args := []any{businessID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
		query += ` ORDER BY updated_at DESC, id DESC LIMIT $3 OFFSET $4`
		args = append(args, limit, offset)
	} else {
		query += ` ORDER BY updated_at DESC, id DESC LIMIT $2 OFFSET $3`
		args = append(args, limit, offset)
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		item, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetCampaign(ctx context.Context, businessID, campaignID uuid.UUID) (*Campaign, error) {
	item, err := scanCampaign(r.db.QueryRow(ctx, `SELECT `+campaignColumns+` FROM campaigns WHERE business_id = $1 AND id = $2`, businessID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateCampaign(ctx context.Context, c Campaign) (*Campaign, error) {
	params, _ := json.Marshal(c.TemplateParameters)
	if c.TemplateParameters == nil {
		params = []byte("[]")
	}
	audience, _ := json.Marshal(c.AudienceDefinition)
	const query = `
		INSERT INTO campaigns (
			business_id, name, description, status, channel, message_kind, template_name, template_language,
			template_parameters, audience_type, audience_definition, message_body, scheduled_at, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING ` + campaignColumns
	return scanCampaign(r.db.QueryRow(ctx, query,
		c.BusinessID, c.Name, c.Description, c.Status, c.Channel, c.MessageKind, c.TemplateName, c.TemplateLanguage,
		params, c.AudienceType, audience, c.MessageBody, c.ScheduledAt, c.CreatedBy, c.UpdatedBy,
	))
}

func (r *Repository) UpdateCampaign(ctx context.Context, c Campaign) (*Campaign, error) {
	params, _ := json.Marshal(c.TemplateParameters)
	if c.TemplateParameters == nil {
		params = []byte("[]")
	}
	audience, _ := json.Marshal(c.AudienceDefinition)
	const query = `
		UPDATE campaigns SET
			name=$3, description=$4, status=$5, message_kind=$6, template_name=$7, template_language=$8,
			template_parameters=$9, audience_definition=$10, message_body=$11, scheduled_at=$12,
			started_at=$13, paused_at=$14, completed_at=$15, cancelled_at=$16, updated_by=$17
		WHERE business_id=$1 AND id=$2
		RETURNING ` + campaignColumns
	item, err := scanCampaign(r.db.QueryRow(ctx, query,
		c.BusinessID, c.ID, c.Name, c.Description, c.Status, c.MessageKind, c.TemplateName, c.TemplateLanguage,
		params, audience, c.MessageBody, c.ScheduledAt, c.StartedAt, c.PausedAt, c.CompletedAt, c.CancelledAt, c.UpdatedBy,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateRun(ctx context.Context, run Run) (*Run, bool, error) {
	const query = `
		INSERT INTO campaign_runs (business_id, campaign_id, status, scheduled_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (business_id, campaign_id) WHERE status IN ('PENDING', 'RUNNING', 'PAUSED')
		DO NOTHING
		RETURNING ` + runColumns
	item, err := scanRun(r.db.QueryRow(ctx, query, run.BusinessID, run.CampaignID, run.Status, run.ScheduledAt))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := r.GetActiveRun(ctx, run.BusinessID, run.CampaignID)
		return existing, false, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return item, true, nil
}

func (r *Repository) GetRun(ctx context.Context, businessID, campaignID, runID uuid.UUID) (*Run, error) {
	item, err := scanRun(r.db.QueryRow(ctx, `SELECT `+runColumns+` FROM campaign_runs WHERE business_id=$1 AND campaign_id=$2 AND id=$3`, businessID, campaignID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return item, err
}

func (r *Repository) GetRunByID(ctx context.Context, businessID, runID uuid.UUID) (*Run, error) {
	item, err := scanRun(r.db.QueryRow(ctx, `SELECT `+runColumns+` FROM campaign_runs WHERE business_id=$1 AND id=$2`, businessID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return item, err
}

func (r *Repository) GetActiveRun(ctx context.Context, businessID, campaignID uuid.UUID) (*Run, error) {
	item, err := scanRun(r.db.QueryRow(ctx, `
		SELECT `+runColumns+` FROM campaign_runs
		WHERE business_id=$1 AND campaign_id=$2 AND status IN ('PENDING','RUNNING','PAUSED')
		ORDER BY created_at DESC LIMIT 1`, businessID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return item, err
}

func (r *Repository) ListRuns(ctx context.Context, businessID, campaignID uuid.UUID, limit, offset int) ([]Run, error) {
	rows, err := r.db.Query(ctx, `SELECT `+runColumns+` FROM campaign_runs WHERE business_id=$1 AND campaign_id=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`, businessID, campaignID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateRun(ctx context.Context, run Run) (*Run, error) {
	const query = `
		UPDATE campaign_runs SET
			status=$4, scheduled_at=$5, started_at=$6, completed_at=$7, paused_at=$8, cancelled_at=$9,
			total_recipients=$10, queued_count=$11, sent_count=$12, delivered_count=$13, read_count=$14, failed_count=$15, skipped_count=$16
		WHERE business_id=$1 AND campaign_id=$2 AND id=$3
		RETURNING ` + runColumns
	item, err := scanRun(r.db.QueryRow(ctx, query,
		run.BusinessID, run.CampaignID, run.ID, run.Status, run.ScheduledAt, run.StartedAt, run.CompletedAt, run.PausedAt, run.CancelledAt,
		run.TotalRecipients, run.QueuedCount, run.SentCount, run.DeliveredCount, run.ReadCount, run.FailedCount, run.SkippedCount,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return item, err
}

func (r *Repository) RefreshRunCounts(ctx context.Context, businessID, runID uuid.UUID) (*Run, error) {
	const query = `
		UPDATE campaign_runs SET
			total_recipients = s.total,
			queued_count = s.queued,
			sent_count = s.sent,
			delivered_count = s.delivered,
			read_count = s.read,
			failed_count = s.failed,
			skipped_count = s.skipped
		FROM (
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE status = 'QUEUED') AS queued,
				COUNT(*) FILTER (WHERE status = 'SENT') AS sent,
				COUNT(*) FILTER (WHERE status = 'DELIVERED') AS delivered,
				COUNT(*) FILTER (WHERE status = 'READ') AS read,
				COUNT(*) FILTER (WHERE status = 'FAILED') AS failed,
				COUNT(*) FILTER (WHERE status = 'SKIPPED') AS skipped
			FROM campaign_recipients
			WHERE business_id = $1 AND campaign_run_id = $2
		) s
		WHERE campaign_runs.business_id = $1 AND campaign_runs.id = $2
		RETURNING ` + runColumns
	item, err := scanRun(r.db.QueryRow(ctx, query, businessID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return item, err
}

func (r *Repository) ListRecipients(ctx context.Context, businessID, runID uuid.UUID, status string, limit, offset int) ([]Recipient, error) {
	query := `SELECT ` + recipientColumns + ` FROM campaign_recipients WHERE business_id=$1 AND campaign_run_id=$2`
	args := []any{businessID, runID}
	if status != "" {
		query += ` AND status=$3`
		args = append(args, status)
		query += ` ORDER BY created_at ASC, id ASC LIMIT $4 OFFSET $5`
		args = append(args, limit, offset)
	} else {
		query += ` ORDER BY created_at ASC, id ASC LIMIT $3 OFFSET $4`
		args = append(args, limit, offset)
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ClaimPendingRecipients(ctx context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error) {
	const query = `
		UPDATE campaign_recipients rec
		SET status = 'QUEUED', queued_at = now()
		WHERE rec.id IN (
			SELECT id FROM campaign_recipients
			WHERE business_id = $1 AND campaign_run_id = $2 AND status = 'PENDING'
			ORDER BY created_at ASC, id ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + recipientColumns
	rows, err := r.db.Query(ctx, query, businessID, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ListQueuedWithoutMessage(ctx context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+recipientColumns+` FROM campaign_recipients
		WHERE business_id=$1 AND campaign_run_id=$2 AND status='QUEUED' AND message_id IS NULL
		ORDER BY queued_at ASC NULLS FIRST, id ASC
		LIMIT $3`, businessID, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetRecipient(ctx context.Context, businessID, recipientID uuid.UUID) (*Recipient, error) {
	item, err := scanRecipient(r.db.QueryRow(ctx, `SELECT `+recipientColumns+` FROM campaign_recipients WHERE business_id=$1 AND id=$2`, businessID, recipientID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return item, err
}

func (r *Repository) UpdateRecipient(ctx context.Context, rec Recipient) (*Recipient, error) {
	const query = `
		UPDATE campaign_recipients SET
			status=$3, message_id=$4, queued_at=$5, sent_at=$6, delivered_at=$7, read_at=$8,
			failed_at=$9, skipped_at=$10, failure_code=$11, failure_reason=$12, external_recipient_id=$13
		WHERE business_id=$1 AND id=$2
		RETURNING ` + recipientColumns
	item, err := scanRecipient(r.db.QueryRow(ctx, query,
		rec.BusinessID, rec.ID, rec.Status, rec.MessageID, rec.QueuedAt, rec.SentAt, rec.DeliveredAt, rec.ReadAt,
		rec.FailedAt, rec.SkippedAt, rec.FailureCode, rec.FailureReason, rec.ExternalRecipientID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return item, err
}

func (r *Repository) FindRecipientByMessage(ctx context.Context, businessID, messageID uuid.UUID) (*Recipient, error) {
	item, err := scanRecipient(r.db.QueryRow(ctx, `SELECT `+recipientColumns+` FROM campaign_recipients WHERE business_id=$1 AND message_id=$2`, businessID, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) FindRecipientMessage(ctx context.Context, businessID, recipientID uuid.UUID) (*uuid.UUID, error) {
	const query = `
		SELECT id FROM messages
		WHERE business_id = $1 AND metadata->>'campaign_recipient_id' = $2
		ORDER BY created_at ASC LIMIT 1`
	var id uuid.UUID
	err := r.db.QueryRow(ctx, query, businessID, recipientID.String()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return &id, nil
}

func (r *Repository) SkipPendingRecipients(ctx context.Context, businessID, runID uuid.UUID, code, reason string) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE campaign_recipients
		SET status='SKIPPED', skipped_at=now(), failure_code=$3, failure_reason=$4
		WHERE business_id=$1 AND campaign_run_id=$2 AND status='PENDING'`,
		businessID, runID, code, reason)
	if err != nil {
		return 0, fmt.Errorf("database: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *Repository) PendingRecipientCount(ctx context.Context, businessID, runID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM campaign_recipients WHERE business_id=$1 AND campaign_run_id=$2 AND status='PENDING'`, businessID, runID).Scan(&n)
	return n, err
}

func (r *Repository) ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+campaignColumns+` FROM campaigns
		WHERE status='SCHEDULED' AND scheduled_at IS NOT NULL AND scheduled_at <= $1
		ORDER BY scheduled_at ASC
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		item, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) TrySchedulerLock(ctx context.Context) (bool, func(), error) {
	var locked bool
	if err := r.db.QueryRow(ctx, `SELECT pg_try_advisory_lock(872316)`).Scan(&locked); err != nil {
		return false, func() {}, err
	}
	if !locked {
		return false, func() {}, nil
	}
	return true, func() {
		_, _ = r.db.Exec(context.Background(), `SELECT pg_advisory_unlock(872316)`)
	}, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCampaign(row scanner) (*Campaign, error) {
	var c Campaign
	var params, audience []byte
	err := row.Scan(
		&c.ID, &c.BusinessID, &c.Name, &c.Description, &c.Status, &c.Channel, &c.MessageKind,
		&c.TemplateName, &c.TemplateLanguage, &params, &c.AudienceType, &audience, &c.MessageBody,
		&c.ScheduledAt, &c.StartedAt, &c.PausedAt, &c.CompletedAt, &c.CancelledAt, &c.CreatedBy, &c.UpdatedBy,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &c.TemplateParameters)
	}
	def, err := decodeAudience(audience)
	if err != nil {
		return nil, err
	}
	c.AudienceDefinition = def
	return &c, nil
}

func scanRun(row scanner) (*Run, error) {
	var run Run
	err := row.Scan(
		&run.ID, &run.BusinessID, &run.CampaignID, &run.Status, &run.ScheduledAt, &run.StartedAt,
		&run.CompletedAt, &run.PausedAt, &run.CancelledAt, &run.TotalRecipients, &run.QueuedCount,
		&run.SentCount, &run.DeliveredCount, &run.ReadCount, &run.FailedCount, &run.SkippedCount,
		&run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	return &run, nil
}

func scanRecipient(row scanner) (*Recipient, error) {
	var rec Recipient
	err := row.Scan(
		&rec.ID, &rec.BusinessID, &rec.CampaignRunID, &rec.CustomerID, &rec.Channel, &rec.ExternalRecipientID,
		&rec.Status, &rec.MessageID, &rec.QueuedAt, &rec.SentAt, &rec.DeliveredAt, &rec.ReadAt,
		&rec.FailedAt, &rec.SkippedAt, &rec.FailureCode, &rec.FailureReason, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	return &rec, nil
}
