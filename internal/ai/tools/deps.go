package tools

import (
	"context"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/menu"
	"github.com/alghifrz/diuk-backend/internal/reservation"
)

type BusinessLookup interface {
	Profile(ctx context.Context, businessID uuid.UUID) (*business.Profile, error)
	FNBSettings(ctx context.Context, businessID uuid.UUID) (*business.FNBSettings, error)
}

type CustomerLookup interface {
	GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*customer.Customer, error)
	ListCustomers(ctx context.Context, businessID uuid.UUID, statusRaw, search, limitRaw, offsetRaw string) ([]customer.Customer, error)
}

type MenuLookup interface {
	ListCategories(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]menu.Category, error)
	ListItems(ctx context.Context, businessID uuid.UUID, statusRaw string, categoryID *uuid.UUID) ([]menu.Item, error)
}

type ReservationAPI interface {
	Availability(ctx context.Context, businessID uuid.UUID, dateRaw, startRaw, durationRaw, partyRaw string) (*reservation.AvailabilityResult, error)
	Create(ctx context.Context, businessID uuid.UUID, in reservation.CreateReservationInput) (*reservation.Reservation, error)
	Confirm(ctx context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error)
	Cancel(ctx context.Context, businessID, reservationID uuid.UUID, reason *string) (*reservation.Reservation, error)
	Get(ctx context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error)
	List(ctx context.Context, businessID uuid.UUID, statusRaw, customerRaw, tableRaw, dateRaw, startDateRaw, endDateRaw, limitRaw, offsetRaw string) ([]reservation.Reservation, error)
}

type KnowledgeHit struct {
	DocumentID string
	Title      string
	Content    string
	Similarity float64
}

type KnowledgeSearchResult struct {
	Available bool
	Reason    string
	Results   []KnowledgeHit
}

type KnowledgeSearch interface {
	SearchKnowledge(ctx context.Context, businessID uuid.UUID, query string, limit int) (*KnowledgeSearchResult, error)
}

type HandoverAPI interface {
	RequestHumanHandover(ctx context.Context, businessID, conversationID uuid.UUID, reasonRaw string) (*conversation.Conversation, error)
}

// HoursLookup reads the weekly opening hours saved in settings.
type HoursLookup interface {
	ListHours(ctx context.Context, businessID uuid.UUID) ([]reservation.Hour, error)
}
