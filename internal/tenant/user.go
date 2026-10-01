package tenant

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// User is application level profile data for a person. Credentials are owned
// by Supabase Auth; AuthUserID points at the matching auth user.
type User struct {
	ID         uuid.UUID
	AuthUserID uuid.UUID
	FullName   string
	Email      string
	Phone      *string
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const userColumns = `id, auth_user_id, full_name, email, phone, status, created_at, updated_at`

// UserRepository reads and writes user profiles.
type UserRepository struct {
	db Querier
}

// NewUserRepository returns a repository backed by db.
func NewUserRepository(db Querier) *UserRepository {
	return &UserRepository{db: db}
}

// CreateUserParams describes a new profile for an existing Supabase Auth user.
type CreateUserParams struct {
	AuthUserID uuid.UUID
	FullName   string
	Email      string
	Phone      *string
}

// Create inserts a user profile and returns the stored row.
func (r *UserRepository) Create(ctx context.Context, params CreateUserParams) (*User, error) {
	const query = `
		INSERT INTO users (auth_user_id, full_name, email, phone)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + userColumns

	row := r.db.QueryRow(ctx, query, params.AuthUserID, params.FullName, NormalizeEmail(params.Email), params.Phone)

	return scanUser(row)
}

// GetByID returns the user with the given id, or ErrNotFound.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	const query = `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	return scanUser(r.db.QueryRow(ctx, query, id))
}

// GetByAuthUserID looks a profile up by its Supabase Auth user id.
func (r *UserRepository) GetByAuthUserID(ctx context.Context, authUserID uuid.UUID) (*User, error) {
	const query = `SELECT ` + userColumns + ` FROM users WHERE auth_user_id = $1`

	return scanUser(r.db.QueryRow(ctx, query, authUserID))
}

// GetActiveByAuthUserID returns the active profile for a Supabase Auth user.
// Missing and inactive users both surface as ErrNotFound.
func (r *UserRepository) GetActiveByAuthUserID(ctx context.Context, authUserID uuid.UUID) (*User, error) {
	const query = `
		SELECT ` + userColumns + `
		FROM users
		WHERE auth_user_id = $1 AND status = $2`

	return scanUser(r.db.QueryRow(ctx, query, authUserID, StatusActive))
}

// GetByEmail looks a profile up by email address.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	const query = `SELECT ` + userColumns + ` FROM users WHERE email = $1`

	return scanUser(r.db.QueryRow(ctx, query, NormalizeEmail(email)))
}

// UpdateUserParams carries the mutable fields of a profile. The Supabase Auth
// user id is immutable.
type UpdateUserParams struct {
	FullName string
	Email    string
	Phone    *string
	Status   Status
}

// Update overwrites the mutable fields of a user profile.
func (r *UserRepository) Update(ctx context.Context, id uuid.UUID, params UpdateUserParams) (*User, error) {
	const query = `
		UPDATE users
		SET full_name = $2, email = $3, phone = $4, status = $5
		WHERE id = $1
		RETURNING ` + userColumns

	row := r.db.QueryRow(ctx, query, id, params.FullName, NormalizeEmail(params.Email), params.Phone, params.Status)

	return scanUser(row)
}

// NormalizeEmail returns the storage form of an email address. The users table
// only accepts lower case addresses so lookups stay case insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.AuthUserID, &u.FullName, &u.Email, &u.Phone, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, wrapError(err)
	}

	return &u, nil
}
