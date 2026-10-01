package reservation

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
)

type memoryStore struct {
	mu           sync.Mutex
	ctxs         map[uuid.UUID]BookingContext
	customers    map[uuid.UUID]CustomerRecord
	custBiz      map[uuid.UUID]uuid.UUID
	tables       map[uuid.UUID]TableRecord
	reservations map[uuid.UUID]*Reservation
	hours        map[uuid.UUID][]Hour
	exceptions   map[uuid.UUID][]ScheduleException
	payments     map[uuid.UUID]*payment.Payment
	blocks       []memoryBlock
	menuItems    map[uuid.UUID]memoryMenuItem
	items        map[uuid.UUID][]ReservationItem
}

type memoryMenuItem struct {
	BusinessID uuid.UUID
	Ref        MenuItemRef
}

type memoryBlock struct {
	BusinessID uuid.UUID
	AreaID     *uuid.UUID
	TableID    *uuid.UUID
	StartAt    time.Time
	EndAt      time.Time
	Status     string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		ctxs:         map[uuid.UUID]BookingContext{},
		customers:    map[uuid.UUID]CustomerRecord{},
		custBiz:      map[uuid.UUID]uuid.UUID{},
		tables:       map[uuid.UUID]TableRecord{},
		reservations: map[uuid.UUID]*Reservation{},
		hours:        map[uuid.UUID][]Hour{},
		exceptions:   map[uuid.UUID][]ScheduleException{},
		payments:     map[uuid.UUID]*payment.Payment{},
		menuItems:    map[uuid.UUID]memoryMenuItem{},
		items:        map[uuid.UUID][]ReservationItem{},
	}
}

func (m *memoryStore) seedContext(businessID uuid.UUID, tz string, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctxs[businessID] = BookingContext{
		Timezone: tz, ReservationEnabled: enabled,
		ReservationSlotDurationMinutes: 90,
		ReservationMinPartySize:        1,
		ReservationMaxPartySize:        8,
		ReservationMinAdvanceMinutes:   60,
		ReservationMaxAdvanceDays:      30,
		Currency:                       "IDR",
		DepositEnabled:                 false,
		DepositType:                    "FIXED",
		DepositValue:                   0,
	}
}

func (m *memoryStore) addCustomer(businessID uuid.UUID, name, status string, phone *string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.customers[id] = CustomerRecord{ID: id, Name: name, Phone: phone, Status: status}
	m.custBiz[id] = businessID
	return id
}

func (m *memoryStore) addTable(businessID uuid.UUID, name, areaName string, capacity int, tableStatus, areaStatus string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.tables[id] = TableRecord{
		ID: id, Name: name, Capacity: capacity, Status: tableStatus,
		AreaID: uuid.New(), AreaName: areaName, AreaStatus: areaStatus, BusinessID: businessID,
	}
	return id
}

func (m *memoryStore) RunInTx(_ context.Context, fn func(Store) error) error {
	return fn(m)
}

func (m *memoryStore) GetBookingContext(_ context.Context, businessID uuid.UUID) (*BookingContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.ctxs[businessID]
	if !ok {
		return nil, ErrNotFound
	}
	copy := c
	return &copy, nil
}

func (m *memoryStore) GetCustomer(_ context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.customers[customerID]
	if !ok || m.custBiz[customerID] != businessID {
		return nil, ErrCustomerNotFound
	}
	return &c, nil
}

func (m *memoryStore) GetTable(_ context.Context, businessID, tableID uuid.UUID) (*TableRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tables[tableID]
	if !ok || t.BusinessID != businessID {
		return nil, ErrTableNotFound
	}
	return &t, nil
}

func (m *memoryStore) LockTable(ctx context.Context, businessID, tableID uuid.UUID) (*TableRecord, error) {
	return m.GetTable(ctx, businessID, tableID)
}

func (m *memoryStore) FindAvailableTables(_ context.Context, businessID uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID) ([]TableRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TableRecord, 0)
	for _, t := range m.tables {
		if t.BusinessID != businessID || t.Status != "ACTIVE" || t.AreaStatus != "ACTIVE" || t.Capacity < partySize {
			continue
		}
		if m.conflictLocked(businessID, t.ID, start, end, excludeID) {
			continue
		}
		if m.blockLocked(businessID, &t.AreaID, &t.ID, start, end) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Capacity != out[j].Capacity {
			return out[i].Capacity < out[j].Capacity
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out, nil
}

func (m *memoryStore) HasConflict(_ context.Context, businessID, tableID uuid.UUID, start, end time.Time, excludeID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conflictLocked(businessID, tableID, start, end, excludeID), nil
}

func (m *memoryStore) HasCalendarBlock(_ context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blockLocked(businessID, areaID, tableID, start, end), nil
}

func (m *memoryStore) seedBlock(businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks = append(m.blocks, memoryBlock{
		BusinessID: businessID, AreaID: areaID, TableID: tableID, StartAt: start, EndAt: end, Status: "ACTIVE",
	})
}

func (m *memoryStore) blockLocked(businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) bool {
	for _, b := range m.blocks {
		if b.BusinessID != businessID || b.Status != "ACTIVE" {
			continue
		}
		if !overlaps(b.StartAt, b.EndAt, start, end) {
			continue
		}
		if b.AreaID == nil && b.TableID == nil {
			return true
		}
		if b.TableID == nil && areaID != nil && b.AreaID != nil && *b.AreaID == *areaID {
			return true
		}
		if tableID != nil && b.TableID != nil && *b.TableID == *tableID {
			return true
		}
	}
	return false
}

func (m *memoryStore) conflictLocked(businessID, tableID uuid.UUID, start, end time.Time, excludeID uuid.UUID) bool {
	for _, r := range m.reservations {
		if r.BusinessID != businessID || r.TableID == nil || *r.TableID != tableID {
			continue
		}
		if r.ID == excludeID {
			continue
		}
		if r.Status != StatusPending && r.Status != StatusConfirmed {
			continue
		}
		if overlaps(r.StartAt, r.EndAt, start, end) {
			return true
		}
	}
	return false
}

func (m *memoryStore) CreateReservation(_ context.Context, res Reservation) (*Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if res.TableID != nil && m.conflictLocked(res.BusinessID, *res.TableID, res.StartAt, res.EndAt, uuid.Nil) {
		return nil, ErrTableUnavailable
	}
	now := time.Now().UTC()
	stored := res
	stored.ID = uuid.New()
	stored.CreatedAt = now
	stored.UpdatedAt = now
	m.hydrate(&stored)
	m.reservations[stored.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) GetReservation(_ context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reservations[reservationID]
	if !ok || r.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *r
	m.hydrate(&copy)
	return &copy, nil
}

func (m *memoryStore) LockReservation(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	return m.GetReservation(ctx, businessID, reservationID)
}

func (m *memoryStore) ListReservations(_ context.Context, businessID uuid.UUID, params ListParams) ([]Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Reservation, 0)
	for _, r := range m.reservations {
		if r.BusinessID != businessID {
			continue
		}
		if params.Status != "" && r.Status != params.Status {
			continue
		}
		if params.CustomerID != nil && r.CustomerID != *params.CustomerID {
			continue
		}
		if params.TableID != nil && (r.TableID == nil || *r.TableID != *params.TableID) {
			continue
		}
		if params.StartFrom != nil && r.StartAt.Before(*params.StartFrom) {
			continue
		}
		if params.StartTo != nil && !r.StartAt.Before(*params.StartTo) {
			continue
		}
		copy := *r
		m.hydrate(&copy)
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartAt.Equal(out[j].StartAt) {
			return out[i].StartAt.Before(out[j].StartAt)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	if params.Offset > len(out) {
		return []Reservation{}, nil
	}
	out = out[params.Offset:]
	if params.Limit < len(out) {
		out = out[:params.Limit]
	}
	return out, nil
}

func (m *memoryStore) UpdateReservation(_ context.Context, businessID uuid.UUID, res Reservation) (*Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.reservations[res.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrNotFound
	}
	if res.TableID != nil && (res.Status == StatusPending || res.Status == StatusConfirmed) &&
		m.conflictLocked(businessID, *res.TableID, res.StartAt, res.EndAt, res.ID) {
		return nil, ErrTableUnavailable
	}
	current.CustomerID = res.CustomerID
	current.TableID = res.TableID
	current.PartySize = res.PartySize
	current.StartAt = res.StartAt
	current.EndAt = res.EndAt
	current.Status = res.Status
	current.Notes = res.Notes
	current.CancellationReason = res.CancellationReason
	current.ConfirmedAt = res.ConfirmedAt
	current.CompletedAt = res.CompletedAt
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	m.hydrate(&copy)
	return &copy, nil
}

func (m *memoryStore) ListHours(_ context.Context, businessID uuid.UUID) ([]Hour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := append([]Hour{}, m.hours[businessID]...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].DayOfWeek != items[j].DayOfWeek {
			return items[i].DayOfWeek < items[j].DayOfWeek
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

func (m *memoryStore) ReplaceHours(_ context.Context, businessID uuid.UUID, hours []Hour) ([]Hour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := make([]Hour, 0, len(hours))
	now := time.Now().UTC()
	for _, h := range hours {
		h.ID = uuid.New()
		h.BusinessID = businessID
		h.CreatedAt = now
		h.UpdatedAt = now
		stored = append(stored, h)
	}
	m.hours[businessID] = stored
	return append([]Hour{}, stored...), nil
}

func (m *memoryStore) ListExceptions(_ context.Context, businessID uuid.UUID) ([]ScheduleException, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ScheduleException{}, m.exceptions[businessID]...), nil
}

func (m *memoryStore) GetException(_ context.Context, businessID, exceptionID uuid.UUID) (*ScheduleException, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exceptions[businessID] {
		if e.ID == exceptionID {
			copy := e
			return &copy, nil
		}
	}
	return nil, ErrExceptionNotFound
}

func (m *memoryStore) CreateException(_ context.Context, ex ScheduleException) (*ScheduleException, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.exceptions[ex.BusinessID] {
		if existing.ExceptionDate.Equal(ex.ExceptionDate) {
			return nil, ErrExceptionExists
		}
	}
	now := time.Now().UTC()
	ex.ID = uuid.New()
	ex.CreatedAt = now
	ex.UpdatedAt = now
	m.exceptions[ex.BusinessID] = append(m.exceptions[ex.BusinessID], ex)
	copy := ex
	return &copy, nil
}

func (m *memoryStore) UpdateException(_ context.Context, businessID uuid.UUID, ex ScheduleException) (*ScheduleException, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.exceptions[businessID]
	for i, existing := range items {
		if existing.ID == ex.ID {
			for j, other := range items {
				if i != j && other.ExceptionDate.Equal(ex.ExceptionDate) {
					return nil, ErrExceptionExists
				}
			}
			ex.CreatedAt = existing.CreatedAt
			ex.UpdatedAt = time.Now().UTC()
			items[i] = ex
			m.exceptions[businessID] = items
			copy := ex
			return &copy, nil
		}
	}
	return nil, ErrExceptionNotFound
}

func (m *memoryStore) DeleteException(_ context.Context, businessID, exceptionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.exceptions[businessID]
	for i, e := range items {
		if e.ID == exceptionID {
			m.exceptions[businessID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return ErrExceptionNotFound
}

func (m *memoryStore) CreateInitialPayment(_ context.Context, businessID, reservationID uuid.UUID, paymentType string, orderTotal money.Amount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, ok := m.reservations[reservationID]
	if !ok || res.BusinessID != businessID {
		return ErrNotFound
	}
	if existing, ok := m.payments[reservationID]; ok && existing.BusinessID == businessID {
		return nil
	}
	snapshot := m.ctxs[businessID]
	currency := snapshot.Currency
	if currency == "" {
		currency = "IDR"
	}
	settings := payment.DepositSettings{
		Enabled:  snapshot.DepositEnabled,
		Type:     snapshot.DepositType,
		Value:    snapshot.DepositValue,
		Currency: currency,
	}
	amount, resolvedType, err := payment.AmountDue(settings, paymentType, orderTotal)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		resolvedType = payment.PaymentTypeNone
		amount = 0
	}
	now := time.Now().UTC()
	m.payments[reservationID] = &payment.Payment{
		ID:            uuid.New(),
		BusinessID:    businessID,
		ReservationID: reservationID,
		Status:        payment.StatusUnpaid,
		PaymentType:   resolvedType,
		Amount:        amount,
		Currency:      currency,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return nil
}

func (m *memoryStore) AssertPaymentForConfirm(_ context.Context, businessID, reservationID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ctxs[businessID].DepositEnabled {
		return nil
	}
	p, ok := m.payments[reservationID]
	if !ok || p.BusinessID != businessID || (p.Status != payment.StatusPaid && p.Status != payment.StatusDeposit) {
		return ErrPaymentRequired
	}
	return nil
}

func (m *memoryStore) AssertPaymentForComplete(_ context.Context, businessID, reservationID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ctxs[businessID].DepositEnabled {
		return nil
	}
	p, ok := m.payments[reservationID]
	if !ok || p.BusinessID != businessID || p.Status != payment.StatusPaid {
		return ErrFullPaymentRequired
	}
	return nil
}

func (m *memoryStore) GetActiveMenuItem(_ context.Context, businessID, menuItemID uuid.UUID) (*MenuItemRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.menuItems[menuItemID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrMenuItemNotFound
	}
	copy := item.Ref
	return &copy, nil
}

func (m *memoryStore) ReplaceReservationItems(_ context.Context, businessID, reservationID uuid.UUID, items []ReservationItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, ok := m.reservations[reservationID]
	if !ok || res.BusinessID != businessID {
		return ErrNotFound
	}
	next := make([]ReservationItem, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		row := item
		row.ID = uuid.New()
		row.BusinessID = businessID
		row.ReservationID = reservationID
		row.CreatedAt = now
		row.UpdatedAt = now
		next = append(next, row)
	}
	m.items[reservationID] = next
	return nil
}

func (m *memoryStore) ListReservationItems(_ context.Context, businessID, reservationID uuid.UUID) ([]ReservationItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, ok := m.reservations[reservationID]
	if !ok || res.BusinessID != businessID {
		return nil, ErrNotFound
	}
	rows := m.items[reservationID]
	out := make([]ReservationItem, len(rows))
	copy(out, rows)
	return out, nil
}

func (m *memoryStore) ListReservationItemsByReservations(_ context.Context, businessID uuid.UUID, reservationIDs []uuid.UUID) (map[uuid.UUID][]ReservationItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[uuid.UUID][]ReservationItem, len(reservationIDs))
	for _, id := range reservationIDs {
		res, ok := m.reservations[id]
		if !ok || res.BusinessID != businessID {
			continue
		}
		rows := m.items[id]
		cp := make([]ReservationItem, len(rows))
		copy(cp, rows)
		out[id] = cp
	}
	return out, nil
}


func (m *memoryStore) addMenuItem(businessID uuid.UUID, name, price, status string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.menuItems[id] = memoryMenuItem{
		BusinessID: businessID,
		Ref: MenuItemRef{
			ID:     id,
			Name:   name,
			Price:  price,
			Status: status,
		},
	}
	return id
}

func (m *memoryStore) seedDeposit(businessID uuid.UUID, enabled bool, typ string, value money.Amount) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ctxs[businessID]
	c.DepositEnabled = enabled
	c.DepositType = typ
	c.DepositValue = value
	if c.Currency == "" {
		c.Currency = "IDR"
	}
	m.ctxs[businessID] = c
}

func (m *memoryStore) paymentFor(reservationID uuid.UUID) *payment.Payment {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.payments[reservationID]
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

func (m *memoryStore) setPaymentStatus(reservationID uuid.UUID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.payments[reservationID]
	if p == nil {
		return
	}
	now := time.Now().UTC()
	actor := uuid.New()
	p.Status = status
	p.UpdatedAt = now
	switch status {
	case payment.StatusPaid:
		p.ConfirmedAt = &now
		p.ConfirmedBy = &actor
		p.RefundedAt = nil
		p.RefundedBy = nil
	case payment.StatusRefunded:
		p.RefundedAt = &now
		p.RefundedBy = &actor
	}
}

func (m *memoryStore) hydrate(r *Reservation) {
	if c, ok := m.customers[r.CustomerID]; ok {
		r.Customer = CustomerSummary{ID: c.ID, Name: c.Name, Phone: c.Phone}
	}
	if r.TableID != nil {
		if t, ok := m.tables[*r.TableID]; ok {
			r.Table = &TableSummary{ID: t.ID, Name: t.Name, Capacity: t.Capacity, AreaID: t.AreaID, AreaName: t.AreaName}
		}
	}
}

func ptr(v string) *string { return &v }
