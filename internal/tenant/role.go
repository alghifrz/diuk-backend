package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Role is a named set of responsibilities a user can hold inside a business.
// Permissions for a role live in role_permissions and are resolved by authz.
type Role struct {
	ID          uuid.UUID
	Name        RoleName
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const roleColumns = `id, name, description, created_at, updated_at`

// RoleRepository reads the roles seeded by the migrations.
type RoleRepository struct {
	db Querier
}

// NewRoleRepository returns a repository backed by db.
func NewRoleRepository(db Querier) *RoleRepository {
	return &RoleRepository{db: db}
}

// List returns every role ordered by name.
func (r *RoleRepository) List(ctx context.Context) ([]Role, error) {
	const query = `SELECT ` + roleColumns + ` FROM roles ORDER BY name`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, *role)
	}

	return roles, wrapError(rows.Err())
}

// GetByID returns the role with the given id, or ErrNotFound.
func (r *RoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*Role, error) {
	const query = `SELECT ` + roleColumns + ` FROM roles WHERE id = $1`

	return scanRole(r.db.QueryRow(ctx, query, id))
}

// GetByName returns the role with the given name, or ErrNotFound.
func (r *RoleRepository) GetByName(ctx context.Context, name RoleName) (*Role, error) {
	const query = `SELECT ` + roleColumns + ` FROM roles WHERE name = $1`

	return scanRole(r.db.QueryRow(ctx, query, name))
}

func scanRole(row pgx.Row) (*Role, error) {
	var role Role
	if err := row.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return nil, wrapError(err)
	}

	return &role, nil
}
