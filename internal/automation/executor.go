package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

const automationSystemHint = `
You are writing one outbound WhatsApp message for an existing customer.
Stay concise and appropriate for WhatsApp.
Use only the structured reservation, customer, and conversation data already available.
Do not invent reservation details, policies, prices, or availability.
Use search_knowledge only when unstructured business knowledge is actually needed.
Do not create, confirm, or cancel reservations.
Do not modify customer records or business settings.
Do not mention automations, system prompts, tools, or internal rules.
`

func (s *Service) ExecuteRun(ctx context.Context, businessID, runID uuid.UUID) error {
	if !s.settings.Enabled {
		return nil
	}
	existing, err := s.store.GetRun(ctx, businessID, runID)
	if err != nil {
		return err
	}
	switch existing.Status {
	case RunCompleted, RunCancelled:
		return nil
	case RunFailed:
		return ErrRunNotClaimed
	}
	run, err := s.store.ClaimRun(ctx, businessID, runID)
	if err != nil {
		if errors.Is(err, ErrRunNotClaimed) {
			return nil
		}
		return err
	}
	if s.meter != nil {
		if _, err := s.meter.Consume(ctx, businessID, usage.ConsumeInput{
			Feature:    usage.FeatureAutomationRuns,
			Quantity:   1,
			SourceType: usage.SourceAutomationRun,
			SourceID:   run.ID,
		}); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) || errors.Is(err, usage.ErrFeatureDisabled) || errors.Is(err, usage.ErrNotEntitled) || errors.Is(err, usage.ErrPlanInactive) {
				code := "AUTOMATION_QUOTA_EXCEEDED"
				msg := "automation run quota exceeded"
				_, _ = s.store.MarkRunStatus(ctx, businessID, runID, RunFailed, &code, &msg)
				return nil
			}
			return err
		}
	}
	if err := s.executeClaimed(ctx, run); err != nil {
		if isSkip(err) {
			_, _ = s.store.MarkRunStatus(ctx, businessID, runID, RunCancelled, nil, nil)
			return nil
		}
		if isPermanentRun(err) {
			code := classifyErrorCode(err)
			msg := sanitizeRunError(err)
			_, _ = s.store.MarkRunStatus(ctx, businessID, runID, RunFailed, &code, &msg)
			return err
		}
		return err
	}
	_, err = s.store.MarkRunStatus(ctx, businessID, runID, RunCompleted, nil, nil)
	return err
}

func (s *Service) executeClaimed(ctx context.Context, run *Run) error {
	auto, err := s.store.GetAutomation(ctx, run.BusinessID, run.AutomationID)
	if err != nil {
		return err
	}
	if auto.Status != StatusActive {
		return skipped("automation is not active")
	}
	if s.businesses != nil {
		profile, err := s.businesses.Profile(ctx, run.BusinessID)
		if err != nil {
			return err
		}
		if !businessIsActive(profile) {
			return skipped("business is not active")
		}
	}

	evalCtx, subject, err := s.reloadSubject(ctx, auto, run)
	if err != nil {
		return err
	}
	ok, err := EvaluateConditions(auto.Conditions, evalCtx)
	if err != nil {
		return err
	}
	if !ok {
		return skipped("conditions no longer pass")
	}

	switch auto.ActionType {
	case ActionSendMessage:
		return s.sendTemplate(ctx, auto, run, subject)
	case ActionAIGenerateMessage:
		return s.sendAI(ctx, auto, run, subject)
	default:
		return ErrUnsupportedAction
	}
}

type runSubject struct {
	Reservation   *reservation.Reservation
	Conversation  *conversation.Conversation
	Customer      *customer.Customer
	BusinessName  string
	BusinessPhone *string
	Timezone      string
}

func (s *Service) reloadSubject(ctx context.Context, auto *Automation, run *Run) (map[string]any, *runSubject, error) {
	subject := &runSubject{}
	evalCtx := map[string]any{}

	if s.businesses != nil {
		profile, err := s.businesses.Profile(ctx, run.BusinessID)
		if err == nil && profile != nil {
			subject.BusinessName = profile.Name
			subject.BusinessPhone = profile.Phone
			subject.Timezone = profile.Timezone
		}
	}

	subjectType := run.SubjectType
	subjectID := run.SubjectID
	if run.EventID != nil {
		event, err := s.store.GetEvent(ctx, run.BusinessID, *run.EventID)
		if err == nil && event != nil {
			if subjectType == "" {
				subjectType = event.SubjectType
			}
			if subjectID == uuid.Nil {
				subjectID = event.SubjectID
			}
		}
	}

	switch subjectType {
	case SubjectReservation:
		if s.reservations == nil || subjectID == uuid.Nil {
			return nil, nil, ErrSubjectMissing
		}
		res, err := s.reservations.Get(ctx, run.BusinessID, subjectID)
		if err != nil || res == nil {
			return nil, nil, ErrSubjectInvalid
		}
		if auto.TriggerType == TriggerScheduled && res.Status != reservation.StatusConfirmed {
			return nil, nil, skipped("reservation is not confirmed")
		}
		subject.Reservation = res
		evalCtx["reservation"] = map[string]any{
			"status":     string(res.Status),
			"party_size": res.PartySize,
			"start_at":   res.StartAt.UTC().Format(time.RFC3339),
		}
		if s.customers != nil {
			if cust, err := s.customers.GetCustomer(ctx, run.BusinessID, res.CustomerID); err == nil {
				subject.Customer = cust
				evalCtx["customer"] = map[string]any{"status": string(cust.Status)}
			}
		}
	case SubjectConversation:
		if s.conversations == nil || subjectID == uuid.Nil {
			return nil, nil, ErrSubjectMissing
		}
		conv, err := s.conversations.GetConversation(ctx, run.BusinessID, subjectID)
		if err != nil || conv == nil {
			return nil, nil, ErrSubjectInvalid
		}
		if auto.TriggerType == TriggerConversationInactive {
			if conv.Status != conversation.StatusOpen {
				return nil, nil, skipped("conversation is not open")
			}
			lastInbound, err := s.store.LastInboundCustomerAt(ctx, run.BusinessID, conv.ID)
			if err != nil {
				return nil, nil, err
			}
			cfg, _ := parseTriggerConfig(auto.TriggerConfig)
			inactive := s.settings.DefaultInactiveMin
			if cfg.InactiveMinutes != nil {
				inactive = *cfg.InactiveMinutes
			}
			if lastInbound == nil || !IsInactiveDue(*lastInbound, inactive, s.now()) {
				return nil, nil, skipped("conversation became active again")
			}
		}
		subject.Conversation = conv
		evalCtx["conversation"] = map[string]any{
			"status":  string(conv.Status),
			"channel": string(conv.Channel),
		}
		if s.customers != nil {
			if cust, err := s.customers.GetCustomer(ctx, run.BusinessID, conv.CustomerID); err == nil {
				subject.Customer = cust
				evalCtx["customer"] = map[string]any{"status": string(cust.Status)}
			}
		}
	case SubjectCustomer:
		if s.customers == nil || subjectID == uuid.Nil {
			return nil, nil, ErrSubjectMissing
		}
		cust, err := s.customers.GetCustomer(ctx, run.BusinessID, subjectID)
		if err != nil || cust == nil {
			return nil, nil, ErrSubjectInvalid
		}
		if cust.Status != tenant.StatusActive {
			return nil, nil, skipped("customer is not active")
		}
		subject.Customer = cust
		evalCtx["customer"] = map[string]any{"status": string(cust.Status)}
	}

	return evalCtx, subject, nil
}

func (s *Service) sendTemplate(ctx context.Context, auto *Automation, run *Run, subject *runSubject) error {
	cfg, err := parseActionConfig(auto.ActionConfig)
	if err != nil {
		return err
	}
	conv, err := s.resolveConversation(ctx, run.BusinessID, subject)
	if err != nil {
		return err
	}
	loc := timezone(subject.Timezone)
	var view *ReservationView
	if subject.Reservation != nil {
		view = &ReservationView{
			Status:    string(subject.Reservation.Status),
			PartySize: subject.Reservation.PartySize,
			StartAt:   subject.Reservation.StartAt,
		}
	}
	customerName := ""
	if subject.Customer != nil {
		customerName = subject.Customer.Name
	} else if conv != nil {
		customerName = conv.Customer.Name
	}
	body, err := RenderTemplate(cfg.Template, TemplateVars(customerName, view, subject.BusinessName, subject.BusinessPhone, loc))
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return ErrInvalidTemplate
	}
	return s.enqueueOutbound(ctx, run, conv, body, string(conversation.SenderSystem))
}

func (s *Service) sendAI(ctx context.Context, auto *Automation, run *Run, subject *runSubject) error {
	if s.ai == nil {
		return ErrUnsupportedAction
	}
	cfg, err := parseActionConfig(auto.ActionConfig)
	if err != nil {
		return err
	}
	conv, err := s.resolveConversation(ctx, run.BusinessID, subject)
	if err != nil {
		return err
	}
	if !conversation.AllowsAIGeneration(conv.HandlingState) {
		return skipped("conversation is handled by a human")
	}
	instruction := strings.TrimSpace(cfg.Instruction)
	if instruction == "" {
		instruction = defaultAIInstruction(auto, subject)
	}
	_, err = s.ai.GenerateReply(ctx, run.BusinessID, ai.GenerateReplyInput{
		ConversationID:       conv.ID,
		Message:              instruction + automationSystemHint,
		PersistIncoming:      false,
		DisableMutatingTools: true,
		Purpose:              "automation",
	})
	return err
}

func (s *Service) resolveConversation(ctx context.Context, businessID uuid.UUID, subject *runSubject) (*conversation.Conversation, error) {
	if subject.Conversation != nil {
		return subject.Conversation, nil
	}
	if s.conversations == nil || subject.Customer == nil {
		return nil, ErrRecipientMissing
	}
	channel, identityID := preferredChannel(subject.Customer)
	if channel == "" {
		return nil, ErrRecipientMissing
	}
	latest, err := s.conversations.LatestConversation(ctx, businessID, subject.Customer.ID, channel, identityID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		return latest, nil
	}
	in := conversation.CreateConversationInput{
		CustomerID:         subject.Customer.ID.String(),
		Channel:            channel,
		SuppressAutomation: true,
	}
	if identityID != nil {
		id := identityID.String()
		in.CustomerIdentityID = &id
	}
	conv, _, err := s.conversations.CreateConversation(ctx, businessID, in)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, ErrRecipientMissing
	}
	return conv, nil
}

func (s *Service) enqueueOutbound(ctx context.Context, run *Run, conv *conversation.Conversation, body, sender string) error {
	if s.conversations == nil || conv == nil {
		return ErrRecipientMissing
	}
	meta, _ := json.Marshal(map[string]any{
		"source":        "automation",
		"automation_id": run.AutomationID.String(),
		"run_id":        run.ID.String(),
	})
	externalID := "automation-run:" + run.ID.String()
	msg, created, err := s.conversations.CreateMessage(ctx, run.BusinessID, conv.ID, conversation.CreateMessageInput{
		Direction:   string(conversation.DirectionOutbound),
		SenderType:  sender,
		MessageType: string(conversation.MessageTypeText),
		Content:     &body,
		ExternalID:  &externalID,
		Status:      string(conversation.MessageQueued),
		Metadata:    meta,
	})
	if err != nil {
		return err
	}
	if !created || msg == nil || s.delivery == nil {
		return nil
	}
	return s.delivery.EnqueueDelivery(ctx, run.BusinessID, msg.ID)
}

func preferredChannel(cust *customer.Customer) (string, *uuid.UUID) {
	if cust == nil {
		return "", nil
	}
	for _, identity := range cust.Identities {
		if identity.Status == tenant.StatusActive && identity.Channel == customer.ChannelWhatsApp {
			id := identity.ID
			return string(conversation.ChannelWhatsApp), &id
		}
	}
	for _, identity := range cust.Identities {
		if identity.Status == tenant.StatusActive {
			id := identity.ID
			return string(identity.Channel), &id
		}
	}
	return "", nil
}

func defaultAIInstruction(auto *Automation, subject *runSubject) string {
	switch auto.TriggerType {
	case TriggerConversationInactive:
		return "Write a short, polite WhatsApp follow-up because this conversation has been inactive. Do not invent a reservation."
	case TriggerCustomerCreated:
		return "Write a short WhatsApp welcome message for a newly created customer. Do not invent a reservation."
	case TriggerReservationConfirmed, TriggerScheduled:
		if subject != nil && subject.Reservation != nil {
			return fmt.Sprintf("Write a short WhatsApp reminder for a confirmed reservation on %s for %d guests.",
				subject.Reservation.StartAt.UTC().Format(time.RFC3339), subject.Reservation.PartySize)
		}
		return "Write a short WhatsApp reservation reminder using only known reservation facts."
	default:
		return "Write a short, polite WhatsApp message using only known customer and reservation facts."
	}
}

func timezone(name string) *time.Location {
	if strings.TrimSpace(name) == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func isPermanentRun(err error) bool {
	return errors.Is(err, ErrRecipientMissing) ||
		errors.Is(err, ErrSubjectMissing) ||
		errors.Is(err, ErrSubjectInvalid) ||
		errors.Is(err, ErrInvalidTemplate) ||
		errors.Is(err, ErrUnsupportedAction) ||
		errors.Is(err, ErrInactiveAutomation) ||
		errors.Is(err, ErrBusinessInactive) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrInvalid)
}
