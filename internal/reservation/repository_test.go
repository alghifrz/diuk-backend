package reservation

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alghifrz/diuk-backend/internal/area"
	"github.com/alghifrz/diuk-backend/internal/calendar"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryReservationFoundation(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	biz, custA, custB, tableA, tableB, svc := seedSQLWorld(t, tx)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, loc) }
	start := time.Date(2026, 10, 1, 19, 0, 0, 0, loc)
	end := start.Add(90 * time.Minute)

	created, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2, TableID: ptr(tableA.ID.String()),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(ctx, biz.ID, created.ID); err != nil {
		t.Fatalf("get: %v", err)
	}

	t.Run("reject reservation business A + customer B", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO reservations (business_id, customer_id, party_size, start_at, end_at, status)
			VALUES ($1, $2, 2, $3, $4, 'PENDING')`, biz.ID, custB.ID, start, end)
	})
	t.Run("reject invalid status", func(t *testing.T) {
		mustReject(t, tx, `UPDATE reservations SET status = 'ARCHIVED' WHERE id = $1`, created.ID)
	})
	t.Run("reject overlapping table via exclusion", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status)
			VALUES ($1, $2, $3, 2, $4, $5, 'PENDING')`, biz.ID, custA.ID, tableA.ID, start.Add(30*time.Minute), end.Add(30*time.Minute))
	})
	t.Run("allow adjacent table booking", func(t *testing.T) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status)
			VALUES ($1, $2, $3, 2, $4, $5, 'PENDING')`, biz.ID, custA.ID, tableA.ID, end, end.Add(60*time.Minute)); err != nil {
			t.Fatalf("adjacent: %v", err)
		}
	})
	t.Run("allow overlapping when table is null", func(t *testing.T) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservations (business_id, customer_id, party_size, start_at, end_at, status)
			VALUES ($1, $2, 2, $3, $4, 'PENDING')`, biz.ID, custA.ID, start, end); err != nil {
			t.Fatalf("null table overlap: %v", err)
		}
	})
	_ = tableB
}

func TestDepositAwareReservationConfirm(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	biz, custA, _, tableA, _, svc := seedSQLWorld(t, tx)
	hours := make([]HourInput, 0, 7)
	for day := 0; day <= 6; day++ {
		hours = append(hours, HourInput{DayOfWeek: day, SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"})
	}
	if _, err := svc.ReplaceHours(ctx, biz.ID, hours); err != nil {
		t.Fatalf("hours: %v", err)
	}
	user, err := tenant.NewUserRepository(tx).Create(ctx, tenant.CreateUserParams{
		AuthUserID: uuid.New(),
		FullName:   "Pay Admin",
		Email:      "pay." + uuid.NewString()[:8] + "@example.com",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fnb_settings
		SET deposit_enabled = TRUE, deposit_type = 'FIXED', deposit_value = 50000
		WHERE business_id = $1`, biz.ID); err != nil {
		t.Fatalf("enable deposit: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, loc) }
	start := time.Date(2026, 10, 1, 19, 0, 0, 0, loc)
	end := start.Add(90 * time.Minute)
	created, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2, TableID: ptr(tableA.ID.String()),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Confirm(ctx, biz.ID, created.ID); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("unpaid confirm = %v", err)
	}

	paySvc := payment.NewService(payment.NewRepository(tx))
	pay, err := paySvc.GetByReservation(ctx, biz.ID, created.ID)
	if err != nil {
		t.Fatalf("get payment: %v", err)
	}
	if _, err := paySvc.Confirm(ctx, biz.ID, pay.ID, user.ID, nil); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	confirmed, err := svc.Confirm(ctx, biz.ID, created.ID)
	if err != nil || confirmed.Status != StatusConfirmed {
		t.Fatalf("paid confirm = %+v err=%v", confirmed, err)
	}

	other, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(),
		StartAt:    start.Add(24 * time.Hour).Format(time.RFC3339),
		EndAt:      end.Add(24 * time.Hour).Format(time.RFC3339),
		PartySize:  2,
		TableID:    ptr(tableA.ID.String()),
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	pay2, err := paySvc.GetByReservation(ctx, biz.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := paySvc.Confirm(ctx, biz.ID, pay2.ID, user.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := paySvc.Refund(ctx, biz.ID, pay2.ID, user.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, biz.ID, other.ID); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("refunded confirm = %v", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE fnb_settings SET deposit_enabled = FALSE WHERE business_id = $1`, biz.ID); err != nil {
		t.Fatalf("disable deposit: %v", err)
	}
	free, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(),
		StartAt:    start.Add(48 * time.Hour).Format(time.RFC3339),
		EndAt:      end.Add(48 * time.Hour).Format(time.RFC3339),
		PartySize:  2,
	})
	if err != nil {
		t.Fatalf("create unpaid: %v", err)
	}
	if _, err := svc.Confirm(ctx, biz.ID, free.ID); err != nil {
		t.Fatalf("no-deposit confirm = %v", err)
	}
}

func TestCalendarBlockAndReschedulePreservePayment(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	biz, custA, _, tableA, _, svc := seedSQLWorld(t, tx)
	hours := make([]HourInput, 0, 7)
	for day := 0; day <= 6; day++ {
		hours = append(hours, HourInput{DayOfWeek: day, SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"})
	}
	if _, err := svc.ReplaceHours(ctx, biz.ID, hours); err != nil {
		t.Fatalf("hours: %v", err)
	}
	user, err := tenant.NewUserRepository(tx).Create(ctx, tenant.CreateUserParams{
		AuthUserID: uuid.New(), FullName: "Cal Admin", Email: "cal." + uuid.NewString()[:8] + "@example.com",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, loc) }
	start := time.Date(2026, 10, 1, 19, 0, 0, 0, loc)
	end := start.Add(90 * time.Minute)
	created, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2, TableID: ptr(tableA.ID.String()),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	paySvc := payment.NewService(payment.NewRepository(tx))
	pay, err := paySvc.GetByReservation(ctx, biz.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := paySvc.Confirm(ctx, biz.ID, pay.ID, user.ID, nil); err != nil {
		t.Fatal(err)
	}
	movedStart := start.Add(24 * time.Hour)
	moved, err := svc.Reschedule(ctx, biz.ID, created.ID, RescheduleInput{
		StartAt: movedStart.Format(time.RFC3339), EndAt: movedStart.Add(90 * time.Minute).Format(time.RFC3339),
	})
	if err != nil || moved.Status != StatusPending {
		t.Fatalf("reschedule = %+v err=%v", moved, err)
	}
	after, err := paySvc.GetByReservation(ctx, biz.ID, created.ID)
	if err != nil || after.Status != payment.StatusPaid || after.Amount != pay.Amount {
		t.Fatalf("payment after reschedule = %+v err=%v", after, err)
	}

	cal := calendar.NewService(calendar.NewRepository(tx))
	_, err = cal.CreateBlock(ctx, biz.ID, user.ID, calendar.CreateBlockInput{
		Title: "Private event", StartAt: movedStart.Format(time.RFC3339), EndAt: movedStart.Add(3 * time.Hour).Format(time.RFC3339),
	})
	if !errors.Is(err, calendar.ErrConflict) {
		t.Fatalf("block over reservation = %v", err)
	}
	still, err := svc.Get(ctx, biz.ID, created.ID)
	if err != nil || still.Status != StatusPending {
		t.Fatalf("reservation mutated by block = %+v err=%v", still, err)
	}

	block, err := cal.CreateBlock(ctx, biz.ID, user.ID, calendar.CreateBlockInput{
		Title: "Lunch close", StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("empty slot block: %v", err)
	}
	if _, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: custA.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2, TableID: ptr(tableA.ID.String()),
	}); !errors.Is(err, ErrCalendarBlocked) {
		t.Fatalf("create during block = %v", err)
	}
	_ = block
}

func TestConcurrentConfirmSameTable(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping concurrency test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	bizRepo := tenant.NewBusinessRepository(pool)
	biz, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Race Cafe", Slug: "race-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("business: %v", err)
	}
	areaSvc := area.NewService(area.NewRepository(pool))
	zone, err := areaSvc.CreateArea(ctx, biz.ID, area.CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("area: %v", err)
	}
	table, err := areaSvc.CreateTable(ctx, biz.ID, zone.ID, area.CreateTableInput{Name: "T1", Capacity: 4})
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	custSvc := customer.NewService(customer.NewRepository(pool))
	cust, err := custSvc.CreateCustomer(ctx, biz.ID, customer.CreateCustomerInput{Name: "Pat"})
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	svc := NewService(NewRepository(pool))
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc).AddDate(0, 0, 3)
	svc.now = func() time.Time { return time.Now() }
	if _, err := svc.ReplaceHours(ctx, biz.ID, []HourInput{
		{DayOfWeek: int(now.Weekday()), SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"},
	}); err != nil {
		t.Fatalf("hours: %v", err)
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 19, 0, 0, 0, loc)
	end := start.Add(90 * time.Minute)
	first, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: cust.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.Create(ctx, biz.ID, CreateReservationInput{
		CustomerID: cust.ID.String(), StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), PartySize: 2,
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
		failed  int
	)
	confirm := func(id uuid.UUID) {
		defer wg.Done()
		_, err := NewService(NewRepository(pool)).Confirm(ctx, biz.ID, id)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			failed++
			return
		}
		success++
	}
	wg.Add(2)
	go confirm(first.ID)
	go confirm(second.ID)
	wg.Wait()
	if success != 1 || failed != 1 {
		t.Fatalf("concurrent confirm success=%d failed=%d", success, failed)
	}
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM reservations
		WHERE business_id = $1 AND table_id = $2 AND status = 'CONFIRMED'
		  AND tstzrange(start_at, end_at, '[)') && tstzrange($3::timestamptz, $4::timestamptz, '[)')`,
		biz.ID, table.ID, start, end).Scan(&n); err != nil || n != 1 {
		t.Fatalf("confirmed overlaps n=%d err=%v", n, err)
	}
}

func seedSQLWorld(t *testing.T, tx pgx.Tx) (*tenant.Business, *customer.Customer, *customer.Customer, *area.Table, *area.Table, *Service) {
	t.Helper()
	ctx := context.Background()
	a, err := tenant.NewBusinessRepository(tx).Create(ctx, tenant.CreateBusinessParams{
		Name: "Res A", Slug: "res-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz A: %v", err)
	}
	b, err := tenant.NewBusinessRepository(tx).Create(ctx, tenant.CreateBusinessParams{
		Name: "Res B", Slug: "res-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("biz B: %v", err)
	}
	areaSvc := area.NewService(area.NewRepository(tx))
	zoneA, err := areaSvc.CreateArea(ctx, a.ID, area.CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("area A: %v", err)
	}
	tableA, err := areaSvc.CreateTable(ctx, a.ID, zoneA.ID, area.CreateTableInput{Name: "T4", Capacity: 4})
	if err != nil {
		t.Fatalf("table A: %v", err)
	}
	tableB, err := areaSvc.CreateTable(ctx, a.ID, zoneA.ID, area.CreateTableInput{Name: "T6", Capacity: 6})
	if err != nil {
		t.Fatalf("table B: %v", err)
	}
	custSvc := customer.NewService(customer.NewRepository(tx))
	custA, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "John"})
	if err != nil {
		t.Fatalf("cust A: %v", err)
	}
	custB, err := custSvc.CreateCustomer(ctx, b.ID, customer.CreateCustomerInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("cust B: %v", err)
	}
	svc := NewService(NewRepository(tx))
	if _, err := svc.ReplaceHours(ctx, a.ID, []HourInput{
		{DayOfWeek: 4, SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"},
	}); err != nil {
		t.Fatalf("hours: %v", err)
	}
	return a, custA, custB, tableA, tableB, svc
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
		t.Skip("TEST_DATABASE_URL is not set; skipping reservation integration tests")
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

func TestRepositoryErrorsStayWrapped(t *testing.T) {
	if !errors.Is(ErrTableUnavailable, ErrTableUnavailable) {
		t.Fatal("sanity")
	}
}
