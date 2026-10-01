package usage

import (
	"fmt"
	"strings"
	"time"
)

// MonthlyPeriod returns the half-open calendar month [start, end) that
// contains now in the business timezone. Timestamps are absolute instants;
// callers persist them as TIMESTAMPTZ.
func MonthlyPeriod(now time.Time, tzName string) (start, end time.Time, timezone string, err error) {
	tzName = strings.TrimSpace(tzName)
	if tzName == "" {
		return time.Time{}, time.Time{}, "", invalid("timezone is required")
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.Time{}, time.Time{}, "", invalid("timezone is invalid")
	}
	local := now.In(loc)
	start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	end = start.AddDate(0, 1, 0)
	return start, end, loc.String(), nil
}

func remaining(used int64, limit *int) *int64 {
	if limit == nil {
		return nil
	}
	left := int64(*limit) - used
	if left < 0 {
		left = 0
	}
	return &left
}

func wouldExceed(used int64, increment int, limit *int) bool {
	if limit == nil {
		return false
	}
	return used+int64(increment) > int64(*limit)
}

func parseLimitOffset(limitRaw, offsetRaw string, defaultLimit, maxLimit int) (int, int, error) {
	limit := defaultLimit
	offset := 0
	if strings.TrimSpace(limitRaw) != "" {
		n, err := parseNonNegInt(limitRaw)
		if err != nil || n < 1 {
			return 0, 0, invalid("limit is invalid")
		}
		limit = n
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if strings.TrimSpace(offsetRaw) != "" {
		n, err := parseNonNegInt(offsetRaw)
		if err != nil {
			return 0, 0, invalid("offset is invalid")
		}
		offset = n
	}
	return limit, offset, nil
}

func parseNonNegInt(raw string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, invalid("value is invalid")
	}
	return n, nil
}
