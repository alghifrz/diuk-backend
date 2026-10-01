package ai

import (
	"encoding/json"
	"time"
)

type settingsJSON struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Temperature     *float64 `json:"temperature"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
	Enabled         bool     `json:"enabled"`
	UpdatedAt       string   `json:"updated_at"`
}

type updateSettingsRequest struct {
	Enabled         *bool            `json:"enabled"`
	Provider        *string          `json:"provider"`
	Model           *string          `json:"model"`
	Temperature     *json.RawMessage `json:"temperature"`
	MaxOutputTokens *json.RawMessage `json:"max_output_tokens"`
}

type createPromptRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	SystemPrompt string  `json:"system_prompt"`
}

type updatePromptRequest struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	SystemPrompt *string `json:"system_prompt"`
}

type promptJSON struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	SystemPrompt string  `json:"system_prompt"`
	Version      int     `json:"version"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type generateRequest struct {
	ConversationID  string `json:"conversation_id"`
	Message         string `json:"message"`
	PersistIncoming *bool  `json:"persist_incoming"`
}

type generateJSON struct {
	Reply      string `json:"reply"`
	MessageID  string `json:"message_id"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	ToolRounds int    `json:"tool_rounds"`
	ToolCalls  int    `json:"tool_calls"`
}

func settingsResponse(s *Settings) settingsJSON {
	return settingsJSON{
		Provider:        s.Provider,
		Model:           s.Model,
		Temperature:     s.Temperature,
		MaxOutputTokens: s.MaxOutputTokens,
		Enabled:         s.Enabled,
		UpdatedAt:       s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func promptResponse(p *Prompt) promptJSON {
	return promptJSON{
		ID:           p.ID.String(),
		Name:         p.Name,
		Description:  p.Description,
		SystemPrompt: p.SystemPrompt,
		Version:      p.Version,
		Status:       p.Status,
		CreatedAt:    p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func promptsResponse(items []Prompt) []promptJSON {
	out := make([]promptJSON, 0, len(items))
	for i := range items {
		out = append(out, promptResponse(&items[i]))
	}
	return out
}

func generateResponse(r *GenerateReplyResult) generateJSON {
	return generateJSON{
		Reply:      r.Reply,
		MessageID:  r.MessageID.String(),
		Provider:   r.Provider,
		Model:      r.Model,
		ToolRounds: r.ToolRounds,
		ToolCalls:  r.ToolCalls,
	}
}
