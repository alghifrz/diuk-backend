package reservation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	reservationSelect = `
		r.id, r.business_id, r.customer_id, r.table_id, r.party_size, r.start_at, r.end_at,
		r.status, r.notes, r.cancellation_reason, r.confirmed_at, r.completed_at, r.created_at, r.updated_at,
		c.id, c.name, c.phone,
		t.id, t.name, t.capacity, a.id, a.name`
	hourColumns      = `id, business_id, day_of_week, sort_order, open_time, close_time, created_at, updated_at`
	exceptionColumns = `id, business_id, exception_date, is_closed, open_time, close_time, note, created_at, updated_at`
)

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Store interface {
	RunInTx(ctx context.Context, fn func(Store) error) error
	GetBookingContext(ctx context.Context, businessID uuid.UUID) (*BookingContext, error)
	GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error)
	GetTable(ctx context.Context, businessID, tableID uuid.UUID) (*TableRecord, error)
	LockTable(ctx context.Context, businessID, tableID uuid.UUID) (*TableRecord, error)
	FindAvailableTables(ctx context.Context, businessID uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID) ([]TableRecord, error)
	HasConflict(ctx context.Context, businessID uuid.UUID, tableID uuid.UUID, start, end time.Time, excludeID uuid.UUID) (bool, error)
	HasCalendarBlock(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) (bool, error)

	CreateReservation(ctx context.Context, res Reservation) (*Reservation, error)
	GetReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error)
	LockReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error)
	ListReservations(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Reservation, error)
	UpdateReservation(ctx context.Context, businessID uuid.UUID, res Reservation) (*Reservation, error)

	ListHours(ctx context.Context, businessID uuid.UUID) ([]Hour, error)
	ReplaceHours(ctx context.Context, businessID uuid.UUID, hours []Hour) ([]Hour, error)
	ListExceptions(ctx context.Context, businessID uuid.UUID) ([]ScheduleException, error)
	GetException(ctx context.Context, businessID, exceptionID uuid.UUID) (*ScheduleException, error)
	CreateException(ctx context.Context, ex ScheduleException) (*ScheduleException, error)
	UpdateException(ctx context.Context, businessID uuid.UUID, ex ScheduleException) (*ScheduleException, error)
	DeleteException(ctx context.Context, businessID, exceptionID uuid.UUID) error

	CreateInitialPayment(ctx context.Context, businessID, reservationID uuid.UUID, paymentType string, orderTotal money.Amount) error
	AssertPaymentForConfirm(ctx context.Context, businessID, reservationID uuid.UUID) error
	AssertPaymentForComplete(ctx context.Context, businessID, reservationID uuid.UUID) error

	GetActiveMenuItem(ctx context.Context, businessID, menuItemID uuid.UUID) (*MenuItemRef, error)
	ReplaceReservationItems(ctx context.Context, businessID, reservationID uuid.UUID, items []ReservationItem) error
	ListReservationItems(ctx context.Context, businessID, reservationID uuid.UUID) ([]ReservationItem, error)
	ListReservationItemsByReservations(ctx context.Context, businessID uuid.UUID, reservationIDs []uuid.UUID) (map[uuid.UUID][]ReservationItem, error)
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) RunInTx(ctx context.Context, fn func(Store) error) error {
	b, ok := r.db.(beginner)
	if !ok {
		return fn(r)
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(NewRepository(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) GetBookingContext(ctx context.Context, businessID uuid.UUID) (*BookingContext, error) {
	const query = `
		SELECT b.timezone, f.reservation_enabled, f.reservation_slot_duration_minutes,
		       f.reservation_min_party_size, f.reservation_max_party_size,
		       f.reservation_min_advance_minutes, f.reservation_max_advance_days,
		       f.currency, f.deposit_enabled, f.deposit_type, f.deposit_value
		FROM businesses b
		JOIN fnb_settings f ON f.business_id = b.id
		WHERE b.id = $1`
	var out BookingContext
	if err := r.db.QueryRow(ctx, query, businessID).Scan(
		&out.Timezone, &out.ReservationEnabled, &out.ReservationSlotDurationMinutes,
		&out.ReservationMinPartySize, &out.ReservationMaxPartySize,
		&out.ReservationMinAdvanceMinutes, &out.ReservationMaxAdvanceDays,
		&out.Currency, &out.DepositEnabled, &out.DepositType, &out.DepositValue,
	); err != nil {
		return nil, mapDBError(err, "context")
	}
	return &out, nil
}

func (r *Repository) CreateInitialPayment(ctx context.Context, businessID, reservationID uuid.UUID, paymentType string, orderTotal money.Amount) error {
	_, err := payment.NewService(payment.NewRepository(r.db)).CreateInitialWithOrder(ctx, businessID, reservationID, payment.CreatePaymentInput{
		PaymentType: paymentType,
		OrderTotal:  orderTotal,
	})
	return err
}

func (r *Repository) AssertPaymentForConfirm(ctx context.Context, businessID, reservationID uuid.UUID) error {
	err := payment.NewService(payment.NewRepository(r.db)).AssertConfirmable(ctx, businessID, reservationID)
	if errors.Is(err, payment.ErrPaymentRequired) {
		return ErrPaymentRequired
	}
	return err
}

func (r *Repository) AssertPaymentForComplete(ctx context.Context, businessID, reservationID uuid.UUID) error {
	err := payment.NewService(payment.NewRepository(r.db)).AssertCompletable(ctx, businessID, reservationID)
	if errors.Is(err, payment.ErrFullPaymentRequired) {
		return ErrFullPaymentRequired
	}
	return err
}

func (r *Repository) GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error) {
	const query = `SELECT id, name, phone, status FROM customers WHERE business_id = $1 AND id = $2`
	var rec CustomerRecord
	if err := r.db.QueryRow(ctx, query, businessID, customerID).Scan(&rec.ID, &rec.Name, &rec.Phone, &rec.Status); err != nil {
		return nil, mapDBError(err, "customer")
	}
	return &rec, nil
}

func (r *Repository) GetTable(ctx context.Context, businessID, tableID uuid.UUID) (*TableRecord, error) {
	return r.scanTable(ctx, businessID, tableID, false)
}

func (r *Repository) LockTable(ctx context.Context, businessID, tableID uuid.UUID) (*TableRecord, error) {
	return r.scanTable(ctx, businessID, tableID, true)
}

func (r *Repository) scanTable(ctx context.Context, businessID, tableID uuid.UUID, lock bool) (*TableRecord, error) {
	query := `
		SELECT t.id, t.name, t.capacity, t.status, a.id, a.name, a.status, a.business_id
		FROM tables t
		JOIN areas a ON a.id = t.area_id
		WHERE t.id = $1 AND a.business_id = $2`
	if lock {
		query += ` FOR UPDATE OF t`
	}
	var rec TableRecord
	if err := r.db.QueryRow(ctx, query, tableID, businessID).Scan(
		&rec.ID, &rec.Name, &rec.Capacity, &rec.Status, &rec.AreaID, &rec.AreaName, &rec.AreaStatus, &rec.BusinessID,
	); err != nil {
		return nil, mapDBError(err, "table")
	}
	return &rec, nil
}

func (r *Repository) FindAvailableTables(ctx context.Context, businessID uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID) ([]TableRecord, error) {
	const query = `
		SELECT t.id, t.name, t.capacity, t.status, a.id, a.name, a.status, a.business_id
		FROM tables t
		JOIN areas a ON a.id = t.area_id
		WHERE a.business_id = $1
		  AND t.status = 'ACTIVE'
		  AND a.status = 'ACTIVE'
		  AND t.capacity >= $2
		  AND NOT EXISTS (
		      SELECT 1 FROM reservations r
		      WHERE r.table_id = t.id
		        AND r.business_id = $1
		        AND r.status IN ('PENDING', 'CONFIRMED')
		        AND tstzrange(r.start_at, r.end_at, '[)') && tstzrange($3::timestamptz, $4::timestamptz, '[)')
		        AND ($5::uuid IS NULL OR r.id <> $5)
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM calendar_blocks b
		      WHERE b.business_id = $1
		        AND b.status = 'ACTIVE'
		        AND tstzrange(b.start_at, b.end_at, '[)') && tstzrange($3::timestamptz, $4::timestamptz, '[)')
		        AND (
		            (b.area_id IS NULL AND b.table_id IS NULL)
		            OR (b.table_id IS NULL AND b.area_id = a.id)
		            OR (b.table_id = t.id)
		        )
		  )
		ORDER BY t.capacity ASC, t.id ASC`
	rows, err := r.db.Query(ctx, query, businessID, partySize, start, end, nilIfNil(excludeID))
	if err != nil {
		return nil, mapDBError(err, "table")
	}
	defer rows.Close()
	out := make([]TableRecord, 0)
	for rows.Next() {
		var rec TableRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Capacity, &rec.Status, &rec.AreaID, &rec.AreaName, &rec.AreaStatus, &rec.BusinessID); err != nil {
			return nil, mapDBError(err, "table")
		}
		out = append(out, rec)
	}
	return out, mapDBError(rows.Err(), "table")
}

func (r *Repository) HasConflict(ctx context.Context, businessID uuid.UUID, tableID uuid.UUID, start, end time.Time, excludeID uuid.UUID) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM reservations
			WHERE business_id = $1
			  AND table_id = $2
			  AND status IN ('PENDING', 'CONFIRMED')
			  AND tstzrange(start_at, end_at, '[)') && tstzrange($3::timestamptz, $4::timestamptz, '[)')
			  AND ($5::uuid IS NULL OR id <> $5)
		)`
	var exists bool
	if err := r.db.QueryRow(ctx, query, businessID, tableID, start, end, nilIfNil(excludeID)).Scan(&exists); err != nil {
		return false, mapDBError(err, "reservation")
	}
	return exists, nil
}

func (r *Repository) HasCalendarBlock(ctx context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM calendar_blocks
			WHERE business_id = $1
			  AND status = 'ACTIVE'
			  AND tstzrange(start_at, end_at, '[)') && tstzrange($2::timestamptz, $3::timestamptz, '[)')
			  AND (
			      (area_id IS NULL AND table_id IS NULL)
			      OR ($4::uuid IS NOT NULL AND table_id IS NULL AND area_id = $4)
			      OR ($5::uuid IS NOT NULL AND table_id = $5)
			  )
		)`
	var exists bool
	if err := r.db.QueryRow(ctx, query, businessID, start, end, areaID, tableID).Scan(&exists); err != nil {
		return false, mapDBError(err, "calendar")
	}
	return exists, nil
}

func (r *Repository) CreateReservation(ctx context.Context, res Reservation) (*Reservation, error) {
	const query = `
		INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, res.BusinessID, res.CustomerID, res.TableID, res.PartySize, res.StartAt, res.EndAt, res.Status, res.Notes).Scan(&id); err != nil {
		return nil, mapDBError(err, "reservation")
	}
	return r.GetReservation(ctx, res.BusinessID, id)
}

func (r *Repository) GetReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	const query = `
		SELECT ` + reservationSelect + `
		FROM reservations r
		JOIN customers c ON c.id = r.customer_id AND c.business_id = r.business_id
		LEFT JOIN tables t ON t.id = r.table_id
		LEFT JOIN areas a ON a.id = t.area_id AND a.business_id = r.business_id
		WHERE r.business_id = $1 AND r.id = $2`
	return scanReservation(r.db.QueryRow(ctx, query, businessID, reservationID))
}

func (r *Repository) LockReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	const query = `
		SELECT id FROM reservations
		WHERE business_id = $1 AND id = $2
		FOR UPDATE`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, reservationID).Scan(&id); err != nil {
		return nil, mapDBError(err, "reservation")
	}
	return r.GetReservation(ctx, businessID, reservationID)
}

func (r *Repository) ListReservations(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Reservation, error) {
	query := `
		SELECT ` + reservationSelect + `
		FROM reservations r
		JOIN customers c ON c.id = r.customer_id AND c.business_id = r.business_id
		LEFT JOIN tables t ON t.id = r.table_id
		LEFT JOIN areas a ON a.id = t.area_id AND a.business_id = r.business_id
		WHERE r.business_id = $1`
	args := []any{businessID}
	n := 2
	if params.Status != "" {
		query += fmt.Sprintf(` AND r.status = $%d`, n)
		args = append(args, params.Status)
		n++
	}
	if params.CustomerID != nil {
		query += fmt.Sprintf(` AND r.customer_id = $%d`, n)
		args = append(args, *params.CustomerID)
		n++
	}
	if params.TableID != nil {
		query += fmt.Sprintf(` AND r.table_id = $%d`, n)
		args = append(args, *params.TableID)
		n++
	}
	if params.StartFrom != nil {
		query += fmt.Sprintf(` AND r.start_at >= $%d`, n)
		args = append(args, *params.StartFrom)
		n++
	}
	if params.StartTo != nil {
		query += fmt.Sprintf(` AND r.start_at < $%d`, n)
		args = append(args, *params.StartTo)
		n++
	}
	query += fmt.Sprintf(` ORDER BY r.start_at ASC, r.id ASC LIMIT $%d OFFSET $%d`, n, n+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, "reservation")
	}
	defer rows.Close()
	out := make([]Reservation, 0)
	for rows.Next() {
		item, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "reservation")
}

func (r *Repository) UpdateReservation(ctx context.Context, businessID uuid.UUID, res Reservation) (*Reservation, error) {
	const query = `
		UPDATE reservations
		SET customer_id = $3, table_id = $4, party_size = $5, start_at = $6, end_at = $7,
		    status = $8, notes = $9, cancellation_reason = $10, confirmed_at = $11, completed_at = $12
		WHERE business_id = $1 AND id = $2
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, res.ID, res.CustomerID, res.TableID, res.PartySize, res.StartAt, res.EndAt, res.Status, res.Notes, res.CancellationReason, res.ConfirmedAt, res.CompletedAt).Scan(&id); err != nil {
		return nil, mapDBError(err, "reservation")
	}
	return r.GetReservation(ctx, businessID, res.ID)
}

func (r *Repository) ListHours(ctx context.Context, businessID uuid.UUID) ([]Hour, error) {
	const query = `SELECT ` + hourColumns + ` FROM business_hours WHERE business_id = $1 ORDER BY day_of_week ASC, sort_order ASC`
	rows, err := r.db.Query(ctx, query, businessID)
	if err != nil {
		return nil, mapDBError(err, "hours")
	}
	defer rows.Close()
	out := make([]Hour, 0)
	for rows.Next() {
		item, err := scanHour(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "hours")
}

func (r *Repository) ReplaceHours(ctx context.Context, businessID uuid.UUID, hours []Hour) ([]Hour, error) {
	if _, err := r.db.Exec(ctx, `DELETE FROM business_hours WHERE business_id = $1`, businessID); err != nil {
		return nil, mapDBError(err, "hours")
	}
	for _, h := range hours {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO business_hours (business_id, day_of_week, sort_order, open_time, close_time)
			VALUES ($1, $2, $3, $4::time, $5::time)`, businessID, h.DayOfWeek, h.SortOrder, h.OpenTime, h.CloseTime); err != nil {
			return nil, mapDBError(err, "hours")
		}
	}
	return r.ListHours(ctx, businessID)
}

func (r *Repository) ListExceptions(ctx context.Context, businessID uuid.UUID) ([]ScheduleException, error) {
	const query = `SELECT ` + exceptionColumns + ` FROM business_schedule_exceptions WHERE business_id = $1 ORDER BY exception_date ASC`
	rows, err := r.db.Query(ctx, query, businessID)
	if err != nil {
		return nil, mapDBError(err, "exception")
	}
	defer rows.Close()
	out := make([]ScheduleException, 0)
	for rows.Next() {
		item, err := scanException(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "exception")
}

func (r *Repository) GetException(ctx context.Context, businessID, exceptionID uuid.UUID) (*ScheduleException, error) {
	const query = `SELECT ` + exceptionColumns + ` FROM business_schedule_exceptions WHERE business_id = $1 AND id = $2`
	return scanException(r.db.QueryRow(ctx, query, businessID, exceptionID))
}

func (r *Repository) CreateException(ctx context.Context, ex ScheduleException) (*ScheduleException, error) {
	const query = `
		INSERT INTO business_schedule_exceptions (business_id, exception_date, is_closed, open_time, close_time, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, ex.BusinessID, ex.ExceptionDate, ex.IsClosed, timeArg(ex.OpenTime), timeArg(ex.CloseTime), ex.Note).Scan(&id); err != nil {
		return nil, mapDBError(err, "exception")
	}
	return r.GetException(ctx, ex.BusinessID, id)
}

func (r *Repository) UpdateException(ctx context.Context, businessID uuid.UUID, ex ScheduleException) (*ScheduleException, error) {
	const query = `
		UPDATE business_schedule_exceptions
		SET exception_date = $3, is_closed = $4, open_time = $5, close_time = $6, note = $7
		WHERE business_id = $1 AND id = $2
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, ex.ID, ex.ExceptionDate, ex.IsClosed, timeArg(ex.OpenTime), timeArg(ex.CloseTime), ex.Note).Scan(&id); err != nil {
		return nil, mapDBError(err, "exception")
	}
	return r.GetException(ctx, businessID, ex.ID)
}

func (r *Repository) DeleteException(ctx context.Context, businessID, exceptionID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM business_schedule_exceptions WHERE business_id = $1 AND id = $2`, businessID, exceptionID)
	if err != nil {
		return mapDBError(err, "exception")
	}
	if tag.RowsAffected() == 0 {
		return ErrExceptionNotFound
	}
	return nil
}

func nilIfNil(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func timeArg(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReservation(row scanner) (*Reservation, error) {
	var (
		r         Reservation
		tableUUID *uuid.UUID
		tableName *string
		tableCap  *int
		areaUUID  *uuid.UUID
		areaName  *string
	)
	err := row.Scan(
		&r.ID, &r.BusinessID, &r.CustomerID, &r.TableID, &r.PartySize, &r.StartAt, &r.EndAt,
		&r.Status, &r.Notes, &r.CancellationReason, &r.ConfirmedAt, &r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.Customer.ID, &r.Customer.Name, &r.Customer.Phone,
		&tableUUID, &tableName, &tableCap, &areaUUID, &areaName,
	)
	if err != nil {
		return nil, mapDBError(err, "reservation")
	}
	if tableUUID != nil && tableName != nil && tableCap != nil && areaUUID != nil && areaName != nil {
		r.Table = &TableSummary{ID: *tableUUID, Name: *tableName, Capacity: *tableCap, AreaID: *areaUUID, AreaName: *areaName}
	}
	return &r, nil
}

func scanHour(row scanner) (*Hour, error) {
	var h Hour
	var openAt, closeAt time.Time
	if err := row.Scan(&h.ID, &h.BusinessID, &h.DayOfWeek, &h.SortOrder, &openAt, &closeAt, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return nil, mapDBError(err, "hours")
	}
	h.OpenTime = clockFromTime(openAt)
	h.CloseTime = clockFromTime(closeAt)
	return &h, nil
}

func scanException(row scanner) (*ScheduleException, error) {
	var e ScheduleException
	var openAt, closeAt *time.Time
	if err := row.Scan(&e.ID, &e.BusinessID, &e.ExceptionDate, &e.IsClosed, &openAt, &closeAt, &e.Note, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, mapDBError(err, "exception")
	}
	if openAt != nil {
		s := clockFromTime(*openAt)
		e.OpenTime = &s
	}
	if closeAt != nil {
		s := clockFromTime(*closeAt)
		e.CloseTime = &s
	}
	return &e, nil
}

func mapDBError(err error, kind string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		switch kind {
		case "customer":
			return ErrCustomerNotFound
		case "table":
			return ErrTableNotFound
		case "exception":
			return ErrExceptionNotFound
		default:
			return ErrNotFound
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01":
			return ErrTableUnavailable
		case "23505":
			if kind == "exception" {
				return ErrExceptionExists
			}
			return ErrConflict
		case "23514", "22007", "22008":
			return invalid("invalid value")
		}
	}
	return fmt.Errorf("database: %w", err)
}
