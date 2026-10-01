package calendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const blockColumns = `id, business_id, area_id, table_id, title, description, start_at, end_at, status, created_by, created_at, updated_at`

type dbConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store interface {
	Timezone(ctx context.Context, businessID uuid.UUID) (string, error)
	ListHours(ctx context.Context, businessID uuid.UUID) ([]Hour, error)
	ListExceptions(ctx context.Context, businessID uuid.UUID) ([]ScheduleException, error)
	ResolveScope(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID) (areaName, tableName string, err error)
	GetBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error)
	InsertBlock(ctx context.Context, b Block) (*Block, error)
	UpdateBlock(ctx context.Context, businessID uuid.UUID, b Block) (*Block, error)
	CancelBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error)
	ListBlocks(ctx context.Context, businessID uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]Block, error)
	OverlappingReservations(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) ([]ReservationConflict, error)
	ListReservationEvents(ctx context.Context, businessID uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]ReservationEvent, error)
}

type Repository struct {
	db dbConn
}

func NewRepository(db dbConn) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Timezone(ctx context.Context, businessID uuid.UUID) (string, error) {
	var tz string
	if err := r.db.QueryRow(ctx, `SELECT timezone FROM businesses WHERE id = $1`, businessID).Scan(&tz); err != nil {
		return "", mapDBError(err)
	}
	return tz, nil
}

func (r *Repository) ListHours(ctx context.Context, businessID uuid.UUID) ([]Hour, error) {
	rows, err := r.db.Query(ctx, `
		SELECT day_of_week, open_time, close_time
		FROM business_hours
		WHERE business_id = $1
		ORDER BY day_of_week, sort_order, open_time`, businessID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]Hour, 0)
	for rows.Next() {
		var h Hour
		var openAt, closeAt time.Time
		if err := rows.Scan(&h.DayOfWeek, &openAt, &closeAt); err != nil {
			return nil, mapDBError(err)
		}
		h.OpenTime = clockFromTime(openAt)
		h.CloseTime = clockFromTime(closeAt)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *Repository) ListExceptions(ctx context.Context, businessID uuid.UUID) ([]ScheduleException, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, exception_date, is_closed, open_time, close_time, note
		FROM business_schedule_exceptions
		WHERE business_id = $1
		ORDER BY exception_date`, businessID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]ScheduleException, 0)
	for rows.Next() {
		var e ScheduleException
		var openAt, closeAt *time.Time
		if err := rows.Scan(&e.ID, &e.ExceptionDate, &e.IsClosed, &openAt, &closeAt, &e.Note); err != nil {
			return nil, mapDBError(err)
		}
		if openAt != nil {
			s := clockFromTime(*openAt)
			e.OpenTime = &s
		}
		if closeAt != nil {
			s := clockFromTime(*closeAt)
			e.CloseTime = &s
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ResolveScope(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID) (string, string, error) {
	if areaID == nil && tableID == nil {
		return "", "", nil
	}
	if tableID != nil && areaID == nil {
		return "", "", ErrInvalidScope
	}
	if tableID != nil {
		var areaName, tableName string
		err := r.db.QueryRow(ctx, `
			SELECT a.name, t.name
			FROM tables t
			JOIN areas a ON a.id = t.area_id
			WHERE a.business_id = $1 AND a.id = $2 AND t.id = $3`, businessID, *areaID, *tableID).Scan(&areaName, &tableName)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "", ErrTableMissing
			}
			return "", "", mapDBError(err)
		}
		return areaName, tableName, nil
	}
	var areaName string
	err := r.db.QueryRow(ctx, `SELECT name FROM areas WHERE business_id = $1 AND id = $2`, businessID, *areaID).Scan(&areaName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrAreaMissing
		}
		return "", "", mapDBError(err)
	}
	return areaName, "", nil
}

func (r *Repository) GetBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	return scanBlock(r.db.QueryRow(ctx, `SELECT `+blockColumns+` FROM calendar_blocks WHERE id = $1 AND business_id = $2`, blockID, businessID))
}

func (r *Repository) InsertBlock(ctx context.Context, b Block) (*Block, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO calendar_blocks (business_id, area_id, table_id, title, description, start_at, end_at, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+blockColumns,
		b.BusinessID, b.AreaID, b.TableID, b.Title, b.Description, b.StartAt, b.EndAt, b.Status, b.CreatedBy)
	return scanBlock(row)
}

func (r *Repository) UpdateBlock(ctx context.Context, businessID uuid.UUID, b Block) (*Block, error) {
	return scanBlock(r.db.QueryRow(ctx, `
		UPDATE calendar_blocks
		SET title = $3, description = $4, start_at = $5, end_at = $6
		WHERE id = $1 AND business_id = $2 AND status = 'ACTIVE'
		RETURNING `+blockColumns, b.ID, businessID, b.Title, b.Description, b.StartAt, b.EndAt))
}

func (r *Repository) CancelBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	item, err := scanBlock(r.db.QueryRow(ctx, `
		UPDATE calendar_blocks
		SET status = 'CANCELLED'
		WHERE id = $1 AND business_id = $2
		RETURNING `+blockColumns, blockID, businessID))
	return item, err
}

func (r *Repository) ListBlocks(ctx context.Context, businessID uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]Block, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+blockColumns+`
		FROM calendar_blocks
		WHERE business_id = $1
		  AND tstzrange(start_at, end_at, '[)') && tstzrange($2::timestamptz, $3::timestamptz, '[)')
		  AND ($4 = '' OR status = $4)
		  AND ($5::uuid IS NULL OR area_id = $5 OR table_id IN (SELECT id FROM tables WHERE area_id = $5))
		  AND ($6::uuid IS NULL OR table_id = $6)
		ORDER BY start_at ASC, id ASC`, businessID, from, to, status, areaID, tableID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]Block, 0)
	for rows.Next() {
		item, err := scanBlockRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) OverlappingReservations(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) ([]ReservationConflict, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id, r.start_at, r.end_at, r.status
		FROM reservations r
		LEFT JOIN tables t ON t.id = r.table_id
		WHERE r.business_id = $1
		  AND r.status IN ('PENDING', 'CONFIRMED')
		  AND tstzrange(r.start_at, r.end_at, '[)') && tstzrange($2::timestamptz, $3::timestamptz, '[)')
		  AND (
		      ($4::uuid IS NULL AND $5::uuid IS NULL)
		      OR ($5::uuid IS NOT NULL AND r.table_id = $5)
		      OR ($5::uuid IS NULL AND $4::uuid IS NOT NULL AND t.area_id = $4)
		  )
		ORDER BY r.start_at, r.id`, businessID, start, end, areaID, tableID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]ReservationConflict, 0)
	for rows.Next() {
		var item ReservationConflict
		if err := rows.Scan(&item.ReservationID, &item.StartAt, &item.EndAt, &item.Status); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListReservationEvents(ctx context.Context, businessID uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]ReservationEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id, r.start_at, r.end_at, r.status, r.party_size, r.customer_id, c.name,
		       t.id, t.name, a.id, a.name, p.status
		FROM reservations r
		JOIN customers c ON c.id = r.customer_id AND c.business_id = r.business_id
		LEFT JOIN tables t ON t.id = r.table_id
		LEFT JOIN areas a ON a.id = t.area_id
		LEFT JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
		WHERE r.business_id = $1
		  AND tstzrange(r.start_at, r.end_at, '[)') && tstzrange($2::timestamptz, $3::timestamptz, '[)')
		  AND ($4 = '' OR r.status = $4)
		  AND ($4 <> '' OR r.status <> 'CANCELLED')
		  AND ($5::uuid IS NULL OR a.id = $5)
		  AND ($6::uuid IS NULL OR r.table_id = $6)
		ORDER BY r.start_at ASC, r.id ASC`, businessID, from, to, status, areaID, tableID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]ReservationEvent, 0)
	for rows.Next() {
		var e ReservationEvent
		var tableID, areaID *uuid.UUID
		var tableName, areaName *string
		if err := rows.Scan(&e.ID, &e.StartAt, &e.EndAt, &e.Status, &e.PartySize, &e.CustomerID, &e.CustomerName, &tableID, &tableName, &areaID, &areaName, &e.PaymentStatus); err != nil {
			return nil, err
		}
		e.ReservationID = e.ID
		e.Title = e.CustomerName
		e.TableID = tableID
		e.AreaID = areaID
		if tableName != nil {
			e.TableName = *tableName
		}
		if areaName != nil {
			e.AreaName = *areaName
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanBlock(row pgx.Row) (*Block, error) {
	item, err := scanBlockRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, mapDBError(err)
	}
	return item, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBlockRow(row rowScanner) (*Block, error) {
	var b Block
	if err := row.Scan(&b.ID, &b.BusinessID, &b.AreaID, &b.TableID, &b.Title, &b.Description, &b.StartAt, &b.EndAt, &b.Status, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01":
			return ErrConflict
		case "23514", "23503":
			return ErrInvalidScope
		}
	}
	return fmt.Errorf("database: %w", err)
}

func clockFromTime(t time.Time) string {
	return t.Format("15:04")
}
