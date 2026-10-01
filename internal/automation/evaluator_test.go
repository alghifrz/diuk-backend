package automation

import (
	"testing"
	"time"
)

func TestEvaluateConditionsOperators(t *testing.T) {
	ctx := map[string]any{
		"reservation": map[string]any{
			"status":     "CONFIRMED",
			"party_size": 4,
			"start_at":   time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		"customer":     map[string]any{"status": "ACTIVE"},
		"conversation": map[string]any{"status": "OPEN", "channel": "WHATSAPP"},
	}

	cases := []struct {
		raw  string
		want bool
	}{
		{`[{"field":"reservation.status","operator":"EQ","value":"CONFIRMED"}]`, true},
		{`[{"field":"reservation.status","operator":"NEQ","value":"CANCELLED"}]`, true},
		{`[{"field":"reservation.status","operator":"IN","value":["CONFIRMED","PENDING"]}]`, true},
		{`[{"field":"reservation.status","operator":"NOT_IN","value":["CANCELLED"]}]`, true},
		{`[{"field":"reservation.party_size","operator":"GT","value":3}]`, true},
		{`[{"field":"reservation.party_size","operator":"GTE","value":4}]`, true},
		{`[{"field":"reservation.party_size","operator":"LT","value":5}]`, true},
		{`[{"field":"reservation.party_size","operator":"LTE","value":4}]`, true},
		{`[{"field":"reservation.status","operator":"EXISTS"}]`, true},
		{`[{"field":"reservation.notes","operator":"NOT_EXISTS"}]`, true},
		{`[{"field":"reservation.status","operator":"EQ","value":"PENDING"}]`, false},
		{`[{"field":"reservation.party_size","operator":"GT","value":"big"}]`, false},
		{`[{"field":"reservation.missing","operator":"EQ","value":"x"}]`, false},
	}
	for _, tt := range cases {
		got, err := EvaluateConditions([]byte(tt.raw), ctx)
		if err != nil {
			t.Fatalf("%s err=%v", tt.raw, err)
		}
		if got != tt.want {
			t.Errorf("%s = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestEvaluateConditionsInvalidTypes(t *testing.T) {
	ok, err := EvaluateConditions([]byte(`not-json`), map[string]any{})
	if err == nil || ok {
		t.Fatalf("invalid json = %v %v", ok, err)
	}
}
