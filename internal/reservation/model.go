// Package reservation owns bookings, availability, weekly hours, and schedule exceptions.
package reservation

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusConfirmed Status = "CONFIRMED"
	StatusCancelled Status = "CANCELLED"
	StatusCompleted Status = "COMPLETED"
)

type Reservation struct {
	ID                 uuid.UUID
	BusinessID         uuid.UUID
	CustomerID         uuid.UUID
	TableID            *uuid.UUID
	PartySize          int
	StartAt            time.Time
	EndAt              time.Time
	Status             Status
	Notes              *string
	CancellationReason *string
	ConfirmedAt        *time.Time
	CompletedAt        *time.Time
	Customer           CustomerSummary
	Table              *TableSummary
	Items              []ReservationItem
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ReservationItem is a pre-order line captured at booking time.
type ReservationItem struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	ReservationID uuid.UUID
	MenuItemID    uuid.UUID
	MenuItemName  string
	Quantity      int
	UnitPrice     string
	Notes         *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MenuItemRef struct {
	ID     uuid.UUID
	Name   string
	Price  string
	Status string
}

type CustomerSummary struct {
	ID    uuid.UUID
	Name  string
	Phone *string
}

type TableSummary struct {
	ID       uuid.UUID
	Name     string
	Capacity int
	AreaID   uuid.UUID
	AreaName string
}

type Hour struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	DayOfWeek  int
	SortOrder  int
	OpenTime   string
	CloseTime  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ScheduleException struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	ExceptionDate time.Time
	IsClosed      bool
	OpenTime      *string
	CloseTime     *string
	Note          *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type BookingContext struct {
	Timezone                       string
	ReservationEnabled             bool
	ReservationSlotDurationMinutes int
	ReservationMinPartySize        int
	ReservationMaxPartySize        int
	ReservationMinAdvanceMinutes   int
	ReservationMaxAdvanceDays      int
	Currency                       string
	DepositEnabled                 bool
	DepositType                    string
	DepositValue                   money.Amount
}

type CustomerRecord struct {
	ID     uuid.UUID
	Name   string
	Phone  *string
	Status string
}

type TableRecord struct {
	ID         uuid.UUID
	Name       string
	Capacity   int
	Status     string
	AreaID     uuid.UUID
	AreaName   string
	AreaStatus string
	BusinessID uuid.UUID
}

type ListParams struct {
	Status     Status
	CustomerID *uuid.UUID
	TableID    *uuid.UUID
	StartFrom  *time.Time
	StartTo    *time.Time
	Limit      int
	Offset     int
}

type AvailabilityQuery struct {
	Date            time.Time
	Start           time.Time
	End             time.Time
	PartySize       int
	DurationMinutes int
}

type AvailabilityResult struct {
	Date      string
	StartAt   time.Time
	EndAt     time.Time
	PartySize int
	Available bool
	Tables    []TableSummary
}

type CreateReservationInput struct {
	CustomerID  string
	StartAt     string
	EndAt       string
	PartySize   int
	TableID     *string
	Notes       *string
	Items       []ReservationItemInput
	PaymentType string
}

type ReservationItemInput struct {
	MenuItemID string  `json:"menu_item_id"`
	Quantity   int     `json:"quantity"`
	Notes      *string `json:"notes"`
}

type RescheduleInput struct {
	StartAt   string
	EndAt     string
	PartySize *int
	TableID   *string
}

type UpdateReservationInput struct {
	CustomerID *string
	StartAt    *string
	EndAt      *string
	PartySize  *int
	TableID    *string
	ClearTable bool
	Notes      *string
}

type HourInput struct {
	DayOfWeek int
	SortOrder int
	OpenTime  string
	CloseTime string
}

type ExceptionInput struct {
	ExceptionDate string
	IsClosed      bool
	OpenTime      *string
	CloseTime     *string
	Note          *string
}

type UpdateExceptionInput struct {
	ExceptionDate *string
	IsClosed      *bool
	OpenTime      *string
	CloseTime     *string
	Note          *string
}
