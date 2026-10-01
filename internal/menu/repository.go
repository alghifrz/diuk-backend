package menu

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
	categoryColumns = `c.id, c.business_id, c.name, c.description, c.status, c.sort_order, c.created_at, c.updated_at`
	categoryCounts  = `COUNT(i.id)::int AS item_count, COUNT(i.id) FILTER (WHERE i.status = 'ACTIVE')::int AS active_item_count`
	itemColumns     = `i.id, i.business_id, i.category_id, c.name AS category_name, i.name, i.description, i.price::text, i.image_path, i.status, i.sort_order, i.created_at, i.updated_at`
)

// Store is the persistence used by the service.
type Store interface {
	ListCategories(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Category, error)
	GetCategory(ctx context.Context, businessID, categoryID uuid.UUID) (*Category, error)
	CreateCategory(ctx context.Context, businessID uuid.UUID, name string, description *string, sortOrder int) (*Category, error)
	UpdateCategory(ctx context.Context, businessID uuid.UUID, category Category) (*Category, error)
	CountActiveItems(ctx context.Context, businessID, categoryID uuid.UUID) (int, error)

	ListItems(ctx context.Context, businessID uuid.UUID, status tenant.Status, categoryID *uuid.UUID) ([]Item, error)
	GetItem(ctx context.Context, businessID, itemID uuid.UUID) (*Item, error)
	CreateItem(ctx context.Context, item Item) (*Item, error)
	UpdateItem(ctx context.Context, businessID uuid.UUID, item Item) (*Item, error)
}

// Repository talks to PostgreSQL. Tenant identity always comes from the caller.
type Repository struct {
	db tenant.Querier
}

// NewRepository returns a repository backed by db.
func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListCategories(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Category, error) {
	const query = `
		SELECT ` + categoryColumns + `, ` + categoryCounts + `
		FROM menu_categories c
		LEFT JOIN menu_items i ON i.category_id = c.id AND i.business_id = c.business_id
		WHERE c.business_id = $1 AND c.status = $2
		GROUP BY c.id
		ORDER BY c.sort_order ASC, c.name ASC`

	rows, err := r.db.Query(ctx, query, businessID, status)
	if err != nil {
		return nil, mapDBError(err, false)
	}
	defer rows.Close()

	out := make([]Category, 0)
	for rows.Next() {
		cat, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *cat)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err, false)
	}
	return out, nil
}

func (r *Repository) GetCategory(ctx context.Context, businessID, categoryID uuid.UUID) (*Category, error) {
	const query = `
		SELECT ` + categoryColumns + `, ` + categoryCounts + `
		FROM menu_categories c
		LEFT JOIN menu_items i ON i.category_id = c.id AND i.business_id = c.business_id
		WHERE c.business_id = $1 AND c.id = $2
		GROUP BY c.id`

	return scanCategory(r.db.QueryRow(ctx, query, businessID, categoryID))
}

func (r *Repository) CreateCategory(ctx context.Context, businessID uuid.UUID, name string, description *string, sortOrder int) (*Category, error) {
	const query = `
		INSERT INTO menu_categories (business_id, name, description, status, sort_order)
		VALUES ($1, $2, $3, 'ACTIVE', $4)
		RETURNING id, business_id, name, description, status, sort_order, created_at, updated_at`

	var c Category
	err := r.db.QueryRow(ctx, query, businessID, name, description, sortOrder).Scan(
		&c.ID, &c.BusinessID, &c.Name, &c.Description, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, mapDBError(err, false)
	}
	return &c, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, businessID uuid.UUID, category Category) (*Category, error) {
	const query = `
		UPDATE menu_categories
		SET name = $3, description = $4, status = $5, sort_order = $6
		WHERE business_id = $1 AND id = $2
		RETURNING id`

	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, category.ID, category.Name, category.Description, category.Status, category.SortOrder).Scan(&id); err != nil {
		return nil, mapDBError(err, false)
	}
	return r.GetCategory(ctx, businessID, category.ID)
}

func (r *Repository) CountActiveItems(ctx context.Context, businessID, categoryID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)::int
		FROM menu_items
		WHERE business_id = $1 AND category_id = $2 AND status = 'ACTIVE'`

	var n int
	if err := r.db.QueryRow(ctx, query, businessID, categoryID).Scan(&n); err != nil {
		return 0, mapDBError(err, true)
	}
	return n, nil
}

func (r *Repository) ListItems(ctx context.Context, businessID uuid.UUID, status tenant.Status, categoryID *uuid.UUID) ([]Item, error) {
	query := `
		SELECT ` + itemColumns + `
		FROM menu_items i
		JOIN menu_categories c ON c.id = i.category_id AND c.business_id = i.business_id
		WHERE i.business_id = $1 AND i.status = $2`
	args := []any{businessID, status}
	if categoryID != nil {
		query += ` AND i.category_id = $3`
		args = append(args, *categoryID)
	}
	query += ` ORDER BY c.sort_order ASC, i.sort_order ASC, i.name ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, true)
	}
	defer rows.Close()

	out := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err, true)
	}
	return out, nil
}

func (r *Repository) GetItem(ctx context.Context, businessID, itemID uuid.UUID) (*Item, error) {
	const query = `
		SELECT ` + itemColumns + `
		FROM menu_items i
		JOIN menu_categories c ON c.id = i.category_id AND c.business_id = i.business_id
		WHERE i.business_id = $1 AND i.id = $2`

	return scanItem(r.db.QueryRow(ctx, query, businessID, itemID))
}

func (r *Repository) CreateItem(ctx context.Context, item Item) (*Item, error) {
	const query = `
		INSERT INTO menu_items (id, business_id, category_id, name, description, price, image_path, status, sort_order)
		SELECT $1, $2, c.id, $4, $5, $6::numeric, $7, 'ACTIVE', $8
		FROM menu_categories c
		WHERE c.id = $3 AND c.business_id = $2
		RETURNING id`

	var id uuid.UUID
	err := r.db.QueryRow(ctx, query, item.ID, item.BusinessID, item.CategoryID, item.Name, item.Description, item.Price, item.ImagePath, item.SortOrder).Scan(&id)
	if err != nil {
		return nil, mapDBError(err, true)
	}
	return r.GetItem(ctx, item.BusinessID, item.ID)
}

func (r *Repository) UpdateItem(ctx context.Context, businessID uuid.UUID, item Item) (*Item, error) {
	const query = `
		UPDATE menu_items
		SET name = $3, description = $4, price = $5::numeric, image_path = $6, status = $7, sort_order = $8
		WHERE business_id = $1 AND id = $2
		RETURNING id`

	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, item.ID, item.Name, item.Description, item.Price, item.ImagePath, item.Status, item.SortOrder).Scan(&id); err != nil {
		return nil, mapDBError(err, true)
	}
	return r.GetItem(ctx, businessID, item.ID)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCategory(row scanner) (*Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.BusinessID, &c.Name, &c.Description, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.ItemCount, &c.ActiveItemCount)
	if err != nil {
		return nil, mapDBError(err, false)
	}
	return &c, nil
}

func scanItem(row scanner) (*Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.BusinessID, &item.CategoryID, &item.CategoryName, &item.Name, &item.Description, &item.Price, &item.ImagePath, &item.Status, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err, true)
	}
	return &item, nil
}

func mapDBError(err error, item bool) error {
	if errors.Is(err, pgx.ErrNoRows) {
		if item {
			return ErrItemNotFound
		}
		return ErrCategoryNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "uq_menu_categories_business_id_name" {
				return ErrCategoryNameExists
			}
		case "23514":
			return invalid("invalid value")
		}
	}

	return fmt.Errorf("database: %w", err)
}
