package ai

import (
	"context"

	"github.com/google/uuid"
)

func (s *Service) ListPrompts(ctx context.Context, businessID uuid.UUID, name, statusRaw string) ([]Prompt, error) {
	var nameFilter string
	if name != "" {
		parsed, err := normalizePromptName(name)
		if err != nil {
			return nil, err
		}
		nameFilter = parsed
	}
	status, err := parsePromptStatus(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListPrompts(ctx, businessID, nameFilter, status)
}

func (s *Service) GetPrompt(ctx context.Context, businessID, promptID uuid.UUID) (*Prompt, error) {
	return s.store.GetPrompt(ctx, businessID, promptID)
}

func (s *Service) CreatePrompt(ctx context.Context, businessID, userID uuid.UUID, in CreatePromptInput) (*Prompt, error) {
	name, err := normalizePromptName(in.Name)
	if err != nil {
		return nil, err
	}
	body, err := normalizePromptBody(in.SystemPrompt)
	if err != nil {
		return nil, err
	}
	version, err := s.store.NextPromptVersion(ctx, businessID, name)
	if err != nil {
		return nil, err
	}
	prompt := Prompt{
		BusinessID:   businessID,
		Name:         name,
		Description:  normalizeOptionalText(in.Description),
		SystemPrompt: body,
		Version:      version,
		Status:       PromptDraft,
	}
	if userID != uuid.Nil {
		prompt.CreatedBy = &userID
	}
	return s.store.CreatePrompt(ctx, prompt)
}

func (s *Service) UpdatePrompt(ctx context.Context, businessID, promptID uuid.UUID, in UpdatePromptInput) (*Prompt, error) {
	current, err := s.store.GetPrompt(ctx, businessID, promptID)
	if err != nil {
		return nil, err
	}
	if current.Status != PromptDraft {
		return nil, ErrPromptNotDraft
	}
	next := *current
	if in.Name != nil {
		name, err := normalizePromptName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		next.Description = normalizeOptionalText(in.Description)
	}
	if in.SystemPrompt != nil {
		body, err := normalizePromptBody(*in.SystemPrompt)
		if err != nil {
			return nil, err
		}
		next.SystemPrompt = body
	}
	return s.store.UpdatePrompt(ctx, businessID, next)
}

func (s *Service) ActivatePrompt(ctx context.Context, businessID, promptID uuid.UUID) (*Prompt, error) {
	var activated *Prompt
	err := s.store.RunInTx(ctx, func(store Store) error {
		current, err := store.GetPrompt(ctx, businessID, promptID)
		if err != nil {
			return err
		}
		if err := store.ArchiveActive(ctx, businessID, current.Name); err != nil {
			return err
		}
		current.Status = PromptActive
		activated, err = store.UpdatePrompt(ctx, businessID, *current)
		return err
	})
	return activated, err
}

func (s *Service) DeletePrompt(ctx context.Context, businessID, promptID uuid.UUID) (*Prompt, error) {
	current, err := s.store.GetPrompt(ctx, businessID, promptID)
	if err != nil {
		return nil, err
	}
	current.Status = PromptArchived
	return s.store.UpdatePrompt(ctx, businessID, *current)
}

func (s *Service) activePromptBody(ctx context.Context, businessID uuid.UUID) (string, error) {
	prompt, err := s.store.GetActivePrompt(ctx, businessID, DefaultPromptName)
	if err != nil {
		return "", err
	}
	if prompt != nil {
		return prompt.SystemPrompt, nil
	}
	items, err := s.store.ListPrompts(ctx, businessID, "", PromptActive)
	if err != nil {
		return "", err
	}
	if len(items) > 0 {
		return items[0].SystemPrompt, nil
	}
	return DefaultSystemPrompt, nil
}
