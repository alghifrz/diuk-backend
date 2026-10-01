package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testService(store Store) *Service {
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	return svc
}

func intPtr(n int) *int { return &n }

func standardPlan() (Plan, []Entitlement) {
	p := Plan{ID: PlanIDStandard, Code: PlanStandard, Name: "Standard", Status: PlanActive}
	ents := []Entitlement{
		{ID: uuid.New(), PlanID: p.ID, FeatureKey: FeatureAIResponses, LimitValue: intPtr(25000), Period: PeriodMonthly, Enabled: true},
		{ID: uuid.New(), PlanID: p.ID, FeatureKey: FeatureCampaignRecipients, LimitValue: intPtr(100000), Period: PeriodMonthly, Enabled: true},
		{ID: uuid.New(), PlanID: p.ID, FeatureKey: FeatureAutomationRuns, LimitValue: intPtr(10000), Period: PeriodMonthly, Enabled: true},
		{ID: uuid.New(), PlanID: p.ID, FeatureKey: FeatureKnowledgeDocuments, LimitValue: intPtr(100), Period: PeriodNone, Enabled: true},
		{ID: uuid.New(), PlanID: p.ID, FeatureKey: FeatureWhatsAppChannels, LimitValue: intPtr(3), Period: PeriodNone, Enabled: true},
	}
	return p, ents
}

func TestCheckConsumeAndQuotaBoundary(t *testing.T) {
	store := newMemoryStore()
	plan, ents := standardPlan()
	ents[0].LimitValue = intPtr(50000)
	store.seedPlan(plan, ents)
	biz := uuid.New()
	store.assign(biz, "Asia/Jakarta", plan)
	svc := testService(store)
	svc.now = func() time.Time { return time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC) }

	if err := svc.Check(context.Background(), biz, FeatureAIResponses); err != nil {
		t.Fatalf("check empty: %v", err)
	}
	got, err := svc.currentUsage(context.Background(), biz, FeatureAIResponses)
	if err != nil || got.Used != 0 || got.Limit == nil || *got.Limit != 50000 || got.Remaining == nil || *got.Remaining != 50000 {
		t.Fatalf("0/50000 = %+v err=%v", got, err)
	}

	out, err := svc.Consume(context.Background(), biz, ConsumeInput{
		Feature: FeatureAIResponses, Quantity: 1, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	})
	if err != nil || out.Used != 1 || out.Remaining == nil || *out.Remaining != 49999 {
		t.Fatalf("first consume = %+v err=%v", out, err)
	}

	store.mu.Lock()
	for _, c := range store.counters {
		if c.FeatureKey == FeatureAIResponses {
			c.Used = 49999
		}
	}
	store.mu.Unlock()

	out, err = svc.Consume(context.Background(), biz, ConsumeInput{
		Feature: FeatureAIResponses, Quantity: 1, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	})
	if err != nil || out.Used != 50000 {
		t.Fatalf("boundary consume = %+v err=%v", out, err)
	}
	if err := svc.Check(context.Background(), biz, FeatureAIResponses); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("check at cap = %v", err)
	}
	if _, err := svc.Consume(context.Background(), biz, ConsumeInput{
		Feature: FeatureAIResponses, Quantity: 1, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("over cap = %v", err)
	}
}

func TestConsumeIdempotentBySource(t *testing.T) {
	store := newMemoryStore()
	plan, ents := standardPlan()
	store.seedPlan(plan, ents)
	biz := uuid.New()
	store.assign(biz, "Asia/Jakarta", plan)
	svc := testService(store)
	source := uuid.New()
	first, err := svc.Consume(context.Background(), biz, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: source,
	})
	if err != nil || first.Used != 1 || first.Duplicate {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	again, err := svc.Consume(context.Background(), biz, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: source,
	})
	if err != nil || again.Used != 1 || !again.Duplicate {
		t.Fatalf("retry = %+v err=%v", again, err)
	}
}

func TestUnlimitedAndMissingAndInactive(t *testing.T) {
	store := newMemoryStore()
	biz := Plan{ID: PlanIDBusiness, Code: PlanBusiness, Name: "Business", Status: PlanActive}
	store.seedPlan(biz, []Entitlement{
		{ID: uuid.New(), PlanID: biz.ID, FeatureKey: FeatureAIResponses, LimitValue: nil, Period: PeriodMonthly, Enabled: true},
	})
	id := uuid.New()
	store.assign(id, "Asia/Jakarta", biz)
	svc := testService(store)
	out, err := svc.Consume(context.Background(), id, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	})
	if err != nil || out.Limit != nil || out.Remaining != nil {
		t.Fatalf("unlimited = %+v err=%v", out, err)
	}

	other := uuid.New()
	inactive := Plan{ID: uuid.New(), Code: PlanFree, Name: "Free", Status: PlanInactive}
	store.seedPlan(inactive, []Entitlement{
		{ID: uuid.New(), PlanID: inactive.ID, FeatureKey: FeatureAIResponses, LimitValue: intPtr(10), Period: PeriodMonthly, Enabled: true},
	})
	store.assign(other, "Asia/Jakarta", inactive)
	if _, err := svc.Consume(context.Background(), other, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	}); !errors.Is(err, ErrPlanInactive) {
		t.Fatalf("inactive = %v", err)
	}

	missing := uuid.New()
	empty := Plan{ID: uuid.New(), Code: PlanPro, Name: "Pro", Status: PlanActive}
	store.seedPlan(empty, nil)
	store.assign(missing, "Asia/Jakarta", empty)
	if err := svc.Check(context.Background(), missing, FeatureAIResponses); !errors.Is(err, ErrNotEntitled) {
		t.Fatalf("missing entitlement = %v", err)
	}
}

func TestDisabledEntitlementAndResourceLimit(t *testing.T) {
	store := newMemoryStore()
	plan := Plan{ID: uuid.New(), Code: PlanFree, Name: "Free", Status: PlanActive}
	store.seedPlan(plan, []Entitlement{
		{ID: uuid.New(), PlanID: plan.ID, FeatureKey: FeatureAIResponses, LimitValue: intPtr(10), Period: PeriodMonthly, Enabled: false},
		{ID: uuid.New(), PlanID: plan.ID, FeatureKey: FeatureWhatsAppChannels, LimitValue: intPtr(1), Period: PeriodNone, Enabled: true},
	})
	biz := uuid.New()
	store.assign(biz, "Asia/Jakarta", plan)
	svc := testService(store)
	if err := svc.Check(context.Background(), biz, FeatureAIResponses); !errors.Is(err, ErrFeatureDisabled) {
		t.Fatalf("disabled = %v", err)
	}
	if err := svc.CheckResource(context.Background(), biz, FeatureWhatsAppChannels); err != nil {
		t.Fatalf("resource empty: %v", err)
	}
	store.resources[biz.String()+"|"+string(FeatureWhatsAppChannels)] = 1
	if err := svc.CheckResource(context.Background(), biz, FeatureWhatsAppChannels); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("resource full = %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	store := newMemoryStore()
	plan, ents := standardPlan()
	store.seedPlan(plan, ents)
	a, b := uuid.New(), uuid.New()
	store.assign(a, "Asia/Jakarta", plan)
	store.assign(b, "Asia/Makassar", plan)
	svc := testService(store)
	if _, err := svc.Consume(context.Background(), a, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}
	snapA, err := svc.GetUsage(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	snapB, err := svc.GetUsage(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	var usedA, usedB int64
	for _, item := range snapA.Usage {
		if item.Feature == FeatureAIResponses {
			usedA = item.Used
		}
	}
	for _, item := range snapB.Usage {
		if item.Feature == FeatureAIResponses {
			usedB = item.Used
		}
	}
	if usedA != 1 || usedB != 0 {
		t.Fatalf("isolation used A=%d B=%d", usedA, usedB)
	}
}

func TestHistoryExcludesCurrentPeriod(t *testing.T) {
	store := newMemoryStore()
	plan, ents := standardPlan()
	store.seedPlan(plan, ents)
	biz := uuid.New()
	store.assign(biz, "Asia/Jakarta", plan)
	svc := testService(store)
	svc.now = func() time.Time { return time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC) }
	loc, _ := time.LoadLocation("Asia/Jakarta")
	prev := Period{BusinessID: biz, PeriodType: PeriodMonthly, PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, loc), PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, loc), Timezone: "Asia/Jakarta"}
	if _, err := store.EnsurePeriod(context.Background(), prev); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetUsage(context.Background(), biz); err != nil {
		t.Fatal(err)
	}
	hist, err := svc.History(context.Background(), biz, "12", "0")
	if err != nil || len(hist.Items) != 1 {
		t.Fatalf("history = %+v err=%v", hist, err)
	}
	if hist.Items[0].Period.PeriodStart.Month() != time.August {
		t.Fatalf("history month = %s", hist.Items[0].Period.PeriodStart.Month())
	}
}
