package ai

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	mu       sync.Mutex
	settings map[uuid.UUID]*Settings
	prompts  map[uuid.UUID]*Prompt
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		settings: map[uuid.UUID]*Settings{},
		prompts:  map[uuid.UUID]*Prompt{},
	}
}

func (m *memoryStore) RunInTx(_ context.Context, fn func(Store) error) error {
	return fn(m)
}

func (m *memoryStore) GetSettings(_ context.Context, businessID uuid.UUID) (*Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.settings[businessID]
	if !ok {
		return nil, nil
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) UpsertSettings(_ context.Context, settings Settings) (*Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	if current, ok := m.settings[settings.BusinessID]; ok {
		settings.ID = current.ID
		settings.CreatedAt = current.CreatedAt
	} else {
		settings.ID = uuid.New()
		settings.CreatedAt = now
	}
	settings.UpdatedAt = now
	stored := settings
	m.settings[settings.BusinessID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) ListPrompts(_ context.Context, businessID uuid.UUID, name, status string) ([]Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Prompt, 0)
	for _, item := range m.prompts {
		if item.BusinessID != businessID {
			continue
		}
		if name != "" && item.Name != name {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		copy := *item
		out = append(out, copy)
	}
	return out, nil
}

func (m *memoryStore) GetPrompt(_ context.Context, businessID, promptID uuid.UUID) (*Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.prompts[promptID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrPromptNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) GetActivePrompt(_ context.Context, businessID uuid.UUID, name string) (*Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.prompts {
		if item.BusinessID == businessID && item.Name == name && item.Status == PromptActive {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) NextPromptVersion(_ context.Context, businessID uuid.UUID, name string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := 0
	for _, item := range m.prompts {
		if item.BusinessID == businessID && item.Name == name && item.Version > max {
			max = item.Version
		}
	}
	return max + 1, nil
}

func (m *memoryStore) CreatePrompt(_ context.Context, prompt Prompt) (*Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.prompts {
		if item.BusinessID == prompt.BusinessID && item.Name == prompt.Name && item.Version == prompt.Version {
			return nil, invalid("prompt version already exists")
		}
	}
	now := time.Now().UTC()
	prompt.ID = uuid.New()
	prompt.CreatedAt = now
	prompt.UpdatedAt = now
	stored := prompt
	m.prompts[prompt.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) UpdatePrompt(_ context.Context, businessID uuid.UUID, prompt Prompt) (*Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.prompts[prompt.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrPromptNotFound
	}
	prompt.CreatedAt = current.CreatedAt
	prompt.UpdatedAt = time.Now().UTC()
	stored := prompt
	m.prompts[prompt.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) ArchiveActive(_ context.Context, businessID uuid.UUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.prompts {
		if item.BusinessID == businessID && item.Name == name && item.Status == PromptActive {
			item.Status = PromptArchived
			item.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
