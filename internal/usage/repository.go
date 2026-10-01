package usage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type dbConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Store interface {
	GetAssignment(ctx context.Context, businessID uuid.UUID) (*Assignment, error)
	GetPlan(ctx context.Context, planID uuid.UUID) (*Plan, error)
	ListEntitlements(ctx context.Context, planID uuid.UUID) ([]Entitlement, error)
	GetEntitlement(ctx context.Context, planID uuid.UUID, feature FeatureKey) (*Entitlement, error)
	EnsurePeriod(ctx context.Context, p Period) (*Period, error)
	ListHistoryPeriods(ctx context.Context, businessID uuid.UUID, beforeStart time.Time, limit, offset int) ([]Period, error)
	EnsureCounter(ctx context.Context, c Counter) (*Counter, error)
	GetCounter(ctx context.Context, businessID, periodID uuid.UUID, feature FeatureKey) (*Counter, error)
	ListCounters(ctx context.Context, businessID, periodID uuid.UUID) ([]Counter, error)
	ConsumeAtomic(ctx context.Context, in atomicConsume) (*ConsumeResult, error)
	CountResource(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (int64, error)
}

type atomicConsume struct {
	BusinessID uuid.UUID
	Period     Period
	Feature    FeatureKey
	Quantity   int
	Limit      *int
	SourceType string
	SourceID   uuid.UUID
}

type Repository struct {
	db dbConn
}

func NewRepository(db dbConn) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAssignment(ctx context.Context, businessID uuid.UUID) (*Assignment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT b.id, b.timezone, b.plan_id, p.code, p.name, p.status
		FROM businesses b
		JOIN plans p ON p.id = b.plan_id
		WHERE b.id = $1`, businessID)
	var a Assignment
	if err := row.Scan(&a.BusinessID, &a.Timezone, &a.PlanID, &a.PlanCode, &a.PlanName, &a.PlanStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBusinessNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetPlan(ctx context.Context, planID uuid.UUID) (*Plan, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, code, name, description, status, created_at, updated_at
		FROM plans WHERE id = $1`, planID)
	var p Plan
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repository) ListEntitlements(ctx context.Context, planID uuid.UUID) ([]Entitlement, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, plan_id, feature_key, limit_value, period, enabled, created_at, updated_at
		FROM plan_entitlements
		WHERE plan_id = $1
		ORDER BY feature_key`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entitlement
	for rows.Next() {
		var e Entitlement
		if err := rows.Scan(&e.ID, &e.PlanID, &e.FeatureKey, &e.LimitValue, &e.Period, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) GetEntitlement(ctx context.Context, planID uuid.UUID, feature FeatureKey) (*Entitlement, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, plan_id, feature_key, limit_value, period, enabled, created_at, updated_at
		FROM plan_entitlements
		WHERE plan_id = $1 AND feature_key = $2`, planID, feature)
	var e Entitlement
	if err := row.Scan(&e.ID, &e.PlanID, &e.FeatureKey, &e.LimitValue, &e.Period, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotEntitled
		}
		return nil, err
	}
	return &e, nil
}

func (r *Repository) EnsurePeriod(ctx context.Context, p Period) (*Period, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO usage_periods (business_id, period_type, period_start, period_end, timezone)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (business_id, period_type, period_start) DO NOTHING`,
		p.BusinessID, p.PeriodType, p.PeriodStart, p.PeriodEnd, p.Timezone)
	if err != nil {
		return nil, err
	}
	return r.getPeriod(ctx, r.db, p.BusinessID, p.PeriodType, p.PeriodStart)
}

func (r *Repository) getPeriod(ctx context.Context, q rowQuery, businessID uuid.UUID, periodType string, start interface{}) (*Period, error) {
	row := q.QueryRow(ctx, `
		SELECT id, business_id, period_type, period_start, period_end, timezone, created_at, updated_at
		FROM usage_periods
		WHERE business_id = $1 AND period_type = $2 AND period_start = $3`,
		businessID, periodType, start)
	var p Period
	if err := row.Scan(&p.ID, &p.BusinessID, &p.PeriodType, &p.PeriodStart, &p.PeriodEnd, &p.Timezone, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) ListHistoryPeriods(ctx context.Context, businessID uuid.UUID, beforeStart time.Time, limit, offset int) ([]Period, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, business_id, period_type, period_start, period_end, timezone, created_at, updated_at
		FROM usage_periods
		WHERE business_id = $1 AND period_type = 'MONTHLY' AND period_start < $2
		ORDER BY period_start DESC
		LIMIT $3 OFFSET $4`, businessID, beforeStart, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Period
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.BusinessID, &p.PeriodType, &p.PeriodStart, &p.PeriodEnd, &p.Timezone, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) EnsureCounter(ctx context.Context, c Counter) (*Counter, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO usage_counters (business_id, usage_period_id, feature_key, used)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (usage_period_id, feature_key) DO NOTHING`,
		c.BusinessID, c.UsagePeriodID, c.FeatureKey)
	if err != nil {
		return nil, err
	}
	return r.getCounter(ctx, r.db, c.BusinessID, c.UsagePeriodID, c.FeatureKey)
}

func (r *Repository) GetCounter(ctx context.Context, businessID, periodID uuid.UUID, feature FeatureKey) (*Counter, error) {
	return r.getCounter(ctx, r.db, businessID, periodID, feature)
}

func (r *Repository) getCounter(ctx context.Context, q rowQuery, businessID, periodID uuid.UUID, feature FeatureKey) (*Counter, error) {
	row := q.QueryRow(ctx, `
		SELECT id, business_id, usage_period_id, feature_key, used, created_at, updated_at
		FROM usage_counters
		WHERE business_id = $1 AND usage_period_id = $2 AND feature_key = $3`,
		businessID, periodID, feature)
	var c Counter
	if err := row.Scan(&c.ID, &c.BusinessID, &c.UsagePeriodID, &c.FeatureKey, &c.Used, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Counter{BusinessID: businessID, UsagePeriodID: periodID, FeatureKey: feature, Used: 0}, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) ListCounters(ctx context.Context, businessID, periodID uuid.UUID) ([]Counter, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, business_id, usage_period_id, feature_key, used, created_at, updated_at
		FROM usage_counters
		WHERE business_id = $1 AND usage_period_id = $2`, businessID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Counter
	for rows.Next() {
		var c Counter
		if err := rows.Scan(&c.ID, &c.BusinessID, &c.UsagePeriodID, &c.FeatureKey, &c.Used, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) ConsumeAtomic(ctx context.Context, in atomicConsume) (*ConsumeResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO usage_periods (business_id, period_type, period_start, period_end, timezone)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (business_id, period_type, period_start) DO NOTHING`,
		in.Period.BusinessID, in.Period.PeriodType, in.Period.PeriodStart, in.Period.PeriodEnd, in.Period.Timezone); err != nil {
		return nil, err
	}
	period, err := r.getPeriod(ctx, tx, in.BusinessID, in.Period.PeriodType, in.Period.PeriodStart)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO usage_counters (business_id, usage_period_id, feature_key, used)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (usage_period_id, feature_key) DO NOTHING`,
		in.BusinessID, period.ID, in.Feature); err != nil {
		return nil, err
	}

	var consumptionID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO usage_consumptions (business_id, usage_period_id, feature_key, source_type, source_id, quantity)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (business_id, feature_key, source_type, source_id) DO NOTHING
		RETURNING id`,
		in.BusinessID, period.ID, in.Feature, in.SourceType, in.SourceID, in.Quantity).Scan(&consumptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		counter, err := r.getCounter(ctx, tx, in.BusinessID, period.ID, in.Feature)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &ConsumeResult{Used: counter.Used, Limit: in.Limit, Remaining: remaining(counter.Used, in.Limit), Duplicate: true}, nil
	}
	if err != nil {
		return nil, err
	}

	var used int64
	err = tx.QueryRow(ctx, `
		UPDATE usage_counters
		SET used = used + $1
		WHERE business_id = $2
		  AND usage_period_id = $3
		  AND feature_key = $4
		  AND ($5::integer IS NULL OR used + $1 <= $5)
		RETURNING used`,
		in.Quantity, in.BusinessID, period.ID, in.Feature, in.Limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, delErr := tx.Exec(ctx, `DELETE FROM usage_consumptions WHERE id = $1 AND business_id = $2`, consumptionID, in.BusinessID); delErr != nil {
			return nil, delErr
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrQuotaExceeded
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ConsumeResult{Used: used, Limit: in.Limit, Remaining: remaining(used, in.Limit), Duplicate: false}, nil
}

func (r *Repository) CountResource(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (int64, error) {
	var q string
	switch feature {
	case FeatureWhatsAppChannels:
		q = `SELECT COUNT(*) FROM business_channels WHERE business_id = $1 AND channel = 'WHATSAPP' AND status = 'ACTIVE'`
	case FeatureKnowledgeDocuments:
		q = `SELECT COUNT(*) FROM knowledge_documents WHERE business_id = $1 AND status <> 'ARCHIVED'`
	default:
		return 0, fmt.Errorf("%w: feature is not a resource limit", ErrInvalid)
	}
	var n int64
	if err := r.db.QueryRow(ctx, q, businessID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

type rowQuery interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
