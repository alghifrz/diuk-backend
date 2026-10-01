package reservation

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

func TestReservationAuthorization(t *testing.T) {
	owner := uuid.New()
	staff := uuid.New()
	reader := uuid.New()
	biz := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner:  {authz.PermissionReservationRead, authz.PermissionReservationCreate, authz.PermissionReservationUpdate, authz.PermissionReservationConfirm, authz.PermissionReservationCancel, authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
		staff:  {authz.PermissionReservationRead, authz.PermissionReservationCreate, authz.PermissionReservationUpdate, authz.PermissionReservationConfirm, authz.PermissionReservationCancel, authz.PermissionBusinessRead},
		reader: {authz.PermissionReservationRead, authz.PermissionBusinessRead},
	}
	tests := []struct {
		name       string
		method     string
		path       func(*testAPI) string
		roleID     uuid.UUID
		roleName   tenant.RoleName
		body       string
		wantStatus int
	}{
		{name: "unauthenticated", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/reservations" }, wantStatus: http.StatusUnauthorized},
		{name: "staff list", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/reservations" }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "reader cannot create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/reservations" }, roleID: reader, roleName: tenant.RoleStaff, body: `{"customer_id":"` + uuid.NewString() + `","start_at":"2026-10-01T19:00:00+07:00","end_at":"2026-10-01T20:30:00+07:00","party_size":2}`, wantStatus: http.StatusForbidden},
		{name: "staff cannot replace hours", method: http.MethodPut, path: func(*testAPI) string { return "/api/v1/business/hours" }, roleID: staff, roleName: tenant.RoleStaff, body: `{"hours":[]}`, wantStatus: http.StatusForbidden},
		{name: "no delete route", method: http.MethodDelete, path: func(a *testAPI) string { return "/api/v1/reservations/" + a.seedReservation.String() }, roleID: owner, roleName: tenant.RoleOwner, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access := map[uuid.UUID]tenant.Access{}
			if tt.roleID != uuid.Nil {
				access[biz] = tenant.Access{Business: tenant.Business{ID: biz}, Role: tenant.Role{ID: tt.roleID, Name: tt.roleName}}
			}
			api := newTestAPI(t, perms, access)
			if tt.roleID != uuid.Nil {
				api.seed(t, biz)
			}
			rec := api.do(t, tt.method, tt.path(api), biz, tt.body, tt.roleID != uuid.Nil)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestReservationHTTPValidationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	ownerB := uuid.New()
	a, b := uuid.New(), uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionReservationRead, authz.PermissionReservationCreate, authz.PermissionReservationUpdate, authz.PermissionReservationConfirm, authz.PermissionReservationCancel, authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
		ownerB: {authz.PermissionReservationRead, authz.PermissionReservationCreate, authz.PermissionReservationUpdate, authz.PermissionReservationConfirm, authz.PermissionReservationCancel, authz.PermissionBusinessRead, authz.PermissionBusinessUpdate},
	}
	api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	api.seed(t, a)
	api.seedOther(t, b)

	rec := api.do(t, http.MethodPost, "/api/v1/reservations", a, `{"customer_id":`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed = %d", rec.Code)
	}
	rec = api.do(t, http.MethodGet, "/api/v1/reservations/not-a-uuid", a, "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid = %d", rec.Code)
	}
	start, end := slot(19, 0, 90)
	rec = api.do(t, http.MethodPost, "/api/v1/reservations", a, `{"customer_id":"`+api.seedCustomer.String()+`","start_at":"`+end+`","end_at":"`+start+`","party_size":2}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid range = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/reservations", a, `{"customer_id":"`+api.seedCustomer.String()+`","start_at":"`+start+`","end_at":"`+end+`","party_size":2}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	id := decodeID(t, rec)
	rec = api.do(t, http.MethodGet, "/api/v1/reservations/"+id, a, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}
	rec = api.do(t, http.MethodPost, "/api/v1/reservations/"+id+"/confirm", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"CONFIRMED"`)) {
		t.Fatalf("confirm = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/reservations/availability?date=2026-10-01&start_time=19:00&duration_minutes=90&party_size=2", a, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("availability = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/reservations/"+id, b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/reservations", a, `{"customer_id":"`+api.otherCustomer.String()+`","start_at":"`+start+`","end_at":"`+end+`","party_size":2}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign customer = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPut, "/api/v1/business/hours", a, `{"hours":[{"day_of_week":1,"sort_order":0,"open_time":"09:00","close_time":"22:00"}]}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("replace hours = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/business/schedule-exceptions", a, `{"exception_date":"2026-12-25","is_closed":true}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("exception = %d %s", rec.Code, rec.Body.String())
	}
	exID := decodeID(t, rec)
	rec = api.do(t, http.MethodGet, "/api/v1/business/schedule-exceptions/"+exID, b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant exception get via wrong method = %d", rec.Code)
	}
	rec = api.do(t, http.MethodPatch, "/api/v1/business/schedule-exceptions/"+exID, b, `{"note":"x"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant exception = %d %s", rec.Code, rec.Body.String())
	}
}

type testAPI struct {
	handler         http.Handler
	store           *memoryStore
	svc             *Service
	seedCustomer    uuid.UUID
	seedReservation uuid.UUID
	otherCustomer   uuid.UUID
}

func newTestAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	store := newMemoryStore()
	svc := NewService(store)
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, loc) }
	return &testAPI{handler: rebuildHandler(t, svc, perms, access), store: store, svc: svc}
}

func rebuildHandler(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
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
	biz := protected.Group("/business")
	biz.GET("/hours", authorizer.RequirePermission(authz.PermissionBusinessRead), h.GetHours)
	biz.PUT("/hours", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.ReplaceHours)
	biz.PATCH("/hours", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.ReplaceHours)
	biz.GET("/schedule-exceptions", authorizer.RequirePermission(authz.PermissionBusinessRead), h.ListExceptions)
	biz.POST("/schedule-exceptions", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.CreateException)
	biz.PATCH("/schedule-exceptions/:id", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.UpdateException)
	biz.DELETE("/schedule-exceptions/:id", authorizer.RequirePermission(authz.PermissionBusinessUpdate), h.DeleteException)
	res := protected.Group("/reservations")
	res.GET("", authorizer.RequirePermission(authz.PermissionReservationRead), h.ListReservations)
	res.GET("/availability", authorizer.RequirePermission(authz.PermissionReservationRead), h.Availability)
	res.POST("", authorizer.RequirePermission(authz.PermissionReservationCreate), h.CreateReservation)
	res.GET("/:id", authorizer.RequirePermission(authz.PermissionReservationRead), h.GetReservation)
	res.PATCH("/:id", authorizer.RequirePermission(authz.PermissionReservationUpdate), h.UpdateReservation)
	res.POST("/:id/confirm", authorizer.RequirePermission(authz.PermissionReservationConfirm), h.ConfirmReservation)
	res.POST("/:id/cancel", authorizer.RequirePermission(authz.PermissionReservationCancel), h.CancelReservation)
	res.POST("/:id/complete", authorizer.RequirePermission(authz.PermissionReservationUpdate), h.CompleteReservation)
	res.PATCH("/:id/reschedule", authorizer.RequirePermission(authz.PermissionReservationUpdate), h.RescheduleReservation)
	return r
}

func (a *testAPI) seed(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	a.store.seedContext(businessID, "Asia/Jakarta", true)
	if _, err := a.svc.ReplaceHours(context.Background(), businessID, []HourInput{
		{DayOfWeek: 4, SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"},
	}); err != nil {
		t.Fatalf("hours: %v", err)
	}
	a.seedCustomer = a.store.addCustomer(businessID, "John", "ACTIVE", ptr("0811"))
	_ = a.store.addTable(businessID, "T4", "Indoor", 4, "ACTIVE", "ACTIVE")
	start, end := slot(19, 0, 90)
	item, err := a.svc.Create(context.Background(), businessID, CreateReservationInput{
		CustomerID: a.seedCustomer.String(), StartAt: start, EndAt: end, PartySize: 2,
	})
	if err != nil {
		t.Fatalf("seed reservation: %v", err)
	}
	a.seedReservation = item.ID
}

func (a *testAPI) seedOther(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	a.store.seedContext(businessID, "Asia/Jakarta", true)
	a.otherCustomer = a.store.addCustomer(businessID, "Other", "ACTIVE", nil)
}

func (a *testAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body string, withAuth bool) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if withAuth {
		req.Header.Set("Authorization", "Bearer good")
		req.Header.Set("X-Business-ID", businessID.String())
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func decodeID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
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
