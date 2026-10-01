package payment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
)

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) Get(ctx context.Context, businessID, paymentID uuid.UUID) (*Payment, error) {
	return s.store.GetByID(ctx, businessID, paymentID)
}

func (s *Service) GetByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error) {
	return s.store.GetByReservation(ctx, businessID, reservationID)
}

func (s *Service) List(ctx context.Context, businessID uuid.UUID, status, reservationRaw, customerRaw, fromRaw, toRaw, limitRaw, offsetRaw string) ([]Payment, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && status != StatusUnpaid && status != StatusDeposit && status != StatusPaid && status != StatusRefunded {
		return nil, invalid("status is invalid")
	}
	reservationID, err := parseOptionalUUID(reservationRaw)
	if err != nil {
		return nil, invalid("reservation_id is invalid")
	}
	customerID, err := parseOptionalUUID(customerRaw)
	if err != nil {
		return nil, invalid("customer_id is invalid")
	}
	from, err := parseOptionalTime(fromRaw)
	if err != nil {
		return nil, invalid("from is invalid")
	}
	to, err := parseOptionalTime(toRaw)
	if err != nil {
		return nil, invalid("to is invalid")
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	return s.store.List(ctx, businessID, ListParams{
		Status: status, ReservationID: reservationID, CustomerID: customerID,
		CreatedFrom: from, CreatedTo: to, Limit: limit, Offset: offset,
	})
}

func (s *Service) CreateForReservation(ctx context.Context, businessID, reservationID uuid.UUID, notes *string) (*Payment, error) {
	return s.CreateForReservationWithOrder(ctx, businessID, reservationID, CreatePaymentInput{
		PaymentType: PaymentTypeDeposit,
		Notes:       notes,
	})
}

func (s *Service) CreateForReservationWithOrder(ctx context.Context, businessID, reservationID uuid.UUID, in CreatePaymentInput) (*Payment, error) {
	ok, err := s.store.ReservationExists(ctx, businessID, reservationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrReservationMissing
	}
	settings, err := s.store.LoadDepositSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	cleaned, err := normalizeNotes(in.Notes)
	if err != nil {
		return nil, err
	}
	orderTotal := in.OrderTotal
	if orderTotal <= 0 {
		orderTotal, err = s.store.OrderTotal(ctx, businessID, reservationID)
		if err != nil {
			return nil, err
		}
	}
	paymentType := strings.ToUpper(strings.TrimSpace(in.PaymentType))
	if paymentType == "" || paymentType == PaymentTypeNone {
		if settings.Enabled {
			paymentType = PaymentTypeDeposit
		} else {
			paymentType = PaymentTypeNone
		}
	}
	var amount money.Amount
	var resolvedType string
	switch {
	case !settings.Enabled:
		amount, resolvedType = 0, PaymentTypeNone
	case orderTotal <= 0:
		amount = RequiredAmount(*settings)
		resolvedType = PaymentTypeDeposit
	default:
		var calcErr error
		amount, resolvedType, calcErr = AmountDue(*settings, paymentType, orderTotal)
		if calcErr != nil {
			return nil, calcErr
		}
	}
	item, created, err := s.store.Insert(ctx, Payment{
		BusinessID:    businessID,
		ReservationID: reservationID,
		Status:        StatusUnpaid,
		PaymentType:   resolvedType,
		Amount:        amount,
		Currency:      settings.Currency,
		Notes:         cleaned,
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, ErrAlreadyExists
	}
	return item, nil
}

func (s *Service) CreateInitial(ctx context.Context, businessID, reservationID uuid.UUID, notes *string) (*Payment, error) {
	return s.CreateInitialWithOrder(ctx, businessID, reservationID, CreatePaymentInput{
		PaymentType: PaymentTypeNone,
		OrderTotal:  0,
		Notes:       notes,
	})
}

func (s *Service) CreateInitialWithOrder(ctx context.Context, businessID, reservationID uuid.UUID, in CreatePaymentInput) (*Payment, error) {
	settings, err := s.store.LoadDepositSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	cleaned, err := normalizeNotes(in.Notes)
	if err != nil {
		return nil, err
	}
	var amount money.Amount
	var paymentType string
	switch {
	case !settings.Enabled:
		amount, paymentType = 0, PaymentTypeNone
	case (in.PaymentType == "" || in.PaymentType == PaymentTypeNone) && in.OrderTotal <= 0:
		// Legacy fixed deposit without a menu total.
		amount = RequiredAmount(*settings)
		paymentType = PaymentTypeDeposit
	default:
		amount, paymentType, err = AmountDue(*settings, in.PaymentType, in.OrderTotal)
		if err != nil {
			return nil, err
		}
	}
	item, _, err := s.store.Insert(ctx, Payment{
		BusinessID:    businessID,
		ReservationID: reservationID,
		Status:        StatusUnpaid,
		PaymentType:   paymentType,
		Amount:        amount,
		Currency:      settings.Currency,
		Notes:         cleaned,
	})
	return item, err
}

func (s *Service) Confirm(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string) (*Payment, error) {
	current, err := s.store.GetByID(ctx, businessID, paymentID)
	if err != nil {
		return nil, err
	}
	cleaned, err := normalizeNotes(notes)
	if err != nil {
		return nil, err
	}
	switch current.Status {
	case StatusPaid, StatusDeposit:
		return current, nil
	case StatusRefunded:
		return nil, ErrAlreadyRefunded
	case StatusUnpaid:
	default:
		return nil, ErrInvalidState
	}
	item, changed, err := s.store.MarkPaid(ctx, businessID, paymentID, actorID, cleaned, s.now())
	if err != nil {
		return nil, err
	}
	if !changed && item.Status != StatusPaid && item.Status != StatusDeposit {
		if item.Status == StatusRefunded {
			return nil, ErrAlreadyRefunded
		}
		return nil, ErrInvalidState
	}
	return item, nil
}

func (s *Service) MarkFull(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string) (*Payment, error) {
	current, err := s.store.GetByID(ctx, businessID, paymentID)
	if err != nil {
		return nil, err
	}
	cleaned, err := normalizeNotes(notes)
	if err != nil {
		return nil, err
	}
	switch current.Status {
	case StatusPaid:
		if current.PaymentType == PaymentTypeFull {
			return current, nil
		}
	case StatusDeposit:
	case StatusRefunded:
		return nil, ErrAlreadyRefunded
	default:
		return nil, ErrInvalidState
	}
	orderTotal, err := s.store.OrderTotal(ctx, businessID, current.ReservationID)
	if err != nil {
		return nil, err
	}
	if orderTotal <= 0 {
		return nil, invalid("menu items are required for full payment")
	}
	item, changed, err := s.store.MarkFull(ctx, businessID, paymentID, actorID, orderTotal, cleaned, s.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		if item.Status == StatusPaid && item.PaymentType == PaymentTypeFull {
			return item, nil
		}
		if item.Status == StatusRefunded {
			return nil, ErrAlreadyRefunded
		}
		return nil, ErrInvalidState
	}
	return item, nil
}

func (s *Service) Refund(ctx context.Context, businessID, paymentID, actorID uuid.UUID, notes *string) (*Payment, error) {
	current, err := s.store.GetByID(ctx, businessID, paymentID)
	if err != nil {
		return nil, err
	}
	cleaned, err := normalizeNotes(notes)
	if err != nil {
		return nil, err
	}
	switch current.Status {
	case StatusRefunded:
		return nil, ErrAlreadyRefunded
	case StatusUnpaid:
		return nil, ErrInvalidState
	case StatusPaid, StatusDeposit:
	default:
		return nil, ErrInvalidState
	}
	item, changed, err := s.store.MarkRefunded(ctx, businessID, paymentID, actorID, cleaned, s.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		if item.Status == StatusRefunded {
			return nil, ErrAlreadyRefunded
		}
		return nil, ErrInvalidState
	}
	return item, nil
}

func (s *Service) UpdateNotes(ctx context.Context, businessID, paymentID uuid.UUID, notes *string) (*Payment, error) {
	cleaned, err := normalizeNotes(notes)
	if err != nil {
		return nil, err
	}
	return s.store.UpdateNotes(ctx, businessID, paymentID, cleaned)
}

func (s *Service) AssertConfirmable(ctx context.Context, businessID, reservationID uuid.UUID) error {
	settings, err := s.store.LoadDepositSettings(ctx, businessID)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	pay, err := s.store.LockByReservation(ctx, businessID, reservationID)
	if err != nil {
		if err == ErrNotFound {
			return ErrPaymentRequired
		}
		return err
	}
	if pay.Status != StatusPaid && pay.Status != StatusDeposit {
		return ErrPaymentRequired
	}
	return nil
}

// AssertCompletable enforces that a completed reservation is settled in full.
// A deposit-only payment never counts, even when deposits are switched off
// afterwards: the balance must be recorded first.
func (s *Service) AssertCompletable(ctx context.Context, businessID, reservationID uuid.UUID) error {
	settings, err := s.store.LoadDepositSettings(ctx, businessID)
	if err != nil {
		return err
	}
	pay, err := s.store.LockByReservation(ctx, businessID, reservationID)
	if err != nil {
		if err == ErrNotFound {
			if !settings.Enabled {
				return nil
			}
			return ErrFullPaymentRequired
		}
		return err
	}
	switch pay.Status {
	case StatusPaid:
		return nil
	case StatusDeposit:
		return ErrFullPaymentRequired
	default:
		if !settings.Enabled {
			return nil
		}
		return ErrFullPaymentRequired
	}
}

func normalizeNotes(notes *string) (*string, error) {
	if notes == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*notes)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > 2000 {
		return nil, invalid("notes is too long")
	}
	lower := strings.ToLower(trimmed)
	for _, secret := range []string{"cvv", "otp", "password", "card number", "pan "} {
		if strings.Contains(lower, secret) {
			return nil, invalid("notes must not contain sensitive payment details")
		}
	}
	return &trimmed, nil
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseOptionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}

func parseLimitOffset(limitRaw, offsetRaw string) (int, int, error) {
	limit, offset := 50, 0
	if strings.TrimSpace(limitRaw) != "" {
		n, err := parseNonNeg(limitRaw)
		if err != nil || n < 1 {
			return 0, 0, invalid("limit is invalid")
		}
		if n > 100 {
			n = 100
		}
		limit = n
	}
	if strings.TrimSpace(offsetRaw) != "" {
		n, err := parseNonNeg(offsetRaw)
		if err != nil {
			return 0, 0, invalid("offset is invalid")
		}
		offset = n
	}
	return limit, offset, nil
}

func parseNonNeg(raw string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err != nil || n < 0 {
		return 0, err
	}
	return n, nil
}
