package authz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRequirePermission(t *testing.T) {
	ownerRole := uuid.New()
	staffRole := uuid.New()
	checker := &mapChecker{allowed: map[uuid.UUID][]Permission{
		ownerRole: All(),
		staffRole: StaffPermissions(),
	}}

	tests := []struct {
		name       string
		ctx        *auth.Context
		permission Permission
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing auth context",
			permission: PermissionBusinessRead,
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
		},
		{
			name:       "missing tenant",
			ctx:        &auth.Context{UserID: uuid.New()},
			permission: PermissionBusinessRead,
			wantStatus: http.StatusForbidden,
			wantCode:   codeForbidden,
		},
		{
			name: "owner business.update allowed",
			ctx: &auth.Context{
				UserID: uuid.New(), BusinessID: uuid.New(), RoleID: ownerRole, RoleName: tenant.RoleOwner,
			},
			permission: PermissionBusinessUpdate,
			wantStatus: http.StatusOK,
		},
		{
			name: "staff business.update denied",
			ctx: &auth.Context{
				UserID: uuid.New(), BusinessID: uuid.New(), RoleID: staffRole, RoleName: tenant.RoleStaff,
			},
			permission: PermissionBusinessUpdate,
			wantStatus: http.StatusForbidden,
			wantCode:   codeForbidden,
		},
		{
			name: "staff area.read allowed",
			ctx: &auth.Context{
				UserID: uuid.New(), BusinessID: uuid.New(), RoleID: staffRole, RoleName: tenant.RoleStaff,
			},
			permission: PermissionAreaRead,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performAuthz(t, checker, tt.ctx, tt.permission)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" && errorCode(t, rec) != tt.wantCode {
				t.Errorf("code = %q, want %q", errorCode(t, rec), tt.wantCode)
			}
			if rec.Code == http.StatusForbidden {
				if strings.Contains(rec.Body.String(), string(tt.permission)) || strings.Contains(rec.Body.String(), "STAFF") {
					t.Errorf("forbidden response leaked internals: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestRequirePermissionDoesNotTreatLookupErrorAsDenied(t *testing.T) {
	checker := &errorChecker{err: errors.New("connection refused")}
	ctx := &auth.Context{UserID: uuid.New(), BusinessID: uuid.New(), RoleID: uuid.New(), RoleName: tenant.RoleOwner}

	rec := performAuthz(t, checker, ctx, PermissionBusinessRead)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 body=%s", rec.Code, rec.Body.String())
	}
	if errorCode(t, rec) != codeInternal {
		t.Errorf("code = %q, want INTERNAL", errorCode(t, rec))
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("response leaked the database error: %s", rec.Body.String())
	}
}

func TestRequirePermissionUsesCurrentBusinessRole(t *testing.T) {
	staffRole := uuid.New()
	ownerRole := uuid.New()
	userID := uuid.New()
	checker := &mapChecker{allowed: map[uuid.UUID][]Permission{
		staffRole: StaffPermissions(),
		ownerRole: All(),
	}}

	staffRec := performAuthz(t, checker, &auth.Context{
		UserID: userID, BusinessID: uuid.New(), RoleID: staffRole, RoleName: tenant.RoleStaff,
	}, PermissionAreaCreate)
	if staffRec.Code != http.StatusForbidden {
		t.Fatalf("staff status = %d, want 403", staffRec.Code)
	}

	ownerRec := performAuthz(t, checker, &auth.Context{
		UserID: userID, BusinessID: uuid.New(), RoleID: ownerRole, RoleName: tenant.RoleOwner,
	}, PermissionAreaCreate)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200", ownerRec.Code)
	}
}

func performAuthz(t *testing.T, checker Checker, ac *auth.Context, permission Permission) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	m := NewMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), checker)
	r := gin.New()
	r.GET("/protected", func(c *gin.Context) {
		if ac != nil {
			c.Set("auth.context", *ac)
		}
		c.Next()
	}, m.RequirePermission(permission), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/protected", nil))
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error: %v body=%s", err, rec.Body.String())
	}
	return payload.Error.Code
}

type mapChecker struct {
	allowed map[uuid.UUID][]Permission
}

func (m *mapChecker) HasPermission(_ context.Context, roleID uuid.UUID, permission Permission) (bool, error) {
	for _, p := range m.allowed[roleID] {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

func (m *mapChecker) List(_ context.Context, roleID uuid.UUID) ([]string, error) {
	out := make([]string, 0, len(m.allowed[roleID]))
	for _, p := range m.allowed[roleID] {
		out = append(out, string(p))
	}
	return out, nil
}

type errorChecker struct {
	err error
}

func (e *errorChecker) HasPermission(context.Context, uuid.UUID, Permission) (bool, error) {
	return false, e.err
}

func (e *errorChecker) List(context.Context, uuid.UUID) ([]string, error) {
	return nil, e.err
}
