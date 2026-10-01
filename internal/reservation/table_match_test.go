package reservation

import "testing"

func TestMatchPreferredTableIndoorMeja(t *testing.T) {
	tables := []TableSummary{
		{Name: "Meja 1", AreaName: "Outdoor"},
		{Name: "Meja 4", AreaName: "Indoor"},
		{Name: "Meja 2", AreaName: "Indoor"},
	}
	got := MatchPreferredTable("Indoor, Meja 4", tables)
	if got == nil || got.Name != "Meja 4" || got.AreaName != "Indoor" {
		t.Fatalf("got = %+v", got)
	}
}

func TestMatchPreferredTableNumberFallback(t *testing.T) {
	tables := []TableSummary{
		{Name: "Table 8", AreaName: "Outdoor"},
		{Name: "Table 4", AreaName: "Indoor"},
	}
	got := MatchPreferredTable("indoor meja 4", tables)
	if got == nil || got.Name != "Table 4" {
		t.Fatalf("got = %+v", got)
	}
}

func TestMatchPreferredTableRequiresConfidence(t *testing.T) {
	tables := []TableSummary{
		{Name: "Meja 1", AreaName: "Indoor"},
		{Name: "Meja 2", AreaName: "Outdoor"},
	}
	if got := MatchPreferredTable("window seat please", tables); got != nil {
		t.Fatalf("unexpected match = %+v", got)
	}
}
