package conversation

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memCustomer struct {
	businessID uuid.UUID
	CustomerRecord
	Phone *string
	Email *string
}

type memIdentity struct {
	businessID uuid.UUID
	IdentityRecord
	ExternalID string
}

type memoryStore struct {
	mu            sync.Mutex
	customers     map[uuid.UUID]*memCustomer
	identities    map[uuid.UUID]*memIdentity
	conversations map[uuid.UUID]*Conversation
	messages      map[uuid.UUID]*Message
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		customers:     map[uuid.UUID]*memCustomer{},
		identities:    map[uuid.UUID]*memIdentity{},
		conversations: map[uuid.UUID]*Conversation{},
		messages:      map[uuid.UUID]*Message{},
	}
}

func (m *memoryStore) addCustomer(businessID uuid.UUID, name, status string, phone, email *string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.customers[id] = &memCustomer{
		businessID:     businessID,
		CustomerRecord: CustomerRecord{ID: id, Status: status, Name: name},
		Phone:          phone,
		Email:          email,
	}
	return id
}

func (m *memoryStore) addIdentity(businessID, customerID uuid.UUID, channel Channel, externalID, status string) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	m.identities[id] = &memIdentity{
		businessID:     businessID,
		IdentityRecord: IdentityRecord{ID: id, CustomerID: customerID, Channel: channel, Status: status},
		ExternalID:     externalID,
	}
	return id
}

func (m *memoryStore) RunInTx(_ context.Context, fn func(Store) error) error {
	return fn(m)
}

func (m *memoryStore) ListConversations(_ context.Context, businessID uuid.UUID, params ListParams) ([]Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Conversation, 0)
	term := strings.ToLower(strings.Trim(params.Search, `%`))
	for _, conv := range m.conversations {
		if conv.BusinessID != businessID || conv.Status != params.Status {
			continue
		}
		if params.Channel != "" && conv.Channel != params.Channel {
			continue
		}
		if params.AssignedUserID != nil && (conv.AssignedUserID == nil || *conv.AssignedUserID != *params.AssignedUserID) {
			continue
		}
		if params.CustomerID != nil && conv.CustomerID != *params.CustomerID {
			continue
		}
		if params.UnreadOnly && conv.UnreadCount <= 0 {
			continue
		}
		if term != "" {
			cust := m.customers[conv.CustomerID]
			if cust == nil || cust.businessID != businessID {
				continue
			}
			if !strings.Contains(strings.ToLower(cust.Name), term) &&
				!strings.Contains(strings.ToLower(deref(cust.Phone)), term) &&
				!strings.Contains(strings.ToLower(deref(cust.Email)), term) {
				continue
			}
		}
		out = append(out, m.withPreview(m.hydrate(*conv)))
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].LastMessageAt, out[j].LastMessageAt
		if li == nil && lj != nil {
			return false
		}
		if li != nil && lj == nil {
			return true
		}
		if li != nil && lj != nil && !li.Equal(*lj) {
			return li.After(*lj)
		}
		return out[i].ID.String() > out[j].ID.String()
	})
	if params.Offset > len(out) {
		return []Conversation{}, nil
	}
	out = out[params.Offset:]
	if params.Limit < len(out) {
		out = out[:params.Limit]
	}
	return out, nil
}

func (m *memoryStore) GetConversation(_ context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conv, ok := m.conversations[conversationID]
	if !ok || conv.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := m.hydrate(*conv)
	return &copy, nil
}

func (m *memoryStore) LockConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	return m.GetConversation(ctx, businessID, conversationID)
}

func (m *memoryStore) FindOpenConversation(_ context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, conv := range m.conversations {
		if conv.BusinessID != businessID || conv.CustomerID != customerID || conv.Channel != channel || conv.Status != StatusOpen {
			continue
		}
		if identityID == nil && conv.CustomerIdentityID == nil {
			copy := m.hydrate(*conv)
			return &copy, nil
		}
		if identityID != nil && conv.CustomerIdentityID != nil && *conv.CustomerIdentityID == *identityID {
			copy := m.hydrate(*conv)
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) FindLatestConversation(_ context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *Conversation
	for _, conv := range m.conversations {
		if conv.BusinessID != businessID || conv.CustomerID != customerID || conv.Channel != channel {
			continue
		}
		if !sameOptionalUUID(conv.CustomerIdentityID, identityID) {
			continue
		}
		if best == nil || conv.UpdatedAt.After(best.UpdatedAt) {
			copy := m.hydrate(*conv)
			best = &copy
		}
	}
	return best, nil
}

func (m *memoryStore) CreateConversation(_ context.Context, conv Conversation) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.conversations {
		if existing.BusinessID == conv.BusinessID && existing.CustomerID == conv.CustomerID &&
			existing.Channel == conv.Channel && existing.Status == StatusOpen &&
			sameOptionalUUID(existing.CustomerIdentityID, conv.CustomerIdentityID) {
			return nil, ErrOpenConversationExists
		}
	}
	now := time.Now().UTC()
	stored := conv
	stored.ID = uuid.New()
	stored.Status = StatusOpen
	stored.UnreadCount = 0
	if stored.HandlingState == "" {
		stored.HandlingState = HandlingAIActive
	}
	stored.CreatedAt = now
	stored.UpdatedAt = now
	m.conversations[stored.ID] = &stored
	copy := m.hydrate(stored)
	return &copy, nil
}

func (m *memoryStore) UpdateConversation(_ context.Context, businessID uuid.UUID, conv Conversation) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.conversations[conv.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrNotFound
	}
	current.Status = conv.Status
	current.AssignedUserID = conv.AssignedUserID
	current.HandlingState = normalizeHandling(conv.HandlingState)
	current.HandoverReason = conv.HandoverReason
	current.HandoverRequestedAt = conv.HandoverRequestedAt
	current.HumanStartedAt = conv.HumanStartedAt
	current.ResolvedAt = conv.ResolvedAt
	current.UpdatedAt = time.Now().UTC()
	copy := m.hydrate(*current)
	return &copy, nil
}

func (m *memoryStore) MarkRead(_ context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.conversations[conversationID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrNotFound
	}
	current.UnreadCount = 0
	current.UpdatedAt = time.Now().UTC()
	copy := m.hydrate(*current)
	return &copy, nil
}

func (m *memoryStore) MarkInboundRead(_ context.Context, businessID, conversationID uuid.UUID) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	type hit struct {
		at time.Time
		id string
	}
	hits := make([]hit, 0)
	for _, msg := range m.messages {
		if msg.BusinessID != businessID || msg.ConversationID != conversationID {
			continue
		}
		if msg.Direction != DirectionInbound || msg.SenderType != SenderCustomer || msg.Status != MessageReceived {
			continue
		}
		msg.Status = MessageRead
		msg.UpdatedAt = time.Now().UTC()
		if msg.ExternalID != nil && strings.TrimSpace(*msg.ExternalID) != "" {
			hits = append(hits, hit{at: msg.CreatedAt, id: strings.TrimSpace(*msg.ExternalID)})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at.Before(hits[j].at) })
	ids := make([]string, 0, len(hits))
	for _, item := range hits {
		ids = append(ids, item.id)
	}
	return ids, nil
}

func (m *memoryStore) ApplyMessageSideEffects(_ context.Context, businessID, conversationID uuid.UUID, at time.Time, unreadDelta int, reopen bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.conversations[conversationID]
	if !ok || current.BusinessID != businessID {
		return ErrNotFound
	}
	current.LastMessageAt = &at
	current.UnreadCount += unreadDelta
	if reopen {
		current.Status = StatusOpen
	}
	current.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *memoryStore) GetCustomer(_ context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.customers[customerID]
	if !ok || c.businessID != businessID {
		return nil, ErrCustomerNotFound
	}
	copy := c.CustomerRecord
	return &copy, nil
}

func (m *memoryStore) GetIdentity(_ context.Context, businessID, identityID uuid.UUID) (*IdentityRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.identities[identityID]
	if !ok || i.businessID != businessID {
		return nil, ErrIdentityNotFound
	}
	copy := i.IdentityRecord
	return &copy, nil
}

func (m *memoryStore) CreateMessage(_ context.Context, msg Message) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg.ExternalID != nil {
		for _, existing := range m.messages {
			if existing.BusinessID == msg.BusinessID && existing.ConversationID == msg.ConversationID &&
				existing.ExternalID != nil && *existing.ExternalID == *msg.ExternalID {
				return nil, ErrDuplicateExternalID
			}
		}
	}
	now := time.Now().UTC()
	stored := msg
	stored.ID = uuid.New()
	stored.CreatedAt = now
	stored.UpdatedAt = now
	m.messages[stored.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) ListMessages(_ context.Context, businessID, conversationID uuid.UUID, params MessageListParams) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0)
	for _, msg := range m.messages {
		if msg.BusinessID != businessID || msg.ConversationID != conversationID {
			continue
		}
		if params.Before != nil && !msg.CreatedAt.Before(*params.Before) {
			continue
		}
		if params.After != nil && !msg.CreatedAt.After(*params.After) {
			continue
		}
		copy := *msg
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	if params.Offset > len(out) {
		return []Message{}, nil
	}
	out = out[params.Offset:]
	if params.Limit < len(out) {
		out = out[:params.Limit]
	}
	return out, nil
}

func (m *memoryStore) ListRecentMessages(_ context.Context, businessID, conversationID uuid.UUID, limit int) ([]Message, error) {
	all, err := m.ListMessages(context.Background(), businessID, conversationID, MessageListParams{Limit: 100000, Offset: 0})
	if err != nil {
		return nil, err
	}
	if limit < len(all) {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func (m *memoryStore) GetMessageByExternalID(_ context.Context, businessID, conversationID uuid.UUID, externalID string) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.messages {
		if msg.BusinessID == businessID && msg.ConversationID == conversationID && msg.ExternalID != nil && *msg.ExternalID == externalID {
			copy := *msg
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) FindMessageByExternalID(_ context.Context, businessID uuid.UUID, externalID string) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.messages {
		if msg.BusinessID == businessID && msg.ExternalID != nil && *msg.ExternalID == externalID {
			copy := *msg
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) GetMessage(_ context.Context, businessID, messageID uuid.UUID) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[messageID]
	if !ok || msg.BusinessID != businessID {
		return nil, ErrMessageNotFound
	}
	copy := *msg
	return &copy, nil
}

func (m *memoryStore) ListHumanInbox(_ context.Context, businessID uuid.UUID, states []HandlingState, limit, offset int) ([]HumanInboxItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed := map[HandlingState]struct{}{}
	for _, state := range states {
		allowed[state] = struct{}{}
	}
	out := []HumanInboxItem{}
	for _, conv := range m.conversations {
		if conv.BusinessID != businessID {
			continue
		}
		if _, ok := allowed[normalizeHandling(conv.HandlingState)]; !ok {
			continue
		}
		item := HumanInboxItem{Conversation: m.withPreview(m.hydrate(*conv))}
		out = append(out, item)
	}
	if offset > len(out) {
		return []HumanInboxItem{}, nil
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryStore) ClaimForDelivery(_ context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[messageID]
	if !ok || msg.BusinessID != businessID || msg.MessageType == MessageTypeInternalNote {
		return nil, nil
	}
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	switch msg.Status {
	case MessageQueued:
	case MessageSending:
		if time.Since(msg.UpdatedAt) < staleAfter {
			return nil, nil
		}
	default:
		return nil, nil
	}
	msg.Status = MessageSending
	msg.UpdatedAt = time.Now().UTC()
	copy := *msg
	return &copy, nil
}

func (m *memoryStore) UpdateMessageDelivery(_ context.Context, businessID, messageID uuid.UUID, status MessageStatus, externalID *string) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[messageID]
	if !ok || msg.BusinessID != businessID {
		return nil, ErrMessageNotFound
	}
	if externalID != nil {
		for _, existing := range m.messages {
			if existing.ID != messageID && existing.BusinessID == businessID && existing.ExternalID != nil && *existing.ExternalID == *externalID {
				return nil, ErrDuplicateExternalID
			}
		}
		msg.ExternalID = externalID
	}
	msg.Status = status
	msg.UpdatedAt = time.Now().UTC()
	copy := *msg
	return &copy, nil
}

func (m *memoryStore) hydrate(conv Conversation) Conversation {
	conv.HandlingState = normalizeHandling(conv.HandlingState)
	if cust, ok := m.customers[conv.CustomerID]; ok {
		conv.Customer = CustomerSummary{ID: cust.ID, Name: cust.Name, Phone: cust.Phone, Email: cust.Email}
	}
	if conv.CustomerIdentityID != nil {
		if ident, ok := m.identities[*conv.CustomerIdentityID]; ok {
			conv.Identity = &IdentitySummary{ID: ident.ID, Channel: ident.Channel, ExternalID: ident.ExternalID}
		}
	}
	if conv.AssignedUserID != nil && conv.AssignedUser != nil {
		conv.AssignedUser = &UserSummary{ID: *conv.AssignedUserID, FullName: conv.AssignedUser.FullName}
	}
	return conv
}

func (m *memoryStore) withPreview(conv Conversation) Conversation {
	var latest *Message
	for _, msg := range m.messages {
		if msg.BusinessID != conv.BusinessID || msg.ConversationID != conv.ID || msg.MessageType == MessageTypeInternalNote {
			continue
		}
		if latest == nil || msg.CreatedAt.After(latest.CreatedAt) {
			copy := *msg
			latest = &copy
		}
	}
	if latest != nil {
		conv.LastMessage = &MessagePreview{
			Content:    latest.Content,
			CreatedAt:  latest.CreatedAt,
			SenderType: latest.SenderType,
			Direction:  latest.Direction,
			Status:     latest.Status,
		}
	}
	return conv
}

func sameOptionalUUID(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptr(v string) *string { return &v }
