package automation

import (
	"context"
	"time"
)

func (s *Service) RunScheduler(ctx context.Context) error {
	if !s.settings.Enabled {
		<-ctx.Done()
		return nil
	}
	ticker := time.NewTicker(s.settings.SchedulerInterval)
	defer ticker.Stop()
	_ = s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("automation scheduler tick failed", "error", err)
			}
		}
	}
}

func (s *Service) Tick(ctx context.Context) error {
	if !s.settings.Enabled {
		return nil
	}
	locked, unlock, err := s.store.TrySchedulerLock(ctx)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer unlock()

	autos, err := s.store.ListActiveScheduled(ctx)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	created := 0
	for _, auto := range autos {
		if created >= s.settings.MaxRunsPerTick {
			break
		}
		n, err := s.scheduleAutomation(ctx, auto, now, s.settings.MaxRunsPerTick-created)
		if err != nil {
			s.log.Error("automation schedule failed", "automation_id", auto.ID, "error", err)
			continue
		}
		created += n
	}
	return nil
}

func (s *Service) scheduleAutomation(ctx context.Context, auto Automation, now time.Time, remaining int) (int, error) {
	switch auto.TriggerType {
	case TriggerScheduled:
		return s.scheduleReservationReminders(ctx, auto, now, remaining)
	case TriggerConversationInactive:
		return s.scheduleInactiveFollowups(ctx, auto, now, remaining)
	default:
		return 0, nil
	}
}

func (s *Service) scheduleReservationReminders(ctx context.Context, auto Automation, now time.Time, remaining int) (int, error) {
	cfg, err := parseTriggerConfig(auto.TriggerConfig)
	if err != nil || cfg.OffsetMinutes == nil {
		return 0, err
	}
	offset := *cfg.OffsetMinutes
	dueBefore := now.Add(-time.Duration(offset) * time.Minute)
	lookback := dueBefore.Add(-7 * 24 * time.Hour)
	candidates, err := s.store.ListDueReservations(ctx, auto.BusinessID, dueBefore, lookback, remaining)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, cand := range candidates {
		if created >= remaining {
			break
		}
		key := ScheduledReservationKey(cand.ID, offset)
		exists, err := s.store.HasRunForKey(ctx, auto.ID, key)
		if err != nil || exists {
			continue
		}
		evalCtx := map[string]any{
			"reservation": map[string]any{
				"status":     cand.Status,
				"party_size": cand.PartySize,
				"start_at":   cand.StartAt.UTC().Format(time.RFC3339),
			},
		}
		ok, err := EvaluateConditions(auto.Conditions, evalCtx)
		if err != nil || !ok {
			continue
		}
		scheduledAt := ScheduledAt(cand.StartAt, offset)
		if _, inserted, err := s.createScheduledRun(ctx, auto, SubjectReservation, cand.ID, key, scheduledAt); err != nil {
			s.log.Error("reservation reminder run failed", "automation_id", auto.ID, "reservation_id", cand.ID, "error", err)
			continue
		} else if inserted {
			created++
		}
	}
	return created, nil
}

func (s *Service) scheduleInactiveFollowups(ctx context.Context, auto Automation, now time.Time, remaining int) (int, error) {
	cfg, err := parseTriggerConfig(auto.TriggerConfig)
	if err != nil || cfg.InactiveMinutes == nil {
		return 0, err
	}
	inactive := *cfg.InactiveMinutes
	inactiveSince := now.Add(-time.Duration(inactive) * time.Minute)
	candidates, err := s.store.ListInactiveConversations(ctx, auto.BusinessID, inactiveSince, remaining)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, cand := range candidates {
		if created >= remaining {
			break
		}
		if !IsInactiveDue(cand.LastInboundAt, inactive, now) {
			continue
		}
		key := ScheduledInactiveKey(cand.ID, cand.LastInboundAt)
		exists, err := s.store.HasRunForKey(ctx, auto.ID, key)
		if err != nil || exists {
			continue
		}
		evalCtx := map[string]any{
			"conversation": map[string]any{
				"status":  cand.Status,
				"channel": cand.Channel,
			},
		}
		ok, err := EvaluateConditions(auto.Conditions, evalCtx)
		if err != nil || !ok {
			continue
		}
		if _, inserted, err := s.createScheduledRun(ctx, auto, SubjectConversation, cand.ID, key, now); err != nil {
			s.log.Error("inactive follow-up run failed", "automation_id", auto.ID, "conversation_id", cand.ID, "error", err)
			continue
		} else if inserted {
			created++
		}
	}
	return created, nil
}
