package usage

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlanFree     = "FREE"
	PlanStandard = "STANDARD"
	PlanPlus     = "PLUS"
	PlanPro      = "PRO"
	PlanBusiness = "BUSINESS"

	PlanActive   = "ACTIVE"
	PlanInactive = "INACTIVE"

	FeatureAIResponses        FeatureKey = "AI_RESPONSES"
	FeatureCampaignRecipients FeatureKey = "CAMPAIGN_RECIPIENTS"
	FeatureAutomationRuns     FeatureKey = "AUTOMATION_RUNS"
	FeatureKnowledgeDocuments FeatureKey = "KNOWLEDGE_DOCUMENTS"
	FeatureWhatsAppChannels   FeatureKey = "WHATSAPP_CHANNELS"

	PeriodMonthly = "MONTHLY"
	PeriodNone    = "NONE"

	SourceAIGeneration      = "AI_GENERATION"
	SourceCampaignRecipient = "CAMPAIGN_RECIPIENT"
	SourceAutomationRun     = "AUTOMATION_RUN"
)

var (
	PlanIDFree     = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	PlanIDStandard = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	PlanIDPlus     = uuid.MustParse("00000000-0000-4000-8000-000000000005")
	PlanIDPro      = uuid.MustParse("00000000-0000-4000-8000-000000000003")
	PlanIDBusiness = uuid.MustParse("00000000-0000-4000-8000-000000000004")
)

type FeatureKey string

func (k FeatureKey) String() string { return string(k) }

func (k FeatureKey) IsUsage() bool {
	switch k {
	case FeatureAIResponses, FeatureCampaignRecipients, FeatureAutomationRuns:
		return true
	default:
		return false
	}
}

func (k FeatureKey) IsResource() bool {
	switch k {
	case FeatureKnowledgeDocuments, FeatureWhatsAppChannels:
		return true
	default:
		return false
	}
}

type Plan struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Description *string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Entitlement struct {
	ID         uuid.UUID
	PlanID     uuid.UUID
	FeatureKey FeatureKey
	LimitValue *int
	Period     string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Assignment struct {
	BusinessID uuid.UUID
	Timezone   string
	PlanID     uuid.UUID
	PlanCode   string
	PlanName   string
	PlanStatus string
}

type Period struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	PeriodType  string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Timezone    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Counter struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	UsagePeriodID uuid.UUID
	FeatureKey    FeatureKey
	Used          int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Consumption struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	UsagePeriodID uuid.UUID
	FeatureKey    FeatureKey
	SourceType    string
	SourceID      uuid.UUID
	Quantity      int
	CreatedAt     time.Time
}

type ConsumeInput struct {
	Feature    FeatureKey
	Quantity   int
	SourceType string
	SourceID   uuid.UUID
}

type ConsumeResult struct {
	Used      int64
	Limit     *int
	Remaining *int64
	Duplicate bool
}

type FeatureUsage struct {
	Feature   FeatureKey
	Used      int64
	Limit     *int
	Remaining *int64
}

type Snapshot struct {
	Plan   Plan
	Period Period
	Usage  []FeatureUsage
}

type HistoryItem struct {
	Period Period
	Usage  []FeatureUsage
}

type History struct {
	Plan   Plan
	Items  []HistoryItem
	Limit  int
	Offset int
}
