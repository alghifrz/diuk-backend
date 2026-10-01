package campaign

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestParseAudienceUnknownAndValid(t *testing.T) {
	if _, err := parseAudience(json.RawMessage(`{"sql":"1=1"}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown field = %v", err)
	}
	if _, err := parseAudience(json.RawMessage(`{"customer_status":"NOPE"}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad status = %v", err)
	}
	got, err := parseAudience(json.RawMessage(`{
		"customer_status":"active",
		"channels":["whatsapp"],
		"tags":["regular"],
		"created_after":"2026-01-01",
		"last_reservation_status":"completed"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerStatus == nil || *got.CustomerStatus != "ACTIVE" {
		t.Fatalf("status = %+v", got.CustomerStatus)
	}
	if len(got.Channels) != 1 || got.Channels[0] != ChannelWhatsApp {
		t.Fatalf("channels = %+v", got.Channels)
	}
	if got.LastReservationStatus == nil || *got.LastReservationStatus != "COMPLETED" {
		t.Fatalf("reservation = %+v", got.LastReservationStatus)
	}
}

func TestParseScheduledAtPast(t *testing.T) {
	now, err := time.Parse(time.RFC3339, "2026-09-24T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseScheduledAt(ptr("2026-09-24T09:50:00Z"), now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past = %v", err)
	}
	got, err := parseScheduledAt(ptr("2026-09-24T10:00:30Z"), now)
	if err != nil || got == nil {
		t.Fatalf("near future = %v", err)
	}
}
