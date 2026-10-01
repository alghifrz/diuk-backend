package authz

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is the slice of pgx used by the repository. Both *pgxpool.Pool and
// pgx.Tx satisfy it.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Repository reads role-permission mappings from PostgreSQL.
type Repository struct {
	db Querier
}

// NewRepository returns a repository backed by db.
func NewRepository(db Querier) *Repository {
	return &Repository{db: db}
}

// GetPermissionsByRoleID returns the permission codes granted to roleID,
// ordered alphabetically. The role id must come from the tenant context.
func (r *Repository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	const query = `
		SELECT p.code
		FROM role_permissions rp
		INNER JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.code`

	rows, err := r.db.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	if codes == nil {
		codes = []string{}
	}

	return codes, nil
}

// HasPermission reports whether roleID is mapped to the given permission code.
func (r *Repository) HasPermission(ctx context.Context, roleID uuid.UUID, permission Permission) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM role_permissions rp
			INNER JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.role_id = $1 AND p.code = $2
		)`

	var allowed bool
	if err := r.db.QueryRow(ctx, query, roleID, permission).Scan(&allowed); err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}

	return allowed, nil
}
