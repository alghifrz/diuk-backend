package automation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
	StatusArchived = "ARCHIVED"

	TriggerReservationConfirmed = "RESERVATION_CONFIRMED"
	TriggerReservationCancelled = "RESERVATION_CANCELLED"
	TriggerConversationCreated  = "CONVERSATION_CREATED"
	TriggerConversationInactive = "CONVERSATION_INACTIVE"
	TriggerCustomerCreated      = "CUSTOMER_CREATED"
	TriggerScheduled            = "SCHEDULED"

	ActionSendMessage       = "SEND_MESSAGE"
	ActionAIGenerateMessage = "AI_GENERATE_MESSAGE"

	RunPending   = "PENDING"
	RunRunning   = "RUNNING"
	RunCompleted = "COMPLETED"
	RunFailed    = "FAILED"
	RunCancelled = "CANCELLED"

	EventReservationConfirmed       = "reservation.confirmed"
	EventReservationCancelled       = "reservation.cancelled"
	EventConversationCreated        = "conversation.created"
	EventConversationHumanRequested = "conversation.human_requested"
	EventConversationHumanStarted   = "conversation.human_started"
	EventConversationHumanResolved  = "conversation.human_resolved"
	EventConversationAIResumed      = "conversation.ai_resumed"
	EventCustomerCreated            = "customer.created"

	SubjectReservation  = "reservation"
	SubjectConversation = "conversation"
	SubjectCustomer     = "customer"

	MaxNameLen        = 120
	MaxDescriptionLen = 2000
	MaxConfigBytes    = 8192
	MaxConditions     = 20
	MaxTemplateLen    = 4000

	RefReservationStartAt = "reservation.start_at"

	OpEQ        = "EQ"
	OpNEQ       = "NEQ"
	OpIN        = "IN"
	OpNOTIN     = "NOT_IN"
	OpGT        = "GT"
	OpGTE       = "GTE"
	OpLT        = "LT"
	OpLTE       = "LTE"
	OpEXISTS    = "EXISTS"
	OpNOTExists = "NOT_EXISTS"
)

// Automation is a tenant-scoped rule: one trigger, a condition list, one action.
type Automation struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	Name          string
	Description   *string
	Status        string
	TriggerType   string
	TriggerConfig json.RawMessage
	Conditions    json.RawMessage
	ActionType    string
	ActionConfig  json.RawMessage
	CreatedBy     *uuid.UUID
	UpdatedBy     *uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Event is a durable domain event used for evaluation and idempotency.
type Event struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	Type           string
	SubjectType    string
	SubjectID      uuid.UUID
	IdempotencyKey string
	Payload        map[string]any
	OccurredAt     time.Time
	CreatedAt      time.Time
}

// Run is one execution of an automation against an event or scheduled key.
type Run struct {
	ID           uuid.UUID
	BusinessID   uuid.UUID
	AutomationID uuid.UUID
	EventID      *uuid.UUID
	ExecutionKey *string
	SubjectType  string
	SubjectID    uuid.UUID
	Status       string
	ScheduledAt  *time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	FailedAt     *time.Time
	Attempt      int
	ErrorCode    *string
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type TriggerConfig struct {
	OffsetMinutes   *int   `json:"offset_minutes,omitempty"`
	Reference       string `json:"reference,omitempty"`
	InactiveMinutes *int   `json:"inactive_minutes,omitempty"`
}

type ActionConfig struct {
	Template    string `json:"template,omitempty"`
	Instruction string `json:"instruction,omitempty"`
}

type Settings struct {
	Enabled            bool
	SchedulerInterval  time.Duration
	MaxRunsPerTick     int
	DefaultInactiveMin int
}

type ReservationCandidate struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	CustomerID uuid.UUID
	Status     string
	PartySize  int
	StartAt    time.Time
}

type ConversationCandidate struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	CustomerID    uuid.UUID
	IdentityID    *uuid.UUID
	Status        string
	Channel       string
	LastInboundAt time.Time
}
