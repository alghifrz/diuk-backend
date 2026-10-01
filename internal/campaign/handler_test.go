package campaign

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

func TestCampaignEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	managerA := uuid.New()
	staffA := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	full := []authz.Permission{authz.PermissionCampaignRead, authz.PermissionCampaignCreate, authz.PermissionCampaignUpdate, authz.PermissionCampaignDelete}
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: full, managerA: full, staffA: {authz.PermissionCampaignRead}, ownerB: full,
	}
	api := newCampaignAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	body := `{"name":"Promo","channel":"WHATSAPP","message_kind":"TEMPLATE","template_name":"hello","template_language":"id","audience_definition":{"customer_status":"ACTIVE"}}`
	rec := api.doJSON(t, http.MethodPost, "/api/v1/campaigns", businessA, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A = %d %s", rec.Code, rec.Body.String())
	}
	idA := decodeDataID(t, rec)

	rec = api.doJSON(t, http.MethodGet, "/api/v1/campaigns/"+idA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant get = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.doJSON(t, http.MethodGet, "/api/v1/campaigns/"+idA+"/runs", businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant runs = %d %s", rec.Code, rec.Body.String())
	}

	staffAPI := newCampaignAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.doJSON(t, http.MethodPost, "/api/v1/campaigns", businessA, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff create = %d", rec.Code)
	}
	for _, path := range []string{"/schedule", "/start", "/pause", "/resume", "/cancel"} {
		rec = staffAPI.doJSON(t, http.MethodPost, "/api/v1/campaigns/"+idA+path, businessA, `{"scheduled_at":"2030-01-01T00:00:00Z"}`)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("staff %s = %d", path, rec.Code)
		}
	}
	rec = staffAPI.doJSON(t, http.MethodDelete, "/api/v1/campaigns/"+idA, businessA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff delete = %d", rec.Code)
	}
	rec = staffAPI.doJSON(t, http.MethodGet, "/api/v1/campaigns", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("staff list = %d", rec.Code)
	}

	managerAPI := newCampaignAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: managerA, Name: tenant.RoleManager}},
	})
	rec = managerAPI.doJSON(t, http.MethodPost, "/api/v1/campaigns", businessA, `{"name":"Mgr","message_kind":"TEMPLATE","template_name":"hello","template_language":"id"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manager create = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/campaigns/audience/preview", businessA, `{"audience_definition":{"customer_status":"ACTIVE"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.doJSON(t, http.MethodPost, "/api/v1/campaigns/audience/preview", businessA, `{"audience_definition":{"sql":"1=1"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown filter = %d %s", rec.Code, rec.Body.String())
	}
}

type campaignAPI struct {
	handler http.Handler
}

func newCampaignAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *campaignAPI {
	t.Helper()
	store := newMemoryStore()
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store, &stubPublisher{}, stubBusiness{}, stubConversations{store: store}, Settings{})
	return &campaignAPI{handler: rebuildCampaignHandler(t, svc, perms, access)}
}

func rebuildCampaignHandler(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
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
	camps := protected.Group("/campaigns")
	camps.POST("/audience/preview", authorizer.RequirePermission(authz.PermissionCampaignRead), h.PreviewAudience)
	camps.GET("", authorizer.RequirePermission(authz.PermissionCampaignRead), h.ListCampaigns)
	camps.POST("", authorizer.RequirePermission(authz.PermissionCampaignCreate), h.CreateCampaign)
	camps.GET("/:id", authorizer.RequirePermission(authz.PermissionCampaignRead), h.GetCampaign)
	camps.PATCH("/:id", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.UpdateCampaign)
	camps.DELETE("/:id", authorizer.RequirePermission(authz.PermissionCampaignDelete), h.DeleteCampaign)
	camps.POST("/:id/schedule", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.ScheduleCampaign)
	camps.POST("/:id/start", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.StartCampaign)
	camps.POST("/:id/pause", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.PauseCampaign)
	camps.POST("/:id/resume", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.ResumeCampaign)
	camps.POST("/:id/cancel", authorizer.RequirePermission(authz.PermissionCampaignUpdate), h.CancelCampaign)
	camps.GET("/:id/runs", authorizer.RequirePermission(authz.PermissionCampaignRead), h.ListRuns)
	camps.GET("/:id/runs/:run_id", authorizer.RequirePermission(authz.PermissionCampaignRead), h.GetRun)
	camps.GET("/:id/runs/:run_id/recipients", authorizer.RequirePermission(authz.PermissionCampaignRead), h.ListRecipients)
	return r
}

func (a *campaignAPI) doJSON(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
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
