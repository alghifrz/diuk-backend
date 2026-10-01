package customer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryCustomerCRM(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	svc := NewService(NewRepository(tx))
	bizRepo := tenant.NewBusinessRepository(tx)

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "CRM A", Slug: "crm-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "CRM B", Slug: "crm-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	custA, err := svc.CreateCustomer(ctx, a.ID, CreateCustomerInput{Name: "John", Phone: ptr("0811"), Email: ptr("john@example.com")})
	if err != nil {
		t.Fatalf("customer A: %v", err)
	}
	custB, err := svc.CreateCustomer(ctx, b.ID, CreateCustomerInput{Name: "John", Phone: ptr("0811")})
	if err != nil {
		t.Fatalf("customer B: %v", err)
	}

	if _, err := svc.CreateIdentity(ctx, a.ID, custA.ID, CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "6281"}); err != nil {
		t.Fatalf("identity A: %v", err)
	}
	if err := expectError(t, tx, func(sp pgx.Tx) error {
		_, err := NewService(NewRepository(sp)).CreateIdentity(ctx, a.ID, custA.ID, CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "6281"})
		return err
	}); !errors.Is(err, ErrIdentityExists) {
		t.Fatalf("duplicate identity = %v", err)
	}
	if _, err := svc.CreateIdentity(ctx, b.ID, custB.ID, CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "6281"}); err != nil {
		t.Fatalf("same external other business: %v", err)
	}

	tagA, err := svc.CreateTag(ctx, a.ID, CreateTagInput{Name: "VIP"})
	if err != nil {
		t.Fatalf("tag A: %v", err)
	}
	if err := expectError(t, tx, func(sp pgx.Tx) error {
		_, err := NewService(NewRepository(sp)).CreateTag(ctx, a.ID, CreateTagInput{Name: "VIP"})
		return err
	}); !errors.Is(err, ErrTagNameExists) {
		t.Fatalf("duplicate tag = %v", err)
	}
	if _, err := svc.CreateTag(ctx, b.ID, CreateTagInput{Name: "VIP"}); err != nil {
		t.Fatalf("same tag name other business: %v", err)
	}

	if _, created, err := svc.AssignTag(ctx, a.ID, custA.ID, tagA.ID); err != nil || !created {
		t.Fatalf("assign: created=%v err=%v", created, err)
	}
	listed, err := svc.ListCustomers(ctx, a.ID, "", "john", "10", "0")
	if err != nil || len(listed) != 1 || listed[0].IdentityCount != 1 || listed[0].ActiveTagCount != 1 {
		t.Fatalf("list/search = %+v err=%v", listed, err)
	}
	if _, err := svc.GetCustomer(ctx, b.ID, custA.ID); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}

	t.Run("crm stats sort and tag filter", func(t *testing.T) {
		now := time.Now()
		done := now.Add(-48 * time.Hour)
		upcoming := now.Add(48 * time.Hour)
		var completedID, upcomingID, cancelledID uuid.UUID
		insert := func(status string, start time.Time, completedAt *time.Time) uuid.UUID {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `
				INSERT INTO reservations (business_id, customer_id, party_size, start_at, end_at, status, completed_at)
				VALUES ($1, $2, 2, $3, $4, $5, $6) RETURNING id`,
				a.ID, custA.ID, start, start.Add(90*time.Minute), status, completedAt,
			).Scan(&id); err != nil {
				t.Fatalf("reservation %s: %v", status, err)
			}
			return id
		}
		completedID = insert("COMPLETED", done, &done)
		upcomingID = insert("PENDING", upcoming, nil)
		cancelledID = insert("CANCELLED", upcoming.Add(24*time.Hour), nil)
		_ = upcomingID
		_ = cancelledID
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservation_payments (business_id, reservation_id, status, amount, currency)
			VALUES ($1, $2, 'UNPAID', 125000, 'IDR')`, a.ID, completedID); err != nil {
			t.Fatalf("payment: %v", err)
		}
		user, err := tenant.NewUserRepository(tx).Create(ctx, tenant.CreateUserParams{
			AuthUserID: uuid.New(), FullName: "CRM Admin", Email: "crm." + uuid.NewString()[:8] + "@example.com",
		})
		if err != nil {
			t.Fatalf("user: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE reservation_payments
			SET status = 'PAID', confirmed_at = now(), confirmed_by = $2
			WHERE reservation_id = $1`, completedID, user.ID); err != nil {
			t.Fatalf("mark paid: %v", err)
		}

		got, err := svc.GetCustomer(ctx, a.ID, custA.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Stats.ReservationCount != 3 || got.Stats.CompletedCount != 1 || got.Stats.CancelledCount != 1 {
			t.Fatalf("stats counts = %+v", got.Stats)
		}
		if got.Stats.TotalSpent != money.FromMajor(125000) {
			t.Fatalf("total spent = %v", got.Stats.TotalSpent)
		}
		if got.Stats.LastVisitAt == nil || got.Stats.NextReservationAt == nil {
			t.Fatalf("visit timestamps = %+v", got.Stats)
		}

		for _, sortBy := range []string{"newest", "name", "spend", "visits", "last_visit"} {
			rows, err := svc.ListCustomersWithOptions(ctx, a.ID, ListOptions{Sort: sortBy, Limit: "10"})
			if err != nil || len(rows) != 1 || rows[0].Stats.CompletedCount != 1 {
				t.Fatalf("sort %s = %+v err=%v", sortBy, rows, err)
			}
		}
		byTag, err := svc.ListCustomersWithOptions(ctx, a.ID, ListOptions{Tag: tagA.ID.String()})
		if err != nil || len(byTag) != 1 {
			t.Fatalf("tag filter = %+v err=%v", byTag, err)
		}
		byOtherTag, err := svc.ListCustomersWithOptions(ctx, a.ID, ListOptions{Tag: uuid.NewString()})
		if err != nil || len(byOtherTag) != 0 {
			t.Fatalf("unknown tag filter = %+v err=%v", byOtherTag, err)
		}
		if _, err := svc.ListCustomersWithOptions(ctx, a.ID, ListOptions{Sort: "bogus"}); err == nil {
			t.Fatal("expected invalid sort error")
		}
	})

	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM customers WHERE id = $1`, custA.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if _, err := svc.DeleteCustomer(ctx, a.ID, custA.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM customers WHERE id = $1`, custA.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("physical delete happened: n=%d err=%v", n, err)
	}

	t.Run("reject identity for foreign customer", func(t *testing.T) {
		err := expectError(t, tx, func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `
				INSERT INTO customer_identities (business_id, customer_id, channel, external_id)
				VALUES ($1, $2, 'WEBSITE', 'cross')`, a.ID, custB.ID)
			return err
		})
		if err == nil {
			t.Fatal("expected composite FK rejection")
		}
	})
	t.Run("reject customer A + tag B", func(t *testing.T) {
		tagB, err := svc.CreateTag(ctx, b.ID, CreateTagInput{Name: "Other"})
		if err != nil {
			t.Fatalf("tag B: %v", err)
		}
		err = expectError(t, tx, func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `
				INSERT INTO customer_tags (customer_id, tag_id, business_id)
				VALUES ($1, $2, $3)`, custA.ID, tagB.ID, a.ID)
			return err
		})
		if err == nil {
			t.Fatal("expected composite FK rejection")
		}
	})
	t.Run("reject invalid status", func(t *testing.T) {
		err := expectError(t, tx, func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE customers SET status = 'DELETED' WHERE id = $1`, custA.ID)
			return err
		})
		if err == nil {
			t.Fatal("expected status check rejection")
		}
	})
	t.Run("reject invalid channel", func(t *testing.T) {
		err := expectError(t, tx, func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `
				INSERT INTO customer_identities (business_id, customer_id, channel, external_id)
				VALUES ($1, $2, 'SMS', 'x')`, a.ID, custA.ID)
			return err
		})
		if err == nil {
			t.Fatal("expected channel check rejection")
		}
	})
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
		t.Skip("TEST_DATABASE_URL is not set; skipping customer integration tests")
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
