package calendar

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive    = "ACTIVE"
	StatusCancelled = "CANCELLED"

	TypeReservation   = "RESERVATION"
	TypeBusinessBlock = "BUSINESS_BLOCK"
	TypeAreaBlock     = "AREA_BLOCK"
	TypeTableBlock    = "TABLE_BLOCK"

	ScopeBusiness = "BUSINESS"
	ScopeArea     = "AREA"
	ScopeTable    = "TABLE"
)

type Block struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	AreaID      *uuid.UUID
	TableID     *uuid.UUID
	Title       string
	Description *string
	StartAt     time.Time
	EndAt       time.Time
	Status      string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	AreaName    string
	TableName   string
}

type ReservationConflict struct {
	ReservationID uuid.UUID
	StartAt       time.Time
	EndAt         time.Time
	Status        string
}

type ReservationEvent struct {
	ID            uuid.UUID
	ReservationID uuid.UUID
	Title         string
	StartAt       time.Time
	EndAt         time.Time
	Status        string
	PartySize     int
	CustomerID    uuid.UUID
	CustomerName  string
	TableID       *uuid.UUID
	TableName     string
	AreaID        *uuid.UUID
	AreaName      string
	PaymentStatus *string
}

type Event struct {
	ID            uuid.UUID
	Type          string
	ReservationID *uuid.UUID
	Title         string
	StartAt       time.Time
	EndAt         time.Time
	Status        string
	Scope         string
	Description   *string
	CustomerID    *uuid.UUID
	CustomerName  string
	PartySize     *int
	TableID       *uuid.UUID
	TableName     string
	AreaID        *uuid.UUID
	AreaName      string
	PaymentStatus *string
}

type DayView struct {
	Date      string
	Timezone  string
	Closed    bool
	Hours     []HourInterval
	Exception *ExceptionInfo
	Events    []Event
}

type HourInterval struct {
	OpenTime  string
	CloseTime string
}

type ExceptionInfo struct {
	ID        uuid.UUID
	IsClosed  bool
	OpenTime  *string
	CloseTime *string
	Note      *string
}

type Hour struct {
	DayOfWeek int
	OpenTime  string
	CloseTime string
}

type ScheduleException struct {
	ID            uuid.UUID
	ExceptionDate time.Time
	IsClosed      bool
	OpenTime      *string
	CloseTime     *string
	Note          *string
}

type CreateBlockInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
	AreaID      *string `json:"area_id"`
	TableID     *string `json:"table_id"`
}

type UpdateBlockInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	StartAt     *string `json:"start_at"`
	EndAt       *string `json:"end_at"`
}

func (b Block) Scope() string {
	switch {
	case b.TableID != nil:
		return ScopeTable
	case b.AreaID != nil:
		return ScopeArea
	default:
		return ScopeBusiness
	}
}

func (b Block) EventType() string {
	switch b.Scope() {
	case ScopeTable:
		return TypeTableBlock
	case ScopeArea:
		return TypeAreaBlock
	default:
		return TypeBusinessBlock
	}
}
