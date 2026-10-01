package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Business is a tenant of the platform. Every future business owned domain
// (areas, menu, customers, reservations) hangs off this entity.
type Business struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Type      BusinessType
	Timezone  string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

const businessColumns = `id, name, slug, business_type, timezone, status, created_at, updated_at`

// BusinessRepository reads and writes businesses.
type BusinessRepository struct {
	db Querier
}

// NewBusinessRepository returns a repository backed by db.
func NewBusinessRepository(db Querier) *BusinessRepository {
	return &BusinessRepository{db: db}
}

// CreateBusinessParams describes a new business. The database generates the id
// and the timestamps.
type CreateBusinessParams struct {
	Name     string
	Slug     string
	Type     BusinessType
	Timezone string
}

// Create inserts a business and returns the stored row.
func (r *BusinessRepository) Create(ctx context.Context, params CreateBusinessParams) (*Business, error) {
	const query = `
		INSERT INTO businesses (name, slug, business_type, timezone)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + businessColumns

	return scanBusiness(r.db.QueryRow(ctx, query, params.Name, params.Slug, params.Type, params.Timezone))
}

// GetByID returns the business with the given id, or ErrNotFound.
func (r *BusinessRepository) GetByID(ctx context.Context, id uuid.UUID) (*Business, error) {
	const query = `SELECT ` + businessColumns + ` FROM businesses WHERE id = $1`

	return scanBusiness(r.db.QueryRow(ctx, query, id))
}

// GetBySlug returns the business with the given slug, or ErrNotFound.
func (r *BusinessRepository) GetBySlug(ctx context.Context, slug string) (*Business, error) {
	const query = `SELECT ` + businessColumns + ` FROM businesses WHERE slug = $1`

	return scanBusiness(r.db.QueryRow(ctx, query, slug))
}

// List returns businesses ordered by creation time, newest first.
func (r *BusinessRepository) List(ctx context.Context, limit, offset int32) ([]Business, error) {
	const query = `
		SELECT ` + businessColumns + `
		FROM businesses
		ORDER BY created_at DESC, id
		LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()

	var businesses []Business
	for rows.Next() {
		business, err := scanBusiness(rows)
		if err != nil {
			return nil, err
		}
		businesses = append(businesses, *business)
	}

	return businesses, wrapError(rows.Err())
}

// UpdateBusinessParams carries the mutable fields of a business. The slug and
// the business type are immutable for now.
type UpdateBusinessParams struct {
	Name     string
	Timezone string
	Status   Status
}

// Update overwrites the mutable fields of a business.
func (r *BusinessRepository) Update(ctx context.Context, id uuid.UUID, params UpdateBusinessParams) (*Business, error) {
	const query = `
		UPDATE businesses
		SET name = $2, timezone = $3, status = $4
		WHERE id = $1
		RETURNING ` + businessColumns

	return scanBusiness(r.db.QueryRow(ctx, query, id, params.Name, params.Timezone, params.Status))
}

func scanBusiness(row pgx.Row) (*Business, error) {
	var b Business
	err := row.Scan(&b.ID, &b.Name, &b.Slug, &b.Type, &b.Timezone, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, wrapError(err)
	}

	return &b, nil
}
