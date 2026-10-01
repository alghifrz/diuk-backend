package business

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryProfileAndSettings(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	svc := NewService(NewRepository(tx))

	created, err := tenant.NewBusinessRepository(tx).Create(ctx, tenant.CreateBusinessParams{
		Name:     "Warung Test",
		Slug:     "warung-" + uuid.NewString()[:8],
		Type:     tenant.BusinessTypeFNB,
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create business: %v", err)
	}

	profile, err := svc.Profile(ctx, created.ID)
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if profile.Slug != created.Slug || profile.Description != nil {
		t.Errorf("profile = %+v", profile)
	}

	settings, err := svc.FNBSettings(ctx, created.ID)
	if err != nil {
		t.Fatalf("FNBSettings() error = %v", err)
	}
	if settings.Currency != "IDR" || settings.ReservationSlotDurationMinutes != 30 || !settings.ReservationEnabled {
		t.Errorf("default settings = %+v", settings)
	}
	if settings.DepositEnabled || settings.DepositType != "FIXED" || settings.DepositValue != 0 {
		t.Errorf("default deposit = %+v", settings)
	}

	updated, err := svc.UpdateProfile(ctx, created.ID, UpdateProfileInput{
		Name:        ptr("Warung Baru"),
		Description: ptr("Nasi goreng"),
		Email:       ptr("  Owner@Example.COM "),
		WebsiteURL:  ptr("https://warung.example.com"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if updated.Name != "Warung Baru" || updated.Slug != created.Slug {
		t.Errorf("updated profile = %+v", updated)
	}
	if updated.Email == nil || *updated.Email != "owner@example.com" {
		t.Errorf("email = %v", updated.Email)
	}

	patched, err := svc.UpdateFNBSettings(ctx, created.ID, UpdateFNBSettingsInput{
		ReservationSlotDurationMinutes: ptrInt(60),
	})
	if err != nil {
		t.Fatalf("UpdateFNBSettings() error = %v", err)
	}
	if patched.ReservationSlotDurationMinutes != 60 || patched.ReservationMaxPartySize != 20 {
		t.Errorf("patched settings = %+v", patched)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM fnb_settings WHERE business_id = $1`, created.ID).Scan(&count); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 1 {
		t.Fatalf("fnb_settings rows = %d, want 1", count)
	}
}

func TestKasikoCoffeeHasSingleDefaultSettings(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()

	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM fnb_settings s
		JOIN businesses b ON b.id = s.business_id
		WHERE b.slug = 'kasiko-coffee'`).Scan(&n)
	if err != nil {
		t.Fatalf("count kasiko settings: %v", err)
	}
	if n == 0 {
		t.Skip("kasiko-coffee is not in this database")
	}
	if n != 1 {
		t.Fatalf("kasiko-coffee fnb_settings rows = %d, want 1", n)
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping business integration tests")
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
