package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/reservation"
)

type stubTool struct {
	name string
}

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return "stub" }
func (s stubTool) InputSchema() Schema {
	return objectSchema(map[string]Schema{}, nil)
}
func (s stubTool) Execute(context.Context, ExecContext, map[string]any) (Result, error) {
	return success(map[string]any{"ok": true})
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(stubTool{name: "alpha"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Register(stubTool{name: "alpha"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := reg.Get("missing"); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("unknown = %v", err)
	}
	got, err := reg.Get("alpha")
	if err != nil || got.Name() != "alpha" {
		t.Fatalf("get = %v err=%v", got, err)
	}
	if len(reg.List()) != 1 {
		t.Fatalf("list = %d", len(reg.List()))
	}
}

func TestValidateArguments(t *testing.T) {
	schema := objectSchema(map[string]Schema{
		"party_size":       intSchema(1, ""),
		"date":             stringSchema("", "date"),
		"start_time":       stringSchema("", "time"),
		"duration_minutes": intSchema(1, ""),
	}, []string{"party_size", "date", "start_time", "duration_minutes"})

	valid := json.RawMessage(`{"party_size":4,"date":"2026-10-01","start_time":"19:00","duration_minutes":90}`)
	if _, err := Validate(schema, valid); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if _, err := Validate(schema, json.RawMessage(`{"date":"2026-10-01","start_time":"19:00","duration_minutes":90}`)); err == nil {
		t.Fatal("missing required")
	}
	if _, err := Validate(schema, json.RawMessage(`{"party_size":"4","date":"2026-10-01","start_time":"19:00","duration_minutes":90}`)); err == nil {
		t.Fatal("invalid type")
	}
	if _, err := Validate(schema, json.RawMessage(`{"party_size":4,"date":"2026-10-01","start_time":"19:00","duration_minutes":90,"extra":true}`)); err == nil {
		t.Fatal("extra properties")
	}
	if _, err := Validate(schema, json.RawMessage(`{"party_size":0,"date":"2026-10-01","start_time":"19:00","duration_minutes":90}`)); err == nil {
		t.Fatal("invalid party size")
	}
	if _, err := Validate(schema, json.RawMessage(`{"party_size":4,"date":"01-10-2026","start_time":"19:00","duration_minutes":90}`)); err == nil {
		t.Fatal("invalid date")
	}
	if _, err := Validate(schema, json.RawMessage(`{"party_size":4,"date":"2026-10-01","start_time":"7pm","duration_minutes":90}`)); err == nil {
		t.Fatal("invalid time")
	}
}

type recordingReservations struct {
	availCalls   int
	createCalls  int
	confirmCalls int
	cancelCalls  int
	lastBusiness uuid.UUID
	lastCreate   reservation.CreateReservationInput
	item         reservation.Reservation
}

func (r *recordingReservations) Availability(_ context.Context, businessID uuid.UUID, date, _, _, _ string) (*reservation.AvailabilityResult, error) {
	r.availCalls++
	r.lastBusiness = businessID
	return &reservation.AvailabilityResult{
		Date:      date,
		Available: true,
		Tables: []reservation.TableSummary{
			{ID: uuid.New(), Name: "Table 7", Capacity: 6, AreaName: "Indoor"},
			{ID: uuid.New(), Name: "Table 1", Capacity: 6, AreaName: "Outdoor"},
		},
	}, nil
}
func (r *recordingReservations) Create(_ context.Context, businessID uuid.UUID, in reservation.CreateReservationInput) (*reservation.Reservation, error) {
	r.createCalls++
	r.lastBusiness = businessID
	r.lastCreate = in
	item := r.item
	item.BusinessID = businessID
	return &item, nil
}
func (r *recordingReservations) Confirm(_ context.Context, businessID, _ uuid.UUID) (*reservation.Reservation, error) {
	r.confirmCalls++
	r.lastBusiness = businessID
	item := r.item
	item.Status = reservation.StatusConfirmed
	return &item, nil
}
func (r *recordingReservations) Cancel(_ context.Context, businessID, _ uuid.UUID, _ *string) (*reservation.Reservation, error) {
	r.cancelCalls++
	r.lastBusiness = businessID
	item := r.item
	item.Status = reservation.StatusCancelled
	return &item, nil
}
func (r *recordingReservations) Get(_ context.Context, _, _ uuid.UUID) (*reservation.Reservation, error) {
	item := r.item
	return &item, nil
}
func (r *recordingReservations) List(context.Context, uuid.UUID, string, string, string, string, string, string, string, string) ([]reservation.Reservation, error) {
	return []reservation.Reservation{r.item}, nil
}

func TestReservationToolsDelegate(t *testing.T) {
	biz := uuid.New()
	cust := uuid.New()
	resID := uuid.New()
	api := &recordingReservations{item: reservation.Reservation{ID: resID, CustomerID: cust, PartySize: 2, Status: reservation.StatusPending}}
	exec := ExecContext{BusinessID: biz, CustomerID: cust}

	if _, err := (CheckAvailability{Reservations: api}).Execute(context.Background(), exec, map[string]any{
		"date": "2026-10-01", "start_time": "19:00", "duration_minutes": 90, "party_size": 4,
	}); err != nil || api.availCalls != 1 || api.lastBusiness != biz {
		t.Fatalf("availability delegate calls=%d err=%v", api.availCalls, err)
	}
	if _, err := (CreateReservation{Reservations: api}).Execute(context.Background(), exec, map[string]any{
		"start_at": "2026-10-01T19:00:00Z", "end_at": "2026-10-01T20:30:00Z", "party_size": 4,
		"items": []any{map[string]any{"menu_item_id": uuid.New().String(), "quantity": 1}},
	}); err != nil || api.createCalls != 1 {
		t.Fatalf("create delegate = %v calls=%d", err, api.createCalls)
	}
	if _, err := (ConfirmReservation{Reservations: api}).Execute(context.Background(), exec, map[string]any{
		"reservation_id": resID.String(),
	}); err != nil || api.confirmCalls != 1 {
		t.Fatalf("confirm delegate = %v", err)
	}
	if _, err := (CancelReservation{Reservations: api}).Execute(context.Background(), exec, map[string]any{
		"reservation_id": resID.String(),
	}); err != nil || api.cancelCalls != 1 {
		t.Fatalf("cancel delegate = %v", err)
	}

	foreign := ExecContext{BusinessID: biz, CustomerID: uuid.New()}
	result, err := (CancelReservation{Reservations: api}).Execute(context.Background(), foreign, map[string]any{
		"reservation_id": resID.String(),
	})
	if err != nil || result.Error == "" {
		t.Fatalf("other customer cancel = %+v err=%v", result, err)
	}
	if api.cancelCalls != 1 {
		t.Fatalf("cancel must not run for another customer, calls=%d", api.cancelCalls)
	}

	injected := uuid.New()
	result, err = (CreateReservation{Reservations: api}).Execute(context.Background(), exec, map[string]any{
		"customer_id": injected.String(), "start_at": "2026-10-01T19:00:00Z", "end_at": "2026-10-01T20:30:00Z", "party_size": 2,
		"items": []any{map[string]any{"menu_item_id": uuid.New().String(), "quantity": 1}},
	})
	if err != nil || result.Error == "" {
		t.Fatalf("injected customer = %+v err=%v", result, err)
	}
	if api.createCalls != 1 {
		t.Fatalf("create must ignore foreign customer_id, calls=%d", api.createCalls)
	}
}
