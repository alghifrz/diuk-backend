package usage

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type UsageMetrics interface {
	ObserveUsage(feature, result string)
}

type Service struct {
	log     *slog.Logger
	store   Store
	now     func() time.Time
	metrics UsageMetrics
}

func NewService(log *slog.Logger, store Store) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{log: log, store: store, now: time.Now}
}

func (s *Service) SetMetrics(m UsageMetrics) { s.metrics = m }

func (s *Service) Check(ctx context.Context, businessID uuid.UUID, feature FeatureKey) error {
	if feature.IsResource() {
		return s.CheckResource(ctx, businessID, feature)
	}
	usage, err := s.currentUsage(ctx, businessID, feature)
	if err != nil {
		return err
	}
	if wouldExceed(usage.Used, 1, usage.Limit) {
		s.observe(feature, "exceeded")
		return ErrQuotaExceeded
	}
	return nil
}

func (s *Service) CheckResource(ctx context.Context, businessID uuid.UUID, feature FeatureKey) error {
	if !feature.IsResource() {
		return invalid("feature is not a resource limit")
	}
	ent, err := s.entitlement(ctx, businessID, feature)
	if err != nil {
		return err
	}
	used, err := s.store.CountResource(ctx, businessID, feature)
	if err != nil {
		return err
	}
	if wouldExceed(used, 1, ent.LimitValue) {
		s.observe(feature, "exceeded")
		return ErrQuotaExceeded
	}
	return nil
}

func (s *Service) Consume(ctx context.Context, businessID uuid.UUID, in ConsumeInput) (*ConsumeResult, error) {
	if !in.Feature.IsUsage() {
		return nil, invalid("feature is not a metered usage limit")
	}
	if in.Quantity < 1 {
		in.Quantity = 1
	}
	if in.SourceType == "" || in.SourceID == uuid.Nil {
		return nil, invalid("usage source is required")
	}
	assign, err := s.assignment(ctx, businessID)
	if err != nil {
		return nil, err
	}
	ent, err := s.store.GetEntitlement(ctx, assign.PlanID, in.Feature)
	if err != nil {
		return nil, err
	}
	if !ent.Enabled {
		s.observe(in.Feature, "exceeded")
		return nil, ErrFeatureDisabled
	}
	start, end, tz, err := MonthlyPeriod(s.now(), assign.Timezone)
	if err != nil {
		return nil, err
	}
	out, err := s.store.ConsumeAtomic(ctx, atomicConsume{
		BusinessID: businessID,
		Period: Period{
			BusinessID:  businessID,
			PeriodType:  PeriodMonthly,
			PeriodStart: start,
			PeriodEnd:   end,
			Timezone:    tz,
		},
		Feature:    in.Feature,
		Quantity:   in.Quantity,
		Limit:      ent.LimitValue,
		SourceType: in.SourceType,
		SourceID:   in.SourceID,
	})
	if err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			s.observe(in.Feature, "exceeded")
		}
		return nil, err
	}
	if out.Duplicate {
		s.observe(in.Feature, "duplicate")
	} else {
		s.observe(in.Feature, "consumed")
	}
	s.log.Info("usage consumed",
		"business_id", businessID,
		"feature", in.Feature,
		"result", consumeResultLabel(out),
		"duplicate", out.Duplicate,
	)
	return out, nil
}

func (s *Service) snapshot(ctx context.Context, businessID uuid.UUID) (Snapshot, error) {
	assign, err := s.assignment(ctx, businessID)
	if err != nil {
		return Snapshot{}, err
	}
	ents, err := s.store.ListEntitlements(ctx, assign.PlanID)
	if err != nil {
		return Snapshot{}, err
	}
	period, err := s.ensureCurrentPeriod(ctx, assign)
	if err != nil {
		return Snapshot{}, err
	}
	counters, err := s.store.ListCounters(ctx, businessID, period.ID)
	if err != nil {
		return Snapshot{}, err
	}
	usedBy := map[FeatureKey]int64{}
	for _, c := range counters {
		usedBy[c.FeatureKey] = c.Used
	}
	features := make([]FeatureUsage, 0, len(ents))
	for _, ent := range ents {
		item := FeatureUsage{Feature: ent.FeatureKey, Limit: ent.LimitValue}
		if ent.FeatureKey.IsResource() {
			n, err := s.store.CountResource(ctx, businessID, ent.FeatureKey)
			if err != nil {
				return Snapshot{}, err
			}
			item.Used = n
		} else {
			item.Used = usedBy[ent.FeatureKey]
		}
		if ent.Enabled {
			item.Remaining = remaining(item.Used, ent.LimitValue)
		} else {
			zero := int64(0)
			item.Remaining = &zero
		}
		features = append(features, item)
	}
	return Snapshot{
		Plan:   Plan{ID: assign.PlanID, Code: assign.PlanCode, Name: assign.PlanName, Status: assign.PlanStatus},
		Period: *period,
		Usage:  features,
	}, nil
}

func (s *Service) History(ctx context.Context, businessID uuid.UUID, limitRaw, offsetRaw string) (History, error) {
	limit, offset, err := parseLimitOffset(limitRaw, offsetRaw, 12, 24)
	if err != nil {
		return History{}, err
	}
	assign, err := s.assignment(ctx, businessID)
	if err != nil {
		return History{}, err
	}
	ents, err := s.store.ListEntitlements(ctx, assign.PlanID)
	if err != nil {
		return History{}, err
	}
	start, _, _, err := MonthlyPeriod(s.now(), assign.Timezone)
	if err != nil {
		return History{}, err
	}
	periods, err := s.store.ListHistoryPeriods(ctx, businessID, start, limit, offset)
	if err != nil {
		return History{}, err
	}
	items := make([]HistoryItem, 0, len(periods))
	for _, p := range periods {
		counters, err := s.store.ListCounters(ctx, businessID, p.ID)
		if err != nil {
			return History{}, err
		}
		usedBy := map[FeatureKey]int64{}
		for _, c := range counters {
			usedBy[c.FeatureKey] = c.Used
		}
		features := make([]FeatureUsage, 0, len(ents))
		for _, ent := range ents {
			if !ent.FeatureKey.IsUsage() {
				continue
			}
			used := usedBy[ent.FeatureKey]
			features = append(features, FeatureUsage{
				Feature:   ent.FeatureKey,
				Used:      used,
				Limit:     ent.LimitValue,
				Remaining: remaining(used, ent.LimitValue),
			})
		}
		items = append(items, HistoryItem{Period: p, Usage: features})
	}
	return History{
		Plan:   Plan{ID: assign.PlanID, Code: assign.PlanCode, Name: assign.PlanName, Status: assign.PlanStatus},
		Items:  items,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (s *Service) assignment(ctx context.Context, businessID uuid.UUID) (*Assignment, error) {
	assign, err := s.store.GetAssignment(ctx, businessID)
	if err != nil {
		return nil, err
	}
	if assign.PlanStatus != PlanActive {
		return nil, ErrPlanInactive
	}
	return assign, nil
}

func (s *Service) entitlement(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (*Entitlement, error) {
	assign, err := s.assignment(ctx, businessID)
	if err != nil {
		return nil, err
	}
	ent, err := s.store.GetEntitlement(ctx, assign.PlanID, feature)
	if err != nil {
		return nil, err
	}
	if !ent.Enabled {
		return nil, ErrFeatureDisabled
	}
	return ent, nil
}

func (s *Service) currentUsage(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (FeatureUsage, error) {
	ent, err := s.entitlement(ctx, businessID, feature)
	if err != nil {
		return FeatureUsage{}, err
	}
	assign, err := s.assignment(ctx, businessID)
	if err != nil {
		return FeatureUsage{}, err
	}
	period, err := s.ensureCurrentPeriod(ctx, assign)
	if err != nil {
		return FeatureUsage{}, err
	}
	if _, err := s.store.EnsureCounter(ctx, Counter{BusinessID: businessID, UsagePeriodID: period.ID, FeatureKey: feature}); err != nil {
		return FeatureUsage{}, err
	}
	counter, err := s.store.GetCounter(ctx, businessID, period.ID, feature)
	if err != nil {
		return FeatureUsage{}, err
	}
	return FeatureUsage{
		Feature:   feature,
		Used:      counter.Used,
		Limit:     ent.LimitValue,
		Remaining: remaining(counter.Used, ent.LimitValue),
	}, nil
}

func (s *Service) ensureCurrentPeriod(ctx context.Context, assign *Assignment) (*Period, error) {
	start, end, tz, err := MonthlyPeriod(s.now(), assign.Timezone)
	if err != nil {
		return nil, err
	}
	return s.store.EnsurePeriod(ctx, Period{
		BusinessID:  assign.BusinessID,
		PeriodType:  PeriodMonthly,
		PeriodStart: start,
		PeriodEnd:   end,
		Timezone:    tz,
	})
}

func (s *Service) observe(feature FeatureKey, result string) {
	if s.metrics == nil {
		return
	}
	s.metrics.ObserveUsage(feature.String(), result)
}

func consumeResultLabel(out *ConsumeResult) string {
	if out == nil {
		return "error"
	}
	if out.Duplicate {
		return "duplicate"
	}
	return "consumed"
}
