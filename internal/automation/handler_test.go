package automation

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

func TestAutomationEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	staffA := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionAutomationRead, authz.PermissionAutomationCreate, authz.PermissionAutomationUpdate, authz.PermissionAutomationDelete},
		staffA: {authz.PermissionAutomationRead},
		ownerB: {authz.PermissionAutomationRead, authz.PermissionAutomationCreate, authz.PermissionAutomationUpdate, authz.PermissionAutomationDelete},
	}
	api := newAutomationAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	rec := api.doJSON(t, http.MethodPost, "/api/v1/automations", businessA, `{"name":"Welcome","trigger_type":"CUSTOMER_CREATED","action_type":"SEND_MESSAGE","action_config":{"template":"Halo {{customer.first_name}}"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A = %d %s", rec.Code, rec.Body.String())
	}
	idA := decodeDataID(t, rec)

	rec = api.doJSON(t, http.MethodGet, "/api/v1/automations/"+idA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant get = %d %s", rec.Code, rec.Body.String())
	}

	staffAPI := newAutomationAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	staffAPI.handler = rebuildAutomationHandler(t, api.svc, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.doJSON(t, http.MethodPost, "/api/v1/automations", businessA, `{"name":"Staff","trigger_type":"CUSTOMER_CREATED","action_type":"SEND_MESSAGE","action_config":{"template":"Halo {{customer.first_name}}"}}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff create = %d %s", rec.Code, rec.Body.String())
	}
	rec = staffAPI.doJSON(t, http.MethodGet, "/api/v1/automations", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("staff list = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/automations/"+idA+"/activate", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("activate = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.doJSON(t, http.MethodGet, "/api/v1/automations/"+idA+"/runs", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("runs = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.doJSON(t, http.MethodPost, "/api/v1/automations", businessA, `{"name":"","trigger_type":"CUSTOMER_CREATED","action_type":"SEND_MESSAGE"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid = %d %s", rec.Code, rec.Body.String())
	}
}

type automationAPI struct {
	handler http.Handler
	svc     *Service
}

func newAutomationAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *automationAPI {
	t.Helper()
	store := newMemoryStore()
	svc, _ := testService(store, true)
	return &automationAPI{handler: rebuildAutomationHandler(t, svc, perms, access), svc: svc}
}

func rebuildAutomationHandler(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, svc)
	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	autos := protected.Group("/automations")
	autos.GET("", authorizer.RequirePermission(authz.PermissionAutomationRead), h.ListAutomations)
	autos.POST("", authorizer.RequirePermission(authz.PermissionAutomationCreate), h.CreateAutomation)
	autos.GET("/:id", authorizer.RequirePermission(authz.PermissionAutomationRead), h.GetAutomation)
	autos.PATCH("/:id", authorizer.RequirePermission(authz.PermissionAutomationUpdate), h.UpdateAutomation)
	autos.DELETE("/:id", authorizer.RequirePermission(authz.PermissionAutomationDelete), h.DeleteAutomation)
	autos.POST("/:id/activate", authorizer.RequirePermission(authz.PermissionAutomationUpdate), h.ActivateAutomation)
	autos.POST("/:id/deactivate", authorizer.RequirePermission(authz.PermissionAutomationUpdate), h.DeactivateAutomation)
	autos.GET("/:id/runs", authorizer.RequirePermission(authz.PermissionAutomationRead), h.ListRuns)
	return r
}

func (a *automationAPI) doJSON(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func decodeDataID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload.Data.ID == "" {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return payload.Data.ID
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
