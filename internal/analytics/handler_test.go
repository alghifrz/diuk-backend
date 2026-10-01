package analytics

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

func TestAnalyticsAuthorizationAndIsolation(t *testing.T) {
	owner := uuid.New()
	staff := uuid.New()
	a, b := uuid.New(), uuid.New()
	store := newMemoryStore()
	store.overview[a] = Overview{TotalConversations: 5}
	store.overview[b] = Overview{TotalConversations: 8}
	handler := newAnalyticsHandler(t, store, map[uuid.UUID][]authz.Permission{
		owner: {authz.PermissionAnalyticsRead},
		staff: {},
	}, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
	})

	rec := do(handler, http.MethodGet, "/api/v1/analytics/overview", a, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"total_conversations":5`)) {
		t.Fatalf("owner A = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(handler, http.MethodGet, "/api/v1/analytics/overview", b, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"total_conversations":8`)) {
		t.Fatalf("owner B = %d %s", rec.Code, rec.Body.String())
	}

	staffHandler := newAnalyticsHandler(t, store, map[uuid.UUID][]authz.Permission{
		staff: {},
	}, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: staff, Name: tenant.RoleStaff}},
	})
	rec = do(staffHandler, http.MethodGet, "/api/v1/analytics/overview", a, true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff without analytics.read = %d", rec.Code)
	}

	rec = do(handler, http.MethodGet, "/api/v1/analytics/overview?from=2026-09-24&to=2026-09-01", a, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad range = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(handler, http.MethodGet, "/api/v1/analytics/conversion", a, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated conversion = %d", rec.Code)
	}

	for _, path := range []string{
		"/api/v1/analytics/conversations",
		"/api/v1/analytics/customers",
		"/api/v1/analytics/reservations",
		"/api/v1/analytics/conversion",
		"/api/v1/analytics/revenue",
		"/api/v1/analytics/ai",
		"/api/v1/analytics/handover",
		"/api/v1/analytics/automation",
		"/api/v1/analytics/messaging",
		"/api/v1/analytics/campaigns",
	} {
		rec = do(handler, http.MethodGet, path+"?from=2026-09-01&to=2026-09-24", a, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func newAnalyticsHandler(t *testing.T, store *memoryStore, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubAuthUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, NewService(store, stubTZ{tz: "UTC"}))
	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	g := protected.Group("/analytics")
	g.GET("/overview", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Overview)
	g.GET("/conversations", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Conversations)
	g.GET("/customers", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Customers)
	g.GET("/reservations", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Reservations)
	g.GET("/conversion", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Conversion)
	g.GET("/revenue", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Revenue)
	g.GET("/ai", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.AI)
	g.GET("/handover", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Handover)
	g.GET("/automation", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Automation)
	g.GET("/messaging", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Messaging)
	g.GET("/campaigns", authorizer.RequirePermission(authz.PermissionAnalyticsRead), h.Campaigns)
	return r
}

func do(h http.Handler, method, path string, businessID uuid.UUID, withAuth bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if withAuth {
		req.Header.Set("Authorization", "Bearer good")
		req.Header.Set("X-Business-ID", businessID.String())
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type stubVerifier struct{ id uuid.UUID }

func (s stubVerifier) Verify(token string) (uuid.UUID, error) {
	if token != "good" {
		return uuid.Nil, auth.ErrInvalidToken
	}
	return s.id, nil
}

type stubAuthUsers struct{ user *tenant.User }

func (s stubAuthUsers) GetActiveByAuthUserID(context.Context, uuid.UUID) (*tenant.User, error) {
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

var _ = json.Marshal
