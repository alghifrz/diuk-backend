package payment

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
)

type memoryStore struct {
	mu           sync.Mutex
	settings     map[uuid.UUID]DepositSettings
	reservations map[uuid.UUID]uuid.UUID
	customers    map[uuid.UUID]uuid.UUID
	payments     map[uuid.UUID]*Payment
	orderTotals  map[uuid.UUID]money.Amount
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		settings:     map[uuid.UUID]DepositSettings{},
		reservations: map[uuid.UUID]uuid.UUID{},
		customers:    map[uuid.UUID]uuid.UUID{},
		payments:     map[uuid.UUID]*Payment{},
		orderTotals:  map[uuid.UUID]money.Amount{},
	}
}

func (m *memoryStore) seedSettings(businessID uuid.UUID, s DepositSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Currency == "" {
		s.Currency = "IDR"
	}
	if s.Type == "" {
		s.Type = DepositFixed
	}
	m.settings[businessID] = s
}

func (m *memoryStore) addReservation(businessID, reservationID, customerID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reservations[reservationID] = businessID
	m.customers[reservationID] = customerID
}

func (m *memoryStore) GetByID(_ context.Context, businessID, paymentID uuid.UUID) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[paymentID]
	if !ok || p.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *p
	return &copy, nil
}

func (m *memoryStore) GetByReservation(_ context.Context, businessID, reservationID uuid.UUID) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.payments {
		if p.BusinessID == businessID && p.ReservationID == reservationID {
			copy := *p
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memoryStore) LockByReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Payment, error) {
	return m.GetByReservation(ctx, businessID, reservationID)
}

func (m *memoryStore) ReservationExists(_ context.Context, businessID, reservationID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner, ok := m.reservations[reservationID]
	return ok && owner == businessID, nil
}

func (m *memoryStore) LoadDepositSettings(_ context.Context, businessID uuid.UUID) (*DepositSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.settings[businessID]
	if !ok {
		s = DepositSettings{Currency: "IDR", Type: DepositFixed}
	}
	copy := s
	return &copy, nil
}

func (m *memoryStore) OrderTotal(_ context.Context, _ uuid.UUID, reservationID uuid.UUID) (money.Amount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.orderTotals[reservationID], nil
}

func (m *memoryStore) Insert(_ context.Context, p Payment) (*Payment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.payments {
		if existing.BusinessID == p.BusinessID && existing.ReservationID == p.ReservationID {
			copy := *existing
			return &copy, false, nil
		}
	}
	now := time.Now().UTC()
	stored := p
	stored.ID = uuid.New()
	stored.CreatedAt = now
	stored.UpdatedAt = now
	m.payments[stored.ID] = &stored
	copy := stored
	return &copy, true, nil
}

func (m *memoryStore) MarkPaid(_ context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[paymentID]
	if !ok || p.BusinessID != businessID {
		return nil, false, ErrNotFound
	}
	if p.Status != StatusUnpaid {
		copy := *p
		return &copy, false, nil
	}
	p.Status = CollectedStatusForType(p.PaymentType)
	utc := at.UTC()
	p.ConfirmedAt = &utc
	p.ConfirmedBy = &actorID
	if notes != nil {
		p.Notes = notes
	}
	p.UpdatedAt = utc
	copy := *p
	return &copy, true, nil
}

func (m *memoryStore) MarkFull(_ context.Context, businessID, paymentID, actorID uuid.UUID, amount money.Amount, notes *string, at time.Time) (*Payment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[paymentID]
	if !ok || p.BusinessID != businessID {
		return nil, false, ErrNotFound
	}
	if p.Status == StatusPaid && p.PaymentType == PaymentTypeFull {
		copy := *p
		return &copy, false, nil
	}
	if p.Status != StatusDeposit && p.Status != StatusPaid {
		copy := *p
		return &copy, false, nil
	}
	p.Status = StatusPaid
	p.PaymentType = PaymentTypeFull
	p.Amount = amount
	utc := at.UTC()
	// The full-payment moment is what revenue reports are dated by.
	p.ConfirmedAt = &utc
	p.ConfirmedBy = &actorID
	if notes != nil {
		p.Notes = notes
	}
	p.UpdatedAt = utc
	copy := *p
	return &copy, true, nil
}

func (m *memoryStore) MarkRefunded(_ context.Context, businessID, paymentID, actorID uuid.UUID, notes *string, at time.Time) (*Payment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[paymentID]
	if !ok || p.BusinessID != businessID {
		return nil, false, ErrNotFound
	}
	if p.Status != StatusPaid && p.Status != StatusDeposit {
		copy := *p
		return &copy, false, nil
	}
	p.Status = StatusRefunded
	utc := at.UTC()
	p.RefundedAt = &utc
	p.RefundedBy = &actorID
	if notes != nil {
		p.Notes = notes
	}
	p.UpdatedAt = utc
	copy := *p
	return &copy, true, nil
}

func (m *memoryStore) UpdateNotes(_ context.Context, businessID, paymentID uuid.UUID, notes *string) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.payments[paymentID]
	if !ok || p.BusinessID != businessID {
		return nil, ErrNotFound
	}
	p.Notes = notes
	p.UpdatedAt = time.Now().UTC()
	copy := *p
	return &copy, nil
}

func (m *memoryStore) List(_ context.Context, businessID uuid.UUID, params ListParams) ([]Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Payment, 0)
	for _, p := range m.payments {
		if p.BusinessID != businessID {
			continue
		}
		if params.Status != "" && p.Status != params.Status {
			continue
		}
		if params.ReservationID != nil && p.ReservationID != *params.ReservationID {
			continue
		}
		if params.CustomerID != nil && m.customers[p.ReservationID] != *params.CustomerID {
			continue
		}
		if params.CreatedFrom != nil && p.CreatedAt.Before(*params.CreatedFrom) {
			continue
		}
		if params.CreatedTo != nil && !p.CreatedAt.Before(*params.CreatedTo) {
			continue
		}
		copy := *p
		out = append(out, copy)
	}
	if params.Offset > len(out) {
		return []Payment{}, nil
	}
	out = out[params.Offset:]
	if params.Limit > 0 && params.Limit < len(out) {
		out = out[:params.Limit]
	}
	return out, nil
}
