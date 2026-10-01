package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

type Publisher interface {
	EnqueueCampaignStart(ctx context.Context, businessID, runID uuid.UUID) error
	EnqueueCampaignDispatch(ctx context.Context, businessID, runID uuid.UUID) error
}

type BusinessLookup interface {
	ActiveTimezone(ctx context.Context, businessID uuid.UUID) (string, error)
}

type ConversationAPI interface {
	CreateConversation(ctx context.Context, businessID uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error)
	CreateMessage(ctx context.Context, businessID, conversationID uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error)
}

type Service struct {
	log           *slog.Logger
	store         Store
	publish       Publisher
	business      BusinessLookup
	conversations ConversationAPI
	settings      Settings
	metrics       Metrics
	meter         usage.Meter
	now           func() time.Time
}

func NewService(log *slog.Logger, store Store, publish Publisher, business BusinessLookup, conversations ConversationAPI, settings Settings) *Service {
	if log == nil {
		log = slog.Default()
	}
	if settings.BatchSize < 1 {
		settings.BatchSize = 50
	}
	if settings.DispatchInterval <= 0 {
		settings.DispatchInterval = 200 * time.Millisecond
	}
	if settings.SchedulerInterval <= 0 {
		settings.SchedulerInterval = 30 * time.Second
	}
	if settings.MaxStartsPerTick < 1 {
		settings.MaxStartsPerTick = 20
	}
	return &Service{
		log: log, store: store, publish: publish, business: business,
		conversations: conversations, settings: settings, now: time.Now,
	}
}

func (s *Service) SetMetrics(m Metrics) { s.metrics = m }

func (s *Service) SetUsageMeter(m usage.Meter) { s.meter = m }

func (s *Service) List(ctx context.Context, businessID uuid.UUID, status, limitRaw, offsetRaw string) ([]Campaign, error) {
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && !isKnownCampaignStatus(status) {
		return nil, invalid("status is invalid")
	}
	return s.store.ListCampaigns(ctx, businessID, status, limit, offset)
}

func (s *Service) Get(ctx context.Context, businessID, campaignID uuid.UUID) (*Campaign, error) {
	return s.store.GetCampaign(ctx, businessID, campaignID)
}

func (s *Service) Create(ctx context.Context, businessID, userID uuid.UUID, in CreateCampaignInput) (*Campaign, error) {
	if err := s.requireActive(ctx, businessID); err != nil {
		return nil, err
	}
	c, err := s.buildCampaign(businessID, &userID, in)
	if err != nil {
		return nil, err
	}
	c.Status = StatusDraft
	if in.ScheduledAt != nil && strings.TrimSpace(*in.ScheduledAt) != "" {
		return nil, invalid("use the schedule action to set scheduled_at")
	}
	return s.store.CreateCampaign(ctx, c)
}

func (s *Service) Update(ctx context.Context, businessID, campaignID, userID uuid.UUID, in UpdateCampaignInput) (*Campaign, error) {
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusDraft {
		return nil, badTransition("only draft campaigns can be edited")
	}
	next := *current
	if in.Name != nil {
		name, err := parseName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		desc, err := parseOptionalText(in.Description, 2000, "description")
		if err != nil {
			return nil, err
		}
		next.Description = desc
	}
	if in.MessageKind != nil {
		kind, err := parseKind(*in.MessageKind)
		if err != nil {
			return nil, err
		}
		next.MessageKind = kind
	}
	if in.TemplateName != nil {
		next.TemplateName, err = parseOptionalText(in.TemplateName, maxTemplateName, "template_name")
		if err != nil {
			return nil, err
		}
	}
	if in.TemplateLanguage != nil {
		next.TemplateLanguage, err = parseOptionalText(in.TemplateLanguage, maxTemplateLang, "template_language")
		if err != nil {
			return nil, err
		}
	}
	if in.TemplateParameters != nil {
		next.TemplateParameters = in.TemplateParameters
	}
	if in.Audience != nil {
		aud, err := parseAudience(in.Audience)
		if err != nil {
			return nil, err
		}
		next.AudienceDefinition = aud
	}
	if in.MessageBody != nil {
		next.MessageBody, err = parseOptionalText(in.MessageBody, maxBodyLen, "message_body")
		if err != nil {
			return nil, err
		}
	}
	if err := validatePayload(next); err != nil {
		return nil, err
	}
	next.UpdatedBy = &userID
	return s.store.UpdateCampaign(ctx, next)
}

func (s *Service) Delete(ctx context.Context, businessID, campaignID, userID uuid.UUID) (*Campaign, error) {
	return s.Cancel(ctx, businessID, campaignID, userID)
}

func (s *Service) Schedule(ctx context.Context, businessID, campaignID, userID uuid.UUID, scheduledAt string) (*Campaign, error) {
	if err := s.requireActive(ctx, businessID); err != nil {
		return nil, err
	}
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusDraft && current.Status != StatusScheduled {
		return nil, badTransition("campaign cannot be scheduled from " + current.Status)
	}
	when, err := parseScheduledAt(&scheduledAt, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if when == nil {
		return nil, invalid("scheduled_at is required")
	}
	if err := validatePayload(*current); err != nil {
		return nil, err
	}
	next := *current
	next.Status = StatusScheduled
	next.ScheduledAt = when
	next.UpdatedBy = &userID
	return s.store.UpdateCampaign(ctx, next)
}

func (s *Service) Start(ctx context.Context, businessID, campaignID, userID uuid.UUID) (*Campaign, error) {
	if err := s.requireActive(ctx, businessID); err != nil {
		return nil, err
	}
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusRunning {
		if run, runErr := s.store.GetActiveRun(ctx, businessID, campaignID); runErr == nil && s.publish != nil {
			_ = s.publish.EnqueueCampaignStart(ctx, businessID, run.ID)
		}
		return current, nil
	}
	if current.Status != StatusDraft && current.Status != StatusScheduled && current.Status != StatusPaused {
		return nil, badTransition("campaign cannot be started from " + current.Status)
	}
	if err := validatePayload(*current); err != nil {
		return nil, err
	}
	run, _, err := s.store.CreateRun(ctx, Run{BusinessID: businessID, CampaignID: campaignID, Status: RunPending})
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	next := *current
	next.Status = StatusRunning
	if next.StartedAt == nil {
		next.StartedAt = &now
	}
	next.PausedAt = nil
	next.UpdatedBy = &userID
	updated, err := s.store.UpdateCampaign(ctx, next)
	if err != nil {
		return nil, err
	}
	if s.publish != nil {
		if err := s.publish.EnqueueCampaignStart(ctx, businessID, run.ID); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func (s *Service) Pause(ctx context.Context, businessID, campaignID, userID uuid.UUID) (*Campaign, error) {
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusScheduled && current.Status != StatusRunning {
		if current.Status == StatusPaused {
			return current, nil
		}
		return nil, badTransition("campaign cannot be paused from " + current.Status)
	}
	now := s.now().UTC()
	next := *current
	next.Status = StatusPaused
	next.PausedAt = &now
	next.UpdatedBy = &userID
	updated, err := s.store.UpdateCampaign(ctx, next)
	if err != nil {
		return nil, err
	}
	if run, runErr := s.store.GetActiveRun(ctx, businessID, campaignID); runErr == nil {
		run.Status = RunPaused
		run.PausedAt = &now
		_, _ = s.store.UpdateRun(ctx, *run)
	}
	return updated, nil
}

func (s *Service) Resume(ctx context.Context, businessID, campaignID, userID uuid.UUID) (*Campaign, error) {
	if err := s.requireActive(ctx, businessID); err != nil {
		return nil, err
	}
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusPaused {
		return nil, badTransition("campaign cannot be resumed from " + current.Status)
	}
	run, runErr := s.store.GetActiveRun(ctx, businessID, campaignID)
	now := s.now().UTC()
	next := *current
	next.Status = StatusRunning
	next.UpdatedBy = &userID
	next.PausedAt = nil
	if runErr == nil {
		run.Status = RunRunning
		run.PausedAt = nil
		if run.StartedAt == nil {
			run.StartedAt = &now
		}
		if _, err := s.store.UpdateRun(ctx, *run); err != nil {
			return nil, err
		}
		if s.publish != nil {
			_ = s.publish.EnqueueCampaignDispatch(ctx, businessID, run.ID)
		}
	} else {
		run, _, err := s.store.CreateRun(ctx, Run{BusinessID: businessID, CampaignID: campaignID, Status: RunPending})
		if err != nil {
			return nil, err
		}
		if s.publish != nil {
			_ = s.publish.EnqueueCampaignStart(ctx, businessID, run.ID)
		}
	}
	return s.store.UpdateCampaign(ctx, next)
}

func (s *Service) Cancel(ctx context.Context, businessID, campaignID, userID uuid.UUID) (*Campaign, error) {
	current, err := s.store.GetCampaign(ctx, businessID, campaignID)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusCancelled {
		return current, nil
	}
	if current.Status == StatusCompleted {
		return nil, badTransition("completed campaigns cannot be cancelled")
	}
	now := s.now().UTC()
	next := *current
	next.Status = StatusCancelled
	next.CancelledAt = &now
	next.UpdatedBy = &userID
	updated, err := s.store.UpdateCampaign(ctx, next)
	if err != nil {
		return nil, err
	}
	if run, runErr := s.store.GetActiveRun(ctx, businessID, campaignID); runErr == nil {
		_, _ = s.store.SkipPendingRecipients(ctx, businessID, run.ID, SkipCampaignCancelled, "campaign cancelled")
		run.Status = RunCancelled
		run.CancelledAt = &now
		_, _ = s.store.UpdateRun(ctx, *run)
		_, _ = s.store.RefreshRunCounts(ctx, businessID, run.ID)
	}
	return updated, nil
}

func (s *Service) ListRuns(ctx context.Context, businessID, campaignID uuid.UUID, limitRaw, offsetRaw string) ([]Run, error) {
	if _, err := s.store.GetCampaign(ctx, businessID, campaignID); err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListRuns(ctx, businessID, campaignID, limit, offset)
}

func (s *Service) GetRun(ctx context.Context, businessID, campaignID, runID uuid.UUID) (*Run, error) {
	if _, err := s.store.GetCampaign(ctx, businessID, campaignID); err != nil {
		return nil, err
	}
	return s.store.GetRun(ctx, businessID, campaignID, runID)
}

func (s *Service) ListRecipients(ctx context.Context, businessID, campaignID, runID uuid.UUID, status, limitRaw, offsetRaw string) ([]Recipient, error) {
	if _, err := s.store.GetRun(ctx, businessID, campaignID, runID); err != nil {
		return nil, err
	}
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw)
	if err != nil {
		return nil, err
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	return s.store.ListRecipients(ctx, businessID, runID, status, limit, offset)
}

func (s *Service) Preview(ctx context.Context, businessID uuid.UUID, channel string, raw json.RawMessage) (AudiencePreview, error) {
	ch, err := parseChannel(channel)
	if err != nil {
		return AudiencePreview{}, err
	}
	aud, err := parseAudience(raw)
	if err != nil {
		return AudiencePreview{}, err
	}
	return s.store.PreviewAudience(ctx, businessID, ch, aud)
}

func (s *Service) requireActive(ctx context.Context, businessID uuid.UUID) error {
	if s.business == nil {
		return nil
	}
	if _, err := s.business.ActiveTimezone(ctx, businessID); err != nil {
		if errors.Is(err, ErrBusinessInactive) {
			return err
		}
		return err
	}
	return nil
}

func (s *Service) buildCampaign(businessID uuid.UUID, userID *uuid.UUID, in CreateCampaignInput) (Campaign, error) {
	name, err := parseName(in.Name)
	if err != nil {
		return Campaign{}, err
	}
	ch, err := parseChannel(in.Channel)
	if err != nil {
		return Campaign{}, err
	}
	kind, err := parseKind(in.MessageKind)
	if err != nil {
		return Campaign{}, err
	}
	desc, err := parseOptionalText(in.Description, 2000, "description")
	if err != nil {
		return Campaign{}, err
	}
	body, err := parseOptionalText(in.MessageBody, maxBodyLen, "message_body")
	if err != nil {
		return Campaign{}, err
	}
	tplName, err := parseOptionalText(in.TemplateName, maxTemplateName, "template_name")
	if err != nil {
		return Campaign{}, err
	}
	tplLang, err := parseOptionalText(in.TemplateLanguage, maxTemplateLang, "template_language")
	if err != nil {
		return Campaign{}, err
	}
	if len(in.TemplateParameters) > maxTemplateParams {
		return Campaign{}, invalid("template_parameters is too long")
	}
	aud, err := parseAudience(in.Audience)
	if err != nil {
		return Campaign{}, err
	}
	c := Campaign{
		BusinessID: businessID, Name: name, Description: desc, Channel: ch, MessageKind: kind,
		TemplateName: tplName, TemplateLanguage: tplLang, TemplateParameters: in.TemplateParameters,
		AudienceType: AudienceFilter, AudienceDefinition: aud, MessageBody: body,
		CreatedBy: userID, UpdatedBy: userID,
	}
	if err := validatePayload(c); err != nil {
		return Campaign{}, err
	}
	return c, nil
}

func validatePayload(c Campaign) error {
	if c.MessageKind == KindTemplate {
		if c.TemplateName == nil || c.TemplateLanguage == nil {
			return invalid("template_name and template_language are required")
		}
	}
	if c.MessageKind == KindText && c.MessageBody == nil {
		return invalid("message_body is required for text campaigns")
	}
	return nil
}

func isKnownCampaignStatus(status string) bool {
	switch status {
	case StatusDraft, StatusScheduled, StatusRunning, StatusPaused, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}
