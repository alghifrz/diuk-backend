package area

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	areaColumns        = `a.id, a.business_id, a.name, a.description, a.status, a.created_at, a.updated_at`
	areaCounts         = `COUNT(t.id)::int AS table_count, COUNT(t.id) FILTER (WHERE t.status = 'ACTIVE')::int AS active_table_count`
	tableColumns       = `t.id, t.area_id, t.name, t.capacity, t.status, t.created_at, t.updated_at`
	tableInsertColumns = `id, area_id, name, capacity, status, created_at, updated_at`
)

// Store is the persistence used by the service.
type Store interface {
	ListAreas(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Area, error)
	GetArea(ctx context.Context, businessID, areaID uuid.UUID) (*Area, error)
	CreateArea(ctx context.Context, businessID uuid.UUID, name string, description *string) (*Area, error)
	UpdateArea(ctx context.Context, businessID uuid.UUID, area Area) (*Area, error)
	CountActiveTables(ctx context.Context, businessID, areaID uuid.UUID) (int, error)

	ListTables(ctx context.Context, businessID, areaID uuid.UUID, status tenant.Status) ([]Table, error)
	GetTable(ctx context.Context, businessID, areaID, tableID uuid.UUID) (*Table, error)
	CreateTable(ctx context.Context, businessID, areaID uuid.UUID, name string, capacity int) (*Table, error)
	UpdateTable(ctx context.Context, businessID, areaID uuid.UUID, table Table) (*Table, error)
}

// Repository talks to PostgreSQL. Tenant identity always comes from the caller.
type Repository struct {
	db tenant.Querier
}

// NewRepository returns a repository backed by db.
func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListAreas(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Area, error) {
	const query = `
		SELECT ` + areaColumns + `, ` + areaCounts + `
		FROM areas a
		LEFT JOIN tables t ON t.area_id = a.id
		WHERE a.business_id = $1 AND a.status = $2
		GROUP BY a.id
		ORDER BY a.name ASC`

	rows, err := r.db.Query(ctx, query, businessID, status)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	areas := make([]Area, 0)
	for rows.Next() {
		area, err := scanArea(rows)
		if err != nil {
			return nil, err
		}
		areas = append(areas, *area)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}
	return areas, nil
}

func (r *Repository) GetArea(ctx context.Context, businessID, areaID uuid.UUID) (*Area, error) {
	const query = `
		SELECT ` + areaColumns + `, ` + areaCounts + `
		FROM areas a
		LEFT JOIN tables t ON t.area_id = a.id
		WHERE a.business_id = $1 AND a.id = $2
		GROUP BY a.id`

	return scanArea(r.db.QueryRow(ctx, query, businessID, areaID))
}

func (r *Repository) CreateArea(ctx context.Context, businessID uuid.UUID, name string, description *string) (*Area, error) {
	const query = `
		INSERT INTO areas (business_id, name, description, status)
		VALUES ($1, $2, $3, 'ACTIVE')
		RETURNING id, business_id, name, description, status, created_at, updated_at`

	var a Area
	err := r.db.QueryRow(ctx, query, businessID, name, description).Scan(
		&a.ID, &a.BusinessID, &a.Name, &a.Description, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, mapDBError(err)
	}
	return &a, nil
}

func (r *Repository) UpdateArea(ctx context.Context, businessID uuid.UUID, area Area) (*Area, error) {
	const query = `
		UPDATE areas
		SET name = $3, description = $4, status = $5
		WHERE business_id = $1 AND id = $2
		RETURNING id`

	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, area.ID, area.Name, area.Description, area.Status).Scan(&id); err != nil {
		return nil, mapDBError(err)
	}
	return r.GetArea(ctx, businessID, area.ID)
}

func (r *Repository) CountActiveTables(ctx context.Context, businessID, areaID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)::int
		FROM tables t
		JOIN areas a ON a.id = t.area_id
		WHERE t.area_id = $1
		  AND a.business_id = $2
		  AND t.status = 'ACTIVE'`

	var n int
	if err := r.db.QueryRow(ctx, query, areaID, businessID).Scan(&n); err != nil {
		return 0, mapDBError(err)
	}
	return n, nil
}

func (r *Repository) ListTables(ctx context.Context, businessID, areaID uuid.UUID, status tenant.Status) ([]Table, error) {
	const query = `
		SELECT ` + tableColumns + `
		FROM tables t
		JOIN areas a ON a.id = t.area_id
		WHERE t.area_id = $1
		  AND a.business_id = $2
		  AND t.status = $3
		ORDER BY t.name ASC`

	rows, err := r.db.Query(ctx, query, areaID, businessID, status)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	tables := make([]Table, 0)
	for rows.Next() {
		table, err := scanTable(rows)
		if err != nil {
			return nil, err
		}
		tables = append(tables, *table)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}
	return tables, nil
}

func (r *Repository) GetTable(ctx context.Context, businessID, areaID, tableID uuid.UUID) (*Table, error) {
	const query = `
		SELECT ` + tableColumns + `
		FROM tables t
		JOIN areas a ON a.id = t.area_id
		WHERE t.id = $1
		  AND t.area_id = $2
		  AND a.business_id = $3`

	return scanTable(r.db.QueryRow(ctx, query, tableID, areaID, businessID))
}

func (r *Repository) CreateTable(ctx context.Context, businessID, areaID uuid.UUID, name string, capacity int) (*Table, error) {
	const query = `
		INSERT INTO tables (area_id, name, capacity, status)
		SELECT a.id, $3, $4, 'ACTIVE'
		FROM areas a
		WHERE a.id = $2 AND a.business_id = $1
		RETURNING ` + tableInsertColumns

	return scanTable(r.db.QueryRow(ctx, query, businessID, areaID, name, capacity))
}

func (r *Repository) UpdateTable(ctx context.Context, businessID, areaID uuid.UUID, table Table) (*Table, error) {
	const query = `
		UPDATE tables t
		SET name = $4, capacity = $5, status = $6
		FROM areas a
		WHERE t.id = $3
		  AND t.area_id = $2
		  AND a.id = t.area_id
		  AND a.business_id = $1
		RETURNING ` + tableColumns

	return scanTable(r.db.QueryRow(ctx, query, businessID, areaID, table.ID, table.Name, table.Capacity, table.Status))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanArea(row scanner) (*Area, error) {
	var a Area
	err := row.Scan(&a.ID, &a.BusinessID, &a.Name, &a.Description, &a.Status, &a.CreatedAt, &a.UpdatedAt, &a.TableCount, &a.ActiveTableCount)
	if err != nil {
		return nil, mapDBError(err)
	}
	return &a, nil
}

func scanTable(row scanner) (*Table, error) {
	var t Table
	err := row.Scan(&t.ID, &t.AreaID, &t.Name, &t.Capacity, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return &t, nil
}

func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "uq_areas_business_id_name":
				return ErrAreaNameExists
			case "uq_tables_area_id_name":
				return ErrTableNameExists
			}
		case "23514":
			return invalid("invalid value")
		}
	}

	return fmt.Errorf("database: %w", err)
}
