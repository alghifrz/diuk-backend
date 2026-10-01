// Package provider is the LLM adapter layer. Domain packages never import
// provider SDK or HTTP types from here.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotConfigured = errors.New("ai provider is not configured")
	ErrRequest       = errors.New("ai provider request failed")
	ErrResponse      = errors.New("ai provider response is invalid")
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type LLMProvider interface {
	Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error)
}

type GenerateRequest struct {
	Model       string
	Temperature *float64
	MaxTokens   *int
	Messages    []Message
	Tools       []ToolDefinition
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type GenerateResponse struct {
	Content   string
	ToolCalls []ToolCall
	Model     string
}

type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}
