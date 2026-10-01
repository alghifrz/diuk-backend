package business

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestBusinessEndpointsAuthorization(t *testing.T) {
	ownerRole := uuid.New()
	managerRole := uuid.New()
	staffRole := uuid.New()
	businessID := uuid.New()

	tests := []struct {
		name       string
		method     string
		path       string
		roleID     uuid.UUID
		roleName   tenant.RoleName
		wantStatus int
	}{
		{name: "owner get profile", method: http.MethodGet, path: "/api/v1/business", roleID: ownerRole, roleName: tenant.RoleOwner, wantStatus: http.StatusOK},
		{name: "owner patch profile", method: http.MethodPatch, path: "/api/v1/business", roleID: ownerRole, roleName: tenant.RoleOwner, wantStatus: http.StatusOK},
		{name: "manager get profile", method: http.MethodGet, path: "/api/v1/business", roleID: managerRole, roleName: tenant.RoleManager, wantStatus: http.StatusOK},
		{name: "manager patch profile", method: http.MethodPatch, path: "/api/v1/business", roleID: managerRole, roleName: tenant.RoleManager, wantStatus: http.StatusForbidden},
		{name: "staff get profile", method: http.MethodGet, path: "/api/v1/business", roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "staff patch profile", method: http.MethodPatch, path: "/api/v1/business", roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "owner get settings", method: http.MethodGet, path: "/api/v1/business/fnb-settings", roleID: ownerRole, roleName: tenant.RoleOwner, wantStatus: http.StatusOK},
		{name: "manager patch settings", method: http.MethodPatch, path: "/api/v1/business/fnb-settings", roleID: managerRole, roleName: tenant.RoleManager, wantStatus: http.StatusForbidden},
		{name: "staff patch settings", method: http.MethodPatch, path: "/api/v1/business/fnb-settings", roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestAPI(t, businessID, map[uuid.UUID][]authz.Permission{
				ownerRole:   {authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
				managerRole: {authz.PermissionBusinessRead},
				staffRole:   {authz.PermissionBusinessRead},
			}, map[uuid.UUID]tenant.Access{
				businessID: {Business: tenant.Business{ID: businessID, Name: "Kasiko"}, Role: tenant.Role{ID: tt.roleID, Name: tt.roleName}},
			})

			body := "{}"
			if tt.method == http.MethodPatch && tt.path == "/api/v1/business" {
				body = `{"name":"Kasiko Coffee Jakarta"}`
			}
			if tt.method == http.MethodPatch && tt.path == "/api/v1/business/fnb-settings" {
				body = `{"reservation_slot_duration_minutes":45}`
			}

			rec := h.do(t, tt.method, tt.path, businessID, body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHandlerUsesTenantContextNotRequestBusinessID(t *testing.T) {
	ownerRole := uuid.New()
	staffRole := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()

	h := newTestAPI(t, businessA, map[uuid.UUID][]authz.Permission{
		ownerRole: {authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
		staffRole: {authz.PermissionBusinessRead},
	}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerRole, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: staffRole, Name: tenant.RoleStaff}},
	})
	h.store.profiles[businessB] = &Profile{ID: businessB, Name: "Business B", Slug: "business-b", Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta", Status: tenant.StatusActive}
	h.store.settings[businessB] = defaultSettings(businessB)

	rec := h.do(t, http.MethodPatch, "/api/v1/business", businessB, `{"name":"Hacked","business_id":"`+businessA.String()+`"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("update as STAFF of B status = %d, want 403 body=%s", rec.Code, rec.Body.String())
	}
	if h.store.profiles[businessA].Name != "Kasiko Coffee" {
		t.Error("STAFF on B was able to change business A")
	}

	rec = h.do(t, http.MethodGet, "/api/v1/business?business_id="+businessA.String(), businessB, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get as STAFF of B status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(businessB.String())) {
		t.Errorf("GET used a request business id instead of tenant context: %s", rec.Body.String())
	}
}

func TestGetProfileAndSettingsHTTP(t *testing.T) {
	roleID := uuid.New()
	businessID := uuid.New()
	h := newTestAPI(t, businessID, map[uuid.UUID][]authz.Permission{
		roleID: {authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
	}, map[uuid.UUID]tenant.Access{
		businessID: {Business: tenant.Business{ID: businessID}, Role: tenant.Role{ID: roleID, Name: tenant.RoleOwner}},
	})

	rec := h.do(t, http.MethodGet, "/api/v1/business", businessID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get profile status = %d body=%s", rec.Code, rec.Body.String())
	}
	var profile struct {
		Data profileJSON `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if profile.Data.Slug != "kasiko-coffee" || profile.Data.Name != "Kasiko Coffee" {
		t.Errorf("profile = %+v", profile.Data)
	}

	rec = h.do(t, http.MethodGet, "/api/v1/business/fnb-settings", businessID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings status = %d body=%s", rec.Code, rec.Body.String())
	}
	var settings struct {
		Data fnbSettingsJSON `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if settings.Data.Currency != "IDR" || settings.Data.ReservationSlotDurationMinutes != 30 {
		t.Errorf("settings = %+v", settings.Data)
	}
}

type testAPI struct {
	handler http.Handler
	store   *memoryStore
}

func newTestAPI(t *testing.T, defaultBusiness uuid.UUID, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store := newMemoryStore(defaultBusiness)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()

	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, NewService(store))

	r := gin.New()
	v1 := r.Group("/api/v1")
	protected := v1.Group("")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	biz := protected.Group("/business")
	biz.GET("", authorizer.RequirePermission(authz.PermissionBusinessRead), h.GetProfile)
	biz.PATCH("", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.UpdateProfile)
	biz.GET("/fnb-settings", authorizer.RequirePermission(authz.PermissionBusinessRead), h.GetFNBSettings)
	biz.PATCH("/fnb-settings", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.UpdateFNBSettings)

	return &testAPI{handler: r, store: store}
}

func (a *testAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

type stubVerifier struct{ id uuid.UUID }

func (s stubVerifier) Verify(token string) (uuid.UUID, error) {
	if token != "good" {
		return uuid.Nil, auth.ErrInvalidToken
	}
	return s.id, nil
}

type stubUsers struct{ user *tenant.User }

func (s stubUsers) GetActiveByAuthUserID(context.Context, uuid.UUID) (*tenant.User, error) {
	return s.user, nil
}

type stubAccess struct {
	byBusiness map[uuid.UUID]tenant.Access
	userID     uuid.UUID
}

func (s stubAccess) GetActiveAccess(_ context.Context, userID, businessID uuid.UUID) (*tenant.Access, error) {
	access, ok := s.byBusiness[businessID]
	if !ok {
		return nil, tenant.ErrNotFound
	}
	access.Membership.UserID = userID
	access.Membership.BusinessID = businessID
	access.Business.ID = businessID
	return &access, nil
}

type mapChecker struct {
	allowed map[uuid.UUID][]authz.Permission
}

func (m mapChecker) HasPermission(_ context.Context, roleID uuid.UUID, permission authz.Permission) (bool, error) {
	for _, p := range m.allowed[roleID] {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

func (m mapChecker) List(context.Context, uuid.UUID) ([]string, error) { return nil, nil }
