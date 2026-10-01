package seed

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestLoadInputRequiresAuthUserID(t *testing.T) {
	t.Setenv("SEED_AUTH_USER_ID", "")
	t.Setenv("SEED_USER_EMAIL", "admin@example.com")

	_, err := LoadInput()
	if err == nil || !strings.Contains(err.Error(), "SEED_AUTH_USER_ID") {
		t.Fatalf("LoadInput() error = %v, want it to mention SEED_AUTH_USER_ID", err)
	}
}

func TestLoadInputRejectsInvalidAuthUserID(t *testing.T) {
	t.Setenv("SEED_AUTH_USER_ID", "not-a-uuid")
	t.Setenv("SEED_USER_EMAIL", "admin@example.com")

	_, err := LoadInput()
	if err == nil || !strings.Contains(err.Error(), "valid UUID") {
		t.Fatalf("LoadInput() error = %v, want an invalid UUID error", err)
	}
}

func TestLoadInputRequiresEmail(t *testing.T) {
	t.Setenv("SEED_AUTH_USER_ID", uuid.NewString())
	t.Setenv("SEED_USER_EMAIL", "  ")

	_, err := LoadInput()
	if err == nil || !strings.Contains(err.Error(), "SEED_USER_EMAIL") {
		t.Fatalf("LoadInput() error = %v, want it to mention SEED_USER_EMAIL", err)
	}
}

func TestLoadInputNormalizesEmail(t *testing.T) {
	id := uuid.New()
	t.Setenv("SEED_AUTH_USER_ID", id.String())
	t.Setenv("SEED_USER_EMAIL", "  Admin.Kasiko@Example.COM ")

	in, err := LoadInput()
	if err != nil {
		t.Fatalf("LoadInput() error = %v", err)
	}
	if in.AuthUserID != id {
		t.Errorf("AuthUserID = %s, want %s", in.AuthUserID, id)
	}
	if in.Email != "admin.kasiko@example.com" {
		t.Errorf("Email = %q, want normalized address", in.Email)
	}
}

func TestRunRejectsMissingInput(t *testing.T) {
	_, err := Run(context.Background(), nil, Input{})
	if err == nil || !strings.Contains(err.Error(), "SEED_AUTH_USER_ID") {
		t.Fatalf("Run() error = %v, want a UUID validation error", err)
	}
}

func TestRunFailsWhenOwnerRoleMissing(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	if _, err := tx.Exec(ctx, `
		DELETE FROM business_users
		WHERE role_id = (SELECT id FROM roles WHERE name = $1)`, tenant.RoleOwner); err != nil {
		t.Fatalf("delete OWNER memberships: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM role_permissions
		WHERE role_id = (SELECT id FROM roles WHERE name = $1)`, tenant.RoleOwner); err != nil {
		t.Fatalf("delete OWNER mappings: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE name = $1`, tenant.RoleOwner); err != nil {
		t.Fatalf("delete OWNER role: %v", err)
	}

	_, err := Run(ctx, tx, Input{AuthUserID: uuid.New(), Email: "admin@example.com"})
	if err == nil || !strings.Contains(err.Error(), "OWNER role is missing") {
		t.Fatalf("Run() error = %v, want a missing OWNER role error", err)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	in := Input{AuthUserID: uuid.New(), Email: "admin." + uuid.NewString()[:8] + "@example.com"}

	first, err := Run(ctx, tx, in)
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	second, err := Run(ctx, tx, in)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if first.Business.ID != second.Business.ID || first.User.ID != second.User.ID || first.Membership.ID != second.Membership.ID {
		t.Fatalf("second seed created different rows")
	}
	if first.Role.Name != tenant.RoleOwner || first.Membership.Status != tenant.StatusActive {
		t.Errorf("seeded membership = role %s status %s", first.Role.Name, first.Membership.Status)
	}
	if first.Business.Name != BusinessName || first.User.FullName != UserFullName {
		t.Errorf("seeded names = %q / %q", first.Business.Name, first.User.FullName)
	}

	assertCount(t, tx, 1, `SELECT count(*) FROM businesses WHERE slug = $1`, BusinessSlug)
	assertCount(t, tx, 1, `SELECT count(*) FROM users WHERE auth_user_id = $1`, in.AuthUserID)
	assertCount(t, tx, 1, `SELECT count(*) FROM business_users WHERE business_id = $1 AND user_id = $2`, first.Business.ID, first.User.ID)
}

func TestRunRejectsConflictingBusiness(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM businesses WHERE slug = $1)`, BusinessSlug).Scan(&exists); err != nil {
		t.Fatalf("lookup conflicting business: %v", err)
	}
	if exists {
		if _, err := tx.Exec(ctx, `UPDATE businesses SET name = $1 WHERE slug = $2`, "Other Cafe", BusinessSlug); err != nil {
			t.Fatalf("create conflicting business: %v", err)
		}
	} else if _, err := tenant.NewBusinessRepository(tx).Create(ctx, tenant.CreateBusinessParams{
		Name:     "Other Cafe",
		Slug:     BusinessSlug,
		Type:     tenant.BusinessTypeFNB,
		Timezone: BusinessTimezone,
	}); err != nil {
		t.Fatalf("create conflicting business: %v", err)
	}

	_, err := Run(ctx, tx, Input{AuthUserID: uuid.New(), Email: "admin." + uuid.NewString()[:8] + "@example.com"})
	if err == nil || !strings.Contains(err.Error(), "does not match the development seed") {
		t.Fatalf("Run() error = %v, want a business conflict", err)
	}
	assertCount(t, tx, 1, `SELECT count(*) FROM businesses WHERE slug = $1`, BusinessSlug)
}

func TestRunRejectsConflictingMembershipRole(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	in := Input{AuthUserID: uuid.New(), Email: "admin." + uuid.NewString()[:8] + "@example.com"}

	result, err := Run(ctx, tx, in)
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	staff, err := tenant.NewRoleRepository(tx).GetByName(ctx, tenant.RoleStaff)
	if err != nil {
		t.Fatalf("GetByName(STAFF) error = %v", err)
	}
	if _, err := tenant.NewBusinessUserRepository(tx).Update(ctx, result.Membership.ID, tenant.UpdateBusinessUserParams{
		RoleID: staff.ID,
		Status: tenant.StatusActive,
	}); err != nil {
		t.Fatalf("change membership role: %v", err)
	}

	_, err = Run(ctx, tx, in)
	if err == nil || !strings.Contains(err.Error(), "different role") {
		t.Fatalf("Run() error = %v, want a role conflict", err)
	}

	got, err := tenant.NewBusinessUserRepository(tx).Get(ctx, result.Business.ID, result.User.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.RoleID != staff.ID {
		t.Errorf("membership role was overwritten: got %s, want %s", got.RoleID, staff.ID)
	}
	assertCount(t, tx, 1, `SELECT count(*) FROM business_users WHERE business_id = $1 AND user_id = $2`, result.Business.ID, result.User.ID)
}

func TestRunRejectsInactiveMembership(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	in := Input{AuthUserID: uuid.New(), Email: "admin." + uuid.NewString()[:8] + "@example.com"}

	result, err := Run(ctx, tx, in)
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	if _, err := tenant.NewBusinessUserRepository(tx).Update(ctx, result.Membership.ID, tenant.UpdateBusinessUserParams{
		RoleID: result.Role.ID,
		Status: tenant.StatusInactive,
	}); err != nil {
		t.Fatalf("deactivate membership: %v", err)
	}

	_, err = Run(ctx, tx, in)
	if err == nil || !strings.Contains(err.Error(), "will not reactivate") {
		t.Fatalf("Run() error = %v, want an inactive membership error", err)
	}

	got, err := tenant.NewBusinessUserRepository(tx).Get(ctx, result.Business.ID, result.User.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != tenant.StatusInactive {
		t.Errorf("membership status = %s, want INACTIVE", got.Status)
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping seeder integration tests")
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

func assertCount(t *testing.T, tx pgx.Tx, want int, query string, args ...any) {
	t.Helper()

	var got int
	if err := tx.QueryRow(context.Background(), query, args...).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}
