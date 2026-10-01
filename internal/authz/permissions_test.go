package authz

import (
	"strings"
	"testing"
)

func TestV1PermissionSet(t *testing.T) {
	all := All()
	if len(all) != 52 {
		t.Fatalf("All() count = %d, want 52", len(all))
	}

	seen := map[Permission]struct{}{}
	for _, p := range all {
		if _, ok := seen[p]; ok {
			t.Errorf("duplicate permission %q", p)
		}
		seen[p] = struct{}{}

		resource, action, ok := strings.Cut(string(p), ".")
		if !ok || resource == "" || action == "" || strings.Count(string(p), ".") != 1 {
			t.Errorf("permission %q is not resource.action", p)
		}
	}

	if _, ok := seen["reservation.delete"]; ok {
		t.Error("V1 must not include reservation.delete")
	}

	if got, want := len(ManagerPermissions()), 46; got != want {
		t.Errorf("ManagerPermissions() count = %d, want %d", got, want)
	}
	if got, want := len(StaffPermissions()), 19; got != want {
		t.Errorf("StaffPermissions() count = %d, want %d", got, want)
	}

	manager := permissionSet(ManagerPermissions())
	staff := permissionSet(StaffPermissions())
	for _, p := range ManagerPermissions() {
		if _, ok := seen[p]; !ok {
			t.Errorf("manager permission %q is not in All()", p)
		}
	}
	for _, p := range StaffPermissions() {
		if _, ok := seen[p]; !ok {
			t.Errorf("staff permission %q is not in All()", p)
		}
		if _, ok := manager[p]; !ok {
			t.Errorf("staff permission %q is missing from manager", p)
		}
	}

	deniedManager := []Permission{
		PermissionBusinessUpdate,
		PermissionBusinessUsersCreate,
		PermissionBusinessUsersUpdate,
		PermissionBusinessUsersDelete,
		PermissionPromptCreate,
		PermissionPromptDelete,
	}
	for _, p := range deniedManager {
		if _, ok := manager[p]; ok {
			t.Errorf("manager must not have %q", p)
		}
	}

	deniedStaff := []Permission{
		PermissionBusinessUpdate,
		PermissionBusinessUsersRead,
		PermissionAreaCreate,
		PermissionMenuUpdate,
		PermissionCustomerDelete,
		PermissionPromptRead,
		PermissionChannelRead,
		PermissionKnowledgeCreate,
		PermissionKnowledgeUpdate,
		PermissionKnowledgeDelete,
		PermissionAutomationCreate,
		PermissionAutomationUpdate,
		PermissionAutomationDelete,
		PermissionAnalyticsRead,
		PermissionReportRead,
		PermissionCampaignCreate,
		PermissionCampaignUpdate,
		PermissionCampaignDelete,
		PermissionPaymentUpdate,
		PermissionCalendarUpdate,
	}
	for _, p := range deniedStaff {
		if _, ok := staff[p]; ok {
			t.Errorf("staff must not have %q", p)
		}
	}
}
