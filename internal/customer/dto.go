package customer

import (
	"time"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type CreateCustomerInput struct {
	Name           string
	Phone          *string
	Email          *string
	Notes          *string
	MarketingOptIn *bool
}

type UpdateCustomerInput struct {
	Name           *string
	Phone          *string
	Email          *string
	Notes          *string
	Status         *tenant.Status
	MarketingOptIn *bool
}

type CreateIdentityInput struct {
	Channel     string
	ExternalID  string
	DisplayName *string
}

// EnsureIdentityInput is the inbound channel lookup: reuse the WhatsApp
// identity if it already exists, otherwise create one customer + identity.
type EnsureIdentityInput struct {
	Channel     string
	ExternalID  string
	Name        string
	Phone       *string
	DisplayName *string
}

type UpdateIdentityInput struct {
	DisplayName *string
	Status      *tenant.Status
}

type CreateTagInput struct {
	Name        string
	Description *string
}

type UpdateTagInput struct {
	Name        *string
	Description *string
	Status      *tenant.Status
}

type createCustomerRequest struct {
	Name           string  `json:"name"`
	Phone          *string `json:"phone"`
	Email          *string `json:"email"`
	Notes          *string `json:"notes"`
	MarketingOptIn *bool   `json:"marketing_opt_in"`
}

type updateCustomerRequest struct {
	Name           *string `json:"name"`
	Phone          *string `json:"phone"`
	Email          *string `json:"email"`
	Notes          *string `json:"notes"`
	Status         *string `json:"status"`
	MarketingOptIn *bool   `json:"marketing_opt_in"`
}

type createIdentityRequest struct {
	Channel     string  `json:"channel"`
	ExternalID  string  `json:"external_id"`
	DisplayName *string `json:"display_name"`
}

type updateIdentityRequest struct {
	DisplayName *string `json:"display_name"`
	Status      *string `json:"status"`
}

type createTagRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type updateTagRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type customerJSON struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Phone          *string        `json:"phone"`
	Email          *string        `json:"email"`
	Notes          *string        `json:"notes"`
	Status         string         `json:"status"`
	MarketingOptIn bool           `json:"marketing_opt_in"`
	IdentityCount  int            `json:"identity_count"`
	ActiveTagCount int            `json:"active_tag_count"`
	Stats          statsJSON      `json:"stats"`
	Identities     []identityJSON `json:"identities,omitempty"`
	Tags           []tagJSON      `json:"tags,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type statsJSON struct {
	ReservationCount  int          `json:"reservation_count"`
	CompletedCount    int          `json:"completed_count"`
	CancelledCount    int          `json:"cancelled_count"`
	TotalSpent        money.Amount `json:"total_spent"`
	LastVisitAt       *string      `json:"last_visit_at"`
	NextReservationAt *string      `json:"next_reservation_at"`
}

func statsResponse(s Stats) statsJSON {
	out := statsJSON{
		ReservationCount: s.ReservationCount,
		CompletedCount:   s.CompletedCount,
		CancelledCount:   s.CancelledCount,
		TotalSpent:       s.TotalSpent,
	}
	if s.LastVisitAt != nil {
		v := s.LastVisitAt.UTC().Format(time.RFC3339)
		out.LastVisitAt = &v
	}
	if s.NextReservationAt != nil {
		v := s.NextReservationAt.UTC().Format(time.RFC3339)
		out.NextReservationAt = &v
	}
	return out
}

type identityJSON struct {
	ID          string  `json:"id"`
	Channel     string  `json:"channel"`
	ExternalID  string  `json:"external_id"`
	DisplayName *string `json:"display_name"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type tagJSON struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func customerResponse(c *Customer, includeChildren bool) customerJSON {
	out := customerJSON{
		ID:             c.ID.String(),
		Name:           c.Name,
		Phone:          c.Phone,
		Email:          c.Email,
		Notes:          c.Notes,
		Status:         string(c.Status),
		MarketingOptIn: c.MarketingOptIn,
		IdentityCount:  c.IdentityCount,
		ActiveTagCount: c.ActiveTagCount,
		Stats:          statsResponse(c.Stats),
		CreatedAt:      c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if includeChildren {
		out.Identities = identitiesResponse(c.Identities)
		out.Tags = tagsResponse(c.Tags)
	}
	return out
}

func identityResponse(i *Identity) identityJSON {
	return identityJSON{
		ID:          i.ID.String(),
		Channel:     string(i.Channel),
		ExternalID:  i.ExternalID,
		DisplayName: i.DisplayName,
		Status:      string(i.Status),
		CreatedAt:   i.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   i.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func tagResponse(t *Tag) tagJSON {
	return tagJSON{
		ID:          t.ID.String(),
		Name:        t.Name,
		Description: t.Description,
		Status:      string(t.Status),
		CreatedAt:   t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func customersResponse(items []Customer) []customerJSON {
	out := make([]customerJSON, 0, len(items))
	for i := range items {
		out = append(out, customerResponse(&items[i], false))
	}
	return out
}

func identitiesResponse(items []Identity) []identityJSON {
	out := make([]identityJSON, 0, len(items))
	for i := range items {
		out = append(out, identityResponse(&items[i]))
	}
	return out
}

func tagsResponse(items []Tag) []tagJSON {
	out := make([]tagJSON, 0, len(items))
	for i := range items {
		out = append(out, tagResponse(&items[i]))
	}
	return out
}
