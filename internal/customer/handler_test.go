package customer

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

func TestCustomerAuthorization(t *testing.T) {
	owner := uuid.New()
	manager := uuid.New()
	staff := uuid.New()
	biz := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner:   {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate, authz.PermissionCustomerDelete},
		manager: {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate, authz.PermissionCustomerDelete},
		staff:   {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate},
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
		{name: "unauthenticated", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/customers" }, wantStatus: http.StatusUnauthorized},
		{name: "staff list", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/customers" }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "staff create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/customers" }, roleID: staff, roleName: tenant.RoleStaff, body: `{"name":"Pat"}`, wantStatus: http.StatusCreated},
		{name: "staff delete", method: http.MethodDelete, path: func(a *testAPI) string { return "/api/v1/customers/" + a.seedCustomer.String() }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "owner delete", method: http.MethodDelete, path: func(a *testAPI) string { return "/api/v1/customers/" + a.seedCustomer.String() }, roleID: owner, roleName: tenant.RoleOwner, wantStatus: http.StatusOK},
		{name: "manager create tag", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/tags" }, roleID: manager, roleName: tenant.RoleManager, body: `{"name":"VIP"}`, wantStatus: http.StatusCreated},
		{name: "staff delete tag", method: http.MethodDelete, path: func(a *testAPI) string { return "/api/v1/tags/" + a.seedTag.String() }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
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
			authzHeader := tt.roleID != uuid.Nil
			rec := api.do(t, tt.method, tt.path(api), biz, tt.body, authzHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestCustomerHTTPValidationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	staffB := uuid.New()
	ownerB := uuid.New()
	a, b := uuid.New(), uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate, authz.PermissionCustomerDelete},
		staffB: {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate},
		ownerB: {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate, authz.PermissionCustomerDelete},
	}
	api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b, Name: "B"}, Role: tenant.Role{ID: staffB, Name: tenant.RoleStaff}},
	})

	rec := api.do(t, http.MethodPost, "/api/v1/customers", a, `{"name":`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed json = %d", rec.Code)
	}
	rec = api.do(t, http.MethodGet, "/api/v1/customers/not-a-uuid", a, "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid = %d", rec.Code)
	}
	rec = api.do(t, http.MethodPost, "/api/v1/customers", a, `{"name":"   "}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/customers", a, `{"name":"John","email":"John@Example.com"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A = %d %s", rec.Code, rec.Body.String())
	}
	custA := decodeID(t, rec)
	rec = api.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/identities", a, `{"channel":"WHATSAPP","external_id":"6281"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("identity A = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/identities", a, `{"channel":"WHATSAPP","external_id":"6281"}`, true)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("IDENTITY_ALREADY_EXISTS")) {
		t.Fatalf("duplicate identity = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/tags", a, `{"name":"VIP"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("tag A = %d %s", rec.Code, rec.Body.String())
	}
	tagA := decodeID(t, rec)
	rec = api.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/tags/"+tagA, a, "", true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/tags/"+tagA, a, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("idempotent assign = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/customers/"+custA, b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user1@B get A = %d", rec.Code)
	}

	user2 := &testAPI{store: api.store}
	user2.handler = rebuildHandler(t, api.store, map[uuid.UUID][]authz.Permission{
		ownerB: {authz.PermissionCustomerRead, authz.PermissionCustomerCreate, authz.PermissionCustomerUpdate, authz.PermissionCustomerDelete},
	}, map[uuid.UUID]tenant.Access{
		b: {Business: tenant.Business{ID: b}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	rec = user2.do(t, http.MethodPost, "/api/v1/customers", b, `{"name":"John"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 customer = %d", rec.Code)
	}
	rec = user2.do(t, http.MethodPost, "/api/v1/tags", b, `{"name":"VIP"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 tag = %d", rec.Code)
	}
	tagB := decodeID(t, rec)
	rec = user2.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/tags/"+tagB, b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("assign A customer as B = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/customers/"+custA+"/tags/"+tagB, a, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("assign B tag to A = %d %s", rec.Code, rec.Body.String())
	}
}

type testAPI struct {
	handler      http.Handler
	store        *memoryStore
	seedCustomer uuid.UUID
	seedTag      uuid.UUID
}

func newTestAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	store := newMemoryStore()
	return &testAPI{handler: rebuildHandler(t, store, perms, access), store: store}
}

func rebuildHandler(t *testing.T, store *memoryStore, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, NewService(store))

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	cust := protected.Group("/customers")
	cust.GET("", authorizer.RequirePermission(authz.PermissionCustomerRead), h.ListCustomers)
	cust.POST("", authorizer.RequirePermission(authz.PermissionCustomerCreate), h.CreateCustomer)
	cust.GET("/:id", authorizer.RequirePermission(authz.PermissionCustomerRead), h.GetCustomer)
	cust.PATCH("/:id", authorizer.RequirePermission(authz.PermissionCustomerUpdate), h.UpdateCustomer)
	cust.DELETE("/:id", authorizer.RequirePermission(authz.PermissionCustomerDelete), h.DeleteCustomer)
	cust.GET("/:id/identities", authorizer.RequirePermission(authz.PermissionCustomerRead), h.ListIdentities)
	cust.POST("/:id/identities", authorizer.RequirePermission(authz.PermissionCustomerCreate), h.CreateIdentity)
	cust.PATCH("/:id/identities/:identity_id", authorizer.RequirePermission(authz.PermissionCustomerUpdate), h.UpdateIdentity)
	cust.DELETE("/:id/identities/:identity_id", authorizer.RequirePermission(authz.PermissionCustomerDelete), h.DeleteIdentity)
	cust.GET("/:id/tags", authorizer.RequirePermission(authz.PermissionCustomerRead), h.ListCustomerTags)
	cust.POST("/:id/tags/:tag_id", authorizer.RequirePermission(authz.PermissionCustomerCreate), h.AssignTag)
	cust.DELETE("/:id/tags/:tag_id", authorizer.RequirePermission(authz.PermissionCustomerDelete), h.UnassignTag)
	tags := protected.Group("/tags")
	tags.GET("", authorizer.RequirePermission(authz.PermissionCustomerRead), h.ListTags)
	tags.POST("", authorizer.RequirePermission(authz.PermissionCustomerCreate), h.CreateTag)
	tags.GET("/:id", authorizer.RequirePermission(authz.PermissionCustomerRead), h.GetTag)
	tags.PATCH("/:id", authorizer.RequirePermission(authz.PermissionCustomerUpdate), h.UpdateTag)
	tags.DELETE("/:id", authorizer.RequirePermission(authz.PermissionCustomerDelete), h.DeleteTag)
	return r
}

func (a *testAPI) seed(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	svc := NewService(a.store)
	c, err := svc.CreateCustomer(context.Background(), businessID, CreateCustomerInput{Name: "Seed"})
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	tag, err := svc.CreateTag(context.Background(), businessID, CreateTagInput{Name: "SeedTag"})
	if err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	a.seedCustomer = c.ID
	a.seedTag = tag.ID
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
