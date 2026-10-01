package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrUnknownTool = errors.New("unknown tool")
	ErrDuplicate   = errors.New("tool already registered")
	ErrDisabled    = errors.New("tool is disabled")
	ErrArguments   = errors.New("tool arguments are invalid")
)

type ExecContext struct {
	BusinessID     uuid.UUID
	UserID         uuid.UUID
	Role           string
	ConversationID uuid.UUID
	CustomerID     uuid.UUID
	// TurnID identifies one GenerateReply call (one customer message). Booking
	// SOP steps must happen in different turns so the customer actually answered.
	TurnID uuid.UUID
}

type Result struct {
	Data  any
	Error string
	Code  string
}

func (r Result) MarshalJSON() ([]byte, error) {
	if r.Error != "" {
		return json.Marshal(map[string]string{"error": r.Error, "code": r.Code})
	}
	return json.Marshal(r.Data)
}

type Tool interface {
	Name() string
	Description() string
	InputSchema() Schema
	Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error)
}

func stringArg(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func optionalString(input map[string]any, key string) *string {
	value, ok := input[key].(string)
	if !ok {
		return nil
	}
	if value == "" {
		return nil
	}
	return &value
}

func intArg(input map[string]any, key string) int {
	n, _ := asInt(input[key])
	return n
}

func success(data any) (Result, error) {
	return Result{Data: data}, nil
}

func fail(code, message string) (Result, error) {
	return Result{Error: message, Code: code}, nil
}
