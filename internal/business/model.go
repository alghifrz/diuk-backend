// Package business owns the tenant profile and F&B operational settings.
package business

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Profile is the identity of a business. Operational F&B configuration lives
// on FNBSettings, not here.
type Profile struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Type        tenant.BusinessType
	Timezone    string
	Status      tenant.Status
	Description *string
	Phone       *string
	Email       *string
	Address     *string
	WebsiteURL  *string
	LogoURL     *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// FNBSettings is how an FNB tenant takes reservations. It is not reservation
// logic; later jobs read these values.
type FNBSettings struct {
	BusinessID                     uuid.UUID
	Currency                       string
	ReservationEnabled             bool
	ReservationSlotDurationMinutes int
	ReservationMinPartySize        int
	ReservationMaxPartySize        int
	ReservationMinAdvanceMinutes   int
	ReservationMaxAdvanceDays      int
	DepositEnabled                 bool
	DepositType                    string
	DepositValue                   money.Amount
	BankName                       *string
	BankAccountNumber              *string
	BankAccountHolder              *string
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

const (
	defaultCurrency                     = "IDR"
	defaultReservationSlotMinutes       = 30
	defaultReservationMinPartySize      = 1
	defaultReservationMaxPartySize      = 20
	defaultReservationMinAdvanceMinutes = 60
	defaultReservationMaxAdvanceDays    = 30
	depositTypeFixed                    = "FIXED"
	depositTypePercentage               = "PERCENTAGE"
)
