package usage

import (
	"testing"
	"time"
)

func TestMonthlyPeriodJakartaSeptember(t *testing.T) {
	now := time.Date(2026, 9, 24, 15, 4, 5, 0, time.UTC)
	start, end, tz, err := MonthlyPeriod(now, "Asia/Jakarta")
	if err != nil {
		t.Fatalf("MonthlyPeriod: %v", err)
	}
	if tz != "Asia/Jakarta" {
		t.Fatalf("tz = %s", tz)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	wantStart := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("period = %s → %s, want %s → %s", start, end, wantStart, wantEnd)
	}
	if start.Format(time.RFC3339) != "2026-09-01T00:00:00+07:00" || end.Format(time.RFC3339) != "2026-10-01T00:00:00+07:00" {
		t.Fatalf("formatted = %s → %s", start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}

func TestMonthlyPeriodSameAndNextMonth(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Makassar")
	early := time.Date(2026, 9, 1, 0, 0, 0, 1, loc)
	late := time.Date(2026, 9, 30, 23, 59, 59, 0, loc)
	next := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	a, b, _, err := MonthlyPeriod(early, "Asia/Makassar")
	if err != nil {
		t.Fatal(err)
	}
	c, d, _, err := MonthlyPeriod(late, "Asia/Makassar")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(c) || !b.Equal(d) {
		t.Fatalf("same month produced different bounds")
	}
	e, f, _, err := MonthlyPeriod(next, "Asia/Makassar")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Equal(b) {
		t.Fatalf("next month start = %s, want previous end %s", e, b)
	}
	if f.Month() != time.November {
		t.Fatalf("next month end month = %s", f.Month())
	}
}

func TestMonthlyPeriodRejectsInvalidTimezone(t *testing.T) {
	if _, _, _, err := MonthlyPeriod(time.Now(), "Jakarta"); err == nil {
		t.Fatal("expected invalid timezone")
	}
	if _, _, _, err := MonthlyPeriod(time.Now(), ""); err == nil {
		t.Fatal("expected empty timezone")
	}
}
