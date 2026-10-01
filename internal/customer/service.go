package customer

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type EventSink interface {
	CustomerCreated(ctx context.Context, cust *Customer) error
}

// Service applies customer and CRM rules. The business id must come from tenant context.
type Service struct {
	store  Store
	events EventSink
}

func (s *Service) SetEvents(events EventSink) {
	s.events = events
}

// NewService returns a service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListCustomers(ctx context.Context, businessID uuid.UUID, statusRaw, search, limitRaw, offsetRaw string) ([]Customer, error) {
	return s.ListCustomersWithOptions(ctx, businessID, ListOptions{
		Status: statusRaw,
		Search: search,
		Limit:  limitRaw,
		Offset: offsetRaw,
	})
}

// ListCustomersWithOptions lists customers with CRM filters (tag) and sorting.
func (s *Service) ListCustomersWithOptions(ctx context.Context, businessID uuid.UUID, opts ListOptions) ([]Customer, error) {
	status, err := parseStatusFilter(opts.Status)
	if err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	sortBy, err := parseSort(opts.Sort)
	if err != nil {
		return nil, err
	}
	var tagID *uuid.UUID
	if raw := strings.TrimSpace(opts.Tag); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, invalid("tag is invalid")
		}
		tagID = &parsed
	}
	return s.store.ListCustomers(ctx, businessID, ListParams{
		Status: status,
		Search: likePattern(opts.Search),
		TagID:  tagID,
		Sort:   sortBy,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*Customer, error) {
	return s.store.GetCustomer(ctx, businessID, customerID)
}

func (s *Service) FindIdentityByExternal(ctx context.Context, businessID uuid.UUID, channelRaw, externalID string) (*Identity, error) {
	channel, err := parseChannel(channelRaw)
	if err != nil {
		return nil, err
	}
	id, err := normalizeExternalID(externalID)
	if err != nil {
		return nil, err
	}
	return s.store.FindIdentityByExternal(ctx, businessID, channel, id)
}

func (s *Service) CreateCustomer(ctx context.Context, businessID uuid.UUID, in CreateCustomerInput) (*Customer, error) {
	cust, err := s.insertCustomer(ctx, businessID, in)
	if err != nil {
		return nil, err
	}
	s.publishCustomerCreated(ctx, cust)
	return cust, nil
}

func (s *Service) insertCustomer(ctx context.Context, businessID uuid.UUID, in CreateCustomerInput) (*Customer, error) {
	normalized, err := normalizeCreateCustomer(in)
	if err != nil {
		return nil, err
	}
	return s.store.CreateCustomer(ctx, businessID, normalized)
}

func (s *Service) publishCustomerCreated(ctx context.Context, cust *Customer) {
	if s.events == nil || cust == nil {
		return
	}
	if pubErr := s.events.CustomerCreated(ctx, cust); pubErr != nil {
		slog.Default().Warn("automation customer.created failed", "error", pubErr)
	}
}

// EnsureIdentity reuses the channel identity if it already exists.
// customer.created is published only when this call wins the insert.
func (s *Service) EnsureIdentity(ctx context.Context, businessID uuid.UUID, in EnsureIdentityInput) (*Identity, bool, error) {
	channel, err := parseChannel(in.Channel)
	if err != nil {
		return nil, false, err
	}
	externalID, err := normalizeExternalID(in.ExternalID)
	if err != nil {
		return nil, false, err
	}
	displayName, err := normalizeOptionalText(in.DisplayName, maxDisplayNameLen, "display_name")
	if err != nil {
		return nil, false, err
	}

	existing, err := s.store.FindIdentityByExternal(ctx, businessID, channel, externalID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = externalID
	}
	cust, err := s.insertCustomer(ctx, businessID, CreateCustomerInput{
		Name:  name,
		Phone: in.Phone,
	})
	if err != nil {
		return nil, false, err
	}

	identity, err := s.store.CreateIdentity(ctx, Identity{
		BusinessID:  businessID,
		CustomerID:  cust.ID,
		Channel:     channel,
		ExternalID:  externalID,
		DisplayName: displayName,
		Status:      tenant.StatusActive,
	})
	if err != nil {
		if errors.Is(err, ErrIdentityExists) {
			existing, findErr := s.store.FindIdentityByExternal(ctx, businessID, channel, externalID)
			if findErr != nil {
				return nil, false, findErr
			}
			if existing == nil {
				return nil, false, ErrIdentityExists
			}
			return existing, false, nil
		}
		return nil, false, err
	}

	s.publishCustomerCreated(ctx, cust)
	return identity, true, nil
}

func (s *Service) UpdateCustomer(ctx context.Context, businessID, customerID uuid.UUID, in UpdateCustomerInput) (*Customer, error) {
	current, err := s.store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.Name != nil {
		name, err := normalizeName(*in.Name, maxNameLen)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Phone != nil {
		phone, err := normalizeOptionalPhone(in.Phone)
		if err != nil {
			return nil, err
		}
		next.Phone = phone
	}
	if in.Email != nil {
		email, err := normalizeOptionalEmail(in.Email)
		if err != nil {
			return nil, err
		}
		next.Email = email
	}
	if in.Notes != nil {
		notes, err := normalizeOptionalText(in.Notes, 0, "notes")
		if err != nil {
			return nil, err
		}
		next.Notes = notes
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		next.Status = *in.Status
	}
	if in.MarketingOptIn != nil {
		next.MarketingOptIn = *in.MarketingOptIn
	}
	next.BusinessID = current.BusinessID
	return s.store.UpdateCustomer(ctx, businessID, next)
}

func (s *Service) DeleteCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*Customer, error) {
	current, err := s.store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}
	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateCustomer(ctx, businessID, next)
}

func (s *Service) ListIdentities(ctx context.Context, businessID, customerID uuid.UUID, statusRaw string) ([]Identity, error) {
	if _, err := s.store.GetCustomer(ctx, businessID, customerID); err != nil {
		return nil, err
	}
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListIdentities(ctx, businessID, customerID, status)
}

func (s *Service) CreateIdentity(ctx context.Context, businessID, customerID uuid.UUID, in CreateIdentityInput) (*Identity, error) {
	customer, err := s.store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return nil, err
	}
	if customer.Status != tenant.StatusActive {
		return nil, ErrCustomerInactive
	}
	channel, err := parseChannel(in.Channel)
	if err != nil {
		return nil, err
	}
	externalID, err := normalizeExternalID(in.ExternalID)
	if err != nil {
		return nil, err
	}
	displayName, err := normalizeOptionalText(in.DisplayName, maxDisplayNameLen, "display_name")
	if err != nil {
		return nil, err
	}
	return s.store.CreateIdentity(ctx, Identity{
		BusinessID:  businessID,
		CustomerID:  customerID,
		Channel:     channel,
		ExternalID:  externalID,
		DisplayName: displayName,
		Status:      tenant.StatusActive,
	})
}

func (s *Service) UpdateIdentity(ctx context.Context, businessID, customerID, identityID uuid.UUID, in UpdateIdentityInput) (*Identity, error) {
	if _, err := s.store.GetCustomer(ctx, businessID, customerID); err != nil {
		return nil, err
	}
	current, err := s.store.GetIdentity(ctx, businessID, customerID, identityID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.DisplayName != nil {
		displayName, err := normalizeOptionalText(in.DisplayName, maxDisplayNameLen, "display_name")
		if err != nil {
			return nil, err
		}
		next.DisplayName = displayName
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		next.Status = *in.Status
	}
	next.Channel = current.Channel
	next.ExternalID = current.ExternalID
	next.CustomerID = current.CustomerID
	return s.store.UpdateIdentity(ctx, businessID, next)
}

func (s *Service) DeleteIdentity(ctx context.Context, businessID, customerID, identityID uuid.UUID) (*Identity, error) {
	if _, err := s.store.GetCustomer(ctx, businessID, customerID); err != nil {
		return nil, err
	}
	current, err := s.store.GetIdentity(ctx, businessID, customerID, identityID)
	if err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}
	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateIdentity(ctx, businessID, next)
}

func (s *Service) ListTags(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]Tag, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListTags(ctx, businessID, status)
}

func (s *Service) GetTag(ctx context.Context, businessID, tagID uuid.UUID) (*Tag, error) {
	return s.store.GetTag(ctx, businessID, tagID)
}

func (s *Service) CreateTag(ctx context.Context, businessID uuid.UUID, in CreateTagInput) (*Tag, error) {
	name, err := normalizeName(in.Name, maxTagNameLen)
	if err != nil {
		return nil, err
	}
	description, err := normalizeOptionalText(in.Description, maxTagDescLen, "description")
	if err != nil {
		return nil, err
	}
	return s.store.CreateTag(ctx, businessID, name, description)
}

func (s *Service) UpdateTag(ctx context.Context, businessID, tagID uuid.UUID, in UpdateTagInput) (*Tag, error) {
	current, err := s.store.GetTag(ctx, businessID, tagID)
	if err != nil {
		return nil, err
	}
	next := *current
	if in.Name != nil {
		name, err := normalizeName(*in.Name, maxTagNameLen)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		description, err := normalizeOptionalText(in.Description, maxTagDescLen, "description")
		if err != nil {
			return nil, err
		}
		next.Description = description
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		next.Status = *in.Status
	}
	next.BusinessID = current.BusinessID
	return s.store.UpdateTag(ctx, businessID, next)
}

func (s *Service) DeleteTag(ctx context.Context, businessID, tagID uuid.UUID) (*Tag, error) {
	current, err := s.store.GetTag(ctx, businessID, tagID)
	if err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}
	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateTag(ctx, businessID, next)
}

func (s *Service) ListCustomerTags(ctx context.Context, businessID, customerID uuid.UUID) ([]Tag, error) {
	if _, err := s.store.GetCustomer(ctx, businessID, customerID); err != nil {
		return nil, err
	}
	return s.store.ListCustomerTags(ctx, businessID, customerID)
}

func (s *Service) AssignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) (*Tag, bool, error) {
	customer, err := s.store.GetCustomer(ctx, businessID, customerID)
	if err != nil {
		return nil, false, err
	}
	if customer.Status != tenant.StatusActive {
		return nil, false, ErrCustomerInactive
	}
	tag, err := s.store.GetTag(ctx, businessID, tagID)
	if err != nil {
		return nil, false, err
	}
	if tag.Status != tenant.StatusActive {
		return nil, false, ErrTagInactive
	}
	for _, existing := range customer.Tags {
		if existing.ID == tagID {
			return tag, false, nil
		}
	}
	if err := s.store.AssignTag(ctx, businessID, customerID, tagID); err != nil {
		return nil, false, err
	}
	return tag, true, nil
}

func (s *Service) UnassignTag(ctx context.Context, businessID, customerID, tagID uuid.UUID) error {
	if _, err := s.store.GetCustomer(ctx, businessID, customerID); err != nil {
		return err
	}
	if _, err := s.store.GetTag(ctx, businessID, tagID); err != nil {
		return err
	}
	err := s.store.UnassignTag(ctx, businessID, customerID, tagID)
	if err == nil || errors.Is(err, ErrAssignmentNotFound) {
		return nil
	}
	return err
}

func normalizeCreateCustomer(in CreateCustomerInput) (CreateCustomerInput, error) {
	name, err := normalizeName(in.Name, maxNameLen)
	if err != nil {
		return CreateCustomerInput{}, err
	}
	phone, err := normalizeOptionalPhone(in.Phone)
	if err != nil {
		return CreateCustomerInput{}, err
	}
	email, err := normalizeOptionalEmail(in.Email)
	if err != nil {
		return CreateCustomerInput{}, err
	}
	notes, err := normalizeOptionalText(in.Notes, 0, "notes")
	if err != nil {
		return CreateCustomerInput{}, err
	}
	optIn := true
	if in.MarketingOptIn != nil {
		optIn = *in.MarketingOptIn
	}
	return CreateCustomerInput{Name: name, Phone: phone, Email: email, Notes: notes, MarketingOptIn: &optIn}, nil
}
