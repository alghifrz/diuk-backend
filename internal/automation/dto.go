package automation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type CreateAutomationInput struct {
	Name          string
	Description   *string
	TriggerType   string
	TriggerConfig json.RawMessage
	Conditions    json.RawMessage
	ActionType    string
	ActionConfig  json.RawMessage
}

type UpdateAutomationInput struct {
	Name          *string
	Description   *string
	TriggerType   *string
	TriggerConfig json.RawMessage
	Conditions    json.RawMessage
	ActionType    *string
	ActionConfig  json.RawMessage
}

type createRequest struct {
	Name          string          `json:"name"`
	Description   *string         `json:"description"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Conditions    json.RawMessage `json:"conditions"`
	ActionType    string          `json:"action_type"`
	ActionConfig  json.RawMessage `json:"action_config"`
}

type updateRequest struct {
	Name          *string         `json:"name"`
	Description   *string         `json:"description"`
	TriggerType   *string         `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Conditions    json.RawMessage `json:"conditions"`
	ActionType    *string         `json:"action_type"`
	ActionConfig  json.RawMessage `json:"action_config"`
}

type listItemJSON struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	TriggerType string  `json:"trigger_type"`
	ActionType  string  `json:"action_type"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type detailJSON struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   *string         `json:"description"`
	Status        string          `json:"status"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Conditions    json.RawMessage `json:"conditions"`
	ActionType    string          `json:"action_type"`
	ActionConfig  json.RawMessage `json:"action_config"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

type runJSON struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	ScheduledAt *string `json:"scheduled_at"`
	StartedAt   *string `json:"started_at"`
	CompletedAt *string `json:"completed_at"`
	FailedAt    *string `json:"failed_at"`
	Attempt     int     `json:"attempt"`
	ErrorCode   *string `json:"error_code"`
	CreatedAt   string  `json:"created_at"`
}

func listItemResponse(a Automation) listItemJSON {
	return listItemJSON{
		ID:          a.ID.String(),
		Name:        a.Name,
		Description: a.Description,
		Status:      a.Status,
		TriggerType: a.TriggerType,
		ActionType:  a.ActionType,
		CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func detailResponse(a *Automation) detailJSON {
	trigger := a.TriggerConfig
	if len(trigger) == 0 {
		trigger = json.RawMessage("{}")
	}
	conditions := a.Conditions
	if len(conditions) == 0 {
		conditions = json.RawMessage("[]")
	}
	action := a.ActionConfig
	if len(action) == 0 {
		action = json.RawMessage("{}")
	}
	return detailJSON{
		ID:            a.ID.String(),
		Name:          a.Name,
		Description:   a.Description,
		Status:        a.Status,
		TriggerType:   a.TriggerType,
		TriggerConfig: trigger,
		Conditions:    conditions,
		ActionType:    a.ActionType,
		ActionConfig:  action,
		CreatedAt:     a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func runResponse(r Run) runJSON {
	return runJSON{
		ID:          r.ID.String(),
		Status:      r.Status,
		ScheduledAt: formatTimePtr(r.ScheduledAt),
		StartedAt:   formatTimePtr(r.StartedAt),
		CompletedAt: formatTimePtr(r.CompletedAt),
		FailedAt:    formatTimePtr(r.FailedAt),
		Attempt:     r.Attempt,
		ErrorCode:   r.ErrorCode,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func listResponse(items []Automation) []listItemJSON {
	out := make([]listItemJSON, 0, len(items))
	for _, item := range items {
		out = append(out, listItemResponse(item))
	}
	return out
}

func runsResponse(items []Run) []runJSON {
	out := make([]runJSON, 0, len(items))
	for _, item := range items {
		out = append(out, runResponse(item))
	}
	return out
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	s := value.UTC().Format(time.RFC3339)
	return &s
}

func actorID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
