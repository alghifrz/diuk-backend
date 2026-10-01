package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

func (s *Service) DispatchRun(ctx context.Context, businessID, runID uuid.UUID) error {
	started := time.Now()
	run, err := s.store.GetRunByID(ctx, businessID, runID)
	if err != nil {
		return err
	}
	campaign, err := s.store.GetCampaign(ctx, businessID, run.CampaignID)
	if err != nil {
		return err
	}
	if campaign.Status == StatusPaused || campaign.Status == StatusCancelled {
		return nil
	}
	if run.Status != RunRunning {
		return nil
	}

	stuck, err := s.store.ListQueuedWithoutMessage(ctx, businessID, run.ID, s.settings.BatchSize)
	if err != nil {
		return err
	}
	remaining := s.settings.BatchSize - len(stuck)
	batch := stuck
	if remaining > 0 {
		claimed, err := s.store.ClaimPendingRecipients(ctx, businessID, run.ID, remaining)
		if err != nil {
			return err
		}
		batch = append(batch, claimed...)
	}
	for i := range batch {
		if err := s.dispatchRecipient(ctx, campaign, &batch[i]); err != nil {
			s.log.Error("campaign recipient dispatch failed",
				"run_id", run.ID,
				"recipient_id", batch[i].ID,
				"error", err.Error(),
			)
			code := "DISPATCH_FAILED"
			reason := "dispatch failed"
			now := s.now().UTC()
			batch[i].Status = RecipientFailed
			batch[i].FailedAt = &now
			batch[i].FailureCode = &code
			batch[i].FailureReason = &reason
			_, _ = s.store.UpdateRecipient(ctx, batch[i])
			if s.metrics != nil {
				s.metrics.ObserveCampaignRecipient(campaign.Channel, RecipientFailed, "failure")
			}
		}
	}
	if _, err := s.store.RefreshRunCounts(ctx, businessID, run.ID); err != nil {
		return err
	}
	pending, err := s.store.PendingRecipientCount(ctx, businessID, run.ID)
	if err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.ObserveCampaignDispatch(campaign.Channel, time.Since(started))
	}
	if pending == 0 {
		return s.completeIfIdle(ctx, campaign, run)
	}
	if delayed, ok := s.publish.(interface {
		EnqueueCampaignDispatchAfter(ctx context.Context, businessID, runID uuid.UUID, after time.Duration) error
	}); ok {
		return delayed.EnqueueCampaignDispatchAfter(ctx, businessID, run.ID, s.settings.DispatchInterval)
	}
	if s.publish != nil {
		return s.publish.EnqueueCampaignDispatch(ctx, businessID, run.ID)
	}
	return nil
}

func (s *Service) dispatchRecipient(ctx context.Context, campaign *Campaign, rec *Recipient) error {
	if rec.MessageID != nil {
		return nil
	}
	if existing, err := s.store.FindRecipientMessage(ctx, rec.BusinessID, rec.ID); err == nil && existing != nil {
		rec.MessageID = existing
		_, err := s.store.UpdateRecipient(ctx, *rec)
		return err
	}

	optIn, identityID, externalID, err := s.store.CustomerSendable(ctx, rec.BusinessID, rec.CustomerID, campaign.Channel)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if !optIn {
		rec.Status = RecipientSkipped
		rec.SkippedAt = &now
		code := SkipMarketingOptOut
		reason := "customer opted out of marketing"
		rec.FailureCode = &code
		rec.FailureReason = &reason
		_, err := s.store.UpdateRecipient(ctx, *rec)
		if s.metrics != nil {
			s.metrics.ObserveCampaignRecipient(campaign.Channel, RecipientSkipped, "opt_out")
		}
		return err
	}
	if identityID == nil || externalID == "" {
		rec.Status = RecipientSkipped
		rec.SkippedAt = &now
		code := SkipNoIdentity
		reason := "no active channel identity"
		rec.FailureCode = &code
		rec.FailureReason = &reason
		_, err := s.store.UpdateRecipient(ctx, *rec)
		if s.metrics != nil {
			s.metrics.ObserveCampaignRecipient(campaign.Channel, RecipientSkipped, "no_identity")
		}
		return err
	}

	if s.meter != nil {
		if _, err := s.meter.Consume(ctx, rec.BusinessID, usage.ConsumeInput{
			Feature:    usage.FeatureCampaignRecipients,
			Quantity:   1,
			SourceType: usage.SourceCampaignRecipient,
			SourceID:   rec.ID,
		}); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) || errors.Is(err, usage.ErrFeatureDisabled) || errors.Is(err, usage.ErrNotEntitled) || errors.Is(err, usage.ErrPlanInactive) {
				rec.Status = RecipientFailed
				rec.FailedAt = &now
				code := "CAMPAIGN_QUOTA_EXCEEDED"
				reason := "campaign recipient quota exceeded"
				rec.FailureCode = &code
				rec.FailureReason = &reason
				_, updErr := s.store.UpdateRecipient(ctx, *rec)
				if s.metrics != nil {
					s.metrics.ObserveCampaignRecipient(campaign.Channel, RecipientFailed, "quota")
				}
				return updErr
			}
			return err
		}
	}

	if s.conversations == nil {
		return invalid("conversation service is not configured")
	}
	identity := identityID.String()
	conv, _, err := s.conversations.CreateConversation(ctx, rec.BusinessID, conversation.CreateConversationInput{
		CustomerID: rec.CustomerID.String(), CustomerIdentityID: &identity, Channel: campaign.Channel, SuppressAutomation: true,
	})
	if err != nil {
		return err
	}
	content := campaignMessageContent(campaign)
	meta, _ := json.Marshal(map[string]any{
		"campaign_id":           campaign.ID.String(),
		"campaign_run_id":       rec.CampaignRunID.String(),
		"campaign_recipient_id": rec.ID.String(),
		"template_name":         stringOrEmpty(campaign.TemplateName),
		"template_language":     stringOrEmpty(campaign.TemplateLanguage),
		"template_parameters":   campaign.TemplateParameters,
		"message_kind":          campaign.MessageKind,
	})
	msg, _, err := s.conversations.CreateMessage(ctx, rec.BusinessID, conv.ID, conversation.CreateMessageInput{
		Direction:     "OUTBOUND",
		SenderType:    string(conversation.SenderCampaign),
		MessageType:   "TEXT",
		Content:       &content,
		Status:        string(conversation.MessageQueued),
		Metadata:      meta,
		AllowCampaign: true,
	})
	if err != nil {
		return err
	}
	rec.MessageID = &msg.ID
	rec.ExternalRecipientID = &externalID
	rec.Status = RecipientQueued
	if rec.QueuedAt == nil {
		rec.QueuedAt = &now
	}
	if _, err := s.store.UpdateRecipient(ctx, *rec); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.ObserveCampaignRecipient(campaign.Channel, RecipientQueued, "queued")
	}
	return nil
}

func (s *Service) completeIfIdle(ctx context.Context, campaign *Campaign, run *Run) error {
	fresh, err := s.store.RefreshRunCounts(ctx, run.BusinessID, run.ID)
	if err != nil {
		return err
	}
	pending, err := s.store.PendingRecipientCount(ctx, run.BusinessID, run.ID)
	if err != nil || pending > 0 {
		return err
	}
	now := s.now().UTC()
	fresh.Status = RunCompleted
	fresh.CompletedAt = &now
	if _, err := s.store.UpdateRun(ctx, *fresh); err != nil {
		return err
	}
	if campaign.Status == StatusRunning {
		campaign.Status = StatusCompleted
		campaign.CompletedAt = &now
		if _, err := s.store.UpdateCampaign(ctx, *campaign); err != nil {
			return err
		}
	}
	if s.metrics != nil {
		s.metrics.ObserveCampaignRun(campaign.Channel, "completed")
	}
	return nil
}

func (s *Service) OnMessageDelivery(ctx context.Context, businessID, messageID uuid.UUID, status string) error {
	next := recipientFromMessage(status)
	if next == "" {
		return nil
	}
	rec, err := s.store.FindRecipientByMessage(ctx, businessID, messageID)
	if err != nil || rec == nil {
		return err
	}
	if rec.Status == RecipientRead || rec.Status == RecipientFailed || rec.Status == RecipientSkipped {
		return nil
	}
	now := s.now().UTC()
	rec.Status = next
	switch next {
	case RecipientSent:
		rec.SentAt = &now
	case RecipientDelivered:
		rec.DeliveredAt = &now
	case RecipientRead:
		rec.ReadAt = &now
	case RecipientFailed:
		rec.FailedAt = &now
		code := "PROVIDER_FAILED"
		rec.FailureCode = &code
	}
	if _, err := s.store.UpdateRecipient(ctx, *rec); err != nil {
		return err
	}
	_, err = s.store.RefreshRunCounts(ctx, businessID, rec.CampaignRunID)
	return err
}

func campaignMessageContent(c *Campaign) string {
	if c.MessageKind == KindText && c.MessageBody != nil {
		return *c.MessageBody
	}
	if c.TemplateName != nil {
		return *c.TemplateName
	}
	return "campaign"
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
