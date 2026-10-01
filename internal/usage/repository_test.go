package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestUsageRepositoryConcurrencyAndIdempotency(t *testing.T) {
	pool := openUsagePool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), repo)

	bizRepo := tenant.NewBusinessRepository(pool)
	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Usage A", Slug: "usage-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Usage B", Slug: "usage-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("biz B: %v", err)
	}
	t.Cleanup(func() { cleanupUsage(t, pool, a.ID, b.ID) })

	assign, err := repo.GetAssignment(ctx, a.ID)
	if err != nil || assign.PlanCode != PlanPlus {
		t.Fatalf("default plan = %+v err=%v", assign, err)
	}
	ent, err := repo.GetEntitlement(ctx, assign.PlanID, FeatureAIResponses)
	if err != nil || ent.LimitValue == nil || *ent.LimitValue != 50000 {
		t.Fatalf("plus AI limit = %+v err=%v", ent, err)
	}

	start, end, tz, err := MonthlyPeriod(time.Now(), "Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.EnsurePeriod(ctx, Period{
				BusinessID: a.ID, PeriodType: PeriodMonthly, PeriodStart: start, PeriodEnd: end, Timezone: tz,
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent period: %v", err)
		}
	}

	source := uuid.New()
	first, err := svc.Consume(ctx, a.ID, ConsumeInput{Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: source})
	if err != nil || first.Used != 1 {
		t.Fatalf("first consume = %+v err=%v", first, err)
	}
	again, err := svc.Consume(ctx, a.ID, ConsumeInput{Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: source})
	if err != nil || again.Used != 1 || !again.Duplicate {
		t.Fatalf("idempotent consume = %+v err=%v", again, err)
	}

	snapB, err := svc.GetUsage(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snapB.Usage {
		if item.Feature == FeatureAIResponses && item.Used != 0 {
			t.Fatalf("tenant B leaked usage %+v", item)
		}
	}
}

func TestUsageRepositoryConcurrentConsumeRespectsLimit(t *testing.T) {
	pool := openUsagePool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), repo)

	biz, err := tenant.NewBusinessRepository(pool).Create(ctx, tenant.CreateBusinessParams{
		Name: "Usage Race", Slug: "usage-race-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz: %v", err)
	}
	t.Cleanup(func() { cleanupUsage(t, pool, biz.ID) })

	start, end, tz, err := MonthlyPeriod(time.Now(), "Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	period, err := repo.EnsurePeriod(ctx, Period{
		BusinessID: biz.ID, PeriodType: PeriodMonthly, PeriodStart: start, PeriodEnd: end, Timezone: tz,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EnsureCounter(ctx, Counter{BusinessID: biz.ID, UsagePeriodID: period.ID, FeatureKey: FeatureAIResponses}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE usage_counters SET used = 49900
		WHERE business_id = $1 AND usage_period_id = $2 AND feature_key = 'AI_RESPONSES'`,
		biz.ID, period.ID); err != nil {
		t.Fatalf("seed used: %v", err)
	}

	const workers = 200
	var okCount, failCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.Consume(ctx, biz.ID, ConsumeInput{
				Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: uuid.New(),
			})
			if err == nil {
				okCount.Add(1)
				return
			}
			if errors.Is(err, ErrQuotaExceeded) {
				failCount.Add(1)
				return
			}
			t.Errorf("consume: %v", err)
		}()
	}
	wg.Wait()
	if okCount.Load() != 100 {
		t.Fatalf("successes = %d, want 100", okCount.Load())
	}
	if failCount.Load() != workers-100 {
		t.Fatalf("failures = %d, want %d", failCount.Load(), workers-100)
	}
	snap, err := svc.GetUsage(ctx, biz.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.Usage {
		if item.Feature == FeatureAIResponses && item.Used != 50000 {
			t.Fatalf("final used = %d, want 50000", item.Used)
		}
	}
}

func openUsagePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping usage integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupUsage(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		_, _ = pool.Exec(ctx, `DELETE FROM usage_consumptions WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM usage_counters WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM usage_periods WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, id)
	}
}
