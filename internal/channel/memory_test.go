package channel

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	mu    sync.Mutex
	items map[uuid.UUID]*BusinessChannel
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: map[uuid.UUID]*BusinessChannel{}}
}

func (m *memoryStore) List(_ context.Context, businessID uuid.UUID) ([]BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BusinessChannel, 0)
	for _, item := range m.items {
		if item.BusinessID == businessID {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (m *memoryStore) Get(_ context.Context, businessID, channelID uuid.UUID) (*BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[channelID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrChannelNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) FindByPhoneNumberID(_ context.Context, channel, phoneNumberID string) (*BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var fallback *BusinessChannel
	for _, item := range m.items {
		if item.Channel != channel || item.ExternalPhoneNumberID == nil || *item.ExternalPhoneNumberID != phoneNumberID {
			continue
		}
		copy := *item
		if item.Status == StatusActive {
			return &copy, nil
		}
		fallback = &copy
	}
	return fallback, nil
}

func (m *memoryStore) FindActive(_ context.Context, businessID uuid.UUID, channel string) (*BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.BusinessID == businessID && item.Channel == channel && item.Status == StatusActive {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) Create(_ context.Context, item BusinessChannel) (*BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.items {
		if existing.BusinessID == item.BusinessID && existing.Channel == item.Channel && sameOpt(existing.ExternalAccountID, item.ExternalAccountID) {
			return nil, invalid("channel configuration already exists")
		}
		if item.Status == StatusActive && item.ExternalPhoneNumberID != nil && existing.Status == StatusActive &&
			existing.Channel == item.Channel && sameOpt(existing.ExternalPhoneNumberID, item.ExternalPhoneNumberID) {
			return nil, invalid("channel configuration already exists")
		}
	}
	now := time.Now().UTC()
	item.ID = uuid.New()
	item.CreatedAt = now
	item.UpdatedAt = now
	stored := item
	m.items[item.ID] = &stored
	return &item, nil
}

func (m *memoryStore) Update(_ context.Context, businessID uuid.UUID, item BusinessChannel) (*BusinessChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[item.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrChannelNotFound
	}
	item.CreatedAt = current.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	stored := item
	m.items[item.ID] = &stored
	return &item, nil
}

func sameOpt(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptr[T any](v T) *T { return &v }
