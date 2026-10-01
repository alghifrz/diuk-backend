package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BusinessUser is the membership of a user in a business, together with the
// role held there. The same user can be an OWNER in one business and a
// MANAGER in another.
type BusinessUser struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	UserID     uuid.UUID
	RoleID     uuid.UUID
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const businessUserColumns = `id, business_id, user_id, role_id, status, created_at, updated_at`

// Access is an active membership together with the business and role it
// belongs to. Middleware uses this so tenant identity always comes from the
// database, never from the request body.
type Access struct {
	Membership BusinessUser
	Business   Business
	Role       Role
}

// BusinessUserRepository reads and writes memberships.
type BusinessUserRepository struct {
	db Querier
}

// NewBusinessUserRepository returns a repository backed by db.
func NewBusinessUserRepository(db Querier) *BusinessUserRepository {
	return &BusinessUserRepository{db: db}
}

// CreateBusinessUserParams describes a new membership.
type CreateBusinessUserParams struct {
	BusinessID uuid.UUID
	UserID     uuid.UUID
	RoleID     uuid.UUID
}

// Create grants a user access to a business with the given role. It returns
// ErrConflict when the membership already exists.
func (r *BusinessUserRepository) Create(ctx context.Context, params CreateBusinessUserParams) (*BusinessUser, error) {
	const query = `
		INSERT INTO business_users (business_id, user_id, role_id)
		VALUES ($1, $2, $3)
		RETURNING ` + businessUserColumns

	row := r.db.QueryRow(ctx, query, params.BusinessID, params.UserID, params.RoleID)

	return scanBusinessUser(row)
}

// GetActiveAccess returns the user's active membership in an active business,
// together with the role. Missing memberships, inactive memberships and
// inactive businesses all surface as ErrNotFound so callers cannot tell them
// apart.
func (r *BusinessUserRepository) GetActiveAccess(ctx context.Context, userID, businessID uuid.UUID) (*Access, error) {
	const query = `
		SELECT
			bu.id, bu.business_id, bu.user_id, bu.role_id, bu.status, bu.created_at, bu.updated_at,
			b.id, b.name, b.slug, b.business_type, b.timezone, b.status, b.created_at, b.updated_at,
			r.id, r.name, r.description, r.created_at, r.updated_at
		FROM business_users bu
		INNER JOIN businesses b ON b.id = bu.business_id
		INNER JOIN roles r ON r.id = bu.role_id
		WHERE bu.user_id = $1
		  AND bu.business_id = $2
		  AND bu.status = $3
		  AND b.status = $3`

	row := r.db.QueryRow(ctx, query, userID, businessID, StatusActive)

	var access Access
	err := row.Scan(
		&access.Membership.ID, &access.Membership.BusinessID, &access.Membership.UserID, &access.Membership.RoleID, &access.Membership.Status, &access.Membership.CreatedAt, &access.Membership.UpdatedAt,
		&access.Business.ID, &access.Business.Name, &access.Business.Slug, &access.Business.Type, &access.Business.Timezone, &access.Business.Status, &access.Business.CreatedAt, &access.Business.UpdatedAt,
		&access.Role.ID, &access.Role.Name, &access.Role.Description, &access.Role.CreatedAt, &access.Role.UpdatedAt,
	)
	if err != nil {
		return nil, wrapError(err)
	}

	return &access, nil
}

// Get returns the membership of a user in a business, or ErrNotFound.
func (r *BusinessUserRepository) Get(ctx context.Context, businessID, userID uuid.UUID) (*BusinessUser, error) {
	const query = `
		SELECT ` + businessUserColumns + `
		FROM business_users
		WHERE business_id = $1 AND user_id = $2`

	return scanBusinessUser(r.db.QueryRow(ctx, query, businessID, userID))
}

// ListByBusiness returns every membership of a business.
func (r *BusinessUserRepository) ListByBusiness(ctx context.Context, businessID uuid.UUID) ([]BusinessUser, error) {
	const query = `
		SELECT ` + businessUserColumns + `
		FROM business_users
		WHERE business_id = $1
		ORDER BY created_at, id`

	return r.list(ctx, query, businessID)
}

// ListByUser returns every business a user has access to.
func (r *BusinessUserRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]BusinessUser, error) {
	const query = `
		SELECT ` + businessUserColumns + `
		FROM business_users
		WHERE user_id = $1
		ORDER BY created_at, id`

	return r.list(ctx, query, userID)
}

// UpdateBusinessUserParams carries the mutable fields of a membership.
type UpdateBusinessUserParams struct {
	RoleID uuid.UUID
	Status Status
}

// Update changes the role or the status of a membership. Setting the status to
// INACTIVE is how access is revoked; memberships are not deleted.
func (r *BusinessUserRepository) Update(ctx context.Context, id uuid.UUID, params UpdateBusinessUserParams) (*BusinessUser, error) {
	const query = `
		UPDATE business_users
		SET role_id = $2, status = $3
		WHERE id = $1
		RETURNING ` + businessUserColumns

	return scanBusinessUser(r.db.QueryRow(ctx, query, id, params.RoleID, params.Status))
}

func (r *BusinessUserRepository) list(ctx context.Context, query string, arg uuid.UUID) ([]BusinessUser, error) {
	rows, err := r.db.Query(ctx, query, arg)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()

	var memberships []BusinessUser
	for rows.Next() {
		membership, err := scanBusinessUser(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, *membership)
	}

	return memberships, wrapError(rows.Err())
}

func scanBusinessUser(row pgx.Row) (*BusinessUser, error) {
	var bu BusinessUser
	err := row.Scan(&bu.ID, &bu.BusinessID, &bu.UserID, &bu.RoleID, &bu.Status, &bu.CreatedAt, &bu.UpdatedAt)
	if err != nil {
		return nil, wrapError(err)
	}

	return &bu, nil
}
