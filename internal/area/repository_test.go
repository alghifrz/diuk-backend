package area

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryAreasAndTables(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	svc := NewService(NewRepository(tx))
	bizRepo := tenant.NewBusinessRepository(tx)

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Business A", Slug: "area-biz-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Business B", Slug: "area-biz-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	indoorA, err := svc.CreateArea(ctx, a.ID, CreateAreaInput{Name: "Indoor", Description: ptr("Hall")})
	if err != nil {
		t.Fatalf("create indoor A: %v", err)
	}
	if err := expectError(t, tx, func(sp pgx.Tx) error {
		_, err := NewService(NewRepository(sp)).CreateArea(ctx, a.ID, CreateAreaInput{Name: "Indoor"})
		return err
	}); !errors.Is(err, ErrAreaNameExists) {
		t.Fatalf("duplicate A error = %v", err)
	}
	indoorB, err := svc.CreateArea(ctx, b.ID, CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("same name in B: %v", err)
	}

	tableA, err := svc.CreateTable(ctx, a.ID, indoorA.ID, CreateTableInput{Name: "Table 01", Capacity: 4})
	if err != nil {
		t.Fatalf("table A: %v", err)
	}
	if err := expectError(t, tx, func(sp pgx.Tx) error {
		_, err := NewService(NewRepository(sp)).CreateTable(ctx, a.ID, indoorA.ID, CreateTableInput{Name: "Table 01", Capacity: 2})
		return err
	}); !errors.Is(err, ErrTableNameExists) {
		t.Fatalf("duplicate table A error = %v", err)
	}
	if _, err := svc.CreateTable(ctx, b.ID, indoorB.ID, CreateTableInput{Name: "Table 01", Capacity: 2}); err != nil {
		t.Fatalf("table B: %v", err)
	}

	got, err := svc.GetArea(ctx, a.ID, indoorA.ID)
	if err != nil {
		t.Fatalf("get area: %v", err)
	}
	if got.TableCount != 1 || got.ActiveTableCount != 1 {
		t.Errorf("counts = %d/%d", got.TableCount, got.ActiveTableCount)
	}
	if _, err := svc.GetArea(ctx, b.ID, indoorA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant area = %v", err)
	}
	if _, err := svc.GetTable(ctx, a.ID, indoorA.ID, tableA.ID); err != nil {
		t.Fatalf("get table: %v", err)
	}
	if _, err := svc.GetTable(ctx, b.ID, indoorB.ID, tableA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("table leak = %v", err)
	}
	if _, err := svc.GetTable(ctx, a.ID, indoorB.ID, tableA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong area = %v", err)
	}

	if _, err := svc.DeleteArea(ctx, a.ID, indoorA.ID); !errors.Is(err, ErrAreaHasActiveTables) {
		t.Fatalf("delete with tables = %v", err)
	}
	if _, err := svc.DeleteTable(ctx, a.ID, indoorA.ID, tableA.ID); err != nil {
		t.Fatalf("soft delete table: %v", err)
	}
	deactivated, err := svc.DeleteArea(ctx, a.ID, indoorA.ID)
	if err != nil {
		t.Fatalf("soft delete area: %v", err)
	}
	if deactivated.Status != tenant.StatusInactive {
		t.Errorf("area status = %s", deactivated.Status)
	}

	var physical int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM areas WHERE id = $1`, indoorA.ID).Scan(&physical); err != nil {
		t.Fatalf("count area: %v", err)
	}
	if physical != 1 {
		t.Fatalf("area was physically deleted")
	}

	listed, err := svc.ListAreas(ctx, a.ID, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("active areas = %d", len(listed))
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
		t.Skip("TEST_DATABASE_URL is not set; skipping area integration tests")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	return tx
}
