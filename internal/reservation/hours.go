package reservation

import (
	"strconv"
	"time"
)

type clockInterval struct {
	OpenMinutes  int
	CloseMinutes int
}

func minutesOfDay(hour, minute int) int {
	return hour*60 + minute
}

func parseClockInterval(openRaw, closeRaw string) (clockInterval, error) {
	oh, om, err := parseClock(openRaw)
	if err != nil {
		return clockInterval{}, err
	}
	ch, cm, err := parseClock(closeRaw)
	if err != nil {
		return clockInterval{}, err
	}
	open := minutesOfDay(oh, om)
	close := minutesOfDay(ch, cm)
	if open >= close {
		return clockInterval{}, invalid("open_time must be before close_time")
	}
	return clockInterval{OpenMinutes: open, CloseMinutes: close}, nil
}

func localDate(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func sameLocalDate(start, end time.Time, loc *time.Location) bool {
	s := start.In(loc)
	e := end.In(loc)
	return s.Year() == e.Year() && s.YearDay() == e.YearDay()
}

func intervalsForDate(date time.Time, loc *time.Location, hours []Hour, exceptions []ScheduleException) ([]clockInterval, error) {
	local := date.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for _, ex := range exceptions {
		exDate := time.Date(ex.ExceptionDate.Year(), ex.ExceptionDate.Month(), ex.ExceptionDate.Day(), 0, 0, 0, 0, loc)
		if exDate.Equal(day) {
			if ex.IsClosed {
				return nil, nil
			}
			if ex.OpenTime == nil || ex.CloseTime == nil {
				return nil, invalid("schedule exception hours are invalid")
			}
			interval, err := parseClockInterval(*ex.OpenTime, *ex.CloseTime)
			if err != nil {
				return nil, err
			}
			return []clockInterval{interval}, nil
		}
	}

	weekday := int(local.Weekday())
	out := make([]clockInterval, 0)
	for _, h := range hours {
		if h.DayOfWeek != weekday {
			continue
		}
		interval, err := parseClockInterval(h.OpenTime, h.CloseTime)
		if err != nil {
			return nil, err
		}
		out = append(out, interval)
	}
	return out, nil
}

func fitsOneInterval(start, end time.Time, loc *time.Location, intervals []clockInterval) bool {
	if !sameLocalDate(start, end, loc) {
		return false
	}
	s := start.In(loc)
	e := end.In(loc)
	startMin := minutesOfDay(s.Hour(), s.Minute())
	endMin := minutesOfDay(e.Hour(), e.Minute())
	if e.Second() > 0 || e.Nanosecond() > 0 {
		endMin++
	}
	for _, interval := range intervals {
		if startMin >= interval.OpenMinutes && endMin <= interval.CloseMinutes {
			return true
		}
	}
	return false
}

func validateOperatingHours(start, end time.Time, loc *time.Location, hours []Hour, exceptions []ScheduleException) error {
	if !sameLocalDate(start, end, loc) {
		return ErrCrossMidnight
	}
	intervals, err := intervalsForDate(start, loc, hours, exceptions)
	if err != nil {
		return err
	}
	if len(intervals) == 0 || !fitsOneInterval(start, end, loc, intervals) {
		return ErrOutsideHours
	}
	return nil
}

func hoursOverlapSameDay(items []HourInput) error {
	byDay := map[int][]clockInterval{}
	seenSort := map[string]struct{}{}
	for _, item := range items {
		if item.DayOfWeek < 0 || item.DayOfWeek > 6 {
			return invalid("day_of_week must be between 0 and 6")
		}
		if item.SortOrder < 0 {
			return invalid("sort_order must be at least 0")
		}
		sortKey := strconv.Itoa(item.DayOfWeek) + ":" + strconv.Itoa(item.SortOrder)
		if _, ok := seenSort[sortKey]; ok {
			return invalid("sort_order must be unique per day")
		}
		seenSort[sortKey] = struct{}{}
		interval, err := parseClockInterval(item.OpenTime, item.CloseTime)
		if err != nil {
			return err
		}
		for _, existing := range byDay[item.DayOfWeek] {
			if interval.OpenMinutes < existing.CloseMinutes && existing.OpenMinutes < interval.CloseMinutes {
				return invalid("operating hours intervals must not overlap")
			}
		}
		byDay[item.DayOfWeek] = append(byDay[item.DayOfWeek], interval)
	}
	return nil
}
