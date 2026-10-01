package reservation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetActiveMenuItem(ctx context.Context, businessID, menuItemID uuid.UUID) (*MenuItemRef, error) {
	const query = `
		SELECT id, name, price::text, status
		FROM menu_items
		WHERE business_id = $1 AND id = $2`
	var item MenuItemRef
	err := r.db.QueryRow(ctx, query, businessID, menuItemID).Scan(&item.ID, &item.Name, &item.Price, &item.Status)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, ErrMenuItemNotFound
		}
		return nil, mapDBError(err, "menu_item")
	}
	return &item, nil
}

func (r *Repository) ReplaceReservationItems(ctx context.Context, businessID, reservationID uuid.UUID, items []ReservationItem) error {
	if _, err := r.db.Exec(ctx, `
		DELETE FROM reservation_items
		WHERE business_id = $1 AND reservation_id = $2`, businessID, reservationID); err != nil {
		return mapDBError(err, "reservation_item")
	}
	for _, item := range items {
		const query = `
			INSERT INTO reservation_items (
				business_id, reservation_id, menu_item_id, menu_item_name, quantity, unit_price, notes
			) VALUES ($1, $2, $3, $4, $5, $6::numeric, $7)`
		if _, err := r.db.Exec(ctx, query,
			businessID, reservationID, item.MenuItemID, item.MenuItemName, item.Quantity, item.UnitPrice, item.Notes,
		); err != nil {
			return mapDBError(err, "reservation_item")
		}
	}
	return nil
}

func (r *Repository) ListReservationItems(ctx context.Context, businessID, reservationID uuid.UUID) ([]ReservationItem, error) {
	const query = `
		SELECT id, business_id, reservation_id, menu_item_id, menu_item_name, quantity, unit_price::text, notes, created_at, updated_at
		FROM reservation_items
		WHERE business_id = $1 AND reservation_id = $2
		ORDER BY created_at ASC, id ASC`
	rows, err := r.db.Query(ctx, query, businessID, reservationID)
	if err != nil {
		return nil, mapDBError(err, "reservation_item")
	}
	defer rows.Close()

	out := make([]ReservationItem, 0)
	for rows.Next() {
		var item ReservationItem
		if err := rows.Scan(
			&item.ID, &item.BusinessID, &item.ReservationID, &item.MenuItemID, &item.MenuItemName,
			&item.Quantity, &item.UnitPrice, &item.Notes, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, mapDBError(err, "reservation_item")
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListReservationItemsByReservations(ctx context.Context, businessID uuid.UUID, reservationIDs []uuid.UUID) (map[uuid.UUID][]ReservationItem, error) {
	out := make(map[uuid.UUID][]ReservationItem, len(reservationIDs))
	if len(reservationIDs) == 0 {
		return out, nil
	}
	const query = `
		SELECT id, business_id, reservation_id, menu_item_id, menu_item_name, quantity, unit_price::text, notes, created_at, updated_at
		FROM reservation_items
		WHERE business_id = $1 AND reservation_id = ANY($2)
		ORDER BY created_at ASC, id ASC`
	rows, err := r.db.Query(ctx, query, businessID, reservationIDs)
	if err != nil {
		return nil, mapDBError(err, "reservation_item")
	}
	defer rows.Close()

	for rows.Next() {
		var item ReservationItem
		if err := rows.Scan(
			&item.ID, &item.BusinessID, &item.ReservationID, &item.MenuItemID, &item.MenuItemName,
			&item.Quantity, &item.UnitPrice, &item.Notes, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, mapDBError(err, "reservation_item")
		}
		out[item.ReservationID] = append(out[item.ReservationID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return out, nil
}

func errorsIsNoRows(err error) bool {
	return err != nil && (err.Error() == "no rows in result set" || err == pgx.ErrNoRows)
}
