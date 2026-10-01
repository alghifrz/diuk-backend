package customer

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type memoryStore struct {
	mu          sync.Mutex
	customers   map[uuid.UUID]*Customer
	identities  map[uuid.UUID]*Identity
	tags        map[uuid.UUID]*Tag
	assignments map[string]struct{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		customers:   map[uuid.UUID]*Customer{},
		identities:  map[uuid.UUID]*Identity{},
		tags:        map[uuid.UUID]*Tag{},
		assignments: map[string]struct{}{},
	}
}

func assignmentKey(customerID, tagID uuid.UUID) string {
	return customerID.String() + ":" + tagID.String()
}

func (m *memoryStore) ListCustomers(_ context.Context, businessID uuid.UUID, params ListParams) ([]Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Customer, 0)
	for _, c := range m.customers {
		if c.BusinessID != businessID || c.Status != params.Status {
			continue
		}
		if params.Search != "" {
			term := strings.ToLower(strings.Trim(params.Search, `%`))
			if !strings.Contains(strings.ToLower(c.Name), term) &&
				!strings.Contains(strings.ToLower(deref(c.Phone)), term) &&
				!strings.Contains(strings.ToLower(deref(c.Email)), term) {
				continue
			}
		}
		copy := m.withCounts(c)
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID.String() > out[j].ID.String()
	})
	if params.Offset > len(out) {
		return []Customer{}, nil
	}
	out = out[params.Offset:]
	if params.Limit < len(out) {
		out = out[:params.Limit]
	}
	return out, nil
}

func (m *memoryStore) GetCustomer(_ context.Context, businessID, customerID uuid.UUID) (*Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.customers[customerID]
	if !ok || c.BusinessID != businessID {
		return nil, ErrCustomerNotFound
	}
	copy := m.withCounts(c)
	copy.Identities = m.identitiesFor(businessID, customerID, "")
	copy.Tags = m.tagsFor(businessID, customerID)
	return &copy, nil
}

func (m *memoryStore) CreateCustomer(_ context.Context, businessID uuid.UUID, in CreateCustomerInput) (*Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	optIn := true
	if in.MarketingOptIn != nil {
		optIn = *in.MarketingOptIn
	}
	c := &Customer{
		ID: uuid.New(), BusinessID: businessID, Name: in.Name, Phone: in.Phone, Email: in.Email, Notes: in.Notes,
		Status: tenant.StatusActive, MarketingOptIn: optIn, CreatedAt: now, UpdatedAt: now,
	}
	m.customers[c.ID] = c
	copy := *c
	copy.Identities = []Identity{}
	copy.Tags = []Tag{}
	return &copy, nil
}

func (m *memoryStore) UpdateCustomer(_ context.Context, businessID uuid.UUID, customer Customer) (*Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.customers[customer.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrCustomerNotFound
	}
	current.Name = customer.Name
	current.Phone = customer.Phone
	current.Email = customer.Email
	current.Notes = customer.Notes
	current.Status = customer.Status
	current.MarketingOptIn = customer.MarketingOptIn
	current.UpdatedAt = time.Now().UTC()
	copy := m.withCounts(current)
	copy.Identities = m.identitiesFor(businessID, customer.ID, "")
	copy.Tags = m.tagsFor(businessID, customer.ID)
	return &copy, nil
}

func (m *memoryStore) ListIdentities(_ context.Context, businessID, customerID uuid.UUID, status tenant.Status) ([]Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.identitiesFor(businessID, customerID, status), nil
}

func (m *memoryStore) GetIdentity(_ context.Context, businessID, customerID, identityID uuid.UUID) (*Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.identities[identityID]
	if !ok || i.BusinessID != businessID || i.CustomerID != customerID {
		return nil, ErrIdentityNotFound
	}
	copy := *i
	return &copy, nil
}

func (m *memoryStore) FindIdentityByExternal(_ context.Context, businessID uuid.UUID, channel Channel, externalID string) (*Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.identities {
		if item.BusinessID == businessID && item.Channel == channel && item.ExternalID == externalID {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) CreateIdentity(_ context.Context, identity Identity) (*Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.customers[identity.CustomerID]
	if !ok || c.BusinessID != identity.BusinessID {
		return nil, ErrCustomerNotFound
	}
	for _, existing := range m.identities {
		if existing.BusinessID == identity.BusinessID && existing.Channel == identity.Channel && existing.ExternalID == identity.ExternalID {
			return nil, ErrIdentityExists
		}
	}
	now := time.Now().UTC()
	stored := identity
	stored.ID = uuid.New()
	stored.Status = tenant.StatusActive
	stored.CreatedAt = now
	stored.UpdatedAt = now
	m.identities[stored.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) UpdateIdentity(_ context.Context, businessID uuid.UUID, identity Identity) (*Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.identities[identity.ID]
	if !ok || current.BusinessID != businessID || current.CustomerID != identity.CustomerID {
		return nil, ErrIdentityNotFound
	}
	current.DisplayName = identity.DisplayName
	current.Status = identity.Status
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	return &copy, nil
}

func (m *memoryStore) ListTags(_ context.Context, businessID uuid.UUID, status tenant.Status) ([]Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Tag, 0)
	for _, t := range m.tags {
		if t.BusinessID == businessID && t.Status == status {
			copy := *t
			out = append(out, copy)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *memoryStore) GetTag(_ context.Context, businessID, tagID uuid.UUID) (*Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tags[tagID]
	if !ok || t.BusinessID != businessID {
		return nil, ErrTagNotFound
	}
	copy := *t
	return &copy, nil
}

func (m *memoryStore) CreateTag(_ context.Context, businessID uuid.UUID, name string, description *string) (*Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tags {
		if t.BusinessID == businessID && t.Name == name {
			return nil, ErrTagNameExists
		}
	}
	now := time.Now().UTC()
	t := &Tag{ID: uuid.New(), BusinessID: businessID, Name: name, Description: description, Status: tenant.StatusActive, CreatedAt: now, UpdatedAt: now}
	m.tags[t.ID] = t
	copy := *t
	return &copy, nil
}

func (m *memoryStore) UpdateTag(_ context.Context, businessID uuid.UUID, tag Tag) (*Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tags[tag.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrTagNotFound
	}
	for _, t := range m.tags {
		if t.BusinessID == businessID && t.Name == tag.Name && t.ID != tag.ID {
			return nil, ErrTagNameExists
		}
	}
	current.Name = tag.Name
	current.Description = tag.Description
	current.Status = tag.Status
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	return &copy, nil
}

func (m *memoryStore) ListCustomerTags(_ context.Context, businessID, customerID uuid.UUID) ([]Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tagsFor(businessID, customerID), nil
}

func (m *memoryStore) AssignTag(_ context.Context, businessID, customerID, tagID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.customers[customerID]
	if !ok || c.BusinessID != businessID {
		return ErrCustomerNotFound
	}
	t, ok := m.tags[tagID]
	if !ok || t.BusinessID != businessID {
		return ErrTagNotFound
	}
	m.assignments[assignmentKey(customerID, tagID)] = struct{}{}
	return nil
}

func (m *memoryStore) UnassignTag(_ context.Context, businessID, customerID, tagID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := assignmentKey(customerID, tagID)
	if _, ok := m.assignments[key]; !ok {
		return ErrAssignmentNotFound
	}
	c, ok := m.customers[customerID]
	if !ok || c.BusinessID != businessID {
		return ErrAssignmentNotFound
	}
	delete(m.assignments, key)
	return nil
}

func (m *memoryStore) withCounts(c *Customer) Customer {
	copy := *c
	copy.IdentityCount = 0
	copy.ActiveTagCount = 0
	for _, i := range m.identities {
		if i.CustomerID == c.ID && i.BusinessID == c.BusinessID {
			copy.IdentityCount++
		}
	}
	for key := range m.assignments {
		if !strings.HasPrefix(key, c.ID.String()+":") {
			continue
		}
		tagID, err := uuid.Parse(strings.TrimPrefix(key, c.ID.String()+":"))
		if err != nil {
			continue
		}
		if t, ok := m.tags[tagID]; ok && t.Status == tenant.StatusActive {
			copy.ActiveTagCount++
		}
	}
	return copy
}

func (m *memoryStore) identitiesFor(businessID, customerID uuid.UUID, status tenant.Status) []Identity {
	out := make([]Identity, 0)
	for _, i := range m.identities {
		if i.BusinessID != businessID || i.CustomerID != customerID {
			continue
		}
		if status != "" && i.Status != status {
			continue
		}
		copy := *i
		out = append(out, copy)
	}
	return out
}

func (m *memoryStore) tagsFor(businessID, customerID uuid.UUID) []Tag {
	out := make([]Tag, 0)
	for key := range m.assignments {
		if !strings.HasPrefix(key, customerID.String()+":") {
			continue
		}
		tagID, err := uuid.Parse(strings.TrimPrefix(key, customerID.String()+":"))
		if err != nil {
			continue
		}
		t, ok := m.tags[tagID]
		if !ok || t.BusinessID != businessID {
			continue
		}
		copy := *t
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptr(v string) *string { return &v }
