package analytics

import (
	"strings"
	"time"
)

func parseRange(fromRaw, toRaw, timezone string, now time.Time) (Range, error) {
	loc, err := loadLocation(timezone)
	if err != nil {
		return Range{}, err
	}
	nowLocal := now.In(loc)
	defaultTo := startOfNextDay(nowLocal)
	defaultFrom := startOfDay(nowLocal.AddDate(0, 0, -(DefaultLookbackDays - 1)))

	from := defaultFrom
	if strings.TrimSpace(fromRaw) != "" {
		from, err = parseBound(fromRaw, loc, false)
		if err != nil {
			return Range{}, err
		}
	}
	to := defaultTo
	if strings.TrimSpace(toRaw) != "" {
		to, err = parseBound(toRaw, loc, true)
		if err != nil {
			return Range{}, err
		}
	}
	if !from.Before(to) {
		return Range{}, invalid("from must be before to")
	}
	if to.Sub(from) > time.Duration(MaxRangeDays)*24*time.Hour {
		return Range{}, invalid("date range must be at most 366 days")
	}
	return Range{From: from.UTC(), To: to.UTC(), Timezone: loc.String()}, nil
}

func parseBound(raw string, loc *time.Location, end bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		if end {
			return t.AddDate(0, 0, 1), nil
		}
		return t, nil
	}
	return time.Time{}, invalid("from/to must be an ISO date or RFC3339 timestamp")
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfNextDay(t time.Time) time.Time {
	return startOfDay(t).AddDate(0, 0, 1)
}

func loadLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "UTC"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, invalid("timezone is invalid")
	}
	return loc, nil
}

func rate(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}
