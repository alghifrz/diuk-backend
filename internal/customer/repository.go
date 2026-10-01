package customer

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
	customerColumns = `c.id, c.business_id, c.name, c.phone, c.email, c.notes, c.status, c.marketing_opt_in, c.created_at, c.updated_at`
	customerCounts  = `
		COUNT(DISTINCT i.id)::int AS identity_count,
		COUNT(DISTINCT t.id) FILTER (WHERE t.status = 'ACTIVE')::int AS active_tag_count,
		(SELECT COUNT(*) FROM reservations r
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id)::int AS reservation_count,
		(SELECT COUNT(*) FROM reservations r
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id AND r.status = 'COMPLETED')::int AS completed_count,
		(SELECT COUNT(*) FROM reservations r
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id AND r.status = 'CANCELLED')::int AS cancelled_count,
		-- A completed visit is always settled in full: spend is the full order
		-- total (menu items), falling back to the recorded payment amount.
		COALESCE((SELECT SUM(
		      CASE WHEN COALESCE(ot.total, 0) > 0 THEN ot.total ELSE p.amount END)
		  FROM reservations r
		  JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
		  LEFT JOIN LATERAL (
		      SELECT SUM(ri.unit_price * ri.quantity) AS total
		      FROM reservation_items ri
		      WHERE ri.reservation_id = r.id AND ri.business_id = r.business_id
		  ) ot ON true
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id
		    AND r.status = 'COMPLETED' AND p.status IN ('PAID', 'DEPOSIT')), 0) AS total_spent,
		(SELECT MAX(COALESCE(r.completed_at, r.start_at)) FROM reservations r
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id AND r.status = 'COMPLETED') AS last_visit_at,
		(SELECT MIN(r.start_at) FROM reservations r
		  WHERE r.business_id = c.business_id AND r.customer_id = c.id
		    AND r.status IN ('PENDING', 'CONFIRMED') AND r.start_at >= now()) AS next_reservation_at`
	identityColumns = `id, business_id, customer_id, channel, external_id, display_name, status, created_at, updated_at`
	tagColumns      = `id, business_id, name, description, status, created_at, updated_at`
)

// Store is the persistence used by the service.
type Store interface {
	ListCustomers(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Customer, error)
	GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*Customer, error)
	CreateCustomer(ctx context.Context, businessID uuid.UUID, in CreateCustomerInput) (*Customer, error)
	UpdateCustomer(ctx context.Context, businessID uuid.UUID, customer Customer) (*Customer, error)

	ListIdentities(ctx context.Context, businessID, customerID uuid.UUID, status tenant.Status) ([]Identity, error)
	GetIdentity(ctx context.Context, businessID, customerID, identityID uuid.UUID) (*Identity, error)
	FindIdentityByExternal(ctx context.Context, businessID uuid.UUID, channel Channel, externalID string) (*Identity, error)
	CreateIdentity(ctx context.Context, identity Identity) (*Identity, error)
	UpdateIdentity(ctx context.Context, businessID uuid.UUID, identity Identity) (*Identity, error)

	ListTags(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Tag, error)
	GetTag(ctx context.Context, businessID, tagID uuid.UUID) (*Tag, error)
	CreateTag(ctx context.Context, businessID uuid.UUID, name string, description *string) (*Tag, error)
	UpdateTag(ctx context.Context, businessID uuid.UUID, tag Tag) (*Tag, error)

	ListCustomerTags(ctx context.Context, businessID, customerID uuid.UUID) ([]Tag, error)
	AssignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) error
	UnassignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) error
}

// Repository talks to PostgreSQL. Tenant identity always comes from the caller.
type Repository struct {
	db tenant.Querier
}

// NewRepository returns a repository backed by db.
func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListCustomers(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Customer, error) {
	query := `
		SELECT ` + customerColumns + `, ` + customerCounts + `
		FROM customers c
		LEFT JOIN customer_identities i ON i.customer_id = c.id AND i.business_id = c.business_id
		LEFT JOIN customer_tags ct ON ct.customer_id = c.id AND ct.business_id = c.business_id
		LEFT JOIN tags t ON t.id = ct.tag_id AND t.business_id = c.business_id
		WHERE c.business_id = $1 AND c.status = $2`
	args := []any{businessID, params.Status}
	if params.Search != "" {
		args = append(args, params.Search)
		query += fmt.Sprintf(`
		AND (
			c.name ILIKE $%[1]d ESCAPE '\'
			OR COALESCE(c.phone, '') ILIKE $%[1]d ESCAPE '\'
			OR COALESCE(c.email, '') ILIKE $%[1]d ESCAPE '\'
		)`, len(args))
	}
	if params.TagID != nil {
		args = append(args, *params.TagID)
		query += fmt.Sprintf(`
		AND EXISTS (
			SELECT 1 FROM customer_tags ft
			JOIN tags fto ON fto.id = ft.tag_id AND fto.business_id = ft.business_id
			WHERE ft.customer_id = c.id AND ft.business_id = c.business_id
			  AND ft.tag_id = $%d AND fto.status = 'ACTIVE'
		)`, len(args))
	}
	args = append(args, params.Limit, params.Offset)
	query += fmt.Sprintf(`
		GROUP BY c.id
		ORDER BY %s
		LIMIT $%d OFFSET $%d`, listOrderBy(params.Sort), len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, "customer")
	}
	defer rows.Close()

	out := make([]Customer, 0)
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err, "customer")
	}
	return out, nil
}

func (r *Repository) GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*Customer, error) {
	const query = `
		SELECT ` + customerColumns + `, ` + customerCounts + `
		FROM customers c
		LEFT JOIN customer_identities i ON i.customer_id = c.id AND i.business_id = c.business_id
		LEFT JOIN customer_tags ct ON ct.customer_id = c.id AND ct.business_id = c.business_id
		LEFT JOIN tags t ON t.id = ct.tag_id AND t.business_id = c.business_id
		WHERE c.business_id = $1 AND c.id = $2
		GROUP BY c.id`

	c, err := scanCustomer(r.db.QueryRow(ctx, query, businessID, customerID))
	if err != nil {
		return nil, err
	}
	identities, err := r.listIdentitiesAll(ctx, businessID, customerID)
	if err != nil {
		return nil, err
	}
	tags, err := r.ListCustomerTags(ctx, businessID, customerID)
	if err != nil {
		return nil, err
	}
	c.Identities = identities
	c.Tags = tags
	return c, nil
}

func (r *Repository) CreateCustomer(ctx context.Context, businessID uuid.UUID, in CreateCustomerInput) (*Customer, error) {
	const query = `
		INSERT INTO customers (business_id, name, phone, email, notes, status, marketing_opt_in)
		VALUES ($1, $2, $3, $4, $5, 'ACTIVE', $6)
		RETURNING id`

	optIn := true
	if in.MarketingOptIn != nil {
		optIn = *in.MarketingOptIn
	}
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, in.Name, in.Phone, in.Email, in.Notes, optIn).Scan(&id); err != nil {
		return nil, mapDBError(err, "customer")
	}
	return r.GetCustomer(ctx, businessID, id)
}

func (r *Repository) UpdateCustomer(ctx context.Context, businessID uuid.UUID, customer Customer) (*Customer, error) {
	const query = `
		UPDATE customers
		SET name = $3, phone = $4, email = $5, notes = $6, status = $7, marketing_opt_in = $8
		WHERE business_id = $1 AND id = $2
		RETURNING id`

	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, customer.ID, customer.Name, customer.Phone, customer.Email, customer.Notes, customer.Status, customer.MarketingOptIn).Scan(&id); err != nil {
		return nil, mapDBError(err, "customer")
	}
	return r.GetCustomer(ctx, businessID, customer.ID)
}

func (r *Repository) ListIdentities(ctx context.Context, businessID, customerID uuid.UUID, status tenant.Status) ([]Identity, error) {
	const query = `
		SELECT ` + identityColumns + `
		FROM customer_identities
		WHERE business_id = $1 AND customer_id = $2 AND status = $3
		ORDER BY created_at DESC, id DESC`

	return scanIdentities(r.db.Query(ctx, query, businessID, customerID, status))
}

func (r *Repository) listIdentitiesAll(ctx context.Context, businessID, customerID uuid.UUID) ([]Identity, error) {
	const query = `
		SELECT ` + identityColumns + `
		FROM customer_identities
		WHERE business_id = $1 AND customer_id = $2
		ORDER BY created_at DESC, id DESC`

	return scanIdentities(r.db.Query(ctx, query, businessID, customerID))
}

func (r *Repository) GetIdentity(ctx context.Context, businessID, customerID, identityID uuid.UUID) (*Identity, error) {
	const query = `
		SELECT ` + identityColumns + `
		FROM customer_identities
		WHERE id = $1 AND customer_id = $2 AND business_id = $3`

	return scanIdentity(r.db.QueryRow(ctx, query, identityID, customerID, businessID))
}

func (r *Repository) FindIdentityByExternal(ctx context.Context, businessID uuid.UUID, channel Channel, externalID string) (*Identity, error) {
	const query = `
		SELECT ` + identityColumns + `
		FROM customer_identities
		WHERE business_id = $1 AND channel = $2 AND external_id = $3`
	item, err := scanIdentity(r.db.QueryRow(ctx, query, businessID, channel, externalID))
	if errors.Is(err, ErrIdentityNotFound) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) CreateIdentity(ctx context.Context, identity Identity) (*Identity, error) {
	const query = `
		INSERT INTO customer_identities (business_id, customer_id, channel, external_id, display_name, status)
		SELECT $1, c.id, $3, $4, $5, 'ACTIVE'
		FROM customers c
		WHERE c.id = $2 AND c.business_id = $1
		RETURNING ` + identityColumns

	return scanIdentity(r.db.QueryRow(ctx, query, identity.BusinessID, identity.CustomerID, identity.Channel, identity.ExternalID, identity.DisplayName))
}

func (r *Repository) UpdateIdentity(ctx context.Context, businessID uuid.UUID, identity Identity) (*Identity, error) {
	const query = `
		UPDATE customer_identities
		SET display_name = $4, status = $5
		WHERE id = $1 AND customer_id = $2 AND business_id = $3
		RETURNING ` + identityColumns

	return scanIdentity(r.db.QueryRow(ctx, query, identity.ID, identity.CustomerID, businessID, identity.DisplayName, identity.Status))
}

func (r *Repository) ListTags(ctx context.Context, businessID uuid.UUID, status tenant.Status) ([]Tag, error) {
	const query = `
		SELECT ` + tagColumns + `
		FROM tags
		WHERE business_id = $1 AND status = $2
		ORDER BY name ASC`

	return scanTags(r.db.Query(ctx, query, businessID, status))
}

func (r *Repository) GetTag(ctx context.Context, businessID, tagID uuid.UUID) (*Tag, error) {
	const query = `SELECT ` + tagColumns + ` FROM tags WHERE business_id = $1 AND id = $2`
	return scanTag(r.db.QueryRow(ctx, query, businessID, tagID))
}

func (r *Repository) CreateTag(ctx context.Context, businessID uuid.UUID, name string, description *string) (*Tag, error) {
	const query = `
		INSERT INTO tags (business_id, name, description, status)
		VALUES ($1, $2, $3, 'ACTIVE')
		RETURNING ` + tagColumns

	return scanTag(r.db.QueryRow(ctx, query, businessID, name, description))
}

func (r *Repository) UpdateTag(ctx context.Context, businessID uuid.UUID, tag Tag) (*Tag, error) {
	const query = `
		UPDATE tags
		SET name = $3, description = $4, status = $5
		WHERE business_id = $1 AND id = $2
		RETURNING ` + tagColumns

	return scanTag(r.db.QueryRow(ctx, query, businessID, tag.ID, tag.Name, tag.Description, tag.Status))
}

func (r *Repository) ListCustomerTags(ctx context.Context, businessID, customerID uuid.UUID) ([]Tag, error) {
	const query = `
		SELECT t.id, t.business_id, t.name, t.description, t.status, t.created_at, t.updated_at
		FROM customer_tags ct
		JOIN tags t ON t.id = ct.tag_id AND t.business_id = ct.business_id
		WHERE ct.business_id = $1 AND ct.customer_id = $2
		ORDER BY t.name ASC`

	return scanTags(r.db.Query(ctx, query, businessID, customerID))
}

func (r *Repository) AssignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) error {
	const query = `
		INSERT INTO customer_tags (customer_id, tag_id, business_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (customer_id, tag_id) DO NOTHING`

	if _, err := r.db.Exec(ctx, query, customerID, tagID, businessID); err != nil {
		return mapDBError(err, "assignment")
	}
	return nil
}

func (r *Repository) UnassignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) error {
	const query = `
		DELETE FROM customer_tags
		WHERE business_id = $1 AND customer_id = $2 AND tag_id = $3`

	tag, err := r.db.Exec(ctx, query, businessID, customerID, tagID)
	if err != nil {
		return mapDBError(err, "assignment")
	}
	if tag.RowsAffected() == 0 {
		return ErrAssignmentNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

// listOrderBy returns a fixed ORDER BY clause; never interpolates user input.
func listOrderBy(sort Sort) string {
	switch sort {
	case SortName:
		return "LOWER(c.name) ASC, c.id ASC"
	case SortSpend:
		return "total_spent DESC, c.created_at DESC, c.id DESC"
	case SortVisits:
		return "completed_count DESC, c.created_at DESC, c.id DESC"
	case SortLastVisit:
		return "last_visit_at DESC NULLS LAST, c.created_at DESC, c.id DESC"
	default:
		return "c.created_at DESC, c.id DESC"
	}
}

func scanCustomer(row scanner) (*Customer, error) {
	var c Customer
	err := row.Scan(
		&c.ID, &c.BusinessID, &c.Name, &c.Phone, &c.Email, &c.Notes, &c.Status, &c.MarketingOptIn, &c.CreatedAt, &c.UpdatedAt,
		&c.IdentityCount, &c.ActiveTagCount,
		&c.Stats.ReservationCount, &c.Stats.CompletedCount, &c.Stats.CancelledCount,
		&c.Stats.TotalSpent, &c.Stats.LastVisitAt, &c.Stats.NextReservationAt,
	)
	if err != nil {
		return nil, mapDBError(err, "customer")
	}
	return &c, nil
}

func scanIdentity(row scanner) (*Identity, error) {
	var i Identity
	err := row.Scan(&i.ID, &i.BusinessID, &i.CustomerID, &i.Channel, &i.ExternalID, &i.DisplayName, &i.Status, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err, "identity")
	}
	return &i, nil
}

func scanTag(row scanner) (*Tag, error) {
	var t Tag
	err := row.Scan(&t.ID, &t.BusinessID, &t.Name, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err, "tag")
	}
	return &t, nil
}

func scanIdentities(rows pgx.Rows, err error) ([]Identity, error) {
	if err != nil {
		return nil, mapDBError(err, "identity")
	}
	defer rows.Close()
	out := make([]Identity, 0)
	for rows.Next() {
		item, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err, "identity")
	}
	return out, nil
}

func scanTags(rows pgx.Rows, err error) ([]Tag, error) {
	if err != nil {
		return nil, mapDBError(err, "tag")
	}
	defer rows.Close()
	out := make([]Tag, 0)
	for rows.Next() {
		item, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err, "tag")
	}
	return out, nil
}

func mapDBError(err error, kind string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		switch kind {
		case "identity":
			return ErrIdentityNotFound
		case "tag":
			return ErrTagNotFound
		case "assignment":
			return ErrAssignmentNotFound
		default:
			return ErrCustomerNotFound
		}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "uq_customer_identities_business_channel_external":
				return ErrIdentityExists
			case "uq_tags_business_id_name":
				return ErrTagNameExists
			case "customer_tags_pkey":
				return ErrAssignmentExists
			}
		case "23503":
			if kind == "assignment" || kind == "identity" {
				return ErrCustomerNotFound
			}
		case "23514":
			return invalid("invalid value")
		}
	}

	return fmt.Errorf("database: %w", err)
}
