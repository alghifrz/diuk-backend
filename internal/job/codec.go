package job

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type envelope struct {
	JobID       string          `json:"job_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	CreatedAt   time.Time       `json:"created_at"`
	AvailableAt time.Time       `json:"available_at"`
}

func Encode(job Job) ([]byte, error) {
	if err := validateType(job.Type); err != nil {
		return nil, err
	}
	return json.Marshal(envelope{
		JobID:       job.ID.String(),
		Type:        string(job.Type),
		Payload:     job.Payload,
		Attempt:     job.Attempt,
		MaxAttempts: job.MaxAttempts,
		CreatedAt:   job.CreatedAt.UTC(),
		AvailableAt: job.AvailableAt.UTC(),
	})
}

func Decode(raw []byte) (Job, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Job{}, ErrInvalidPayload
	}
	id, err := uuid.Parse(strings.TrimSpace(env.JobID))
	if err != nil {
		return Job{}, ErrInvalidPayload
	}
	jobType := Type(strings.ToUpper(strings.TrimSpace(env.Type)))
	if err := validateType(jobType); err != nil {
		return Job{}, err
	}
	if len(env.Payload) == 0 {
		return Job{}, ErrInvalidPayload
	}
	max := env.MaxAttempts
	if max < 1 {
		max = 3
	}
	return Job{
		ID:          id,
		Type:        jobType,
		Payload:     env.Payload,
		Attempt:     env.Attempt,
		MaxAttempts: max,
		CreatedAt:   env.CreatedAt.UTC(),
		AvailableAt: env.AvailableAt.UTC(),
	}, nil
}

func ParseAIGenerate(payload json.RawMessage) (AIGeneratePayload, error) {
	var out AIGeneratePayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return AIGeneratePayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.ConversationID == uuid.Nil || out.MessageID == uuid.Nil {
		return AIGeneratePayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseMessageDelivery(payload json.RawMessage) (MessageDeliveryPayload, error) {
	var out MessageDeliveryPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return MessageDeliveryPayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.MessageID == uuid.Nil {
		return MessageDeliveryPayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseKnowledgeIngest(payload json.RawMessage) (KnowledgeIngestPayload, error) {
	var out KnowledgeIngestPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return KnowledgeIngestPayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.DocumentID == uuid.Nil {
		return KnowledgeIngestPayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseAutomationEvaluate(payload json.RawMessage) (AutomationEvaluatePayload, error) {
	var out AutomationEvaluatePayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return AutomationEvaluatePayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.EventID == uuid.Nil {
		return AutomationEvaluatePayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseAutomationRun(payload json.RawMessage) (AutomationRunPayload, error) {
	var out AutomationRunPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return AutomationRunPayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.RunID == uuid.Nil {
		return AutomationRunPayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseCampaignStart(payload json.RawMessage) (CampaignStartPayload, error) {
	var out CampaignStartPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return CampaignStartPayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.RunID == uuid.Nil {
		return CampaignStartPayload{}, ErrInvalidPayload
	}
	return out, nil
}

func ParseCampaignDispatch(payload json.RawMessage) (CampaignDispatchPayload, error) {
	var out CampaignDispatchPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return CampaignDispatchPayload{}, ErrInvalidPayload
	}
	if out.BusinessID == uuid.Nil || out.RunID == uuid.Nil {
		return CampaignDispatchPayload{}, ErrInvalidPayload
	}
	return out, nil
}

func validateType(jobType Type) error {
	switch jobType {
	case TypeAIGenerate, TypeMessageDelivery, TypeKnowledgeIngest, TypeAutomationEvaluate, TypeAutomationRun, TypeCampaignStart, TypeCampaignDispatch:
		return nil
	default:
		return ErrUnknownJob
	}
}
