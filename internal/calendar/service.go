package calendar

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CalendarMetrics interface {
	ObserveCalendarBlock(result string)
}

type Service struct {
	store   Store
	now     func() time.Time
	metrics CalendarMetrics
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) SetMetrics(m CalendarMetrics) { s.metrics = m }

func (s *Service) GetBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	return s.store.GetBlock(ctx, businessID, blockID)
}

func (s *Service) CreateBlock(ctx context.Context, businessID, actorID uuid.UUID, in CreateBlockInput) (*Block, error) {
	start, end, err := parseRange(in.StartAt, in.EndAt)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, invalid("title is required")
	}
	if len(title) > 200 {
		return nil, invalid("title is too long")
	}
	notes, err := normalizeNotes(in.Description)
	if err != nil {
		return nil, err
	}
	areaID, err := parseOptionalUUID(in.AreaID)
	if err != nil {
		return nil, invalid("area_id is invalid")
	}
	tableID, err := parseOptionalUUID(in.TableID)
	if err != nil {
		return nil, invalid("table_id is invalid")
	}
	if tableID != nil && areaID == nil {
		return nil, ErrInvalidScope
	}
	if _, _, err := s.store.ResolveScope(ctx, businessID, areaID, tableID); err != nil {
		return nil, err
	}
	conflicts, err := s.store.OverlappingReservations(ctx, businessID, areaID, tableID, start, end)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		s.observeBlock("conflict")
		return nil, conflictError{Conflicts: conflicts}
	}
	item, err := s.store.InsertBlock(ctx, Block{
		BusinessID:  businessID,
		AreaID:      areaID,
		TableID:     tableID,
		Title:       title,
		Description: notes,
		StartAt:     start,
		EndAt:       end,
		Status:      StatusActive,
		CreatedBy:   actorID,
	})
	if err != nil {
		if err == ErrConflict {
			s.observeBlock("conflict")
		}
		return nil, err
	}
	s.observeBlock("created")
	return item, nil
}

func (s *Service) UpdateBlock(ctx context.Context, businessID, blockID uuid.UUID, in UpdateBlockInput) (*Block, error) {
	current, err := s.store.GetBlock(ctx, businessID, blockID)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusActive {
		return nil, invalid("cancelled blocks cannot be updated")
	}
	next := *current
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return nil, invalid("title is required")
		}
		next.Title = title
	}
	if in.Description != nil {
		notes, err := normalizeNotes(in.Description)
		if err != nil {
			return nil, err
		}
		next.Description = notes
	}
	if in.StartAt != nil || in.EndAt != nil {
		startRaw, endRaw := next.StartAt.UTC().Format(time.RFC3339), next.EndAt.UTC().Format(time.RFC3339)
		if in.StartAt != nil {
			startRaw = *in.StartAt
		}
		if in.EndAt != nil {
			endRaw = *in.EndAt
		}
		start, end, err := parseRange(startRaw, endRaw)
		if err != nil {
			return nil, err
		}
		next.StartAt, next.EndAt = start, end
	}
	conflicts, err := s.store.OverlappingReservations(ctx, businessID, next.AreaID, next.TableID, next.StartAt, next.EndAt)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		s.observeBlock("conflict")
		return nil, conflictError{Conflicts: conflicts}
	}
	return s.store.UpdateBlock(ctx, businessID, next)
}

func (s *Service) CancelBlock(ctx context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	return s.store.CancelBlock(ctx, businessID, blockID)
}

func (s *Service) Events(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw, areaRaw, tableRaw, statusRaw string) ([]Event, error) {
	from, to, err := parseBoundedRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	areaID, err := parseOptionalUUIDString(areaRaw)
	if err != nil {
		return nil, invalid("area_id is invalid")
	}
	tableID, err := parseOptionalUUIDString(tableRaw)
	if err != nil {
		return nil, invalid("table_id is invalid")
	}
	status := strings.ToUpper(strings.TrimSpace(statusRaw))
	resStatus, blockStatus := "", StatusActive
	switch status {
	case "", StatusActive:
	case StatusCancelled:
		resStatus, blockStatus = "CANCELLED", StatusCancelled
	case "PENDING", "CONFIRMED", "COMPLETED":
		resStatus, blockStatus = status, StatusActive
	default:
		return nil, invalid("status is invalid")
	}
	reservations, err := s.store.ListReservationEvents(ctx, businessID, from, to, areaID, tableID, resStatus)
	if err != nil {
		return nil, err
	}
	blocks, err := s.store.ListBlocks(ctx, businessID, from, to, areaID, tableID, blockStatus)
	if err != nil {
		return nil, err
	}
	return mergeEvents(reservations, blocks), nil
}

func (s *Service) Day(ctx context.Context, businessID uuid.UUID, dateRaw, areaRaw, tableRaw string) (*DayView, error) {
	tz, err := s.store.Timezone(ctx, businessID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, invalid("timezone is invalid")
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(dateRaw))
	if err != nil {
		return nil, ErrInvalidRange
	}
	from := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	events, err := s.Events(ctx, businessID, from.Format(time.RFC3339), to.Format(time.RFC3339), areaRaw, tableRaw, "")
	if err != nil {
		return nil, err
	}
	hours, err := s.store.ListHours(ctx, businessID)
	if err != nil {
		return nil, err
	}
	exceptions, err := s.store.ListExceptions(ctx, businessID)
	if err != nil {
		return nil, err
	}
	view := &DayView{
		Date:     from.Format("2006-01-02"),
		Timezone: tz,
		Events:   events,
		Hours:    []HourInterval{},
	}
	for _, ex := range exceptions {
		exDate := time.Date(ex.ExceptionDate.Year(), ex.ExceptionDate.Month(), ex.ExceptionDate.Day(), 0, 0, 0, 0, loc)
		if exDate.Equal(time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)) {
			view.Exception = &ExceptionInfo{ID: ex.ID, IsClosed: ex.IsClosed, OpenTime: ex.OpenTime, CloseTime: ex.CloseTime, Note: ex.Note}
			if ex.IsClosed {
				view.Closed = true
				return view, nil
			}
			if ex.OpenTime != nil && ex.CloseTime != nil {
				view.Hours = []HourInterval{{OpenTime: *ex.OpenTime, CloseTime: *ex.CloseTime}}
			}
			return view, nil
		}
	}
	weekday := int(from.Weekday())
	for _, h := range hours {
		if h.DayOfWeek == weekday {
			view.Hours = append(view.Hours, HourInterval{OpenTime: h.OpenTime, CloseTime: h.CloseTime})
		}
	}
	if len(view.Hours) == 0 {
		view.Closed = true
	}
	return view, nil
}

func mergeEvents(reservations []ReservationEvent, blocks []Block) []Event {
	out := make([]Event, 0, len(reservations)+len(blocks))
	for _, r := range reservations {
		id := r.ID
		party := r.PartySize
		out = append(out, Event{
			ID: id, Type: TypeReservation, ReservationID: &id, Title: r.Title,
			StartAt: r.StartAt, EndAt: r.EndAt, Status: r.Status,
			CustomerID: &r.CustomerID, CustomerName: r.CustomerName, PartySize: &party,
			TableID: r.TableID, TableName: r.TableName, AreaID: r.AreaID, AreaName: r.AreaName,
			PaymentStatus: r.PaymentStatus,
		})
	}
	for _, b := range blocks {
		out = append(out, Event{
			ID: b.ID, Type: b.EventType(), Title: b.Title, StartAt: b.StartAt, EndAt: b.EndAt,
			Status: b.Status, Scope: b.Scope(), Description: b.Description,
			TableID: b.TableID, TableName: b.TableName, AreaID: b.AreaID, AreaName: b.AreaName,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartAt.Equal(out[j].StartAt) {
			return out[i].StartAt.Before(out[j].StartAt)
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

func (s *Service) observeBlock(result string) {
	if s.metrics != nil {
		s.metrics.ObserveCalendarBlock(result)
	}
}

func parseRange(startRaw, endRaw string) (time.Time, time.Time, error) {
	start, err := parseRFC3339(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseRFC3339(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, ErrInvalidRange
	}
	return start, end, nil
}

func parseBoundedRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, to, err := parseRange(fromRaw, toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if to.Sub(from) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, ErrInvalidRange
	}
	return from, to, nil
}

func parseRFC3339(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ErrInvalidRange
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, invalid("timestamp is invalid")
	}
	return t.UTC(), nil
}

func parseOptionalUUID(raw *string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	return parseOptionalUUIDString(*raw)
}

func parseOptionalUUIDString(raw string) (*uuid.UUID, error) {
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

func normalizeNotes(notes *string) (*string, error) {
	if notes == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*notes)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > 2000 {
		return nil, invalid("description is too long")
	}
	return &trimmed, nil
}
