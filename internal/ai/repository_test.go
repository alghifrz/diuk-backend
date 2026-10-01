package ai

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryPromptConstraints(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	bizRepo := tenant.NewBusinessRepository(tx)
	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "AI A", Slug: "ai-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "AI B", Slug: "ai-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	svc := NewService(testLogger(), NewRepository(tx), Defaults{Provider: ProviderOpenAI, Model: "gpt-4o-mini", APIKey: "test"}, nil, nil, nil, nil, nil, nil, nil, nil)
	first, err := svc.CreatePrompt(ctx, a.ID, uuid.Nil, CreatePromptInput{Name: "customer_service", SystemPrompt: "v1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ActivatePrompt(ctx, a.ID, first.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	second, err := svc.CreatePrompt(ctx, a.ID, uuid.Nil, CreatePromptInput{Name: "customer_service", SystemPrompt: "v2"})
	if err != nil || second.Version != 2 {
		t.Fatalf("second = %+v err=%v", second, err)
	}
	if _, err := svc.ActivatePrompt(ctx, a.ID, second.ID); err != nil {
		t.Fatalf("activate v2: %v", err)
	}
	got, err := svc.GetPrompt(ctx, a.ID, first.ID)
	if err != nil || got.Status != PromptArchived {
		t.Fatalf("archived previous = %+v err=%v", got, err)
	}
	if _, err := svc.GetPrompt(ctx, b.ID, first.ID); !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}

	mustReject(t, tx, `
		INSERT INTO ai_prompts (business_id, name, system_prompt, version, status)
		VALUES ($1, 'customer_service', 'dup', 2, 'DRAFT')`, a.ID)
	mustReject(t, tx, `
		INSERT INTO ai_settings (business_id, provider, model, temperature, enabled)
		VALUES ($1, 'openai', 'gpt-4o-mini', 3, true)`, a.ID)
	mustReject(t, tx, `
		INSERT INTO ai_settings (business_id, provider, model, enabled)
		VALUES ($1, 'unknown', 'gpt-4o-mini', true)`, a.ID)
}

func mustReject(t *testing.T, tx pgx.Tx, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "SAVEPOINT reject_check"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, err := tx.Exec(ctx, query, args...)
	if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT reject_check"); rbErr != nil {
		t.Fatalf("rollback savepoint: %v", rbErr)
	}
	if err == nil {
		t.Fatal("expected constraint rejection")
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping AI integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
