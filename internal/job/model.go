package job

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeAIGenerate         Type = "AI_GENERATE"
	TypeMessageDelivery    Type = "MESSAGE_DELIVERY"
	TypeKnowledgeIngest    Type = "KNOWLEDGE_INGEST"
	TypeAutomationEvaluate Type = "AUTOMATION_EVALUATE"
	TypeAutomationRun      Type = "AUTOMATION_RUN"
	TypeCampaignStart      Type = "CAMPAIGN_START"
	TypeCampaignDispatch   Type = "CAMPAIGN_DISPATCH"

	OutboxPending   = "PENDING"
	OutboxPublished = "PUBLISHED"
	OutboxFailed    = "FAILED"
)

type Type string

type Job struct {
	ID          uuid.UUID
	Type        Type
	Payload     json.RawMessage
	Attempt     int
	MaxAttempts int
	CreatedAt   time.Time
	AvailableAt time.Time
}

type AIGeneratePayload struct {
	BusinessID     uuid.UUID `json:"business_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
}

type MessageDeliveryPayload struct {
	BusinessID uuid.UUID `json:"business_id"`
	MessageID  uuid.UUID `json:"message_id"`
}

type KnowledgeIngestPayload struct {
	DocumentID uuid.UUID `json:"document_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

type AutomationEvaluatePayload struct {
	EventID    uuid.UUID `json:"event_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

type AutomationRunPayload struct {
	RunID      uuid.UUID `json:"run_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

type CampaignStartPayload struct {
	RunID      uuid.UUID `json:"campaign_run_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

type CampaignDispatchPayload struct {
	RunID      uuid.UUID `json:"campaign_run_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

type DeadLetter struct {
	JobID     uuid.UUID       `json:"job_id"`
	JobType   Type            `json:"job_type"`
	Payload   json.RawMessage `json:"payload"`
	Attempts  int             `json:"attempts"`
	CreatedAt time.Time       `json:"created_at"`
	FailedAt  time.Time       `json:"failed_at"`
	LastError string          `json:"last_error"`
}

type OutboxRow struct {
	ID          uuid.UUID
	JobType     Type
	Payload     json.RawMessage
	Status      string
	Attempts    int
	MaxAttempts int
	AvailableAt time.Time
	CreatedAt   time.Time
	ProcessedAt *time.Time
	LastError   *string
}

func NewAIGenerate(businessID, conversationID, messageID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(AIGeneratePayload{
		BusinessID: businessID, ConversationID: conversationID, MessageID: messageID,
	})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeAIGenerate, payload, maxAttempts), nil
}

func NewMessageDelivery(businessID, messageID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(MessageDeliveryPayload{BusinessID: businessID, MessageID: messageID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeMessageDelivery, payload, maxAttempts), nil
}

func NewKnowledgeIngest(businessID, documentID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(KnowledgeIngestPayload{BusinessID: businessID, DocumentID: documentID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeKnowledgeIngest, payload, maxAttempts), nil
}

func NewAutomationEvaluate(businessID, eventID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(AutomationEvaluatePayload{BusinessID: businessID, EventID: eventID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeAutomationEvaluate, payload, maxAttempts), nil
}

func NewAutomationRun(businessID, runID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(AutomationRunPayload{BusinessID: businessID, RunID: runID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeAutomationRun, payload, maxAttempts), nil
}

func NewCampaignStart(businessID, runID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(CampaignStartPayload{BusinessID: businessID, RunID: runID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeCampaignStart, payload, maxAttempts), nil
}

func NewCampaignDispatch(businessID, runID uuid.UUID, maxAttempts int) (Job, error) {
	payload, err := json.Marshal(CampaignDispatchPayload{BusinessID: businessID, RunID: runID})
	if err != nil {
		return Job{}, err
	}
	return newJob(TypeCampaignDispatch, payload, maxAttempts), nil
}

func newJob(jobType Type, payload json.RawMessage, maxAttempts int) Job {
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	now := time.Now().UTC()
	return Job{
		ID:          uuid.New(),
		Type:        jobType,
		Payload:     payload,
		Attempt:     0,
		MaxAttempts: maxAttempts,
		CreatedAt:   now,
		AvailableAt: now,
	}
}

func (j Job) QueueName() string {
	switch j.Type {
	case TypeAIGenerate:
		return "ai"
	case TypeMessageDelivery:
		return "delivery"
	case TypeKnowledgeIngest:
		return "knowledge"
	case TypeAutomationEvaluate, TypeAutomationRun:
		return "automation"
	case TypeCampaignStart, TypeCampaignDispatch:
		return "campaign"
	default:
		return "unknown"
	}
}
