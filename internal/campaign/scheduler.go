package campaign

import (
	"context"
	"time"

	"github.com/google/uuid"
)

func (s *Service) RunScheduler(ctx context.Context) error {
	ticker := time.NewTicker(s.settings.SchedulerInterval)
	defer ticker.Stop()
	_ = s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("campaign scheduler tick failed", "error", err)
			}
		}
	}
}

func (s *Service) Tick(ctx context.Context) error {
	locked, unlock, err := s.store.TrySchedulerLock(ctx)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer unlock()

	due, err := s.store.ListDueScheduled(ctx, s.now().UTC(), s.settings.MaxStartsPerTick)
	if err != nil {
		return err
	}
	for _, item := range due {
		if err := s.enqueueDue(ctx, item); err != nil {
			s.log.Error("campaign schedule enqueue failed", "campaign_id", item.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) enqueueDue(ctx context.Context, item Campaign) error {
	run, created, err := s.store.CreateRun(ctx, Run{
		BusinessID: item.BusinessID, CampaignID: item.ID, Status: RunPending, ScheduledAt: item.ScheduledAt,
	})
	if err != nil {
		return err
	}
	if s.publish == nil {
		return nil
	}
	if err := s.publish.EnqueueCampaignStart(ctx, item.BusinessID, run.ID); err != nil {
		return err
	}
	if created {
		s.log.Info("campaign start queued",
			"campaign_id", item.ID,
			"run_id", run.ID,
			"result", "scheduled",
		)
	}
	return nil
}

func (s *Service) StartRun(ctx context.Context, businessID, runID uuid.UUID) error {
	if err := s.requireActive(ctx, businessID); err != nil {
		return err
	}
	run, err := s.store.GetRunByID(ctx, businessID, runID)
	if err != nil {
		return err
	}
	campaign, err := s.store.GetCampaign(ctx, businessID, run.CampaignID)
	if err != nil {
		return err
	}
	if campaign.Status == StatusCancelled || campaign.Status == StatusPaused {
		return nil
	}
	if run.Status == RunCancelled || run.Status == RunCompleted || run.Status == RunFailed {
		return nil
	}
	now := s.now().UTC()
	if run.Status == RunPending {
		if _, err := s.store.SnapshotRecipients(ctx, businessID, run.ID, campaign.Channel, campaign.AudienceDefinition); err != nil {
			return err
		}
		run.Status = RunRunning
		run.StartedAt = &now
		if _, err := s.store.UpdateRun(ctx, *run); err != nil {
			return err
		}
	}
	if campaign.Status == StatusDraft || campaign.Status == StatusScheduled {
		campaign.Status = StatusRunning
		campaign.StartedAt = &now
		if _, err := s.store.UpdateCampaign(ctx, *campaign); err != nil {
			return err
		}
	}
	if _, err := s.store.RefreshRunCounts(ctx, businessID, run.ID); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.ObserveCampaignRun(campaign.Channel, "started")
	}
	if s.publish != nil {
		return s.publish.EnqueueCampaignDispatch(ctx, businessID, run.ID)
	}
	return nil
}
