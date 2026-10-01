package automation

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	mu            sync.Mutex
	automations   map[uuid.UUID]Automation
	events        map[uuid.UUID]Event
	eventKeys     map[string]uuid.UUID
	runs          map[uuid.UUID]Run
	eventRuns     map[string]uuid.UUID
	keyRuns       map[string]uuid.UUID
	reservations  []ReservationCandidate
	conversations []ConversationCandidate
	lastInbound   map[uuid.UUID]time.Time
	lockHeld      bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		automations: map[uuid.UUID]Automation{},
		events:      map[uuid.UUID]Event{},
		eventKeys:   map[string]uuid.UUID{},
		runs:        map[uuid.UUID]Run{},
		eventRuns:   map[string]uuid.UUID{},
		keyRuns:     map[string]uuid.UUID{},
		lastInbound: map[uuid.UUID]time.Time{},
	}
}

func (m *memoryStore) CreateAutomation(_ context.Context, item Automation) (*Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	m.automations[item.ID] = item
	out := item
	return &out, nil
}

func (m *memoryStore) GetAutomation(_ context.Context, businessID, automationID uuid.UUID) (*Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.automations[automationID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrNotFound
	}
	out := item
	return &out, nil
}

func (m *memoryStore) ListAutomations(_ context.Context, businessID uuid.UUID, status string) ([]Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Automation{}
	for _, item := range m.automations {
		if item.BusinessID != businessID {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *memoryStore) ListActiveByTrigger(_ context.Context, businessID uuid.UUID, triggerType string) ([]Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Automation{}
	for _, item := range m.automations {
		if item.BusinessID == businessID && item.Status == StatusActive && item.TriggerType == triggerType {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *memoryStore) ListActiveScheduled(_ context.Context) ([]Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Automation{}
	for _, item := range m.automations {
		if item.Status == StatusActive && (item.TriggerType == TriggerScheduled || item.TriggerType == TriggerConversationInactive) {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *memoryStore) UpdateAutomation(_ context.Context, item Automation) (*Automation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.automations[item.ID]
	if !ok || current.BusinessID != item.BusinessID {
		return nil, ErrNotFound
	}
	item.CreatedAt = current.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	m.automations[item.ID] = item
	out := item
	return &out, nil
}

func eventKey(businessID uuid.UUID, eventType, key string) string {
	return businessID.String() + "|" + eventType + "|" + key
}

func (m *memoryStore) InsertEvent(_ context.Context, event Event) (*Event, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := eventKey(event.BusinessID, event.Type, event.IdempotencyKey)
	if id, ok := m.eventKeys[key]; ok {
		existing := m.events[id]
		return &existing, false, nil
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	m.events[event.ID] = event
	m.eventKeys[key] = event.ID
	out := event
	return &out, true, nil
}

func (m *memoryStore) GetEvent(_ context.Context, businessID, eventID uuid.UUID) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.events[eventID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrEventNotFound
	}
	out := item
	return &out, nil
}

func (m *memoryStore) InsertRun(_ context.Context, run Run) (*Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run.EventID != nil {
		key := run.AutomationID.String() + "|" + run.EventID.String()
		if id, ok := m.eventRuns[key]; ok {
			existing := m.runs[id]
			return &existing, false, nil
		}
	}
	if run.ExecutionKey != nil {
		key := run.AutomationID.String() + "|" + *run.ExecutionKey
		if id, ok := m.keyRuns[key]; ok {
			existing := m.runs[id]
			return &existing, false, nil
		}
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	m.runs[run.ID] = run
	if run.EventID != nil {
		m.eventRuns[run.AutomationID.String()+"|"+run.EventID.String()] = run.ID
	}
	if run.ExecutionKey != nil {
		m.keyRuns[run.AutomationID.String()+"|"+*run.ExecutionKey] = run.ID
	}
	out := run
	return &out, true, nil
}

func (m *memoryStore) GetRun(_ context.Context, businessID, runID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.runs[runID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrRunNotFound
	}
	out := item
	return &out, nil
}

func (m *memoryStore) ListRuns(_ context.Context, businessID, automationID uuid.UUID, _ int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Run{}
	for _, item := range m.runs {
		if item.BusinessID == businessID && item.AutomationID == automationID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *memoryStore) ClaimRun(_ context.Context, businessID, runID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.runs[runID]
	if !ok || item.BusinessID != businessID || item.Status != RunPending {
		return nil, ErrRunNotClaimed
	}
	now := time.Now().UTC()
	item.Status = RunRunning
	item.StartedAt = &now
	item.Attempt++
	m.runs[runID] = item
	out := item
	return &out, nil
}

func (m *memoryStore) MarkRunStatus(_ context.Context, businessID, runID uuid.UUID, status string, errorCode, errorMessage *string) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.runs[runID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrRunNotFound
	}
	now := time.Now().UTC()
	item.Status = status
	item.ErrorCode = errorCode
	item.ErrorMessage = errorMessage
	if status == RunCompleted {
		item.CompletedAt = &now
	}
	if status == RunFailed {
		item.FailedAt = &now
	}
	m.runs[runID] = item
	out := item
	return &out, nil
}

func (m *memoryStore) CancelPendingRuns(_ context.Context, businessID, automationID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, item := range m.runs {
		if item.BusinessID == businessID && item.AutomationID == automationID && item.Status == RunPending {
			item.Status = RunCancelled
			m.runs[id] = item
		}
	}
	return nil
}

func (m *memoryStore) HasRunForKey(_ context.Context, automationID uuid.UUID, executionKey string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.keyRuns[automationID.String()+"|"+executionKey]
	return ok, nil
}

func (m *memoryStore) ListDueReservations(_ context.Context, businessID uuid.UUID, dueBefore, lookback time.Time, limit int) ([]ReservationCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []ReservationCandidate{}
	for _, item := range m.reservations {
		if item.BusinessID != businessID || item.Status != "CONFIRMED" {
			continue
		}
		if item.StartAt.After(dueBefore) || !item.StartAt.After(lookback) {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryStore) ListInactiveConversations(_ context.Context, businessID uuid.UUID, inactiveSince time.Time, limit int) ([]ConversationCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []ConversationCandidate{}
	for _, item := range m.conversations {
		if item.BusinessID != businessID || item.Status != "OPEN" {
			continue
		}
		if item.LastInboundAt.After(inactiveSince) {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryStore) LastInboundCustomerAt(_ context.Context, _, conversationID uuid.UUID) (*time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.lastInbound[conversationID]
	if !ok {
		return nil, nil
	}
	return &at, nil
}

func (m *memoryStore) TrySchedulerLock(_ context.Context) (bool, func(), error) {
	m.mu.Lock()
	if m.lockHeld {
		m.mu.Unlock()
		return false, func() {}, nil
	}
	m.lockHeld = true
	m.mu.Unlock()
	return true, func() {
		m.mu.Lock()
		m.lockHeld = false
		m.mu.Unlock()
	}, nil
}
