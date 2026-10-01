package authz

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestServiceRejectsEmptyPermission(t *testing.T) {
	svc := NewService(NewRepository(nil))

	allowed, err := svc.HasPermission(context.Background(), uuid.New(), "")
	if err == nil || !strings.Contains(err.Error(), "permission is required") {
		t.Fatalf("HasPermission() error = %v, want a missing permission error", err)
	}
	if allowed {
		t.Error("HasPermission() allowed = true for an empty permission")
	}
}

func TestServiceNilRoleIsDeniedNotAnError(t *testing.T) {
	svc := NewService(NewRepository(nil))

	allowed, err := svc.HasPermission(context.Background(), uuid.Nil, PermissionBusinessRead)
	if err != nil {
		t.Fatalf("HasPermission() error = %v", err)
	}
	if allowed {
		t.Error("HasPermission() allowed = true for a nil role")
	}

	codes, err := svc.List(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(codes) != 0 {
		t.Errorf("List() = %v, want empty", codes)
	}
}
