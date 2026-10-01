package payment

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alghifrz/diuk-backend/internal/area"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestPaymentRepositoryIsolationAndConcurrency(t *testing.T) {
	pool := openPaymentPool(t)
	ctx := context.Background()
	a, custA, user, table := seedPaymentWorld(t, pool)
	b, err := tenant.NewBusinessRepository(pool).Create(ctx, tenant.CreateBusinessParams{
		Name: "Pay B", Slug: "pay-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("biz B: %v", err)
	}
	t.Cleanup(func() { cleanupPayments(t, pool, a.ID, b.ID) })

	if _, err := pool.Exec(ctx, `
		UPDATE fnb_settings
		SET deposit_enabled = TRUE, deposit_type = 'FIXED', deposit_value = 100000
		WHERE business_id = $1`, a.ID); err != nil {
		t.Fatalf("enable deposit: %v", err)
	}

	start, end := futureSlot()
	resID := insertReservation(t, pool, a.ID, custA.ID, table.ID, start, end)

	svc := NewService(NewRepository(pool))
	pay, err := svc.CreateInitial(ctx, a.ID, resID, nil)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if pay.Status != StatusUnpaid || pay.Amount != money.FromMajor(100000) || pay.Currency != "IDR" {
		t.Fatalf("created payment = %+v", pay)
	}
	if _, err := svc.CreateForReservation(ctx, a.ID, resID, nil); err != ErrAlreadyExists {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := svc.Get(ctx, b.ID, pay.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant get = %v", err)
	}
	if _, err := svc.Confirm(ctx, b.ID, pay.ID, user.ID, nil); err != ErrNotFound {
		t.Fatalf("cross-tenant confirm = %v", err)
	}
	if _, err := svc.Refund(ctx, b.ID, pay.ID, user.ID, nil); err != ErrNotFound {
		t.Fatalf("cross-tenant refund = %v", err)
	}

	var wg sync.WaitGroup
	var paid int32
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, err := svc.Confirm(ctx, a.ID, pay.ID, user.ID, ptr("Transfer verified"))
			if err != nil {
				errs <- err
				return
			}
			if item.Status == StatusPaid {
				atomic.AddInt32(&paid, 1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent confirm: %v", err)
	}
	if paid != 16 {
		t.Fatalf("paid responses = %d, want 16 idempotent successes", paid)
	}
	got, err := svc.Get(ctx, a.ID, pay.ID)
	if err != nil || got.Status != StatusPaid || got.ConfirmedBy == nil || *got.ConfirmedBy != user.ID || got.ConfirmedAt == nil {
		t.Fatalf("final paid = %+v err=%v", got, err)
	}

	refunded, err := svc.Refund(ctx, a.ID, pay.ID, user.ID, ptr("manual refund"))
	if err != nil || refunded.Status != StatusRefunded {
		t.Fatalf("refund = %+v err=%v", refunded, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE reservations SET status = 'CANCELLED' WHERE id = $1 AND business_id = $2`, resID, a.ID); err != nil {
		t.Fatalf("cancel reservation: %v", err)
	}
	afterCancel, err := svc.Get(ctx, a.ID, pay.ID)
	if err != nil || afterCancel.Status != StatusRefunded {
		t.Fatalf("cancel mutated payment = %+v err=%v", afterCancel, err)
	}
}

func seedPaymentWorld(t *testing.T, pool *pgxpool.Pool) (*tenant.Business, *customer.Customer, *tenant.User, *area.Table) {
	t.Helper()
	ctx := context.Background()
	a, err := tenant.NewBusinessRepository(pool).Create(ctx, tenant.CreateBusinessParams{
		Name: "Pay A", Slug: "pay-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz A: %v", err)
	}
	user, err := tenant.NewUserRepository(pool).Create(ctx, tenant.CreateUserParams{
		AuthUserID: uuid.New(),
		FullName:   "Pay Admin",
		Email:      "pay." + uuid.NewString()[:8] + "@example.com",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	areaSvc := area.NewService(area.NewRepository(pool))
	zone, err := areaSvc.CreateArea(ctx, a.ID, area.CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("area: %v", err)
	}
	table, err := areaSvc.CreateTable(ctx, a.ID, zone.ID, area.CreateTableInput{Name: "T4", Capacity: 4})
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	cust, err := customer.NewService(customer.NewRepository(pool)).CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "John"})
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	return a, cust, user, table
}

func insertReservation(t *testing.T, pool *pgxpool.Pool, businessID, customerID, tableID uuid.UUID, start, end time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status)
		VALUES ($1, $2, $3, 2, $4, $5, 'PENDING')
		RETURNING id`, businessID, customerID, tableID, start, end).Scan(&id); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	return id
}

func futureSlot() (time.Time, time.Time) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	start := time.Now().In(loc).Add(72 * time.Hour)
	start = time.Date(start.Year(), start.Month(), start.Day(), 19, 0, 0, 0, loc)
	return start, start.Add(90 * time.Minute)
}

func cleanupPayments(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		_, _ = pool.Exec(ctx, `DELETE FROM reservation_payments WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM reservations WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tables WHERE area_id IN (SELECT id FROM areas WHERE business_id = $1)`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM areas WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE business_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, id)
	}
}

func openPaymentPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping payment integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
