package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/money"
)

const paymentColumns = `id, business_id, reservation_id, status, payment_type, amount, currency, notes, confirmed_at, confirmed_by, refunded_at, refunded_by, created_at, updated_at`

type dbConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store interface {
	GetByID(ctx context.Context, businessID, paymentID uuid.UUID) (*Payment, error)
	GetByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error)
	LockByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error)
	ReservationExists(ctx context.Context, businessID, reservationID uuid.UUID) (bool, error)
	LoadDepositSettings(ctx context.Context, businessID uuid.UUID) (*DepositSettings, error)
	OrderTotal(ctx context.Context, businessID, reservationID uuid.UUID) (money.Amount, error)
	Insert(ctx context.Context, p Payment) (*Payment, bool, error)
	MarkPaid(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error)
	MarkFull(ctx context.Context, businessID, paymentID, actorID uuid.UUID, amount money.Amount, notes *string, at time.Time) (*Payment, bool, error)
	MarkRefunded(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error)
	UpdateNotes(ctx context.Context, businessID, paymentID uuid.UUID, notes *string) (*Payment, error)
	List(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Payment, error)
}

type Repository struct {
	db dbConn
}

func NewRepository(db dbConn) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, businessID, paymentID uuid.UUID) (*Payment, error) {
	return scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM reservation_payments WHERE id = $1 AND business_id = $2`, paymentID, businessID))
}

func (r *Repository) GetByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error) {
	return scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM reservation_payments WHERE business_id = $1 AND reservation_id = $2`, businessID, reservationID))
}

func (r *Repository) LockByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error) {
	return scanPayment(r.db.QueryRow(ctx, `
		SELECT `+paymentColumns+`
		FROM reservation_payments
		WHERE business_id = $1 AND reservation_id = $2
		FOR UPDATE`, businessID, reservationID))
}

func (r *Repository) ReservationExists(ctx context.Context, businessID, reservationID uuid.UUID) (bool, error) {
	var ok bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reservations WHERE id = $1 AND business_id = $2)`, reservationID, businessID).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *Repository) LoadDepositSettings(ctx context.Context, businessID uuid.UUID) (*DepositSettings, error) {
	row := r.db.QueryRow(ctx, `
		SELECT currency, deposit_enabled, deposit_type, deposit_value
		FROM fnb_settings WHERE business_id = $1`, businessID)
	var s DepositSettings
	if err := row.Scan(&s.Currency, &s.Enabled, &s.Type, &s.Value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &DepositSettings{Currency: "IDR", Type: DepositFixed}, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *Repository) OrderTotal(ctx context.Context, businessID, reservationID uuid.UUID) (money.Amount, error) {
	var raw string
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(unit_price * quantity), 0)::text
		FROM reservation_items
		WHERE business_id = $1 AND reservation_id = $2`, businessID, reservationID).Scan(&raw)
	if err != nil {
		return 0, err
	}
	return money.Parse(raw)
}

func (r *Repository) Insert(ctx context.Context, p Payment) (*Payment, bool, error) {
	if p.PaymentType == "" {
		p.PaymentType = PaymentTypeNone
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO reservation_payments (business_id, reservation_id, status, payment_type, amount, currency, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (business_id, reservation_id) DO NOTHING
		RETURNING `+paymentColumns,
		p.BusinessID, p.ReservationID, p.Status, p.PaymentType, p.Amount, p.Currency, p.Notes)
	item, err := scanPayment(row)
	if errors.Is(err, ErrNotFound) {
		existing, getErr := r.GetByReservation(ctx, p.BusinessID, p.ReservationID)
		return existing, false, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return item, true, nil
}

func (r *Repository) MarkPaid(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE reservation_payments
		SET status = CASE WHEN payment_type = 'DEPOSIT' THEN 'DEPOSIT' ELSE 'PAID' END,
		    confirmed_at = $3,
		    confirmed_by = $4,
		    notes = COALESCE($5, notes)
		WHERE id = $1 AND business_id = $2 AND status = 'UNPAID'
		RETURNING `+paymentColumns, paymentID, businessID, at.UTC(), actorID, notes)
	item, err := scanPayment(row)
	if err == nil {
		return item, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	current, getErr := r.GetByID(ctx, businessID, paymentID)
	if getErr != nil {
		return nil, false, getErr
	}
	return current, false, nil
}

func (r *Repository) MarkFull(ctx context.Context, businessID, paymentID, actorID uuid.UUID, amount money.Amount, notes *string, at time.Time) (*Payment, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE reservation_payments
		SET status = 'PAID',
		    payment_type = 'FULL',
		    amount = $3,
		    confirmed_at = $4,
		    confirmed_by = $5,
		    notes = COALESCE($6, notes)
		WHERE id = $1 AND business_id = $2
		  AND status IN ('DEPOSIT', 'PAID')
		  AND NOT (status = 'PAID' AND payment_type = 'FULL')
		RETURNING `+paymentColumns, paymentID, businessID, amount, at.UTC(), actorID, notes)
	item, err := scanPayment(row)
	if err == nil {
		return item, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	current, getErr := r.GetByID(ctx, businessID, paymentID)
	if getErr != nil {
		return nil, false, getErr
	}
	return current, false, nil
}

func (r *Repository) MarkRefunded(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE reservation_payments
		SET status = 'REFUNDED',
		    refunded_at = $3,
		    refunded_by = $4,
		    notes = COALESCE($5, notes)
		WHERE id = $1 AND business_id = $2 AND status IN ('PAID', 'DEPOSIT')
		RETURNING `+paymentColumns, paymentID, businessID, at.UTC(), actorID, notes)
	item, err := scanPayment(row)
	if err == nil {
		return item, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	current, getErr := r.GetByID(ctx, businessID, paymentID)
	if getErr != nil {
		return nil, false, getErr
	}
	return current, false, nil
}

func (r *Repository) UpdateNotes(ctx context.Context, businessID, paymentID uuid.UUID, notes *string) (*Payment, error) {
	return scanPayment(r.db.QueryRow(ctx, `
		UPDATE reservation_payments
		SET notes = $3
		WHERE id = $1 AND business_id = $2
		RETURNING `+paymentColumns, paymentID, businessID, notes))
}

func (r *Repository) List(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.business_id, p.reservation_id, p.status, p.payment_type, p.amount, p.currency, p.notes,
		       p.confirmed_at, p.confirmed_by, p.refunded_at, p.refunded_by, p.created_at, p.updated_at
		FROM reservation_payments p
		JOIN reservations r ON r.id = p.reservation_id AND r.business_id = p.business_id
		WHERE p.business_id = $1
		  AND ($2 = '' OR p.status = $2)
		  AND ($3::uuid IS NULL OR p.reservation_id = $3)
		  AND ($4::uuid IS NULL OR r.customer_id = $4)
		  AND ($5::timestamptz IS NULL OR p.created_at >= $5)
		  AND ($6::timestamptz IS NULL OR p.created_at < $6)
		ORDER BY p.created_at DESC, p.id
		LIMIT $7 OFFSET $8`,
		businessID, params.Status, params.ReservationID, params.CustomerID, params.CreatedFrom, params.CreatedTo, params.Limit, params.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		item, err := scanPaymentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanPayment(row pgx.Row) (*Payment, error) {
	var p Payment
	if err := row.Scan(&p.ID, &p.BusinessID, &p.ReservationID, &p.Status, &p.PaymentType, &p.Amount, &p.Currency, &p.Notes, &p.ConfirmedAt, &p.ConfirmedBy, &p.RefundedAt, &p.RefundedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	return &p, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPaymentRow(row rowScanner) (*Payment, error) {
	var p Payment
	if err := row.Scan(&p.ID, &p.BusinessID, &p.ReservationID, &p.Status, &p.PaymentType, &p.Amount, &p.Currency, &p.Notes, &p.ConfirmedAt, &p.ConfirmedBy, &p.RefundedAt, &p.RefundedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
