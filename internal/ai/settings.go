package ai

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *Service) GetSettings(ctx context.Context, businessID uuid.UUID) (*Settings, error) {
	current, err := s.store.GetSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		return current, nil
	}
	return s.store.UpsertSettings(ctx, s.defaultSettings(businessID))
}

func (s *Service) UpdateSettings(ctx context.Context, businessID uuid.UUID, in UpdateSettingsInput) (*Settings, error) {
	current, err := s.GetSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if in.Provider != nil {
		provider, err := parseProvider(*in.Provider)
		if err != nil {
			return nil, err
		}
		next.Provider = provider
	}
	if in.Model != nil {
		model, err := validateModel(*in.Model)
		if err != nil {
			return nil, err
		}
		next.Model = model
	}
	if in.ClearTemp {
		next.Temperature = nil
	} else if in.Temperature != nil {
		if err := validateTemperature(*in.Temperature); err != nil {
			return nil, err
		}
		next.Temperature = in.Temperature
	}
	if in.ClearMaxTokens {
		next.MaxOutputTokens = nil
	} else if in.MaxOutputTokens != nil {
		if err := validateMaxTokens(*in.MaxOutputTokens); err != nil {
			return nil, err
		}
		next.MaxOutputTokens = in.MaxOutputTokens
	}
	return s.store.UpsertSettings(ctx, next)
}

func (s *Service) defaultSettings(businessID uuid.UUID) Settings {
	provider := s.defaults.Provider
	if _, err := parseProvider(provider); err != nil {
		provider = ProviderOpenAI
	}
	model := s.defaults.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	return Settings{
		BusinessID: businessID,
		Provider:   provider,
		Model:      model,
		Enabled:    true,
	}
}

func parseOptionalFloatPatch(raw *json.RawMessage) (*float64, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if string(*raw) == "null" {
		return nil, true, nil
	}
	var value float64
	if err := json.Unmarshal(*raw, &value); err != nil {
		return nil, false, invalid("temperature is invalid")
	}
	return &value, false, nil
}

func parseOptionalIntPatch(raw *json.RawMessage) (*int, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if string(*raw) == "null" {
		return nil, true, nil
	}
	var value int
	if err := json.Unmarshal(*raw, &value); err != nil {
		return nil, false, invalid("max_output_tokens is invalid")
	}
	return &value, false, nil
}
