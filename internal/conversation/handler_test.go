package conversation

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

func TestConversationAuthorization(t *testing.T) {
	owner := uuid.New()
	manager := uuid.New()
	staff := uuid.New()
	reader := uuid.New()
	biz := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner:   {authz.PermissionConversationRead, authz.PermissionConversationUpdate},
		manager: {authz.PermissionConversationRead, authz.PermissionConversationUpdate},
		staff:   {authz.PermissionConversationRead, authz.PermissionConversationUpdate},
		reader:  {authz.PermissionConversationRead},
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
		{name: "unauthenticated", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations" }, wantStatus: http.StatusUnauthorized},
		{name: "staff list", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations" }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "manager human inbox", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations/human-inbox" }, roleID: manager, roleName: tenant.RoleManager, wantStatus: http.StatusOK},
		{name: "owner human inbox", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations/human-inbox" }, roleID: owner, roleName: tenant.RoleOwner, wantStatus: http.StatusOK},
		{name: "reader cannot create", method: http.MethodPost, path: func(*testAPI) string { return "/api/v1/conversations" }, roleID: reader, roleName: tenant.RoleStaff, body: `{"customer_id":"` + uuid.NewString() + `","channel":"WEBSITE"}`, wantStatus: http.StatusForbidden},
		{name: "staff create", method: http.MethodPost, path: func(a *testAPI) string { return "/api/v1/conversations" }, roleID: staff, roleName: tenant.RoleStaff, body: "", wantStatus: http.StatusCreated},
		{name: "reader cannot message", method: http.MethodPost, path: func(a *testAPI) string {
			return "/api/v1/conversations/" + a.seedConversation.String() + "/messages"
		}, roleID: reader, roleName: tenant.RoleStaff, body: `{"direction":"OUTBOUND","sender_type":"USER","content":"hi"}`, wantStatus: http.StatusForbidden},
		{name: "staff human inbox", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations/human-inbox" }, roleID: staff, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "reader human inbox", method: http.MethodGet, path: func(*testAPI) string { return "/api/v1/conversations/human-inbox" }, roleID: reader, roleName: tenant.RoleStaff, wantStatus: http.StatusOK},
		{name: "reader cannot handover", method: http.MethodPost, path: func(a *testAPI) string {
			return "/api/v1/conversations/" + a.seedConversation.String() + "/handover"
		}, roleID: reader, roleName: tenant.RoleStaff, body: `{"reason":"MANUAL_HANDOFF"}`, wantStatus: http.StatusForbidden},
		{name: "staff handover", method: http.MethodPost, path: func(a *testAPI) string {
			return "/api/v1/conversations/" + a.seedConversation.String() + "/handover"
		}, roleID: staff, roleName: tenant.RoleStaff, body: `{"reason":"MANUAL_HANDOFF"}`, wantStatus: http.StatusOK},
		{name: "reader cannot reply", method: http.MethodPost, path: func(a *testAPI) string {
			return "/api/v1/conversations/" + a.seedConversation.String() + "/reply"
		}, roleID: reader, roleName: tenant.RoleStaff, body: `{"content":"hi"}`, wantStatus: http.StatusForbidden},
		{name: "reader cannot note", method: http.MethodPost, path: func(a *testAPI) string {
			return "/api/v1/conversations/" + a.seedConversation.String() + "/notes"
		}, roleID: reader, roleName: tenant.RoleStaff, body: `{"content":"internal"}`, wantStatus: http.StatusForbidden},
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
			body := tt.body
			if tt.name == "staff create" {
				body = `{"customer_id":"` + api.seedCustomer.String() + `","channel":"INSTAGRAM"}`
			}
			rec := api.do(t, tt.method, tt.path(api), biz, body, tt.roleID != uuid.Nil)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestConversationHTTPValidationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	ownerB := uuid.New()
	a, b := uuid.New(), uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionConversationRead, authz.PermissionConversationUpdate},
		ownerB: {authz.PermissionConversationRead, authz.PermissionConversationUpdate},
	}
	api := newTestAPI(t, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	api.seed(t, a)
	api.seedOther(t, b)

	rec := api.do(t, http.MethodGet, "/api/v1/conversations", a, "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec = httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing tenant = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/conversations", a, `{"customer_id":`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed json = %d", rec.Code)
	}
	rec = api.do(t, http.MethodGet, "/api/v1/conversations/not-a-uuid", a, "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid = %d", rec.Code)
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations", a, `{"customer_id":"`+api.seedCustomer.String()+`","channel":"SMS"}`, true)
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("INVALID_CHANNEL")) {
		t.Fatalf("invalid channel = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/conversations", a, `{"customer_id":"`+api.seedCustomer.String()+`","channel":"WEBSITE"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("reuse seed conversation = %d %s", rec.Code, rec.Body.String())
	}
	convA := decodeID(t, rec)

	rec = api.do(t, http.MethodGet, "/api/v1/conversations/"+convA, a, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"unread_count":0`)) == false {
		t.Fatalf("get should not mark read: %s", rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/messages", a, `{"direction":"INBOUND","sender_type":"CUSTOMER","content":"halo"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create message = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/messages", a, `{"direction":"SIDEWAYS","sender_type":"CUSTOMER","content":"x"}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid enum = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/messages", a, `{"direction":"INBOUND","sender_type":"CUSTOMER","content":"dup","external_id":"wa-9"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("external create = %d %s", rec.Code, rec.Body.String())
	}
	firstID := decodeID(t, rec)
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/messages", a, `{"direction":"INBOUND","sender_type":"CUSTOMER","content":"dup","external_id":"wa-9"}`, true)
	if rec.Code != http.StatusOK || decodeID(t, rec) != firstID {
		t.Fatalf("idempotent message = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/conversations?search=John", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(convA)) {
		t.Fatalf("list/search = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPatch, "/api/v1/conversations/"+convA, a, `{"status":"CLOSED"}`, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"CLOSED"`)) {
		t.Fatalf("close = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPatch, "/api/v1/conversations/"+convA+"/read", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"unread_count":0`)) {
		t.Fatalf("mark read = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/conversations/"+convA, b, "", true)
	if rec.Code != http.StatusNotFound || !bytes.Contains(rec.Body.Bytes(), []byte("CONVERSATION_NOT_FOUND")) {
		t.Fatalf("cross-tenant get = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/messages", b, `{"direction":"OUTBOUND","sender_type":"USER","content":"hi"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant message = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/conversations", a, `{"customer_id":"`+api.otherCustomer.String()+`","channel":"WEBSITE"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create with foreign customer = %d %s", rec.Code, rec.Body.String())
	}

	inactive := api.store.addCustomer(a, "Sleepy", "INACTIVE", nil, nil)
	rec = api.do(t, http.MethodPost, "/api/v1/conversations", a, `{"customer_id":"`+inactive.String()+`","channel":"WEBSITE"}`, true)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("CUSTOMER_INACTIVE")) {
		t.Fatalf("inactive customer = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/handover", a, `{"reason":"MANUAL_HANDOFF"}`, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"HUMAN_REQUESTED"`)) {
		t.Fatalf("handover = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/conversations/human-inbox", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(convA)) {
		t.Fatalf("inbox A = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/conversations/human-inbox", b, "", true)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte(convA)) {
		t.Fatalf("inbox B leaked A = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/handover", b, `{"reason":"MANUAL_HANDOFF"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant handover = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/takeover", b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant takeover = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/reply", b, `{"content":"nope"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant reply = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/notes", b, `{"content":"secret"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant note = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/reply", a, `{"content":"Saya bantu"}`, true)
	if rec.Code != http.StatusCreated || !bytes.Contains(rec.Body.Bytes(), []byte(`"USER"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"QUEUED"`)) {
		t.Fatalf("reply = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/notes", a, `{"content":"Need to verify refund"}`, true)
	if rec.Code != http.StatusCreated || !bytes.Contains(rec.Body.Bytes(), []byte(`"INTERNAL_NOTE"`)) {
		t.Fatalf("note = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/resolve", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"RESOLVED"`)) {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/conversations/"+convA+"/resume-ai", a, "", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"AI_ACTIVE"`)) {
		t.Fatalf("resume = %d %s", rec.Code, rec.Body.String())
	}
}

type testAPI struct {
	handler          http.Handler
	store            *memoryStore
	users            stubUsers
	members          stubMembers
	seedCustomer     uuid.UUID
	seedConversation uuid.UUID
	otherCustomer    uuid.UUID
}

func newTestAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	store := newMemoryStore()
	users := stubUsers{users: map[uuid.UUID]*tenant.User{}}
	members := stubMembers{ok: map[string]struct{}{}}
	return &testAPI{
		handler: rebuildHandler(t, store, users, members, perms, access),
		store:   store,
		users:   users,
		members: members,
	}
}

func rebuildHandler(t *testing.T, store *memoryStore, users stubUsers, members stubMembers, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubAuthUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})
	h := NewHandler(log, NewService(store, users, members))

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	convs := protected.Group("/conversations")
	convs.GET("", authorizer.RequirePermission(authz.PermissionConversationRead), h.ListConversations)
	convs.POST("", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.CreateConversation)
	convs.GET("/human-inbox", authorizer.RequirePermission(authz.PermissionConversationRead), h.ListHumanInbox)
	convs.GET("/:id", authorizer.RequirePermission(authz.PermissionConversationRead), h.GetConversation)
	convs.PATCH("/:id", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.UpdateConversation)
	convs.PATCH("/:id/read", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.MarkRead)
	convs.GET("/:id/messages", authorizer.RequirePermission(authz.PermissionConversationRead), h.ListMessages)
	convs.POST("/:id/messages", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.CreateMessage)
	convs.POST("/:id/handover", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.RequestHandover)
	convs.POST("/:id/takeover", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.Takeover)
	convs.POST("/:id/resolve", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.Resolve)
	convs.POST("/:id/resume-ai", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.ResumeAI)
	convs.POST("/:id/reply", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.Reply)
	convs.POST("/:id/notes", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.CreateNote)
	return r
}

func (a *testAPI) seed(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	a.seedCustomer = a.store.addCustomer(businessID, "John Doe", "ACTIVE", ptr("+628123456789"), ptr("john@example.com"))
	svc := NewService(a.store, a.users, a.members)
	conv, _, err := svc.CreateConversation(context.Background(), businessID, CreateConversationInput{
		CustomerID: a.seedCustomer.String(), Channel: "WEBSITE",
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	a.seedConversation = conv.ID
}

func (a *testAPI) seedOther(t *testing.T, businessID uuid.UUID) {
	t.Helper()
	a.otherCustomer = a.store.addCustomer(businessID, "Other", "ACTIVE", nil, nil)
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
