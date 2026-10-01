// Package customer owns tenant CRM profiles, channel identities, and tags.
package customer

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Channel is an external messaging surface a customer can be reached on.
type Channel string

const (
	ChannelWhatsApp  Channel = "WHATSAPP"
	ChannelInstagram Channel = "INSTAGRAM"
	ChannelWebsite   Channel = "WEBSITE"
)

// Customer is a tenant-scoped person. Phone on the profile is not a unique identity.
type Customer struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	Name           string
	Phone          *string
	Email          *string
	Notes          *string
	Status         tenant.Status
	MarketingOptIn bool
	IdentityCount  int
	ActiveTagCount int
	Stats          Stats
	Identities     []Identity
	Tags           []Tag
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Stats are CRM aggregates derived from reservations and their payments.
// TotalSpent only counts PAID/DEPOSIT payments on COMPLETED reservations.
type Stats struct {
	ReservationCount  int
	CompletedCount    int
	CancelledCount    int
	TotalSpent        money.Amount
	LastVisitAt       *time.Time
	NextReservationAt *time.Time
}

// Identity is one external channel account linked to a customer.
type Identity struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	CustomerID  uuid.UUID
	Channel     Channel
	ExternalID  string
	DisplayName *string
	Status      tenant.Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Tag is a tenant-owned CRM label.
type Tag struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	Name        string
	Description *string
	Status      tenant.Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListParams controls customer list pagination and filters.
type ListParams struct {
	Status tenant.Status
	Search string
	TagID  *uuid.UUID
	Sort   Sort
	Limit  int
	Offset int
}

// Sort orders the customer list.
type Sort string

const (
	SortNewest    Sort = "newest"
	SortName      Sort = "name"
	SortSpend     Sort = "spend"
	SortVisits    Sort = "visits"
	SortLastVisit Sort = "last_visit"
)

// ListOptions are the raw (string) list filters coming from HTTP.
type ListOptions struct {
	Status string
	Search string
	Tag    string
	Sort   string
	Limit  string
	Offset string
}
