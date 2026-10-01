package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI is an OpenAI-compatible Chat Completions client. Groq and Ollama
// work through the same adapter when AI_BASE_URL is set.
type OpenAI struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewOpenAI(cfg Config) *OpenAI {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &OpenAI{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: base,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *OpenAI) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if strings.TrimSpace(request.Model) == "" {
		return GenerateResponse{}, fmt.Errorf("%w: model is required", ErrResponse)
	}

	body := chatRequest{
		Model:       request.Model,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		Messages:    encodeMessages(request.Messages),
		Tools:       encodeTools(request.Tools),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("%w: encode request", ErrRequest)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("%w: build request", ErrRequest)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return GenerateResponse{}, ctx.Err()
		}
		return GenerateResponse{}, fmt.Errorf("%w: transport", ErrRequest)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("%w: read response", ErrRequest)
	}
	if resp.StatusCode >= 400 {
		return GenerateResponse{}, fmt.Errorf("%w: http %d", ErrRequest, resp.StatusCode)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return GenerateResponse{}, fmt.Errorf("%w: decode response", ErrResponse)
	}
	if len(parsed.Choices) == 0 {
		return GenerateResponse{}, fmt.Errorf("%w: empty choices", ErrResponse)
	}

	choice := parsed.Choices[0].Message
	out := GenerateResponse{Model: parsed.Model, Content: strings.TrimSpace(choice.Content)}
	for _, call := range choice.ToolCalls {
		args := json.RawMessage(strings.TrimSpace(call.Function.Arguments))
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		if !json.Valid(args) {
			return GenerateResponse{}, fmt.Errorf("%w: tool arguments", ErrResponse)
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: args,
		})
	}
	return out, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func encodeMessages(items []Message) []chatMessage {
	out := make([]chatMessage, 0, len(items))
	for _, item := range items {
		msg := chatMessage{Role: item.Role, Content: item.Content, ToolCallID: item.ToolCallID}
		for _, call := range item.ToolCalls {
			encoded := chatToolCall{ID: call.ID, Type: "function"}
			encoded.Function.Name = call.Name
			encoded.Function.Arguments = string(call.Arguments)
			if encoded.Function.Arguments == "" {
				encoded.Function.Arguments = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, encoded)
		}
		out = append(out, msg)
	}
	return out
}

func encodeTools(items []ToolDefinition) []chatTool {
	if len(items) == 0 {
		return nil
	}
	out := make([]chatTool, 0, len(items))
	for _, item := range items {
		tool := chatTool{Type: "function"}
		tool.Function.Name = item.Name
		tool.Function.Description = item.Description
		tool.Function.Parameters = item.Parameters
		if len(tool.Function.Parameters) == 0 {
			tool.Function.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, tool)
	}
	return out
}
