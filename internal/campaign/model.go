package campaign

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft     = "DRAFT"
	StatusScheduled = "SCHEDULED"
	StatusRunning   = "RUNNING"
	StatusPaused    = "PAUSED"
	StatusCompleted = "COMPLETED"
	StatusCancelled = "CANCELLED"

	RunPending   = "PENDING"
	RunRunning   = "RUNNING"
	RunPaused    = "PAUSED"
	RunCompleted = "COMPLETED"
	RunCancelled = "CANCELLED"
	RunFailed    = "FAILED"

	RecipientPending   = "PENDING"
	RecipientQueued    = "QUEUED"
	RecipientSent      = "SENT"
	RecipientDelivered = "DELIVERED"
	RecipientRead      = "READ"
	RecipientFailed    = "FAILED"
	RecipientSkipped   = "SKIPPED"

	ChannelWhatsApp = "WHATSAPP"

	KindText     = "TEXT"
	KindTemplate = "TEMPLATE"

	AudienceFilter = "FILTER"

	SkipMarketingOptOut   = "MARKETING_OPT_OUT"
	SkipNoIdentity        = "NO_CHANNEL_IDENTITY"
	SkipCampaignCancelled = "CAMPAIGN_CANCELLED"
	SkipCampaignPaused    = "CAMPAIGN_PAUSED"

	defaultListLimit  = 50
	maxListLimit      = 100
	previewSample     = 20
	maxNameLen        = 150
	maxBodyLen        = 4096
	maxTemplateName   = 512
	maxTemplateLang   = 16
	maxTemplateParams = 20
)

type Campaign struct {
	ID                 uuid.UUID
	BusinessID         uuid.UUID
	Name               string
	Description        *string
	Status             string
	Channel            string
	MessageKind        string
	TemplateName       *string
	TemplateLanguage   *string
	TemplateParameters []string
	AudienceType       string
	AudienceDefinition AudienceDefinition
	MessageBody        *string
	ScheduledAt        *time.Time
	StartedAt          *time.Time
	PausedAt           *time.Time
	CompletedAt        *time.Time
	CancelledAt        *time.Time
	CreatedBy          *uuid.UUID
	UpdatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Run struct {
	ID              uuid.UUID
	BusinessID      uuid.UUID
	CampaignID      uuid.UUID
	Status          string
	ScheduledAt     *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	PausedAt        *time.Time
	CancelledAt     *time.Time
	TotalRecipients int
	QueuedCount     int
	SentCount       int
	DeliveredCount  int
	ReadCount       int
	FailedCount     int
	SkippedCount    int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Recipient struct {
	ID                  uuid.UUID
	BusinessID          uuid.UUID
	CampaignRunID       uuid.UUID
	CustomerID          uuid.UUID
	Channel             string
	ExternalRecipientID *string
	Status              string
	MessageID           *uuid.UUID
	QueuedAt            *time.Time
	SentAt              *time.Time
	DeliveredAt         *time.Time
	ReadAt              *time.Time
	FailedAt            *time.Time
	SkippedAt           *time.Time
	FailureCode         *string
	FailureReason       *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AudienceDefinition struct {
	CustomerStatus         *string  `json:"customer_status,omitempty"`
	Channels               []string `json:"channels,omitempty"`
	Tags                   []string `json:"tags,omitempty"`
	CreatedAfter           *string  `json:"created_after,omitempty"`
	CreatedBefore          *string  `json:"created_before,omitempty"`
	HasConversation        *bool    `json:"has_conversation,omitempty"`
	LastConversationAfter  *string  `json:"last_conversation_after,omitempty"`
	LastConversationBefore *string  `json:"last_conversation_before,omitempty"`
	HasReservation         *bool    `json:"has_reservation,omitempty"`
	LastReservationStatus  *string  `json:"last_reservation_status,omitempty"`
	LastReservationAfter   *string  `json:"last_reservation_after,omitempty"`
	LastReservationBefore  *string  `json:"last_reservation_before,omitempty"`
}

type AudiencePreview struct {
	EstimatedCount int
	Sample         []AudienceSample
}

type AudienceSample struct {
	ID     uuid.UUID
	Name   string
	Status string
}

type Settings struct {
	BatchSize         int
	DispatchInterval  time.Duration
	SchedulerInterval time.Duration
	MaxStartsPerTick  int
}

type Metrics interface {
	ObserveCampaignRun(channel, result string)
	ObserveCampaignRecipient(channel, status, result string)
	ObserveCampaignDispatch(channel string, duration time.Duration)
}

func decodeAudience(raw []byte) (AudienceDefinition, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return AudienceDefinition{}, nil
	}
	var out AudienceDefinition
	if err := json.Unmarshal(raw, &out); err != nil {
		return AudienceDefinition{}, invalid("audience_definition is invalid")
	}
	return out, nil
}
