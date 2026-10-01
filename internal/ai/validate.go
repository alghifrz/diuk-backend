package ai

import (
	"strings"
	"unicode"
)

func parseProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ProviderOpenAI, ProviderGroq, ProviderAnthropic, ProviderOllama:
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", invalid("provider is invalid")
	}
}

func parsePromptStatus(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case PromptDraft, PromptActive, PromptArchived:
		return strings.ToUpper(strings.TrimSpace(value)), nil
	default:
		return "", invalid("status is invalid")
	}
}

func normalizePromptName(value string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" {
		return "", invalid("name is required")
	}
	if len(name) > 100 {
		return "", invalid("name must be at most 100 characters")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return "", invalid("name may contain letters, numbers, hyphen, and underscore")
	}
	return name, nil
}

func normalizePromptBody(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", invalid("system_prompt is required")
	}
	if len([]rune(trimmed)) > 20000 {
		return "", invalid("system_prompt is too long")
	}
	return trimmed, nil
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateTemperature(v float64) error {
	if v < 0 || v > 2 {
		return invalid("temperature must be between 0 and 2")
	}
	return nil
}

func validateMaxTokens(v int) error {
	if v < 1 || v > 8192 {
		return invalid("max_output_tokens must be between 1 and 8192")
	}
	return nil
}

func validateModel(value string) (string, error) {
	model := strings.TrimSpace(value)
	if model == "" {
		return "", invalid("model is required")
	}
	if len(model) > 100 {
		return "", invalid("model must be at most 100 characters")
	}
	return model, nil
}
