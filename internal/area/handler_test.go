package area

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

func TestAreaEndpointsAuthorization(t *testing.T) {
	ownerRole := uuid.New()
	managerRole := uuid.New()
	staffRole := uuid.New()
	businessID := uuid.New()

	perms := map[uuid.UUID][]authz.Permission{
		ownerRole:   {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
		managerRole: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
		staffRole:   {authz.PermissionAreaRead},
	}

	tests := []struct {
		name       string
		method     string
		path       func(api *testAPI) string
		roleID     uuid.UUID
		roleName   tenant.RoleName
		body       string
		wantStatus int
	}{
		{name: "owner create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/areas" }, roleID: ownerRole, roleName: tenant.RoleOwner, body: `{"name":"Indoor"}`, wantStatus: http.StatusCreated},
		{name: "manager create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/areas" }, roleID: managerRole, roleName: tenant.RoleManager, body: `{"name":"Outdoor"}`, wantStatus: http.StatusCreated},
		{name: "staff create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/areas" }, roleID: staffRole, roleName: tenant.RoleStaff, body: `{"name":"VIP"}`, wantStatus: http.StatusForbidden},
		{name: "staff list", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/areas" }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "staff get", method: http.MethodGet, path: func(api *testAPI) string { return "/api/v1/areas/" + api.seedArea.String() }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "staff patch", method: http.MethodPatch, path: func(api *testAPI) string { return "/api/v1/areas/" + api.seedArea.String() }, roleID: staffRole, roleName: tenant.RoleStaff, body: `{"name":"Nope"}`, wantStatus: http.StatusForbidden},
		{name: "staff delete", method: http.MethodDelete, path: func(api *testAPI) string { return "/api/v1/areas/" + api.seedArea.String() }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "staff create table", method: http.MethodPost, path: func(api *testAPI) string { return "/api/v1/areas/" + api.seedArea.String() + "/tables" }, roleID: staffRole, roleName: tenant.RoleStaff, body: `{"name":"T1","capacity":2}`, wantStatus: http.StatusForbidden},
		{name: "owner create table", method: http.MethodPost, path: func(api *testAPI) string { return "/api/v1/areas/" + api.seedArea.String() + "/tables" }, roleID: ownerRole, roleName: tenant.RoleOwner, body: `{"name":"Table 01","capacity":4}`, wantStatus: http.StatusCreated},
		{name: "manager patch table", method: http.MethodPatch, path: func(api *testAPI) string {
			return "/api/v1/areas/" + api.seedArea.String() + "/tables/" + api.seedTable.String()
		}, roleID: managerRole, roleName: tenant.RoleManager, body: `{"capacity":6}`, wantStatus: http.StatusOK},
		{name: "staff delete table", method: http.MethodDelete, path: func(api *testAPI) string {
			return "/api/v1/areas/" + api.seedArea.String() + "/tables/" + api.seedTable.String()
		}, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
				businessID: {Business: tenant.Business{ID: businessID, Name: "Kasiko"}, Role: tenant.Role{ID: tt.roleID, Name: tt.roleName}},
			})
			api.seedLayout(t, businessID)

			rec := api.do(t, tt.method, tt.path(api), businessID, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestAreaHTTPValidationAndSoftDelete(t *testing.T) {
	roleID := uuid.New()
	businessID := uuid.New()
	api := newTestAPI(t, map[uuid.UUID][]authz.Permission{
		roleID: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
	}, map[uuid.UUID]tenant.Access{
		businessID: {Business: tenant.Business{ID: businessID}, Role: tenant.Role{ID: roleID, Name: tenant.RoleOwner}},
	})

	rec := api.do(t, http.MethodPost, "/api/v1/areas", businessID, `{"name":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/areas", businessID, `{"name":"Indoor","description":"Main hall"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create area status = %d body=%s", rec.Code, rec.Body.String())
	}
	areaID := decodeDataID(t, rec)

	rec = api.do(t, http.MethodPost, "/api/v1/areas", businessID, `{"name":"Indoor"}`)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("AREA_NAME_ALREADY_EXISTS")) {
		t.Fatalf("duplicate area = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/areas/"+areaID+"/tables", businessID, `{"name":"Table 01","capacity":0}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad capacity status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/areas/"+areaID+"/tables", businessID, `{"name":"Table 01","capacity":4}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create table status = %d body=%s", rec.Code, rec.Body.String())
	}
	tableID := decodeDataID(t, rec)

	rec = api.do(t, http.MethodDelete, "/api/v1/areas/"+areaID, businessID, "")
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("AREA_HAS_ACTIVE_TABLES")) {
		t.Fatalf("delete area with tables = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodDelete, "/api/v1/areas/"+areaID+"/tables/"+tableID, businessID, "")
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"INACTIVE"`)) {
		t.Fatalf("soft delete table = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodDelete, "/api/v1/areas/"+areaID, businessID, "")
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"INACTIVE"`)) {
		t.Fatalf("soft delete area = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/areas/not-a-uuid", businessID, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid status = %d", rec.Code)
	}
}

func TestMultiTenantIsolationHTTP(t *testing.T) {
	ownerA := uuid.New()
	staffB := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()

	api := newTestAPI(t, map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
		staffB: {authz.PermissionAreaRead},
		ownerB: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
	}, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: staffB, Name: tenant.RoleStaff}},
	})

	rec := api.do(t, http.MethodPost, "/api/v1/areas", businessA, `{"name":"Indoor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A area = %d %s", rec.Code, rec.Body.String())
	}
	areaA := decodeDataID(t, rec)
	rec = api.do(t, http.MethodPost, "/api/v1/areas/"+areaA+"/tables", businessA, `{"name":"Table 01","capacity":4}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A table = %d %s", rec.Code, rec.Body.String())
	}
	tableA := decodeDataID(t, rec)

	// User 1 as STAFF of B cannot mutate B and cannot see A's IDs.
	rec = api.do(t, http.MethodGet, "/api/v1/areas/"+areaA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user1@B get A area = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/areas/"+areaA+"/tables/"+tableA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user1@B get A table = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/areas", businessB, `{"name":"Indoor"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff B create = %d", rec.Code)
	}

	// User 2 OWNER of B gets a separate Indoor + Table 01.
	user2 := newTestAPI(t, map[uuid.UUID][]authz.Permission{
		ownerB: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
	}, map[uuid.UUID]tenant.Access{
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	user2.store = api.store
	user2.handler = rebuildHandler(t, user2.store, map[uuid.UUID][]authz.Permission{
		ownerB: {authz.PermissionAreaRead, authz.PermissionAreaCreate, authz.PermissionAreaUpdate, authz.PermissionAreaDelete},
	}, map[uuid.UUID]tenant.Access{
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})

	rec = user2.do(t, http.MethodPost, "/api/v1/areas", businessB, `{"name":"Indoor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 create B area = %d %s", rec.Code, rec.Body.String())
	}
	areaB := decodeDataID(t, rec)
	rec = user2.do(t, http.MethodPost, "/api/v1/areas/"+areaB+"/tables", businessB, `{"name":"Table 01","capacity":2}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 create B table = %d %s", rec.Code, rec.Body.String())
	}
	tableB := decodeDataID(t, rec)

	rec = user2.do(t, http.MethodGet, "/api/v1/areas/"+areaA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user2 get A area = %d", rec.Code)
	}
	rec = user2.do(t, http.MethodGet, "/api/v1/areas/"+areaB+"/tables/"+tableA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("table A under area B = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/areas/"+areaA+"/tables/"+tableB, businessA, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("table B under area A = %d", rec.Code)
	}

	rec = api.do(t, http.MethodGet, "/api/v1/areas", businessA, "")
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(areaA)) || bytes.Contains(rec.Body.Bytes(), []byte(areaB)) {
		t.Fatalf("list A leaked B: %s", rec.Body.String())
	}
}

type testAPI struct {
	handler   http.Handler
	store     *memoryStore
	seedArea  uuid.UUID
	seedTable uuid.UUID
}

func newTestAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	store := newMemoryStore()
	return &testAPI{
		handler: rebuildHandler(t, store, perms, access),
		store:   store,
	}
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
	areas := protected.Group("/areas")
	areas.GET("", authorizer.RequirePermission(authz.PermissionAreaRead), h.ListAreas)
	areas.POST("", authorizer.RequirePermission(authz.PermissionAreaCreate), h.CreateArea)
	areas.GET("/:id", authorizer.RequirePermission(authz.PermissionAreaRead), h.GetArea)
	areas.PATCH("/:id", authorizer.RequirePermission(authz.PermissionAreaUpdate), h.UpdateArea)
	areas.DELETE("/:id", authorizer.RequirePermission(authz.PermissionAreaDelete), h.DeleteArea)
	areas.GET("/:id/tables", authorizer.RequirePermission(authz.PermissionAreaRead), h.ListTables)
	areas.POST("/:id/tables", authorizer.RequirePermission(authz.PermissionAreaCreate), h.CreateTable)
	areas.GET("/:id/tables/:table_id", authorizer.RequirePermission(authz.PermissionAreaRead), h.GetTable)
	areas.PATCH("/:id/tables/:table_id", authorizer.RequirePermission(authz.PermissionAreaUpdate), h.UpdateTable)
	areas.DELETE("/:id/tables/:table_id", authorizer.RequirePermission(authz.PermissionAreaDelete), h.DeleteTable)
	return r
}

func (a *testAPI) seedLayout(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	area, err := NewService(a.store).CreateArea(context.Background(), businessID, CreateAreaInput{Name: "Seed"})
	if err != nil {
		t.Fatalf("seed area: %v", err)
	}
	table, err := NewService(a.store).CreateTable(context.Background(), businessID, area.ID, CreateTableInput{Name: "Seed Table", Capacity: 2})
	if err != nil {
		t.Fatalf("seed table: %v", err)
	}
	a.seedArea = area.ID
	a.seedTable = table.ID
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

func decodeDataID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if payload.Data.ID == "" {
		t.Fatalf("missing id in %s", rec.Body.String())
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
