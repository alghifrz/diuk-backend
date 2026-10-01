package authz

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestPermissionSeed(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions`).Scan(&count); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if count != len(All()) {
		t.Fatalf("permissions count = %d, want %d", count, len(All()))
	}

	var dupes int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT code FROM permissions GROUP BY code HAVING count(*) > 1) d`).Scan(&dupes); err != nil {
		t.Fatalf("count duplicate codes: %v", err)
	}
	if dupes != 0 {
		t.Fatalf("found %d duplicate permission codes", dupes)
	}

	assertRoleSet(t, tx, tenant.RoleOwner, All())
	assertRoleSet(t, tx, tenant.RoleManager, ManagerPermissions())
	assertRoleSet(t, tx, tenant.RoleStaff, StaffPermissions())
}

func TestAuthorizationMatrix(t *testing.T) {
	tx := newTx(t)
	svc := NewService(NewRepository(tx))
	ctx := context.Background()

	roles := map[tenant.RoleName]uuid.UUID{
		tenant.RoleOwner:   roleID(t, tx, tenant.RoleOwner),
		tenant.RoleManager: roleID(t, tx, tenant.RoleManager),
		tenant.RoleStaff:   roleID(t, tx, tenant.RoleStaff),
	}

	cases := []struct {
		role       tenant.RoleName
		permission Permission
		want       bool
	}{
		{tenant.RoleOwner, PermissionBusinessRead, true},
		{tenant.RoleOwner, PermissionBusinessUpdate, true},
		{tenant.RoleOwner, PermissionAreaCreate, true},
		{tenant.RoleOwner, PermissionPromptDelete, true},
		{tenant.RoleOwner, PermissionAnalyticsRead, true},

		{tenant.RoleManager, PermissionBusinessRead, true},
		{tenant.RoleManager, PermissionBusinessUpdate, false},
		{tenant.RoleManager, PermissionAreaCreate, true},
		{tenant.RoleManager, PermissionMenuUpdate, true},
		{tenant.RoleManager, PermissionReservationConfirm, true},
		{tenant.RoleManager, PermissionBusinessUsersCreate, false},
		{tenant.RoleManager, PermissionPromptCreate, false},
		{tenant.RoleManager, PermissionPromptDelete, false},

		{tenant.RoleStaff, PermissionBusinessRead, true},
		{tenant.RoleStaff, PermissionAreaRead, true},
		{tenant.RoleStaff, PermissionAreaCreate, false},
		{tenant.RoleStaff, PermissionMenuRead, true},
		{tenant.RoleStaff, PermissionMenuUpdate, false},
		{tenant.RoleStaff, PermissionCustomerCreate, true},
		{tenant.RoleStaff, PermissionCustomerUpdate, true},
		{tenant.RoleStaff, PermissionCustomerDelete, false},
		{tenant.RoleStaff, PermissionReservationCreate, true},
		{tenant.RoleStaff, PermissionReservationConfirm, true},
		{tenant.RoleStaff, PermissionPromptRead, false},
		{tenant.RoleStaff, PermissionChannelRead, false},
		{tenant.RoleManager, PermissionChannelUpdate, true},
		{tenant.RoleOwner, PermissionChannelCreate, true},
		{tenant.RoleOwner, PermissionKnowledgeDelete, true},
		{tenant.RoleManager, PermissionKnowledgeCreate, true},
		{tenant.RoleManager, PermissionKnowledgeUpdate, true},
		{tenant.RoleStaff, PermissionKnowledgeRead, true},
		{tenant.RoleStaff, PermissionKnowledgeCreate, false},
		{tenant.RoleStaff, PermissionKnowledgeUpdate, false},
		{tenant.RoleStaff, PermissionKnowledgeDelete, false},
		{tenant.RoleOwner, PermissionAutomationDelete, true},
		{tenant.RoleManager, PermissionAutomationCreate, true},
		{tenant.RoleManager, PermissionAutomationUpdate, true},
		{tenant.RoleStaff, PermissionAutomationRead, true},
		{tenant.RoleStaff, PermissionAutomationCreate, false},
		{tenant.RoleStaff, PermissionAutomationUpdate, false},
		{tenant.RoleStaff, PermissionAutomationDelete, false},
		{tenant.RoleStaff, PermissionAnalyticsRead, false},
		{tenant.RoleStaff, PermissionReportRead, false},
		{tenant.RoleOwner, PermissionCampaignDelete, true},
		{tenant.RoleManager, PermissionCampaignCreate, true},
		{tenant.RoleManager, PermissionCampaignUpdate, true},
		{tenant.RoleStaff, PermissionCampaignRead, true},
		{tenant.RoleStaff, PermissionCampaignCreate, false},
		{tenant.RoleStaff, PermissionCampaignUpdate, false},
		{tenant.RoleStaff, PermissionCampaignDelete, false},
		{tenant.RoleOwner, PermissionUsageRead, true},
		{tenant.RoleManager, PermissionUsageRead, true},
		{tenant.RoleStaff, PermissionUsageRead, true},
		{tenant.RoleOwner, PermissionPaymentRead, true},
		{tenant.RoleOwner, PermissionPaymentUpdate, true},
		{tenant.RoleManager, PermissionPaymentRead, true},
		{tenant.RoleManager, PermissionPaymentUpdate, true},
		{tenant.RoleStaff, PermissionPaymentRead, true},
		{tenant.RoleStaff, PermissionPaymentUpdate, false},
		{tenant.RoleOwner, PermissionCalendarRead, true},
		{tenant.RoleOwner, PermissionCalendarUpdate, true},
		{tenant.RoleManager, PermissionCalendarRead, true},
		{tenant.RoleManager, PermissionCalendarUpdate, true},
		{tenant.RoleStaff, PermissionCalendarRead, true},
		{tenant.RoleStaff, PermissionCalendarUpdate, false},
	}

	for _, tt := range cases {
		t.Run(string(tt.role)+"/"+string(tt.permission), func(t *testing.T) {
			got, err := svc.HasPermission(ctx, roles[tt.role], tt.permission)
			if err != nil {
				t.Fatalf("HasPermission() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("HasPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPermissionsFollowCurrentBusinessMembership(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	svc := NewService(NewRepository(tx))

	user, err := tenant.NewUserRepository(tx).Create(ctx, tenant.CreateUserParams{
		AuthUserID: uuid.New(),
		FullName:   "Multi Tenant",
		Email:      "multi." + uuid.NewString()[:8] + "@example.com",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	staffRole := roleID(t, tx, tenant.RoleStaff)
	ownerRole := roleID(t, tx, tenant.RoleOwner)
	businessA := createBusiness(t, tx, "biz-a-"+uuid.NewString()[:8])
	businessB := createBusiness(t, tx, "biz-b-"+uuid.NewString()[:8])

	memberships := tenant.NewBusinessUserRepository(tx)
	if _, err := memberships.Create(ctx, tenant.CreateBusinessUserParams{BusinessID: businessA.ID, UserID: user.ID, RoleID: staffRole}); err != nil {
		t.Fatalf("membership A: %v", err)
	}
	if _, err := memberships.Create(ctx, tenant.CreateBusinessUserParams{BusinessID: businessB.ID, UserID: user.ID, RoleID: ownerRole}); err != nil {
		t.Fatalf("membership B: %v", err)
	}

	accessA, err := memberships.GetActiveAccess(ctx, user.ID, businessA.ID)
	if err != nil {
		t.Fatalf("access A: %v", err)
	}
	accessB, err := memberships.GetActiveAccess(ctx, user.ID, businessB.ID)
	if err != nil {
		t.Fatalf("access B: %v", err)
	}

	staffAllowed, err := svc.HasPermission(ctx, accessA.Role.ID, PermissionAreaCreate)
	if err != nil {
		t.Fatalf("staff check: %v", err)
	}
	if staffAllowed {
		t.Fatal("user received area.create while acting as STAFF in business A")
	}

	ownerAllowed, err := svc.HasPermission(ctx, accessB.Role.ID, PermissionAreaCreate)
	if err != nil {
		t.Fatalf("owner check: %v", err)
	}
	if !ownerAllowed {
		t.Fatal("user was denied area.create while acting as OWNER in business B")
	}

	staffPerms, err := svc.List(ctx, accessA.Role.ID)
	if err != nil {
		t.Fatalf("list staff: %v", err)
	}
	ownerPerms, err := svc.List(ctx, accessB.Role.ID)
	if err != nil {
		t.Fatalf("list owner: %v", err)
	}
	if len(staffPerms) != len(StaffPermissions()) {
		t.Errorf("staff permission count = %d, want %d", len(staffPerms), len(StaffPermissions()))
	}
	if len(ownerPerms) != len(All()) {
		t.Errorf("owner permission count = %d, want %d", len(ownerPerms), len(All()))
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping authorization integration tests")
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

func roleID(t *testing.T, tx pgx.Tx, name tenant.RoleName) uuid.UUID {
	t.Helper()
	role, err := tenant.NewRoleRepository(tx).GetByName(context.Background(), name)
	if err != nil {
		t.Fatalf("GetByName(%s): %v", name, err)
	}
	return role.ID
}

func createBusiness(t *testing.T, tx pgx.Tx, slug string) *tenant.Business {
	t.Helper()
	business, err := tenant.NewBusinessRepository(tx).Create(context.Background(), tenant.CreateBusinessParams{
		Name:     "Biz " + slug,
		Slug:     slug,
		Type:     tenant.BusinessTypeFNB,
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create business: %v", err)
	}
	return business
}

func assertRoleSet(t *testing.T, tx pgx.Tx, role tenant.RoleName, want []Permission) {
	t.Helper()

	got, err := NewRepository(tx).GetPermissionsByRoleID(context.Background(), roleID(t, tx, role))
	if err != nil {
		t.Fatalf("GetPermissionsByRoleID(%s): %v", role, err)
	}

	wantSet := permissionSet(want)
	gotSet := map[Permission]struct{}{}
	for _, code := range got {
		gotSet[Permission(code)] = struct{}{}
	}

	if len(gotSet) != len(wantSet) {
		t.Fatalf("%s permission count = %d, want %d", role, len(gotSet), len(wantSet))
	}
	for p := range wantSet {
		if _, ok := gotSet[p]; !ok {
			t.Errorf("%s is missing %q", role, p)
		}
	}
	for p := range gotSet {
		if _, ok := wantSet[p]; !ok {
			t.Errorf("%s has unexpected %q", role, p)
		}
	}
}
