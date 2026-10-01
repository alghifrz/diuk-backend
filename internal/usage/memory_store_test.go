package usage

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	mu           sync.Mutex
	assignments  map[uuid.UUID]Assignment
	plans        map[uuid.UUID]Plan
	entitlements map[uuid.UUID][]Entitlement
	periods      map[string]*Period
	counters     map[string]*Counter
	consumptions map[string]struct{}
	resources    map[string]int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		assignments:  map[uuid.UUID]Assignment{},
		plans:        map[uuid.UUID]Plan{},
		entitlements: map[uuid.UUID][]Entitlement{},
		periods:      map[string]*Period{},
		counters:     map[string]*Counter{},
		consumptions: map[string]struct{}{},
		resources:    map[string]int64{},
	}
}

func (m *memoryStore) seedPlan(p Plan, ents []Entitlement) {
	m.plans[p.ID] = p
	m.entitlements[p.ID] = ents
}

func (m *memoryStore) assign(businessID uuid.UUID, tz string, p Plan) {
	m.assignments[businessID] = Assignment{
		BusinessID: businessID, Timezone: tz, PlanID: p.ID,
		PlanCode: p.Code, PlanName: p.Name, PlanStatus: p.Status,
	}
}

func periodKey(businessID uuid.UUID, typ string, start time.Time) string {
	return businessID.String() + "|" + typ + "|" + start.UTC().Format(time.RFC3339Nano)
}

func counterKey(periodID uuid.UUID, feature FeatureKey) string {
	return periodID.String() + "|" + string(feature)
}

func consumptionKey(businessID uuid.UUID, feature FeatureKey, sourceType string, sourceID uuid.UUID) string {
	return businessID.String() + "|" + string(feature) + "|" + sourceType + "|" + sourceID.String()
}

func (m *memoryStore) GetAssignment(_ context.Context, businessID uuid.UUID) (*Assignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.assignments[businessID]
	if !ok {
		return nil, ErrBusinessNotFound
	}
	return &a, nil
}

func (m *memoryStore) GetPlan(_ context.Context, planID uuid.UUID) (*Plan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plans[planID]
	if !ok {
		return nil, ErrPlanNotFound
	}
	return &p, nil
}

func (m *memoryStore) ListEntitlements(_ context.Context, planID uuid.UUID) ([]Entitlement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entitlement(nil), m.entitlements[planID]...), nil
}

func (m *memoryStore) GetEntitlement(_ context.Context, planID uuid.UUID, feature FeatureKey) (*Entitlement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entitlements[planID] {
		if e.FeatureKey == feature {
			return &e, nil
		}
	}
	return nil, ErrNotEntitled
}

func (m *memoryStore) EnsurePeriod(_ context.Context, p Period) (*Period, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensurePeriodLocked(p)
}

func (m *memoryStore) ensurePeriodLocked(p Period) (*Period, error) {
	key := periodKey(p.BusinessID, p.PeriodType, p.PeriodStart)
	if existing, ok := m.periods[key]; ok {
		cp := *existing
		return &cp, nil
	}
	p.ID = uuid.New()
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	stored := p
	m.periods[key] = &stored
	cp := stored
	return &cp, nil
}

func (m *memoryStore) ListHistoryPeriods(_ context.Context, businessID uuid.UUID, beforeStart time.Time, limit, offset int) ([]Period, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Period
	for _, p := range m.periods {
		if p.BusinessID == businessID && p.PeriodType == PeriodMonthly && p.PeriodStart.Before(beforeStart) {
			all = append(all, *p)
		}
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].PeriodStart.After(all[i].PeriodStart) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if offset > len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (m *memoryStore) EnsureCounter(_ context.Context, c Counter) (*Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureCounterLocked(c)
}

func (m *memoryStore) ensureCounterLocked(c Counter) (*Counter, error) {
	key := counterKey(c.UsagePeriodID, c.FeatureKey)
	if existing, ok := m.counters[key]; ok {
		cp := *existing
		return &cp, nil
	}
	c.ID = uuid.New()
	c.Used = 0
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	stored := c
	m.counters[key] = &stored
	cp := stored
	return &cp, nil
}

func (m *memoryStore) GetCounter(_ context.Context, businessID, periodID uuid.UUID, feature FeatureKey) (*Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.counters[counterKey(periodID, feature)]; ok && existing.BusinessID == businessID {
		cp := *existing
		return &cp, nil
	}
	return &Counter{BusinessID: businessID, UsagePeriodID: periodID, FeatureKey: feature, Used: 0}, nil
}

func (m *memoryStore) ListCounters(_ context.Context, businessID, periodID uuid.UUID) ([]Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Counter
	for _, c := range m.counters {
		if c.BusinessID == businessID && c.UsagePeriodID == periodID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *memoryStore) ConsumeAtomic(_ context.Context, in atomicConsume) (*ConsumeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	period, err := m.ensurePeriodLocked(in.Period)
	if err != nil {
		return nil, err
	}
	counter, err := m.ensureCounterLocked(Counter{BusinessID: in.BusinessID, UsagePeriodID: period.ID, FeatureKey: in.Feature})
	if err != nil {
		return nil, err
	}
	ckey := consumptionKey(in.BusinessID, in.Feature, in.SourceType, in.SourceID)
	if _, ok := m.consumptions[ckey]; ok {
		return &ConsumeResult{Used: counter.Used, Limit: in.Limit, Remaining: remaining(counter.Used, in.Limit), Duplicate: true}, nil
	}
	if wouldExceed(counter.Used, in.Quantity, in.Limit) {
		return nil, ErrQuotaExceeded
	}
	stored := m.counters[counterKey(period.ID, in.Feature)]
	stored.Used += int64(in.Quantity)
	m.consumptions[ckey] = struct{}{}
	return &ConsumeResult{Used: stored.Used, Limit: in.Limit, Remaining: remaining(stored.Used, in.Limit)}, nil
}

func (m *memoryStore) CountResource(_ context.Context, businessID uuid.UUID, feature FeatureKey) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resources[businessID.String()+"|"+string(feature)], nil
}
