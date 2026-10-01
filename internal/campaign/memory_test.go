package campaign

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryCustomer struct {
	ID                    uuid.UUID
	BusinessID            uuid.UUID
	Name                  string
	Status                string
	MarketingOptIn        bool
	Tags                  []string
	Channels              []string
	ExternalID            string
	IdentityID            uuid.UUID
	CreatedAt             time.Time
	LastConversationAt    *time.Time
	LastReservationStatus *string
	LastReservationAt     *time.Time
}

type memoryStore struct {
	mu         sync.Mutex
	campaigns  map[uuid.UUID]*Campaign
	runs       map[uuid.UUID]*Run
	recipients map[uuid.UUID]*Recipient
	customers  []memoryCustomer
	messages   map[string]uuid.UUID
	lockHeld   bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		campaigns:  map[uuid.UUID]*Campaign{},
		runs:       map[uuid.UUID]*Run{},
		recipients: map[uuid.UUID]*Recipient{},
		messages:   map[string]uuid.UUID{},
	}
}

func (m *memoryStore) ListCampaigns(_ context.Context, businessID uuid.UUID, status string, limit, offset int) ([]Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Campaign{}
	for _, item := range m.campaigns {
		if item.BusinessID != businessID {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID.String() > out[j].ID.String()
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return page(out, limit, offset), nil
}

func (m *memoryStore) GetCampaign(_ context.Context, businessID, campaignID uuid.UUID) (*Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.campaigns[campaignID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) CreateCampaign(_ context.Context, c Campaign) (*Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	c.ID = uuid.New()
	c.CreatedAt = now
	c.UpdatedAt = now
	copy := c
	m.campaigns[c.ID] = &copy
	return &c, nil
}

func (m *memoryStore) UpdateCampaign(_ context.Context, c Campaign) (*Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.campaigns[c.ID]
	if !ok || current.BusinessID != c.BusinessID {
		return nil, ErrNotFound
	}
	c.CreatedAt = current.CreatedAt
	c.UpdatedAt = time.Now().UTC()
	copy := c
	m.campaigns[c.ID] = &copy
	return &c, nil
}

func (m *memoryStore) CreateRun(_ context.Context, run Run) (*Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.runs {
		if existing.BusinessID == run.BusinessID && existing.CampaignID == run.CampaignID {
			switch existing.Status {
			case RunPending, RunRunning, RunPaused:
				copy := *existing
				return &copy, false, nil
			}
		}
	}
	now := time.Now().UTC()
	run.ID = uuid.New()
	run.CreatedAt = now
	run.UpdatedAt = now
	copy := run
	m.runs[run.ID] = &copy
	return &run, true, nil
}

func (m *memoryStore) GetRun(_ context.Context, businessID, campaignID, runID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.runs[runID]
	if !ok || item.BusinessID != businessID || item.CampaignID != campaignID {
		return nil, ErrRunNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) GetRunByID(_ context.Context, businessID, runID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.runs[runID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrRunNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) GetActiveRun(_ context.Context, businessID, campaignID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *Run
	for _, item := range m.runs {
		if item.BusinessID != businessID || item.CampaignID != campaignID {
			continue
		}
		switch item.Status {
		case RunPending, RunRunning, RunPaused:
			if found == nil || item.CreatedAt.After(found.CreatedAt) {
				copy := *item
				found = &copy
			}
		}
	}
	if found == nil {
		return nil, ErrRunNotFound
	}
	return found, nil
}

func (m *memoryStore) ListRuns(_ context.Context, businessID, campaignID uuid.UUID, limit, offset int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Run{}
	for _, item := range m.runs {
		if item.BusinessID == businessID && item.CampaignID == campaignID {
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, limit, offset), nil
}

func (m *memoryStore) UpdateRun(_ context.Context, run Run) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runs[run.ID]
	if !ok || current.BusinessID != run.BusinessID || current.CampaignID != run.CampaignID {
		return nil, ErrRunNotFound
	}
	run.CreatedAt = current.CreatedAt
	run.UpdatedAt = time.Now().UTC()
	copy := run
	m.runs[run.ID] = &copy
	return &run, nil
}

func (m *memoryStore) RefreshRunCounts(_ context.Context, businessID, runID uuid.UUID) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.BusinessID != businessID {
		return nil, ErrRunNotFound
	}
	run.TotalRecipients = 0
	run.QueuedCount, run.SentCount, run.DeliveredCount, run.ReadCount, run.FailedCount, run.SkippedCount = 0, 0, 0, 0, 0, 0
	for _, rec := range m.recipients {
		if rec.BusinessID != businessID || rec.CampaignRunID != runID {
			continue
		}
		run.TotalRecipients++
		switch rec.Status {
		case RecipientQueued:
			run.QueuedCount++
		case RecipientSent:
			run.SentCount++
		case RecipientDelivered:
			run.DeliveredCount++
		case RecipientRead:
			run.ReadCount++
		case RecipientFailed:
			run.FailedCount++
		case RecipientSkipped:
			run.SkippedCount++
		}
	}
	copy := *run
	return &copy, nil
}

func (m *memoryStore) SnapshotRecipients(_ context.Context, businessID, runID uuid.UUID, channel string, def AudienceDefinition) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inserted := 0
	for _, cust := range m.customers {
		if cust.BusinessID != businessID || !matchAudience(cust, def) {
			continue
		}
		exists := false
		for _, rec := range m.recipients {
			if rec.BusinessID == businessID && rec.CampaignRunID == runID && rec.CustomerID == cust.ID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		now := time.Now().UTC()
		rec := Recipient{
			ID: uuid.New(), BusinessID: businessID, CampaignRunID: runID, CustomerID: cust.ID,
			Channel: channel, Status: RecipientPending, CreatedAt: now, UpdatedAt: now,
		}
		if cust.ExternalID != "" {
			ext := cust.ExternalID
			rec.ExternalRecipientID = &ext
		}
		if !cust.MarketingOptIn {
			rec.Status = RecipientSkipped
			rec.SkippedAt = &now
			code := SkipMarketingOptOut
			reason := "customer opted out of marketing"
			rec.FailureCode, rec.FailureReason = &code, &reason
		} else if cust.ExternalID == "" {
			rec.Status = RecipientSkipped
			rec.SkippedAt = &now
			code := SkipNoIdentity
			reason := "no active channel identity"
			rec.FailureCode, rec.FailureReason = &code, &reason
		}
		copy := rec
		m.recipients[rec.ID] = &copy
		inserted++
	}
	return inserted, nil
}

func (m *memoryStore) PreviewAudience(_ context.Context, businessID uuid.UUID, _ string, def AudienceDefinition) (AudiencePreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sample := []AudienceSample{}
	count := 0
	for _, cust := range m.customers {
		if cust.BusinessID != businessID || !matchAudience(cust, def) {
			continue
		}
		count++
		if len(sample) < previewSample {
			sample = append(sample, AudienceSample{ID: cust.ID, Name: cust.Name, Status: cust.Status})
		}
	}
	return AudiencePreview{EstimatedCount: count, Sample: sample}, nil
}

func (m *memoryStore) CustomerSendable(_ context.Context, businessID, customerID uuid.UUID, _ string) (bool, *uuid.UUID, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cust := range m.customers {
		if cust.BusinessID == businessID && cust.ID == customerID {
			var id *uuid.UUID
			if cust.IdentityID != uuid.Nil {
				id = &cust.IdentityID
			}
			return cust.MarketingOptIn, id, cust.ExternalID, nil
		}
	}
	return false, nil, "", ErrNotFound
}

func (m *memoryStore) ListRecipients(_ context.Context, businessID, runID uuid.UUID, status string, limit, offset int) ([]Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Recipient{}
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.CampaignRunID == runID && (status == "" || rec.Status == status) {
			out = append(out, *rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return page(out, limit, offset), nil
}

func (m *memoryStore) ClaimPendingRecipients(_ context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := []*Recipient{}
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.CampaignRunID == runID && rec.Status == RecipientPending {
			pending = append(pending, rec)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })
	if limit < len(pending) {
		pending = pending[:limit]
	}
	now := time.Now().UTC()
	out := []Recipient{}
	for _, rec := range pending {
		rec.Status = RecipientQueued
		rec.QueuedAt = &now
		out = append(out, *rec)
	}
	return out, nil
}

func (m *memoryStore) ListQueuedWithoutMessage(_ context.Context, businessID, runID uuid.UUID, limit int) ([]Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Recipient{}
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.CampaignRunID == runID && rec.Status == RecipientQueued && rec.MessageID == nil {
			out = append(out, *rec)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memoryStore) GetRecipient(_ context.Context, businessID, recipientID uuid.UUID) (*Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.recipients[recipientID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryStore) UpdateRecipient(_ context.Context, rec Recipient) (*Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.recipients[rec.ID]
	if !ok || current.BusinessID != rec.BusinessID {
		return nil, ErrNotFound
	}
	rec.CreatedAt = current.CreatedAt
	rec.UpdatedAt = time.Now().UTC()
	copy := rec
	m.recipients[rec.ID] = &copy
	return &rec, nil
}

func (m *memoryStore) FindRecipientByMessage(_ context.Context, businessID, messageID uuid.UUID) (*Recipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.MessageID != nil && *rec.MessageID == messageID {
			copy := *rec
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) FindRecipientMessage(_ context.Context, businessID, recipientID uuid.UUID) (*uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.messages[businessID.String()+":"+recipientID.String()]; ok {
		return &id, nil
	}
	return nil, nil
}

func (m *memoryStore) SkipPendingRecipients(_ context.Context, businessID, runID uuid.UUID, code, reason string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	n := 0
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.CampaignRunID == runID && rec.Status == RecipientPending {
			rec.Status = RecipientSkipped
			rec.SkippedAt = &now
			rec.FailureCode, rec.FailureReason = &code, &reason
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) PendingRecipientCount(_ context.Context, businessID, runID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, rec := range m.recipients {
		if rec.BusinessID == businessID && rec.CampaignRunID == runID && rec.Status == RecipientPending {
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) ListDueScheduled(_ context.Context, now time.Time, limit int) ([]Campaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Campaign{}
	for _, item := range m.campaigns {
		if item.Status == StatusScheduled && item.ScheduledAt != nil && !item.ScheduledAt.After(now) {
			out = append(out, *item)
		}
	}
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryStore) TrySchedulerLock(_ context.Context) (bool, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lockHeld {
		return false, func() {}, nil
	}
	m.lockHeld = true
	return true, func() {
		m.mu.Lock()
		m.lockHeld = false
		m.mu.Unlock()
	}, nil
}

func matchAudience(c memoryCustomer, def AudienceDefinition) bool {
	if def.CustomerStatus != nil && c.Status != *def.CustomerStatus {
		return false
	}
	if t := optionalAudienceTime(def.CreatedAfter); t != nil && c.CreatedAt.Before(*t) {
		return false
	}
	if t := optionalAudienceTime(def.CreatedBefore); t != nil && !c.CreatedAt.Before(*t) {
		return false
	}
	if len(def.Tags) > 0 {
		set := map[string]struct{}{}
		for _, tag := range c.Tags {
			set[tag] = struct{}{}
		}
		for _, tag := range def.Tags {
			if _, ok := set[tag]; !ok {
				return false
			}
		}
	}
	if len(def.Channels) > 0 {
		ok := false
		chset := map[string]struct{}{}
		for _, ch := range c.Channels {
			chset[ch] = struct{}{}
		}
		for _, ch := range def.Channels {
			if _, found := chset[ch]; found {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	needConv := def.HasConversation != nil || def.LastConversationAfter != nil || def.LastConversationBefore != nil
	if needConv {
		has := c.LastConversationAt != nil
		if def.HasConversation != nil && !*def.HasConversation && has {
			return false
		}
		if (def.HasConversation == nil || *def.HasConversation) && !has {
			return false
		}
		if t := optionalAudienceTime(def.LastConversationAfter); t != nil && (c.LastConversationAt == nil || c.LastConversationAt.Before(*t)) {
			return false
		}
		if t := optionalAudienceTime(def.LastConversationBefore); t != nil && (c.LastConversationAt == nil || !c.LastConversationAt.Before(*t)) {
			return false
		}
	}
	needRes := def.HasReservation != nil || def.LastReservationStatus != nil || def.LastReservationAfter != nil || def.LastReservationBefore != nil
	if needRes {
		has := c.LastReservationAt != nil || c.LastReservationStatus != nil
		if def.HasReservation != nil && !*def.HasReservation && has {
			return false
		}
		if (def.HasReservation == nil || *def.HasReservation || def.LastReservationStatus != nil) && !has {
			return false
		}
		if def.LastReservationStatus != nil && (c.LastReservationStatus == nil || *c.LastReservationStatus != *def.LastReservationStatus) {
			return false
		}
		if t := optionalAudienceTime(def.LastReservationAfter); t != nil && (c.LastReservationAt == nil || c.LastReservationAt.Before(*t)) {
			return false
		}
		if t := optionalAudienceTime(def.LastReservationBefore); t != nil && (c.LastReservationAt == nil || !c.LastReservationAt.Before(*t)) {
			return false
		}
	}
	return true
}

func page[T any](items []T, limit, offset int) []T {
	if offset > len(items) {
		return []T{}
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}
