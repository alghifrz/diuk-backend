package automation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCreateRejectsUnknownTypesAndMalformedConfig(t *testing.T) {
	_, err := normalizeCreate(CreateAutomationInput{Name: " ", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty name = %v", err)
	}
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: "NOPE", ActionType: ActionSendMessage,
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown trigger = %v", err)
	}
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: TriggerCustomerCreated, ActionType: "EXPLODE",
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown action = %v", err)
	}
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: TriggerScheduled, ActionType: ActionSendMessage,
		TriggerConfig: json.RawMessage(`{"offset_minutes":-1440,"reference":"customers.id"}`),
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported reference = %v", err)
	}
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		Conditions:   json.RawMessage(`[{"field":"customers.secret","operator":"EQ","value":"1"}]`),
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported field = %v", err)
	}
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		Conditions:   json.RawMessage(`[{"field":"customer.status","operator":"LIKE","value":"A"}]`),
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported operator = %v", err)
	}
	huge := `{"template":"` + strings.Repeat("a", MaxConfigBytes) + `"}`
	_, err = normalizeCreate(CreateAutomationInput{
		Name: "x", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		ActionConfig: json.RawMessage(huge),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized config = %v", err)
	}
}

func TestNormalizeCreateAcceptsValidDefinitions(t *testing.T) {
	in, err := normalizeCreate(CreateAutomationInput{
		Name:          "Reservation Reminder",
		TriggerType:   TriggerScheduled,
		TriggerConfig: json.RawMessage(`{"offset_minutes":-1440,"reference":"reservation.start_at"}`),
		Conditions:    json.RawMessage(`[{"field":"reservation.status","operator":"IN","value":["CONFIRMED"]}]`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if in.TriggerType != TriggerScheduled || in.ActionType != ActionSendMessage {
		t.Fatalf("normalized = %+v", in)
	}
}

func TestMatchesTrigger(t *testing.T) {
	if !MatchesTrigger(TriggerReservationConfirmed, EventReservationConfirmed) {
		t.Fatal("confirmed")
	}
	if MatchesTrigger(TriggerScheduled, EventReservationConfirmed) {
		t.Fatal("scheduled must not match events")
	}
	if MatchesTrigger(TriggerConversationInactive, EventConversationCreated) {
		t.Fatal("inactive is scheduler-only")
	}
}
