package analytics

import (
	"math"
	"testing"
	"time"
)

func TestParseRangeDefaultAndHalfOpen(t *testing.T) {
	now := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	r, err := parseRange("", "", "Asia/Jakarta", now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Timezone != "Asia/Jakarta" {
		t.Fatalf("tz = %s", r.Timezone)
	}
	if !r.To.After(r.From) {
		t.Fatal("to must be after from")
	}

	r, err = parseRange("2026-09-01", "2026-09-02", "Asia/Jakarta", now)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	wantFrom := time.Date(2026, 9, 1, 0, 0, 0, 0, loc).UTC()
	wantTo := time.Date(2026, 9, 3, 0, 0, 0, 0, loc).UTC()
	if !r.From.Equal(wantFrom) || !r.To.Equal(wantTo) {
		t.Fatalf("range = %s %s want %s %s", r.From, r.To, wantFrom, wantTo)
	}

	if _, err := parseRange("2026-09-02", "2026-09-01", "UTC", now); err == nil {
		t.Fatal("expected inverted range error")
	}
}

func TestRateSafe(t *testing.T) {
	if rate(3, 0) != 0 || math.IsNaN(rate(3, 0)) || math.IsInf(rate(3, 0), 0) {
		t.Fatal("zero denominator must be 0")
	}
	if rate(1, 4) != 0.25 {
		t.Fatalf("rate = %v", rate(1, 4))
	}
}
