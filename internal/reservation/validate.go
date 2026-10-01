package reservation

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultListLimit = 50
	maxListLimit     = 100
	maxNotesLen      = 2000
)

func parseStatus(value string) (Status, error) {
	switch Status(strings.ToUpper(strings.TrimSpace(value))) {
	case StatusPending, StatusConfirmed, StatusCancelled, StatusCompleted:
		return Status(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", invalid("status is invalid")
	}
}

func parseStatusFilter(raw string) (Status, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return parseStatus(raw)
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, invalid("id is invalid")
	}
	return &id, nil
}

func parseRequiredUUID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid(field + " is invalid")
	}
	return id, nil
}

func parseRFC3339(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ErrInvalidTimeRange
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, invalid("timestamp is invalid")
	}
	return t, nil
}

func parseDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, invalid("date is invalid")
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, invalid("date is invalid")
	}
	return t, nil
}

func parseClock(raw string) (int, int, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{"15:04", "15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Hour(), t.Minute(), nil
		}
	}
	return 0, 0, invalid("time is invalid")
}

func formatClock(hour, minute int) string {
	return time.Date(0, 1, 1, hour, minute, 0, 0, time.UTC).Format("15:04")
}

func clockFromTime(t time.Time) string {
	return t.Format("15:04")
}

func parseLimitOffset(limitRaw, offsetRaw string) (int, int, error) {
	limit := defaultListLimit
	if strings.TrimSpace(limitRaw) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(limitRaw))
		if err != nil || n < 1 {
			return 0, 0, invalid("limit is invalid")
		}
		if n > maxListLimit {
			n = maxListLimit
		}
		limit = n
	}
	offset := 0
	if strings.TrimSpace(offsetRaw) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(offsetRaw))
		if err != nil || n < 0 {
			return 0, 0, invalid("offset is invalid")
		}
		offset = n
	}
	return limit, offset, nil
}

func parsePositiveInt(raw, field string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 0, invalid(field + " is invalid")
	}
	return n, nil
}

func normalizeNotes(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > maxNotesLen {
		return nil, invalid("notes must be at most 2000 characters")
	}
	return &trimmed, nil
}

func validateTimeRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return ErrInvalidTimeRange
	}
	return nil
}

func validatePartySize(size int, settings BookingContext) error {
	if size < settings.ReservationMinPartySize || size > settings.ReservationMaxPartySize {
		return ErrPartySize
	}
	return nil
}

func validateAdvance(start, now time.Time, settings BookingContext) error {
	minStart := now.Add(time.Duration(settings.ReservationMinAdvanceMinutes) * time.Minute)
	maxStart := now.AddDate(0, 0, settings.ReservationMaxAdvanceDays)
	if start.Before(minStart) || start.After(maxStart) {
		return ErrOutsideAdvance
	}
	return nil
}

func loadLocation(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" {
		return nil, invalid("timezone is invalid")
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, invalid("timezone is invalid")
	}
	return loc, nil
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}
