// Package tenant holds the core multi-tenant entities of DIUK: businesses
// (the tenants), the users that work in them, the roles a user can hold and
// the membership that links a user to a business.
package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Errors returned by the repositories in this package.
var (
	ErrNotFound         = errors.New("record not found")
	ErrConflict         = errors.New("record already exists")
	ErrInvalidReference = errors.New("referenced record does not exist")
)

// PostgreSQL error codes handled by the repositories.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
)

// Status is the lifecycle state shared by the core entities.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// BusinessType is the category of business a tenant operates.
type BusinessType string

// BusinessTypeFNB covers food and beverage businesses, the only category
// supported at this stage.
const BusinessTypeFNB BusinessType = "FNB"

// RoleName identifies one of the seeded roles.
type RoleName string

const (
	RoleOwner   RoleName = "OWNER"
	RoleManager RoleName = "MANAGER"
	RoleStaff   RoleName = "STAFF"
)

// Querier is the slice of pgx used by the repositories. Both *pgxpool.Pool and
// pgx.Tx satisfy it, so a repository can run standalone or inside a
// transaction without changing its API.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// wrapError maps database errors onto the sentinels of this package so callers
// do not have to know pgx or PostgreSQL error codes.
func wrapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codeUniqueViolation:
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case codeForeignKeyViolation:
			return fmt.Errorf("%w: %s", ErrInvalidReference, pgErr.ConstraintName)
		}
	}

	return err
}
