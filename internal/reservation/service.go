package reservation

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
)

type EventSink interface {
	ReservationConfirmed(ctx context.Context, res *Reservation) error
	ReservationCancelled(ctx context.Context, res *Reservation) error
}

type ScheduleMetrics interface {
	ObserveReservationReschedule(result string)
}

type Service struct {
	store   Store
	now     func() time.Time
	events  EventSink
	metrics ScheduleMetrics
}

func (s *Service) SetEvents(events EventSink) {
	s.events = events
}

func (s *Service) SetMetrics(m ScheduleMetrics) {
	s.metrics = m
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) Get(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	item, err := s.store.GetReservation(ctx, businessID, reservationID)
	if err != nil {
		return nil, err
	}
	if err := s.attachItems(ctx, businessID, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, businessID uuid.UUID, statusRaw, customerRaw, tableRaw, dateRaw, startDateRaw, endDateRaw, limitRaw, offsetRaw string) ([]Reservation, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	customerID, err := parseOptionalUUID(customerRaw)
	if err != nil {
		return nil, invalid("customer_id is invalid")
	}
	tableID, err := parseOptionalUUID(tableRaw)
	if err != nil {
		return nil, invalid("table_id is invalid")
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	snapshot, loc, _, _, err := s.bookingWindow(ctx, businessID)
	if err != nil {
		return nil, err
	}
	_ = snapshot
	var startFrom, startTo *time.Time
	if dateRaw != "" {
		date, err := parseDate(dateRaw)
		if err != nil {
			return nil, err
		}
		from := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
		to := from.AddDate(0, 0, 1)
		startFrom, startTo = &from, &to
	}
	if startDateRaw != "" {
		date, err := parseDate(startDateRaw)
		if err != nil {
			return nil, err
		}
		from := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
		startFrom = &from
	}
	if endDateRaw != "" {
		date, err := parseDate(endDateRaw)
		if err != nil {
			return nil, err
		}
		to := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		startTo = &to
	}
	rows, err := s.store.ListReservations(ctx, businessID, ListParams{
		Status: status, CustomerID: customerID, TableID: tableID, StartFrom: startFrom, StartTo: startTo, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachItemsMany(ctx, businessID, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) Create(ctx context.Context, businessID uuid.UUID, in CreateReservationInput) (*Reservation, error) {
	customerID, start, end, partySize, tableID, notes, err := s.parseCreate(in)
	if err != nil {
		return nil, err
	}
	items, err := s.resolveItems(ctx, s.store, businessID, in.Items)
	if err != nil {
		return nil, err
	}
	orderTotal, err := sumItemPrices(items)
	if err != nil {
		return nil, err
	}
	paymentType := strings.ToUpper(strings.TrimSpace(in.PaymentType))
	var created *Reservation
	err = s.store.RunInTx(ctx, func(store Store) error {
		snapshot, err := store.GetBookingContext(ctx, businessID)
		if err != nil {
			return err
		}
		if snapshot.DepositEnabled {
			if len(items) == 0 {
				return invalid("menu items are required when deposit is enabled")
			}
			if paymentType != payment.PaymentTypeFull && paymentType != payment.PaymentTypeDeposit {
				return invalid("payment_type must be FULL or DEPOSIT when deposit is enabled")
			}
		} else {
			paymentType = payment.PaymentTypeNone
		}
		if err := s.validateBooking(ctx, store, businessID, customerID, tableID, partySize, start, end, uuid.Nil, true); err != nil {
			return err
		}
		item, err := store.CreateReservation(ctx, Reservation{
			BusinessID: businessID,
			CustomerID: customerID,
			TableID:    tableID,
			PartySize:  partySize,
			StartAt:    start,
			EndAt:      end,
			Status:     StatusPending,
			Notes:      notes,
		})
		if err != nil {
			return err
		}
		if len(items) > 0 {
			if err := store.ReplaceReservationItems(ctx, businessID, item.ID, items); err != nil {
				return err
			}
		}
		if err := store.CreateInitialPayment(ctx, item.BusinessID, item.ID, paymentType, orderTotal); err != nil {
			return err
		}
		created = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, businessID, created.ID)
}

func (s *Service) attachItems(ctx context.Context, businessID uuid.UUID, item *Reservation) error {
	if item == nil {
		return nil
	}
	rows, err := s.store.ListReservationItems(ctx, businessID, item.ID)
	if err != nil {
		return err
	}
	if rows == nil {
		rows = []ReservationItem{}
	}
	item.Items = rows
	return nil
}

func (s *Service) attachItemsMany(ctx context.Context, businessID uuid.UUID, items []Reservation) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
		items[i].Items = []ReservationItem{}
	}
	byRes, err := s.store.ListReservationItemsByReservations(ctx, businessID, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if rows := byRes[items[i].ID]; rows != nil {
			items[i].Items = rows
		}
	}
	return nil
}

func (s *Service) resolveItems(ctx context.Context, store Store, businessID uuid.UUID, inputs []ReservationItemInput) ([]ReservationItem, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > 40 {
		return nil, invalid("too many menu items")
	}
	out := make([]ReservationItem, 0, len(inputs))
	seen := map[uuid.UUID]int{}
	for i, in := range inputs {
		menuItemID, err := parseRequiredUUID(in.MenuItemID, "items[].menu_item_id")
		if err != nil {
			return nil, err
		}
		if in.Quantity < 1 || in.Quantity > 99 {
			return nil, ErrInvalidMenuItem
		}
		notes, err := normalizeNotes(in.Notes)
		if err != nil {
			return nil, err
		}
		ref, err := store.GetActiveMenuItem(ctx, businessID, menuItemID)
		if err != nil {
			return nil, err
		}
		if ref.Status != "ACTIVE" {
			return nil, ErrMenuItemInactive
		}
		if prev, ok := seen[menuItemID]; ok {
			out[prev].Quantity += in.Quantity
			if out[prev].Quantity > 99 {
				return nil, ErrInvalidMenuItem
			}
			continue
		}
		seen[menuItemID] = len(out)
		out = append(out, ReservationItem{
			BusinessID:   businessID,
			MenuItemID:   menuItemID,
			MenuItemName: ref.Name,
			Quantity:     in.Quantity,
			UnitPrice:    ref.Price,
			Notes:        notes,
		})
		_ = i
	}
	return out, nil
}

func sumItemPrices(items []ReservationItem) (money.Amount, error) {
	var total money.Amount
	for _, item := range items {
		unit, err := money.Parse(strings.TrimSpace(item.UnitPrice))
		if err != nil {
			return 0, ErrInvalidMenuItem
		}
		line := money.Amount(int64(unit) * int64(item.Quantity))
		total += line
	}
	return money.ClampNonNegative(total), nil
}

func (s *Service) Update(ctx context.Context, businessID, reservationID uuid.UUID, in UpdateReservationInput) (*Reservation, error) {
	var updated *Reservation
	err := s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.LockReservation(ctx, businessID, reservationID)
		if err != nil {
			return err
		}
		if current.Status == StatusCancelled || current.Status == StatusCompleted {
			return ErrCannotUpdate
		}
		next := *current
		if in.CustomerID != nil {
			id, err := parseRequiredUUID(*in.CustomerID, "customer_id")
			if err != nil {
				return err
			}
			next.CustomerID = id
		}
		if in.StartAt != nil {
			t, err := parseRFC3339(*in.StartAt)
			if err != nil {
				return err
			}
			next.StartAt = t
		}
		if in.EndAt != nil {
			t, err := parseRFC3339(*in.EndAt)
			if err != nil {
				return err
			}
			next.EndAt = t
		}
		if in.PartySize != nil {
			next.PartySize = *in.PartySize
		}
		if in.ClearTable {
			if current.Status == StatusConfirmed {
				return invalid("confirmed reservations must keep a table")
			}
			next.TableID = nil
		} else if in.TableID != nil {
			id, err := parseRequiredUUID(*in.TableID, "table_id")
			if err != nil {
				return err
			}
			next.TableID = &id
		}
		if in.Notes != nil {
			notes, err := normalizeNotes(in.Notes)
			if err != nil {
				return err
			}
			next.Notes = notes
		}
		if err := s.validateBooking(ctx, store, businessID, next.CustomerID, next.TableID, next.PartySize, next.StartAt, next.EndAt, current.ID, next.TableID != nil); err != nil {
			return err
		}
		item, err := store.UpdateReservation(ctx, businessID, next)
		if err != nil {
			return err
		}
		updated = item
		return nil
	})
	return updated, err
}

func (s *Service) Reschedule(ctx context.Context, businessID, reservationID uuid.UUID, in RescheduleInput) (*Reservation, error) {
	start, err := parseRFC3339(in.StartAt)
	if err != nil {
		return nil, err
	}
	end, err := parseRFC3339(in.EndAt)
	if err != nil {
		return nil, err
	}
	var updated *Reservation
	err = s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.LockReservation(ctx, businessID, reservationID)
		if err != nil {
			return err
		}
		if current.Status == StatusCancelled || current.Status == StatusCompleted {
			return ErrRescheduleNotAllowed
		}
		if current.Status != StatusPending && current.Status != StatusConfirmed {
			return ErrRescheduleNotAllowed
		}
		next := *current
		next.StartAt = start
		next.EndAt = end
		if in.PartySize != nil {
			next.PartySize = *in.PartySize
		}
		if in.TableID != nil {
			id, err := parseRequiredUUID(*in.TableID, "table_id")
			if err != nil {
				return err
			}
			next.TableID = &id
		}
		if next.Status == StatusConfirmed && next.TableID == nil {
			return invalid("confirmed reservations must keep a table")
		}
		if err := s.validateBooking(ctx, store, businessID, next.CustomerID, next.TableID, next.PartySize, next.StartAt, next.EndAt, current.ID, next.TableID != nil); err != nil {
			if errors.Is(err, ErrTableUnavailable) || errors.Is(err, ErrConflict) {
				return ErrRescheduleConflict
			}
			return err
		}
		item, err := store.UpdateReservation(ctx, businessID, next)
		if err != nil {
			if errors.Is(err, ErrTableUnavailable) {
				return ErrRescheduleConflict
			}
			return err
		}
		updated = item
		return nil
	})
	if s.metrics != nil {
		result := "ok"
		if err != nil {
			result = "conflict"
			if errors.Is(err, ErrRescheduleNotAllowed) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) {
				result = "rejected"
			}
		}
		s.metrics.ObserveReservationReschedule(result)
	}
	return updated, err
}

func (s *Service) Confirm(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	var confirmed *Reservation
	err := s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.LockReservation(ctx, businessID, reservationID)
		if err != nil {
			return err
		}
		if current.Status != StatusPending {
			return ErrInvalidStatus
		}
		if err := store.AssertPaymentForConfirm(ctx, businessID, reservationID); err != nil {
			return err
		}
		if err := s.validateBooking(ctx, store, businessID, current.CustomerID, current.TableID, current.PartySize, current.StartAt, current.EndAt, current.ID, false); err != nil {
			return err
		}
		tableID := current.TableID
		if tableID == nil {
			chosen, err := s.autoAssign(ctx, store, businessID, current.PartySize, current.StartAt, current.EndAt, current.ID, current.Notes)
			if err != nil {
				return err
			}
			tableID = &chosen.ID
		} else {
			if err := s.lockAndCheckTable(ctx, store, businessID, *tableID, current.PartySize, current.StartAt, current.EndAt, current.ID); err != nil {
				return err
			}
		}
		now := s.now()
		next := *current
		next.TableID = tableID
		next.Status = StatusConfirmed
		next.ConfirmedAt = &now
		item, err := store.UpdateReservation(ctx, businessID, next)
		if err != nil {
			return err
		}
		confirmed = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	if confirmed != nil && s.events != nil {
		if pubErr := s.events.ReservationConfirmed(ctx, confirmed); pubErr != nil {
			slog.Default().Warn("automation reservation.confirmed failed", "error", pubErr)
		}
	}
	return confirmed, nil
}

func (s *Service) Cancel(ctx context.Context, businessID, reservationID uuid.UUID, reason *string) (*Reservation, error) {
	var cancelled *Reservation
	err := s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.LockReservation(ctx, businessID, reservationID)
		if err != nil {
			return err
		}
		if current.Status != StatusPending && current.Status != StatusConfirmed {
			return ErrInvalidStatus
		}
		notes, err := normalizeNotes(reason)
		if err != nil {
			return err
		}
		next := *current
		next.Status = StatusCancelled
		next.CancellationReason = notes
		item, err := store.UpdateReservation(ctx, businessID, next)
		if err != nil {
			return err
		}
		cancelled = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cancelled != nil && s.events != nil {
		if pubErr := s.events.ReservationCancelled(ctx, cancelled); pubErr != nil {
			slog.Default().Warn("automation reservation.cancelled failed", "error", pubErr)
		}
	}
	return cancelled, nil
}

func (s *Service) Complete(ctx context.Context, businessID, reservationID uuid.UUID) (*Reservation, error) {
	var completed *Reservation
	err := s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.LockReservation(ctx, businessID, reservationID)
		if err != nil {
			return err
		}
		if current.Status != StatusConfirmed {
			return ErrInvalidStatus
		}
		if err := store.AssertPaymentForComplete(ctx, businessID, reservationID); err != nil {
			return err
		}
		now := s.now()
		next := *current
		next.Status = StatusCompleted
		next.CompletedAt = &now
		item, err := store.UpdateReservation(ctx, businessID, next)
		if err != nil {
			return err
		}
		completed = item
		return nil
	})
	return completed, err
}

func (s *Service) ListHours(ctx context.Context, businessID uuid.UUID) ([]Hour, error) {
	return s.store.ListHours(ctx, businessID)
}

func (s *Service) ReplaceHours(ctx context.Context, businessID uuid.UUID, items []HourInput) ([]Hour, error) {
	if err := hoursOverlapSameDay(items); err != nil {
		return nil, err
	}
	hours := make([]Hour, 0, len(items))
	for _, item := range items {
		interval, err := parseClockInterval(item.OpenTime, item.CloseTime)
		if err != nil {
			return nil, err
		}
		hours = append(hours, Hour{
			DayOfWeek: item.DayOfWeek,
			SortOrder: item.SortOrder,
			OpenTime:  formatClock(interval.OpenMinutes/60, interval.OpenMinutes%60),
			CloseTime: formatClock(interval.CloseMinutes/60, interval.CloseMinutes%60),
		})
	}
	var out []Hour
	err := s.store.RunInTx(ctx, func(store Store) error {
		replaced, err := store.ReplaceHours(ctx, businessID, hours)
		if err != nil {
			return err
		}
		out = replaced
		return nil
	})
	return out, err
}

func (s *Service) ListExceptions(ctx context.Context, businessID uuid.UUID) ([]ScheduleException, error) {
	return s.store.ListExceptions(ctx, businessID)
}

func (s *Service) CreateException(ctx context.Context, businessID uuid.UUID, in ExceptionInput) (*ScheduleException, error) {
	ex, err := normalizeException(businessID, in)
	if err != nil {
		return nil, err
	}
	return s.store.CreateException(ctx, ex)
}

func (s *Service) UpdateException(ctx context.Context, businessID, exceptionID uuid.UUID, in UpdateExceptionInput) (*ScheduleException, error) {
	current, err := s.store.GetException(ctx, businessID, exceptionID)
	if err != nil {
		return nil, err
	}
	next := ExceptionInput{
		ExceptionDate: current.ExceptionDate.Format("2006-01-02"),
		IsClosed:      current.IsClosed,
		OpenTime:      current.OpenTime,
		CloseTime:     current.CloseTime,
		Note:          current.Note,
	}
	if in.ExceptionDate != nil {
		next.ExceptionDate = *in.ExceptionDate
	}
	if in.IsClosed != nil {
		next.IsClosed = *in.IsClosed
	}
	if in.OpenTime != nil {
		next.OpenTime = in.OpenTime
	}
	if in.CloseTime != nil {
		next.CloseTime = in.CloseTime
	}
	if in.Note != nil {
		next.Note = in.Note
	}
	normalized, err := normalizeException(businessID, next)
	if err != nil {
		return nil, err
	}
	normalized.ID = current.ID
	return s.store.UpdateException(ctx, businessID, normalized)
}

func (s *Service) DeleteException(ctx context.Context, businessID, exceptionID uuid.UUID) error {
	return s.store.DeleteException(ctx, businessID, exceptionID)
}

func (s *Service) parseCreate(in CreateReservationInput) (uuid.UUID, time.Time, time.Time, int, *uuid.UUID, *string, error) {
	customerID, err := parseRequiredUUID(in.CustomerID, "customer_id")
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, 0, nil, nil, err
	}
	start, err := parseRFC3339(in.StartAt)
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, 0, nil, nil, err
	}
	end, err := parseRFC3339(in.EndAt)
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, 0, nil, nil, err
	}
	notes, err := normalizeNotes(in.Notes)
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, 0, nil, nil, err
	}
	var tableID *uuid.UUID
	if in.TableID != nil && strings.TrimSpace(*in.TableID) != "" {
		id, err := parseRequiredUUID(*in.TableID, "table_id")
		if err != nil {
			return uuid.Nil, time.Time{}, time.Time{}, 0, nil, nil, err
		}
		tableID = &id
	}
	return customerID, start, end, in.PartySize, tableID, notes, nil
}

func (s *Service) validateBooking(ctx context.Context, store Store, businessID, customerID uuid.UUID, tableID *uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID, requireEnabled bool) error {
	snapshot, err := store.GetBookingContext(ctx, businessID)
	if err != nil {
		return err
	}
	if requireEnabled && !snapshot.ReservationEnabled {
		return ErrDisabled
	}
	if !requireEnabled && !snapshot.ReservationEnabled {
		return ErrDisabled
	}
	if err := validateTimeRange(start, end); err != nil {
		return err
	}
	if err := validatePartySize(partySize, *snapshot); err != nil {
		return err
	}
	if err := validateAdvance(start, s.now(), *snapshot); err != nil {
		return err
	}
	loc, err := loadLocation(snapshot.Timezone)
	if err != nil {
		return err
	}
	hours, err := store.ListHours(ctx, businessID)
	if err != nil {
		return err
	}
	exceptions, err := store.ListExceptions(ctx, businessID)
	if err != nil {
		return err
	}
	if err := validateOperatingHours(start, end, loc, hours, exceptions); err != nil {
		return err
	}
	customer, err := store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return err
	}
	if customer.Status != "ACTIVE" {
		return ErrCustomerInactive
	}
	if tableID != nil {
		return s.lockAndCheckTable(ctx, store, businessID, *tableID, partySize, start, end, excludeID)
	}
	blocked, err := store.HasCalendarBlock(ctx, businessID, nil, nil, start, end)
	if err != nil {
		return err
	}
	if blocked {
		return ErrCalendarBlocked
	}
	return nil
}

func (s *Service) lockAndCheckTable(ctx context.Context, store Store, businessID, tableID uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID) error {
	table, err := store.LockTable(ctx, businessID, tableID)
	if err != nil {
		return err
	}
	if err := validateTableRecord(table, partySize); err != nil {
		return err
	}
	blocked, err := store.HasCalendarBlock(ctx, businessID, &table.AreaID, &tableID, start, end)
	if err != nil {
		return err
	}
	if blocked {
		return ErrCalendarBlocked
	}
	conflict, err := store.HasConflict(ctx, businessID, tableID, start, end, excludeID)
	if err != nil {
		return err
	}
	if conflict {
		return ErrTableUnavailable
	}
	return nil
}

func (s *Service) autoAssign(ctx context.Context, store Store, businessID uuid.UUID, partySize int, start, end time.Time, excludeID uuid.UUID, notes *string) (*TableRecord, error) {
	tables, err := store.FindAvailableTables(ctx, businessID, partySize, start, end, excludeID)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, ErrNoTableAvailable
	}
	chosen := tables[0]
	if notes != nil {
		if preferred := MatchPreferredTableRecord(*notes, tables); preferred != nil {
			chosen = *preferred
		}
	}
	if err := s.lockAndCheckTable(ctx, store, businessID, chosen.ID, partySize, start, end, excludeID); err != nil {
		if err == ErrTableUnavailable || err == ErrCalendarBlocked {
			return nil, ErrNoTableAvailable
		}
		return nil, err
	}
	return &chosen, nil
}

func normalizeException(businessID uuid.UUID, in ExceptionInput) (ScheduleException, error) {
	date, err := parseDate(in.ExceptionDate)
	if err != nil {
		return ScheduleException{}, err
	}
	note, err := normalizeNotes(in.Note)
	if err != nil {
		return ScheduleException{}, err
	}
	ex := ScheduleException{
		BusinessID:    businessID,
		ExceptionDate: date,
		IsClosed:      in.IsClosed,
		Note:          note,
	}
	if in.IsClosed {
		if (in.OpenTime != nil && strings.TrimSpace(*in.OpenTime) != "") || (in.CloseTime != nil && strings.TrimSpace(*in.CloseTime) != "") {
			return ScheduleException{}, invalid("closed exceptions cannot have hours")
		}
		return ex, nil
	}
	if in.OpenTime == nil || in.CloseTime == nil {
		return ScheduleException{}, invalid("open hours are required when the business is open")
	}
	interval, err := parseClockInterval(*in.OpenTime, *in.CloseTime)
	if err != nil {
		return ScheduleException{}, err
	}
	open := formatClock(interval.OpenMinutes/60, interval.OpenMinutes%60)
	close := formatClock(interval.CloseMinutes/60, interval.CloseMinutes%60)
	ex.OpenTime = &open
	ex.CloseTime = &close
	return ex, nil
}
