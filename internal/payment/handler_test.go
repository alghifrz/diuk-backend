package payment

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
	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestPaymentEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	managerA := uuid.New()
	staffA := uuid.New()
	denied := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	resA := uuid.New()
	resB := uuid.New()

	store := newMemoryStore()
	store.seedSettings(businessA, DepositSettings{Enabled: true, Type: DepositFixed, Value: money.FromMajor(100000), Currency: "IDR"})
	store.seedSettings(businessB, DepositSettings{Currency: "IDR", Type: DepositFixed})
	store.addReservation(businessA, resA, uuid.New())
	store.addReservation(businessB, resB, uuid.New())
	svc := NewService(store)
	payA, err := svc.CreateInitial(context.Background(), businessA, resA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateInitial(context.Background(), businessB, resB, nil); err != nil {
		t.Fatal(err)
	}

	read := []authz.Permission{authz.PermissionPaymentRead}
	write := []authz.Permission{authz.PermissionPaymentRead, authz.PermissionPaymentUpdate}
	api := newPaymentAPI(t, svc, map[uuid.UUID][]authz.Permission{
		ownerA: write, managerA: write, staffA: read, ownerB: write,
	}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})

	rec := api.do(t, http.MethodGet, "/api/v1/reservations/"+resA.String()+"/payment", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get payment = %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Amount any    `json:"amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ID != payA.ID.String() || payload.Data.Status != StatusUnpaid {
		t.Fatalf("payload = %+v", payload.Data)
	}

	rec = api.do(t, http.MethodGet, "/api/v1/reservations/"+resB.String()+"/payment", businessA, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/reservation-payments/"+payA.ID.String()+"/confirm", businessB, `{"notes":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant confirm = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/reservation-payments/"+payA.ID.String()+"/confirm", businessA, `{"notes":"Transfer verified"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner confirm = %d %s", rec.Code, rec.Body.String())
	}

	managerAPI := newPaymentAPI(t, svc, map[uuid.UUID][]authz.Permission{managerA: write}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: managerA, Name: tenant.RoleManager}},
	})
	rec = managerAPI.do(t, http.MethodGet, "/api/v1/reservation-payments", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("manager list = %d", rec.Code)
	}

	staffAPI := newPaymentAPI(t, svc, map[uuid.UUID][]authz.Permission{staffA: read}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.do(t, http.MethodGet, "/api/v1/reservations/"+resA.String()+"/payment", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("staff read = %d", rec.Code)
	}
	rec = staffAPI.do(t, http.MethodPost, "/api/v1/reservation-payments/"+payA.ID.String()+"/refund", businessA, `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff refund = %d", rec.Code)
	}
	rec = staffAPI.do(t, http.MethodPost, "/api/v1/reservation-payments/"+payA.ID.String()+"/confirm", businessA, `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff confirm = %d", rec.Code)
	}
	rec = staffAPI.do(t, http.MethodPatch, "/api/v1/reservation-payments/"+payA.ID.String(), businessA, `{"notes":"no"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff patch = %d", rec.Code)
	}

	deniedAPI := newPaymentAPI(t, svc, map[uuid.UUID][]authz.Permission{denied: {}}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: denied, Name: tenant.RoleStaff}},
	})
	rec = deniedAPI.do(t, http.MethodGet, "/api/v1/reservation-payments", businessA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied = %d", rec.Code)
	}
}

type paymentAPI struct {
	handler http.Handler
}

func newPaymentAPI(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *paymentAPI {
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
	protected.GET("/reservations/:id/payment", authorizer.RequirePermission(authz.PermissionPaymentRead), h.GetByReservation)
	protected.POST("/reservations/:id/payment", authorizer.RequirePermission(authz.PermissionPaymentUpdate), h.CreateForReservation)
	protected.GET("/reservation-payments", authorizer.RequirePermission(authz.PermissionPaymentRead), h.ListPayments)
	protected.PATCH("/reservation-payments/:id", authorizer.RequirePermission(authz.PermissionPaymentUpdate), h.UpdatePayment)
	protected.POST("/reservation-payments/:id/confirm", authorizer.RequirePermission(authz.PermissionPaymentUpdate), h.ConfirmPayment)
	protected.POST("/reservation-payments/:id/mark-full", authorizer.RequirePermission(authz.PermissionPaymentUpdate), h.MarkFullPayment)
	protected.POST("/reservation-payments/:id/refund", authorizer.RequirePermission(authz.PermissionPaymentUpdate), h.RefundPayment)
	return &paymentAPI{handler: r}
}

func (a *paymentAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
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
