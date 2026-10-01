package menu

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestMenuEndpointsAuthorization(t *testing.T) {
	ownerRole := uuid.New()
	managerRole := uuid.New()
	staffRole := uuid.New()
	businessID := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerRole:   {authz.PermissionMenuRead, authz.PermissionMenuCreate, authz.PermissionMenuUpdate, authz.PermissionMenuDelete},
		managerRole: {authz.PermissionMenuRead, authz.PermissionMenuCreate, authz.PermissionMenuUpdate, authz.PermissionMenuDelete},
		staffRole:   {authz.PermissionMenuRead},
	}

	tests := []struct {
		name       string
		method     string
		path       func(*testAPI) string
		roleID     uuid.UUID
		roleName   tenant.RoleName
		fields     map[string]string
		wantStatus int
	}{
		{name: "owner create category", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/menu/categories" }, roleID: ownerRole, roleName: tenant.RoleOwner, wantStatus: http.StatusCreated},
		{name: "manager create category", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/menu/categories" }, roleID: managerRole, roleName: tenant.RoleManager, wantStatus: http.StatusCreated},
		{name: "staff create category", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/menu/categories" }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "staff list", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/menu/categories" }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "staff patch", method: http.MethodPatch, path: func(api *testAPI) string { return "/api/v1/menu/categories/" + api.seedCategory.String() }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "staff delete", method: http.MethodDelete, path: func(api *testAPI) string { return "/api/v1/menu/categories/" + api.seedCategory.String() }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
		{name: "staff create item", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/menu/items" }, roleID: staffRole, roleName: tenant.RoleStaff, fields: map[string]string{"name": "X", "price": "1"}, wantStatus: http.StatusForbidden},
		{name: "owner create item", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/menu/items" }, roleID: ownerRole, roleName: tenant.RoleOwner, fields: map[string]string{"name": "Americano", "price": "18000"}, wantStatus: http.StatusCreated},
		{name: "manager patch item", method: http.MethodPatch, path: func(api *testAPI) string { return "/api/v1/menu/items/" + api.seedItem.String() }, roleID: managerRole, roleName: tenant.RoleManager, fields: map[string]string{"price": "19000"}, wantStatus: http.StatusOK},
		{name: "staff delete item", method: http.MethodDelete, path: func(api *testAPI) string { return "/api/v1/menu/items/" + api.seedItem.String() }, roleID: staffRole, roleName: tenant.RoleStaff, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
				businessID: {Business: tenant.Business{ID: businessID, Name: "Kasiko"}, Role: tenant.Role{ID: tt.roleID, Name: tt.roleName}},
			})
			api.seedCatalog(t, businessID)
			body, contentType := jsonOrMultipart(t, tt.method, tt.path(api), tt.fields, api.seedCategory)
			rec := api.do(t, tt.method, tt.path(api), businessID, body, contentType)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMenuHTTPValidationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	staffB := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionMenuRead, authz.PermissionMenuCreate, authz.PermissionMenuUpdate, authz.PermissionMenuDelete},
		staffB: {authz.PermissionMenuRead},
		ownerB: {authz.PermissionMenuRead, authz.PermissionMenuCreate, authz.PermissionMenuUpdate, authz.PermissionMenuDelete},
	}

	api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: staffB, Name: tenant.RoleStaff}},
	})

	rec := api.doJSON(t, http.MethodPost, "/api/v1/menu/categories", businessA, `{"name":"Coffee","description":"Hot drinks"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category A = %d %s", rec.Code, rec.Body.String())
	}
	catA := decodeDataID(t, rec)

	fields := map[string]string{"category_id": catA, "name": "Americano", "price": "18000"}
	body, ctype := multipartBody(t, fields, encodeJPEG(t), "image/jpeg")
	rec = api.do(t, http.MethodPost, "/api/v1/menu/items", businessA, body, ctype)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create item A = %d %s", rec.Code, rec.Body.String())
	}
	itemA := decodeDataID(t, rec)
	if !bytes.Contains(rec.Body.Bytes(), []byte(businessA.String()+"/menu/")) {
		t.Fatalf("image_path missing tenant prefix: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"image_url":"https://signed.example/`)) {
		t.Fatalf("signed url missing: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"image_thumb_url":"https://signed.example/`)) {
		t.Fatalf("thumb url missing: %s", rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/menu/categories", businessA, `{"name":"Coffee"}`)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("CATEGORY_NAME_ALREADY_EXISTS")) {
		t.Fatalf("duplicate = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/menu/categories/"+catA, businessB, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user1@B get A category = %d", rec.Code)
	}
	rec = api.do(t, http.MethodGet, "/api/v1/menu/items/"+itemA, businessB, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user1@B get A item = %d", rec.Code)
	}

	user2 := &testAPI{store: api.store, objects: api.objects}
	user2.handler = rebuildHandler(t, api.store, api.objects, map[uuid.UUID][]authz.Permission{
		ownerB: {authz.PermissionMenuRead, authz.PermissionMenuCreate, authz.PermissionMenuUpdate, authz.PermissionMenuDelete},
	}, map[uuid.UUID]tenant.Access{
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})

	rec = user2.doJSON(t, http.MethodPost, "/api/v1/menu/categories", businessB, `{"name":"Coffee"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 category = %d %s", rec.Code, rec.Body.String())
	}
	catB := decodeDataID(t, rec)
	body, ctype = multipartBody(t, map[string]string{"category_id": catB, "name": "Americano", "price": "17000"}, nil, "")
	rec = user2.do(t, http.MethodPost, "/api/v1/menu/items", businessB, body, ctype)
	if rec.Code != http.StatusCreated {
		t.Fatalf("user2 item = %d %s", rec.Code, rec.Body.String())
	}

	body, ctype = multipartBody(t, map[string]string{"category_id": catA, "name": "Stolen", "price": "1"}, nil, "")
	rec = user2.do(t, http.MethodPost, "/api/v1/menu/items", businessB, body, ctype)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create under other tenant category = %d %s", rec.Code, rec.Body.String())
	}
	rec = user2.do(t, http.MethodGet, "/api/v1/menu/items/"+itemA, businessB, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user2 signed url for A = %d %s", rec.Code, rec.Body.String())
	}
}

type testAPI struct {
	handler      http.Handler
	store        *memoryStore
	objects      *memoryStorage
	seedCategory uuid.UUID
	seedItem     uuid.UUID
}

func newTestAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	store := newMemoryStore()
	objects := newMemoryStorage()
	return &testAPI{
		handler: rebuildHandler(t, store, objects, perms, access),
		store:   store,
		objects: objects,
	}
}

func rebuildHandler(t *testing.T, store *memoryStore, objects *memoryStorage, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, NewService(store, objects, log))

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	cats := protected.Group("/menu/categories")
	cats.GET("", authorizer.RequirePermission(authz.PermissionMenuRead), h.ListCategories)
	cats.POST("", authorizer.RequirePermission(authz.PermissionMenuCreate), h.CreateCategory)
	cats.GET("/:id", authorizer.RequirePermission(authz.PermissionMenuRead), h.GetCategory)
	cats.PATCH("/:id", authorizer.RequirePermission(authz.PermissionMenuUpdate), h.UpdateCategory)
	cats.DELETE("/:id", authorizer.RequirePermission(authz.PermissionMenuDelete), h.DeleteCategory)
	cats.GET("/:id/items", authorizer.RequirePermission(authz.PermissionMenuRead), h.ListCategoryItems)
	items := protected.Group("/menu/items")
	items.GET("", authorizer.RequirePermission(authz.PermissionMenuRead), h.ListItems)
	items.POST("", authorizer.RequirePermission(authz.PermissionMenuCreate), h.CreateItem)
	items.GET("/:id", authorizer.RequirePermission(authz.PermissionMenuRead), h.GetItem)
	items.PATCH("/:id", authorizer.RequirePermission(authz.PermissionMenuUpdate), h.UpdateItem)
	items.DELETE("/:id", authorizer.RequirePermission(authz.PermissionMenuDelete), h.DeleteItem)
	return r
}

func (a *testAPI) seedCatalog(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	svc := NewService(a.store, a.objects, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cat, err := svc.CreateCategory(context.Background(), businessID, CreateCategoryInput{Name: "Seed"})
	if err != nil {
		t.Fatalf("seed category: %v", err)
	}
	item, err := svc.CreateItem(context.Background(), businessID, CreateItemInput{CategoryID: cat.ID.String(), Name: "Seed Item", Price: "1000"})
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	a.seedCategory = cat.ID
	a.seedItem = item.ID
}

func (a *testAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func (a *testAPI) doJSON(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return a.do(t, method, path, businessID, bytes.NewBufferString(body), "application/json")
}

func jsonOrMultipart(t *testing.T, method, path string, fields map[string]string, categoryID uuid.UUID) (*bytes.Buffer, string) {
	t.Helper()
	if method == http.MethodPost && path == "/api/v1/menu/categories" {
		return bytes.NewBufferString(`{"name":"Coffee"}`), "application/json"
	}
	if method == http.MethodPatch && fields == nil {
		return bytes.NewBufferString(`{"name":"Nope"}`), "application/json"
	}
	if fields == nil {
		return nil, ""
	}
	if _, ok := fields["category_id"]; !ok && method == http.MethodPost {
		fields = cloneMap(fields)
		fields["category_id"] = categoryID.String()
	}
	return multipartBody(t, fields, nil, "")
}

func multipartBody(t *testing.T, fields map[string]string, file []byte, fileType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("field %s: %v", k, err)
		}
	}
	if file != nil {
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="image"; filename="photo.bin"`},
			"Content-Type":        {fileType},
		})
		if err != nil {
			t.Fatalf("file part: %v", err)
		}
		if _, err := part.Write(file); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
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
