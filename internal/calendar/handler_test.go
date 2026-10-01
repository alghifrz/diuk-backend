package calendar

import (
	"bytes"
	"context"
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

func TestCalendarEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	managerA := uuid.New()
	staffA := uuid.New()
	denied := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()

	store := newMemoryStore()
	store.seedBusiness(businessA)
	store.seedBusiness(businessB)
	svc := NewService(store)
	block, err := svc.CreateBlock(context.Background(), businessA, ownerA, CreateBlockInput{
		Title: "Closed", StartAt: "2026-10-01T05:00:00Z", EndAt: "2026-10-01T07:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	read := []authz.Permission{authz.PermissionCalendarRead}
	write := []authz.Permission{authz.PermissionCalendarRead, authz.PermissionCalendarUpdate}
	api := newCalendarAPI(t, svc, map[uuid.UUID][]authz.Permission{
		ownerA: write, managerA: write, staffA: read, ownerB: write,
	}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})

	rec := api.do(t, http.MethodGet, "/api/v1/calendar/day?date=2026-10-01", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("day = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/calendar/events?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", businessB, "")
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte(block.ID.String())) {
		t.Fatalf("cross-tenant events = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPatch, "/api/v1/calendar/blocks/"+block.ID.String(), businessB, `{"title":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant patch = %d", rec.Code)
	}

	managerAPI := newCalendarAPI(t, svc, map[uuid.UUID][]authz.Permission{managerA: write}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: managerA, Name: tenant.RoleManager}},
	})
	rec = managerAPI.do(t, http.MethodPost, "/api/v1/calendar/blocks", businessA, `{"title":"Meeting","start_at":"2026-10-01T08:00:00Z","end_at":"2026-10-01T09:00:00Z"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manager create = %d %s", rec.Code, rec.Body.String())
	}

	staffAPI := newCalendarAPI(t, svc, map[uuid.UUID][]authz.Permission{staffA: read}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.do(t, http.MethodGet, "/api/v1/calendar/events?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("staff read = %d", rec.Code)
	}
	rec = staffAPI.do(t, http.MethodPost, "/api/v1/calendar/blocks", businessA, `{"title":"no","start_at":"2026-10-01T12:00:00Z","end_at":"2026-10-01T13:00:00Z"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff create = %d", rec.Code)
	}
	rec = staffAPI.do(t, http.MethodDelete, "/api/v1/calendar/blocks/"+block.ID.String(), businessA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff delete = %d", rec.Code)
	}

	deniedAPI := newCalendarAPI(t, svc, map[uuid.UUID][]authz.Permission{denied: {}}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: denied, Name: tenant.RoleStaff}},
	})
	rec = deniedAPI.do(t, http.MethodGet, "/api/v1/calendar/day?date=2026-10-01", businessA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied = %d", rec.Code)
	}
}

type calendarAPI struct{ handler http.Handler }

func newCalendarAPI(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *calendarAPI {
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
	cal := protected.Group("/calendar")
	cal.GET("/events", authorizer.RequirePermission(authz.PermissionCalendarRead), h.ListEvents)
	cal.GET("/day", authorizer.RequirePermission(authz.PermissionCalendarRead), h.Day)
	cal.POST("/blocks", authorizer.RequirePermission(authz.PermissionCalendarUpdate), h.CreateBlock)
	cal.PATCH("/blocks/:id", authorizer.RequirePermission(authz.PermissionCalendarUpdate), h.UpdateBlock)
	cal.DELETE("/blocks/:id", authorizer.RequirePermission(authz.PermissionCalendarUpdate), h.DeleteBlock)
	return &calendarAPI{handler: r}
}

func (a *calendarAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
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
