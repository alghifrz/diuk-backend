package tenant

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// newTx opens a transaction against the migrated test database named by
// TEST_DATABASE_URL and rolls it back when the test ends, so the tests leave
// no rows behind. The whole file is skipped when the variable is unset.
func newTx(t *testing.T) pgx.Tx {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping repository tests")
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

// withSavepoint runs fn inside a nested transaction and rolls it back, so a
// statement that is expected to fail does not abort the outer transaction.
func withSavepoint(t *testing.T, tx pgx.Tx, fn func(q Querier) error) error {
	t.Helper()

	ctx := context.Background()
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("begin savepoint: %v", err)
	}
	defer func() { _ = nested.Rollback(ctx) }()

	return fn(nested)
}

func createBusiness(t *testing.T, tx pgx.Tx, slug string) *Business {
	t.Helper()

	business, err := NewBusinessRepository(tx).Create(context.Background(), CreateBusinessParams{
		Name:     "Warung " + slug,
		Slug:     slug,
		Type:     BusinessTypeFNB,
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create business: %v", err)
	}

	return business
}

func createUser(t *testing.T, tx pgx.Tx, email string) *User {
	t.Helper()

	user, err := NewUserRepository(tx).Create(context.Background(), CreateUserParams{
		AuthUserID: uuid.New(),
		FullName:   "Test User",
		Email:      email,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	return user
}

func uniqueSlug() string {
	return "biz-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func TestBusinessRepository(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	repo := NewBusinessRepository(tx)

	slug := uniqueSlug()
	created := createBusiness(t, tx, slug)

	if created.ID == uuid.Nil {
		t.Error("Create() did not return a generated id")
	}
	if created.Status != StatusActive {
		t.Errorf("Status = %q, want %q", created.Status, StatusActive)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Create() did not return timestamps")
	}

	found, err := repo.GetBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("GetBySlug() error = %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("GetBySlug() id = %s, want %s", found.ID, created.ID)
	}

	updated, err := repo.Update(ctx, created.ID, UpdateBusinessParams{
		Name:     "Renamed",
		Timezone: "Asia/Makassar",
		Status:   StatusInactive,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Renamed" || updated.Status != StatusInactive {
		t.Errorf("Update() = %+v, want name Renamed and status INACTIVE", updated)
	}

	err = withSavepoint(t, tx, func(q Querier) error {
		_, err := NewBusinessRepository(q).Create(ctx, CreateBusinessParams{
			Name: "Duplicate", Slug: slug, Type: BusinessTypeFNB, Timezone: "Asia/Jakarta",
		})
		return err
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("Create() duplicate slug error = %v, want ErrConflict", err)
	}

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID() unknown error = %v, want ErrNotFound", err)
	}
}

func TestUserRepositoryNormalizesEmail(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	repo := NewUserRepository(tx)

	email := "Owner." + uuid.NewString()[:8] + "@Example.COM"
	created := createUser(t, tx, email)

	if created.Email != strings.ToLower(email) {
		t.Errorf("Email = %q, want %q", created.Email, strings.ToLower(email))
	}

	found, err := repo.GetByEmail(ctx, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("GetByEmail() error = %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("GetByEmail() id = %s, want %s", found.ID, created.ID)
	}

	byAuth, err := repo.GetByAuthUserID(ctx, created.AuthUserID)
	if err != nil {
		t.Fatalf("GetByAuthUserID() error = %v", err)
	}
	if byAuth.ID != created.ID {
		t.Errorf("GetByAuthUserID() id = %s, want %s", byAuth.ID, created.ID)
	}
}

func TestRoleRepositoryReturnsSeededRoles(t *testing.T) {
	ctx := context.Background()
	repo := NewRoleRepository(newTx(t))

	roles, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(roles) < 3 {
		t.Fatalf("List() returned %d roles, want at least 3", len(roles))
	}

	for _, name := range []RoleName{RoleOwner, RoleManager, RoleStaff} {
		role, err := repo.GetByName(ctx, name)
		if err != nil {
			t.Fatalf("GetByName(%q) error = %v", name, err)
		}
		if role.Name != name {
			t.Errorf("GetByName(%q) returned %q", name, role.Name)
		}
	}
}

// A user must be able to hold a different role in each business they belong to.
func TestBusinessUserRepositorySupportsMultipleBusinessesPerUser(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)

	roles := NewRoleRepository(tx)
	owner, err := roles.GetByName(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("GetByName(OWNER) error = %v", err)
	}
	manager, err := roles.GetByName(ctx, RoleManager)
	if err != nil {
		t.Fatalf("GetByName(MANAGER) error = %v", err)
	}

	businessA := createBusiness(t, tx, uniqueSlug())
	businessB := createBusiness(t, tx, uniqueSlug())
	user := createUser(t, tx, "member."+uuid.NewString()[:8]+"@example.com")

	repo := NewBusinessUserRepository(tx)
	if _, err := repo.Create(ctx, CreateBusinessUserParams{BusinessID: businessA.ID, UserID: user.ID, RoleID: owner.ID}); err != nil {
		t.Fatalf("Create() membership A error = %v", err)
	}
	if _, err := repo.Create(ctx, CreateBusinessUserParams{BusinessID: businessB.ID, UserID: user.ID, RoleID: manager.ID}); err != nil {
		t.Fatalf("Create() membership B error = %v", err)
	}

	memberships, err := repo.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(memberships) != 2 {
		t.Fatalf("ListByUser() returned %d memberships, want 2", len(memberships))
	}

	membershipA, err := repo.Get(ctx, businessA.ID, user.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if membershipA.RoleID != owner.ID {
		t.Errorf("membership role = %s, want %s", membershipA.RoleID, owner.ID)
	}

	err = withSavepoint(t, tx, func(q Querier) error {
		_, err := NewBusinessUserRepository(q).Create(ctx, CreateBusinessUserParams{
			BusinessID: businessA.ID, UserID: user.ID, RoleID: manager.ID,
		})
		return err
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("Create() duplicate membership error = %v, want ErrConflict", err)
	}

	err = withSavepoint(t, tx, func(q Querier) error {
		_, err := NewBusinessUserRepository(q).Create(ctx, CreateBusinessUserParams{
			BusinessID: uuid.New(), UserID: user.ID, RoleID: owner.ID,
		})
		return err
	})
	if !errors.Is(err, ErrInvalidReference) {
		t.Errorf("Create() unknown business error = %v, want ErrInvalidReference", err)
	}

	revoked, err := repo.Update(ctx, membershipA.ID, UpdateBusinessUserParams{RoleID: manager.ID, Status: StatusInactive})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if revoked.Status != StatusInactive || revoked.RoleID != manager.ID {
		t.Errorf("Update() = %+v, want manager role and INACTIVE status", revoked)
	}
}

// Deleting a business that still has members must be refused by the database.
func TestBusinessDeletionIsRestricted(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)

	owner, err := NewRoleRepository(tx).GetByName(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("GetByName(OWNER) error = %v", err)
	}

	business := createBusiness(t, tx, uniqueSlug())
	user := createUser(t, tx, "restrict."+uuid.NewString()[:8]+"@example.com")
	if _, err := NewBusinessUserRepository(tx).Create(ctx, CreateBusinessUserParams{
		BusinessID: business.ID,
		UserID:     user.ID,
		RoleID:     owner.ID,
	}); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	err = withSavepoint(t, tx, func(q Querier) error {
		_, err := q.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, business.ID)
		return err
	})
	if err == nil {
		t.Fatal("deleting a business with members succeeded, want a foreign key error")
	}
}

func TestGetActiveByAuthUserID(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)
	repo := NewUserRepository(tx)

	user := createUser(t, tx, "active."+uuid.NewString()[:8]+"@example.com")
	found, err := repo.GetActiveByAuthUserID(ctx, user.AuthUserID)
	if err != nil {
		t.Fatalf("GetActiveByAuthUserID() error = %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("id = %s, want %s", found.ID, user.ID)
	}

	if _, err := repo.Update(ctx, user.ID, UpdateUserParams{
		FullName: user.FullName,
		Email:    user.Email,
		Status:   StatusInactive,
	}); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	if _, err := repo.GetActiveByAuthUserID(ctx, user.AuthUserID); !errors.Is(err, ErrNotFound) {
		t.Errorf("inactive user error = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetActiveByAuthUserID(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user error = %v, want ErrNotFound", err)
	}
}

func TestGetActiveAccess(t *testing.T) {
	ctx := context.Background()
	tx := newTx(t)

	owner, err := NewRoleRepository(tx).GetByName(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("GetByName(OWNER) error = %v", err)
	}

	business := createBusiness(t, tx, uniqueSlug())
	other := createBusiness(t, tx, uniqueSlug())
	user := createUser(t, tx, "access."+uuid.NewString()[:8]+"@example.com")
	repo := NewBusinessUserRepository(tx)

	membership, err := repo.Create(ctx, CreateBusinessUserParams{
		BusinessID: business.ID,
		UserID:     user.ID,
		RoleID:     owner.ID,
	})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	got, err := repo.GetActiveAccess(ctx, user.ID, business.ID)
	if err != nil {
		t.Fatalf("GetActiveAccess() error = %v", err)
	}
	if got.Business.ID != business.ID || got.Role.ID != owner.ID || got.Membership.ID != membership.ID {
		t.Errorf("GetActiveAccess() = %+v", got)
	}

	if _, err := repo.GetActiveAccess(ctx, user.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-member error = %v, want ErrNotFound", err)
	}

	if _, err := repo.Update(ctx, membership.ID, UpdateBusinessUserParams{RoleID: owner.ID, Status: StatusInactive}); err != nil {
		t.Fatalf("deactivate membership: %v", err)
	}
	if _, err := repo.GetActiveAccess(ctx, user.ID, business.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("inactive membership error = %v, want ErrNotFound", err)
	}

	if _, err := repo.Update(ctx, membership.ID, UpdateBusinessUserParams{RoleID: owner.ID, Status: StatusActive}); err != nil {
		t.Fatalf("reactivate membership: %v", err)
	}
	if _, err := NewBusinessRepository(tx).Update(ctx, business.ID, UpdateBusinessParams{
		Name: business.Name, Timezone: business.Timezone, Status: StatusInactive,
	}); err != nil {
		t.Fatalf("deactivate business: %v", err)
	}
	if _, err := repo.GetActiveAccess(ctx, user.ID, business.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("inactive business error = %v, want ErrNotFound", err)
	}
}
