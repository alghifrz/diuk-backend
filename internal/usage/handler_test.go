package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestUsageEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	managerA := uuid.New()
	staffA := uuid.New()
	denied := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	read := []authz.Permission{authz.PermissionUsageRead}
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: read, managerA: read, staffA: read, ownerB: read,
	}

	store := newMemoryStore()
	plan, ents := standardPlan()
	store.seedPlan(plan, ents)
	store.assign(businessA, "Asia/Jakarta", plan)
	store.assign(businessB, "Asia/Makassar", plan)
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	svc.now = func() time.Time { return time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC) }
	if _, err := svc.Consume(context.Background(), businessA, ConsumeInput{
		Feature: FeatureAIResponses, SourceType: SourceAIGeneration, SourceID: uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}

	api := newUsageAPI(t, svc, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})

	rec := api.do(t, http.MethodGet, "/api/v1/usage", businessA)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner usage = %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			Plan struct {
				Code string `json:"code"`
			} `json:"plan"`
			Period struct {
				Start    string `json:"start"`
				Timezone string `json:"timezone"`
			} `json:"period"`
			Usage []struct {
				Feature string `json:"feature"`
				Used    int64  `json:"used"`
				Limit   *int   `json:"limit"`
			} `json:"usage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Plan.Code != PlanStandard || payload.Data.Period.Timezone != "Asia/Jakarta" {
		t.Fatalf("snapshot = %+v", payload.Data)
	}
	if payload.Data.Period.Start != "2026-09-01T00:00:00+07:00" {
		t.Fatalf("period start = %s", payload.Data.Period.Start)
	}
	found := false
	for _, item := range payload.Data.Usage {
		if item.Feature == string(FeatureAIResponses) {
			found = true
			if item.Used != 1 || item.Limit == nil || *item.Limit != 25000 {
				t.Fatalf("ai usage = %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("AI_RESPONSES missing")
	}

	rec = api.do(t, http.MethodGet, "/api/v1/usage", businessB)
	if rec.Code != http.StatusOK {
		t.Fatalf("other usage = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, item := range payload.Data.Usage {
		if item.Feature == string(FeatureAIResponses) && item.Used != 0 {
			t.Fatalf("tenant B saw A usage: %+v", item)
		}
	}

	for _, roleID := range []uuid.UUID{managerA, staffA} {
		roleAPI := newUsageAPI(t, svc, perms, map[uuid.UUID]tenant.Access{
			businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: roleID, Name: tenant.RoleStaff}},
		})
		rec = roleAPI.do(t, http.MethodGet, "/api/v1/usage", businessA)
		if rec.Code != http.StatusOK {
			t.Fatalf("role %s usage = %d", roleID, rec.Code)
		}
		rec = roleAPI.do(t, http.MethodGet, "/api/v1/usage/history", businessA)
		if rec.Code != http.StatusOK {
			t.Fatalf("role %s history = %d", roleID, rec.Code)
		}
	}

	deniedAPI := newUsageAPI(t, svc, map[uuid.UUID][]authz.Permission{denied: {}}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: denied, Name: tenant.RoleStaff}},
	})
	rec = deniedAPI.do(t, http.MethodGet, "/api/v1/usage", businessA)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied = %d", rec.Code)
	}
}

type usageAPI struct {
	handler http.Handler
}

func newUsageAPI(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *usageAPI {
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
	protected.GET("/usage", authorizer.RequirePermission(authz.PermissionUsageRead), h.GetUsage)
	protected.GET("/usage/history", authorizer.RequirePermission(authz.PermissionUsageRead), h.GetHistory)
	return &usageAPI{handler: r}
}

func (a *usageAPI) do(t *testing.T, method, path string, businessID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBuffer(nil))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())
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
