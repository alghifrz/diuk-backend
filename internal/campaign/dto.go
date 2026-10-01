package campaign

import (
	"encoding/json"
	"time"
)

type CreateCampaignInput struct {
	Name               string
	Description        *string
	Channel            string
	MessageKind        string
	TemplateName       *string
	TemplateLanguage   *string
	TemplateParameters []string
	Audience           json.RawMessage
	MessageBody        *string
	ScheduledAt        *string
}

type UpdateCampaignInput struct {
	Name               *string
	Description        *string
	MessageKind        *string
	TemplateName       *string
	TemplateLanguage   *string
	TemplateParameters []string
	Audience           json.RawMessage
	MessageBody        *string
}

type createRequest struct {
	Name               string          `json:"name"`
	Description        *string         `json:"description"`
	Channel            string          `json:"channel"`
	MessageKind        string          `json:"message_kind"`
	TemplateName       *string         `json:"template_name"`
	TemplateLanguage   *string         `json:"template_language"`
	TemplateParameters []string        `json:"template_parameters"`
	Audience           json.RawMessage `json:"audience_definition"`
	MessageBody        *string         `json:"message_body"`
	ScheduledAt        *string         `json:"scheduled_at"`
}

type updateRequest struct {
	Name               *string         `json:"name"`
	Description        *string         `json:"description"`
	MessageKind        *string         `json:"message_kind"`
	TemplateName       *string         `json:"template_name"`
	TemplateLanguage   *string         `json:"template_language"`
	TemplateParameters []string        `json:"template_parameters"`
	Audience           json.RawMessage `json:"audience_definition"`
	MessageBody        *string         `json:"message_body"`
}

type scheduleRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

type previewRequest struct {
	Audience json.RawMessage `json:"audience_definition"`
	Channel  string          `json:"channel"`
}

type campaignJSON struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        *string         `json:"description"`
	Status             string          `json:"status"`
	Channel            string          `json:"channel"`
	MessageKind        string          `json:"message_kind"`
	TemplateName       *string         `json:"template_name"`
	TemplateLanguage   *string         `json:"template_language"`
	TemplateParameters []string        `json:"template_parameters"`
	AudienceType       string          `json:"audience_type"`
	AudienceDefinition json.RawMessage `json:"audience_definition"`
	MessageBody        *string         `json:"message_body"`
	ScheduledAt        *string         `json:"scheduled_at"`
	StartedAt          *string         `json:"started_at"`
	PausedAt           *string         `json:"paused_at"`
	CompletedAt        *string         `json:"completed_at"`
	CancelledAt        *string         `json:"cancelled_at"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type runJSON struct {
	ID              string  `json:"id"`
	CampaignID      string  `json:"campaign_id"`
	Status          string  `json:"status"`
	ScheduledAt     *string `json:"scheduled_at"`
	StartedAt       *string `json:"started_at"`
	CompletedAt     *string `json:"completed_at"`
	PausedAt        *string `json:"paused_at"`
	CancelledAt     *string `json:"cancelled_at"`
	TotalRecipients int     `json:"total_recipients"`
	QueuedCount     int     `json:"queued_count"`
	SentCount       int     `json:"sent_count"`
	DeliveredCount  int     `json:"delivered_count"`
	ReadCount       int     `json:"read_count"`
	FailedCount     int     `json:"failed_count"`
	SkippedCount    int     `json:"skipped_count"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type recipientJSON struct {
	ID          string  `json:"id"`
	CustomerID  string  `json:"customer_id"`
	Channel     string  `json:"channel"`
	Status      string  `json:"status"`
	MessageID   *string `json:"message_id"`
	FailureCode *string `json:"failure_code"`
	QueuedAt    *string `json:"queued_at"`
	SentAt      *string `json:"sent_at"`
	DeliveredAt *string `json:"delivered_at"`
	ReadAt      *string `json:"read_at"`
	FailedAt    *string `json:"failed_at"`
	SkippedAt   *string `json:"skipped_at"`
	CreatedAt   string  `json:"created_at"`
}

type previewJSON struct {
	EstimatedCount int               `json:"estimated_count"`
	Sample         []previewItemJSON `json:"sample_customers"`
}

type previewItemJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func ts(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func campaignResponse(c *Campaign) campaignJSON {
	raw, _ := json.Marshal(c.AudienceDefinition)
	params := c.TemplateParameters
	if params == nil {
		params = []string{}
	}
	return campaignJSON{
		ID: c.ID.String(), Name: c.Name, Description: c.Description, Status: c.Status,
		Channel: c.Channel, MessageKind: c.MessageKind, TemplateName: c.TemplateName,
		TemplateLanguage: c.TemplateLanguage, TemplateParameters: params,
		AudienceType: c.AudienceType, AudienceDefinition: raw, MessageBody: c.MessageBody,
		ScheduledAt: ts(c.ScheduledAt), StartedAt: ts(c.StartedAt), PausedAt: ts(c.PausedAt),
		CompletedAt: ts(c.CompletedAt), CancelledAt: ts(c.CancelledAt),
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func runResponse(r *Run) runJSON {
	return runJSON{
		ID: r.ID.String(), CampaignID: r.CampaignID.String(), Status: r.Status,
		ScheduledAt: ts(r.ScheduledAt), StartedAt: ts(r.StartedAt), CompletedAt: ts(r.CompletedAt),
		PausedAt: ts(r.PausedAt), CancelledAt: ts(r.CancelledAt),
		TotalRecipients: r.TotalRecipients, QueuedCount: r.QueuedCount, SentCount: r.SentCount,
		DeliveredCount: r.DeliveredCount, ReadCount: r.ReadCount, FailedCount: r.FailedCount,
		SkippedCount: r.SkippedCount,
		CreatedAt:    r.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func recipientResponse(r *Recipient) recipientJSON {
	var messageID *string
	if r.MessageID != nil {
		s := r.MessageID.String()
		messageID = &s
	}
	return recipientJSON{
		ID: r.ID.String(), CustomerID: r.CustomerID.String(), Channel: r.Channel, Status: r.Status,
		MessageID: messageID, FailureCode: r.FailureCode,
		QueuedAt: ts(r.QueuedAt), SentAt: ts(r.SentAt), DeliveredAt: ts(r.DeliveredAt),
		ReadAt: ts(r.ReadAt), FailedAt: ts(r.FailedAt), SkippedAt: ts(r.SkippedAt),
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func campaignsResponse(items []Campaign) []campaignJSON {
	out := make([]campaignJSON, 0, len(items))
	for i := range items {
		out = append(out, campaignResponse(&items[i]))
	}
	return out
}

func runsResponse(items []Run) []runJSON {
	out := make([]runJSON, 0, len(items))
	for i := range items {
		out = append(out, runResponse(&items[i]))
	}
	return out
}

func recipientsResponse(items []Recipient) []recipientJSON {
	out := make([]recipientJSON, 0, len(items))
	for i := range items {
		out = append(out, recipientResponse(&items[i]))
	}
	return out
}

func previewResponse(p AudiencePreview) previewJSON {
	sample := make([]previewItemJSON, 0, len(p.Sample))
	for _, item := range p.Sample {
		sample = append(sample, previewItemJSON{ID: item.ID.String(), Name: item.Name, Status: item.Status})
	}
	return previewJSON{EstimatedCount: p.EstimatedCount, Sample: sample}
}
