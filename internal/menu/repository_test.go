package menu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryMenu(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	svc := NewService(NewRepository(tx), newMemoryStorage(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	bizRepo := tenant.NewBusinessRepository(tx)

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Menu A", Slug: "menu-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Menu B", Slug: "menu-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	coffeeA, err := svc.CreateCategory(ctx, a.ID, CreateCategoryInput{Name: "Coffee"})
	if err != nil {
		t.Fatalf("coffee A: %v", err)
	}
	if err := expectError(t, tx, func(sp pgx.Tx) error {
		_, err := NewService(NewRepository(sp), newMemoryStorage(), slog.New(slog.NewTextHandler(io.Discard, nil))).CreateCategory(ctx, a.ID, CreateCategoryInput{Name: "Coffee"})
		return err
	}); !errors.Is(err, ErrCategoryNameExists) {
		t.Fatalf("duplicate = %v", err)
	}
	coffeeB, err := svc.CreateCategory(ctx, b.ID, CreateCategoryInput{Name: "Coffee"})
	if err != nil {
		t.Fatalf("coffee B: %v", err)
	}

	itemA, err := svc.CreateItem(ctx, a.ID, CreateItemInput{CategoryID: coffeeA.ID.String(), Name: "Americano", Price: "18000.50"})
	if err != nil {
		t.Fatalf("item A: %v", err)
	}
	if _, err := svc.CreateItem(ctx, b.ID, CreateItemInput{CategoryID: coffeeB.ID.String(), Name: "Americano", Price: "17000"}); err != nil {
		t.Fatalf("item B: %v", err)
	}
	if _, err := svc.GetItem(ctx, b.ID, itemA.ID); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("cross-tenant item = %v", err)
	}
	if _, err := svc.CreateItem(ctx, b.ID, CreateItemInput{CategoryID: coffeeA.ID.String(), Name: "Stolen", Price: "1"}); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("foreign category = %v", err)
	}

	got, err := svc.GetCategory(ctx, a.ID, coffeeA.ID)
	if err != nil {
		t.Fatalf("get category: %v", err)
	}
	if got.ItemCount != 1 || got.ActiveItemCount != 1 {
		t.Errorf("counts = %d/%d", got.ItemCount, got.ActiveItemCount)
	}

	if _, err := svc.DeleteCategory(ctx, a.ID, coffeeA.ID); !errors.Is(err, ErrCategoryHasActiveItems) {
		t.Fatalf("delete with items = %v", err)
	}
	if _, err := svc.DeleteItem(ctx, a.ID, itemA.ID); err != nil {
		t.Fatalf("soft delete item: %v", err)
	}
	deactivated, err := svc.DeleteCategory(ctx, a.ID, coffeeA.ID)
	if err != nil {
		t.Fatalf("soft delete category: %v", err)
	}
	if deactivated.Status != tenant.StatusInactive {
		t.Errorf("status = %s", deactivated.Status)
	}

	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM menu_items WHERE id = $1`, itemA.ID).Scan(&n); err != nil {
		t.Fatalf("count item: %v", err)
	}
	if n != 1 {
		t.Fatal("item was physically deleted")
	}
}

func expectError(t *testing.T, tx pgx.Tx, fn func(pgx.Tx) error) error {
	t.Helper()
	sp, err := tx.Begin(context.Background())
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = fn(sp)
	_ = sp.Rollback(context.Background())
	return err
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping menu integration tests")
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
