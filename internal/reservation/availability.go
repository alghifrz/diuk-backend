package reservation

import (
	"context"
	"time"

	"github.com/google/uuid"
)

func (s *Service) Availability(ctx context.Context, businessID uuid.UUID, dateRaw, startRaw, durationRaw, partyRaw string) (*AvailabilityResult, error) {
	snapshot, loc, hours, exceptions, err := s.bookingWindow(ctx, businessID)
	if err != nil {
		return nil, err
	}
	if !snapshot.ReservationEnabled {
		return nil, ErrDisabled
	}

	date, err := parseDate(dateRaw)
	if err != nil {
		return nil, err
	}
	sh, sm, err := parseClock(startRaw)
	if err != nil {
		return nil, err
	}
	duration := snapshot.ReservationSlotDurationMinutes
	if durationRaw != "" {
		duration, err = parsePositiveInt(durationRaw, "duration_minutes")
		if err != nil {
			return nil, err
		}
	}
	partySize, err := parsePositiveInt(partyRaw, "party_size")
	if err != nil {
		return nil, err
	}
	if err := validatePartySize(partySize, snapshot); err != nil {
		return nil, err
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), sh, sm, 0, 0, loc)
	end := start.Add(time.Duration(duration) * time.Minute)
	if err := validateTimeRange(start, end); err != nil {
		return nil, err
	}
	if err := validateAdvance(start, s.now(), snapshot); err != nil {
		return nil, err
	}
	if err := validateOperatingHours(start, end, loc, hours, exceptions); err != nil {
		return nil, err
	}

	tables, err := s.store.FindAvailableTables(ctx, businessID, partySize, start, end, uuid.Nil)
	if err != nil {
		return nil, err
	}
	summaries := make([]TableSummary, 0, len(tables))
	for _, t := range tables {
		summaries = append(summaries, TableSummary{
			ID: t.ID, Name: t.Name, Capacity: t.Capacity, AreaID: t.AreaID, AreaName: t.AreaName,
		})
	}
	return &AvailabilityResult{
		Date:      date.Format("2006-01-02"),
		StartAt:   start,
		EndAt:     end,
		PartySize: partySize,
		Available: len(summaries) > 0,
		Tables:    summaries,
	}, nil
}

func (s *Service) bookingWindow(ctx context.Context, businessID uuid.UUID) (BookingContext, *time.Location, []Hour, []ScheduleException, error) {
	snapshot, err := s.store.GetBookingContext(ctx, businessID)
	if err != nil {
		return BookingContext{}, nil, nil, nil, err
	}
	loc, err := loadLocation(snapshot.Timezone)
	if err != nil {
		return BookingContext{}, nil, nil, nil, err
	}
	hours, err := s.store.ListHours(ctx, businessID)
	if err != nil {
		return BookingContext{}, nil, nil, nil, err
	}
	exceptions, err := s.store.ListExceptions(ctx, businessID)
	if err != nil {
		return BookingContext{}, nil, nil, nil, err
	}
	return *snapshot, loc, hours, exceptions, nil
}

func validateTableRecord(table *TableRecord, partySize int) error {
	if table.Status != "ACTIVE" {
		return ErrTableInactive
	}
	if table.AreaStatus != "ACTIVE" {
		return ErrAreaInactive
	}
	if table.Capacity < partySize {
		return ErrCapacity
	}
	return nil
}
