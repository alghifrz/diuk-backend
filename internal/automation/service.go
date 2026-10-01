package automation

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

type Service struct {
	log           *slog.Logger
	store         Store
	publisher     EventPublisher
	delivery      DeliveryEnqueuer
	conversations Conversations
	customers     Customers
	reservations  Reservations
	businesses    Businesses
	ai            AIGenerator
	settings      Settings
	meter         usage.Meter
	now           func() time.Time
}

func NewService(
	log *slog.Logger,
	store Store,
	publisher EventPublisher,
	delivery DeliveryEnqueuer,
	conversations Conversations,
	customers Customers,
	reservations Reservations,
	businesses Businesses,
	ai AIGenerator,
	settings Settings,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if settings.SchedulerInterval <= 0 {
		settings.SchedulerInterval = 30 * time.Second
	}
	if settings.MaxRunsPerTick < 1 {
		settings.MaxRunsPerTick = 50
	}
	if settings.DefaultInactiveMin < 1 {
		settings.DefaultInactiveMin = 120
	}
	return &Service{
		log: log, store: store, publisher: publisher, delivery: delivery,
		conversations: conversations, customers: customers, reservations: reservations,
		businesses: businesses, ai: ai, settings: settings, now: time.Now,
	}
}

func (s *Service) SetUsageMeter(m usage.Meter) { s.meter = m }

func (s *Service) Create(ctx context.Context, businessID, actorID uuid.UUID, in CreateAutomationInput) (*Automation, error) {
	normalized, err := normalizeCreate(in)
	if err != nil {
		return nil, err
	}
	return s.store.CreateAutomation(ctx, Automation{
		BusinessID:    businessID,
		Name:          normalized.Name,
		Description:   normalized.Description,
		Status:        StatusInactive,
		TriggerType:   normalized.TriggerType,
		TriggerConfig: normalized.TriggerConfig,
		Conditions:    normalized.Conditions,
		ActionType:    normalized.ActionType,
		ActionConfig:  normalized.ActionConfig,
		CreatedBy:     ptrUUID(actorID),
		UpdatedBy:     ptrUUID(actorID),
	})
}

func (s *Service) Get(ctx context.Context, businessID, automationID uuid.UUID) (*Automation, error) {
	return s.store.GetAutomation(ctx, businessID, automationID)
}

func (s *Service) List(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]Automation, error) {
	status := strings.ToUpper(strings.TrimSpace(statusRaw))
	if status != "" && status != StatusActive && status != StatusInactive && status != StatusArchived {
		return nil, invalid("status is invalid")
	}
	return s.store.ListAutomations(ctx, businessID, status)
}

func (s *Service) Update(ctx context.Context, businessID, automationID, actorID uuid.UUID, in UpdateAutomationInput) (*Automation, error) {
	current, err := s.store.GetAutomation(ctx, businessID, automationID)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusArchived {
		return nil, invalid("archived automations cannot be updated")
	}
	next := *current
	if in.Name != nil {
		name, err := normalizeName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		description, err := normalizeDescription(in.Description)
		if err != nil {
			return nil, err
		}
		next.Description = description
	}
	if in.TriggerType != nil {
		triggerType, err := normalizeTriggerType(*in.TriggerType)
		if err != nil {
			return nil, err
		}
		next.TriggerType = triggerType
	}
	if len(in.TriggerConfig) > 0 || in.TriggerType != nil {
		raw := in.TriggerConfig
		if len(raw) == 0 {
			raw = next.TriggerConfig
		}
		cfg, err := normalizeTriggerConfig(next.TriggerType, raw)
		if err != nil {
			return nil, err
		}
		next.TriggerConfig = cfg
	}
	if len(in.Conditions) > 0 {
		conditions, err := normalizeConditions(in.Conditions)
		if err != nil {
			return nil, err
		}
		next.Conditions = conditions
	}
	if in.ActionType != nil {
		actionType, err := normalizeActionType(*in.ActionType)
		if err != nil {
			return nil, err
		}
		next.ActionType = actionType
	}
	if len(in.ActionConfig) > 0 || in.ActionType != nil {
		raw := in.ActionConfig
		if len(raw) == 0 {
			raw = next.ActionConfig
		}
		cfg, err := normalizeActionConfig(next.ActionType, raw)
		if err != nil {
			return nil, err
		}
		next.ActionConfig = cfg
	}
	next.UpdatedBy = ptrUUID(actorID)
	return s.store.UpdateAutomation(ctx, next)
}

func (s *Service) Activate(ctx context.Context, businessID, automationID, actorID uuid.UUID) (*Automation, error) {
	return s.setStatus(ctx, businessID, automationID, actorID, StatusActive)
}

func (s *Service) Deactivate(ctx context.Context, businessID, automationID, actorID uuid.UUID) (*Automation, error) {
	return s.setStatus(ctx, businessID, automationID, actorID, StatusInactive)
}

func (s *Service) Archive(ctx context.Context, businessID, automationID, actorID uuid.UUID) (*Automation, error) {
	item, err := s.setStatus(ctx, businessID, automationID, actorID, StatusArchived)
	if err != nil {
		return nil, err
	}
	_ = s.store.CancelPendingRuns(ctx, businessID, automationID)
	return item, nil
}

func (s *Service) ListRuns(ctx context.Context, businessID, automationID uuid.UUID) ([]Run, error) {
	if _, err := s.store.GetAutomation(ctx, businessID, automationID); err != nil {
		return nil, err
	}
	return s.store.ListRuns(ctx, businessID, automationID, 50)
}

func (s *Service) setStatus(ctx context.Context, businessID, automationID, actorID uuid.UUID, status string) (*Automation, error) {
	current, err := s.store.GetAutomation(ctx, businessID, automationID)
	if err != nil {
		return nil, err
	}
	current.Status = status
	current.UpdatedBy = ptrUUID(actorID)
	return s.store.UpdateAutomation(ctx, *current)
}

func (s *Service) Publish(ctx context.Context, event Event) error {
	if !s.settings.Enabled {
		return nil
	}
	if event.BusinessID == uuid.Nil || strings.TrimSpace(event.Type) == "" || strings.TrimSpace(event.IdempotencyKey) == "" {
		return invalid("event is incomplete")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	persisted, created, err := s.store.InsertEvent(ctx, event)
	if err != nil {
		return err
	}
	if !created || persisted == nil || s.publisher == nil {
		return nil
	}
	return s.publisher.EnqueueEvaluate(ctx, persisted.BusinessID, persisted.ID)
}

func (s *Service) ReservationConfirmed(ctx context.Context, res *reservation.Reservation) error {
	if res == nil {
		return nil
	}
	return s.Publish(ctx, EventFromReservation(res, EventReservationConfirmed))
}

func (s *Service) ReservationCancelled(ctx context.Context, res *reservation.Reservation) error {
	if res == nil {
		return nil
	}
	return s.Publish(ctx, EventFromReservation(res, EventReservationCancelled))
}

func (s *Service) ConversationCreated(ctx context.Context, conv *conversation.Conversation) error {
	if conv == nil {
		return nil
	}
	return s.Publish(ctx, EventFromConversation(conv))
}

func (s *Service) ConversationHumanRequested(ctx context.Context, conv *conversation.Conversation) error {
	return s.publishHandling(ctx, conv, EventConversationHumanRequested)
}

func (s *Service) ConversationHumanStarted(ctx context.Context, conv *conversation.Conversation) error {
	return s.publishHandling(ctx, conv, EventConversationHumanStarted)
}

func (s *Service) ConversationHumanResolved(ctx context.Context, conv *conversation.Conversation) error {
	return s.publishHandling(ctx, conv, EventConversationHumanResolved)
}

func (s *Service) ConversationAIResumed(ctx context.Context, conv *conversation.Conversation) error {
	return s.publishHandling(ctx, conv, EventConversationAIResumed)
}

func (s *Service) publishHandling(ctx context.Context, conv *conversation.Conversation, eventType string) error {
	if conv == nil {
		return nil
	}
	return s.Publish(ctx, EventFromConversationHandling(conv, eventType))
}

func (s *Service) CustomerCreated(ctx context.Context, cust *customer.Customer) error {
	if cust == nil {
		return nil
	}
	return s.Publish(ctx, EventFromCustomer(cust))
}

func (s *Service) EvaluateEvent(ctx context.Context, businessID, eventID uuid.UUID) error {
	if !s.settings.Enabled {
		return nil
	}
	event, err := s.store.GetEvent(ctx, businessID, eventID)
	if err != nil {
		return err
	}
	triggerType := triggerForEvent(event.Type)
	if triggerType == "" {
		return nil
	}
	autos, err := s.store.ListActiveByTrigger(ctx, businessID, triggerType)
	if err != nil {
		return err
	}
	evalCtx, err := s.eventContext(ctx, event)
	if err != nil {
		return err
	}
	for _, auto := range autos {
		ok, err := EvaluateConditions(auto.Conditions, evalCtx)
		if err != nil || !ok {
			continue
		}
		if _, err := s.createEventRun(ctx, auto, event); err != nil {
			s.log.Error("automation run create failed", "automation_id", auto.ID, "event_id", event.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) createEventRun(ctx context.Context, auto Automation, event *Event) (*Run, error) {
	run, created, err := s.store.InsertRun(ctx, Run{
		BusinessID:   auto.BusinessID,
		AutomationID: auto.ID,
		EventID:      &event.ID,
		SubjectType:  event.SubjectType,
		SubjectID:    event.SubjectID,
		Status:       RunPending,
	})
	if err != nil {
		return nil, err
	}
	if !created || run == nil || s.publisher == nil {
		return run, nil
	}
	if err := s.publisher.EnqueueRun(ctx, run.BusinessID, run.ID); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) createScheduledRun(ctx context.Context, auto Automation, subjectType string, subjectID uuid.UUID, key string, scheduledAt time.Time) (*Run, bool, error) {
	run, created, err := s.store.InsertRun(ctx, Run{
		BusinessID:   auto.BusinessID,
		AutomationID: auto.ID,
		ExecutionKey: &key,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		Status:       RunPending,
		ScheduledAt:  &scheduledAt,
	})
	if err != nil {
		return nil, false, err
	}
	if !created || run == nil || s.publisher == nil {
		return run, created, nil
	}
	return run, created, s.publisher.EnqueueRun(ctx, run.BusinessID, run.ID)
}

func (s *Service) eventContext(ctx context.Context, event *Event) (map[string]any, error) {
	out := map[string]any{}
	if event.Payload != nil {
		switch event.Type {
		case EventReservationConfirmed, EventReservationCancelled:
			out["reservation"] = map[string]any{
				"status":     event.Payload["status"],
				"party_size": event.Payload["party_size"],
				"start_at":   event.Payload["start_at"],
			}
		case EventConversationCreated:
			out["conversation"] = map[string]any{
				"status":  event.Payload["status"],
				"channel": event.Payload["channel"],
			}
		case EventCustomerCreated:
			out["customer"] = map[string]any{"status": event.Payload["status"]}
		}
	}
	if event.SubjectType == SubjectReservation && event.SubjectID != uuid.Nil && s.reservations != nil {
		res, err := s.reservations.Get(ctx, event.BusinessID, event.SubjectID)
		if err == nil && res != nil {
			out["reservation"] = map[string]any{
				"status":     string(res.Status),
				"party_size": res.PartySize,
				"start_at":   res.StartAt.UTC().Format(time.RFC3339),
			}
		}
	}
	if event.SubjectType == SubjectConversation && event.SubjectID != uuid.Nil && s.conversations != nil {
		conv, err := s.conversations.GetConversation(ctx, event.BusinessID, event.SubjectID)
		if err == nil && conv != nil {
			out["conversation"] = map[string]any{
				"status":  string(conv.Status),
				"channel": string(conv.Channel),
			}
		}
	}
	if event.SubjectType == SubjectCustomer && event.SubjectID != uuid.Nil && s.customers != nil {
		cust, err := s.customers.GetCustomer(ctx, event.BusinessID, event.SubjectID)
		if err == nil && cust != nil {
			out["customer"] = map[string]any{"status": string(cust.Status)}
		}
	}
	return out, nil
}

func triggerForEvent(eventType string) string {
	switch eventType {
	case EventReservationConfirmed:
		return TriggerReservationConfirmed
	case EventReservationCancelled:
		return TriggerReservationCancelled
	case EventConversationCreated:
		return TriggerConversationCreated
	case EventCustomerCreated:
		return TriggerCustomerCreated
	default:
		return ""
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func isSkip(err error) bool {
	var skipped skippedError
	return errors.As(err, &skipped)
}
