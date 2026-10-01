package ai

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

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type testAPI struct {
	handler          http.Handler
	store            *memoryStore
	convs            *fakeConversations
	seedConversation uuid.UUID
	businessID       uuid.UUID
}

func newTestAPI(t *testing.T, businessID uuid.UUID, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access, llm provider.LLMProvider) *testAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	authUserID := uuid.New()
	store := newMemoryStore()
	if businessID == uuid.Nil {
		businessID = uuid.New()
	}
	customerID := uuid.New()
	conversationID := uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
	h := NewHandler(log, svc)
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubAuthUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	aiGroup := protected.Group("/ai")
	aiGroup.GET("/settings", authorizer.RequirePermission(authz.PermissionPromptRead), h.GetSettings)
	aiGroup.PATCH("/settings", authorizer.RequirePermission(authz.PermissionPromptUpdate), h.UpdateSettings)
	aiGroup.GET("/prompts", authorizer.RequirePermission(authz.PermissionPromptRead), h.ListPrompts)
	aiGroup.POST("/prompts", authorizer.RequirePermission(authz.PermissionPromptCreate), h.CreatePrompt)
	aiGroup.GET("/prompts/:id", authorizer.RequirePermission(authz.PermissionPromptRead), h.GetPrompt)
	aiGroup.PATCH("/prompts/:id", authorizer.RequirePermission(authz.PermissionPromptUpdate), h.UpdatePrompt)
	aiGroup.POST("/prompts/:id/activate", authorizer.RequirePermission(authz.PermissionPromptUpdate), h.ActivatePrompt)
	aiGroup.DELETE("/prompts/:id", authorizer.RequirePermission(authz.PermissionPromptDelete), h.DeletePrompt)
	aiGroup.POST("/generate", authorizer.RequirePermission(authz.PermissionConversationUpdate), h.Generate)
	return &testAPI{handler: r, store: store, convs: convs, seedConversation: conversationID, businessID: businessID}
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

func TestAIAuthorization(t *testing.T) {
	owner := uuid.New()
	staff := uuid.New()
	reader := uuid.New()
	biz := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner:  {authz.PermissionPromptRead, authz.PermissionPromptCreate, authz.PermissionPromptUpdate, authz.PermissionPromptDelete, authz.PermissionConversationUpdate},
		staff:  {authz.PermissionConversationUpdate},
		reader: {authz.PermissionPromptRead},
	}
	llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "ok"}}}

	tests := []struct {
		name       string
		method     string
		path       string
		roleID     uuid.UUID
		body       string
		wantStatus int
	}{
		{name: "missing jwt", method: http.MethodGet, path: "/api/v1/ai/settings", wantStatus: http.StatusUnauthorized},
		{name: "staff cannot read settings", method: http.MethodGet, path: "/api/v1/ai/settings", roleID: staff, wantStatus: http.StatusForbidden},
		{name: "owner settings", method: http.MethodGet, path: "/api/v1/ai/settings", roleID: owner, wantStatus: http.StatusOK},
		{name: "reader cannot create prompt", method: http.MethodPost, path: "/api/v1/ai/prompts", roleID: reader, body: `{"name":"customer_service","system_prompt":"hello"}`, wantStatus: http.StatusForbidden},
		{name: "staff can generate", method: http.MethodPost, path: "/api/v1/ai/generate", roleID: staff, body: "", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access := map[uuid.UUID]tenant.Access{}
			if tt.roleID != uuid.Nil {
				access[biz] = tenant.Access{Business: tenant.Business{ID: biz}, Role: tenant.Role{ID: tt.roleID, Name: tenant.RoleOwner}}
			}
			api := newTestAPI(t, biz, perms, access, llm)
			body := tt.body
			if tt.name == "staff can generate" {
				body = `{"conversation_id":"` + api.seedConversation.String() + `","message":"halo","persist_incoming":false}`
			}
			rec := api.do(t, tt.method, tt.path, biz, body, tt.roleID != uuid.Nil)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGenerateHTTP(t *testing.T) {
	owner := uuid.New()
	otherOwner := uuid.New()
	llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "Siap, saya cek dulu."}}}
	perms := map[uuid.UUID][]authz.Permission{
		owner:      {authz.PermissionPromptRead, authz.PermissionPromptUpdate, authz.PermissionConversationUpdate},
		otherOwner: {authz.PermissionPromptRead, authz.PermissionConversationUpdate},
	}
	a, b := uuid.New(), uuid.New()
	api := newTestAPI(t, a, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b}, Role: tenant.Role{ID: otherOwner, Name: tenant.RoleOwner}},
	}, llm)

	rec := api.do(t, http.MethodPost, "/api/v1/ai/generate", a, `{"conversation_id":"`+api.seedConversation.String()+`","message":"halo"}`, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing jwt = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/generate", bytes.NewBufferString(`{"conversation_id":"`+api.seedConversation.String()+`","message":"halo"}`))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing tenant = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/ai/generate", uuid.New(), `{"conversation_id":"`+api.seedConversation.String()+`","message":"halo"}`, true)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid tenant = %d %s", rec.Code, rec.Body.String())
	}

	unknown := uuid.New()
	rec = api.do(t, http.MethodPost, "/api/v1/ai/generate", a, `{"conversation_id":"`+unknown.String()+`","message":"halo","persist_incoming":false}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodPost, "/api/v1/ai/generate", b, `{"conversation_id":"`+api.seedConversation.String()+`","message":"halo","persist_incoming":false}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant conversation = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPost, "/api/v1/ai/generate", a, `{"conversation_id":"`+api.seedConversation.String()+`","message":"halo","persist_incoming":false}`, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("Siap, saya cek dulu.")) {
		t.Fatalf("success = %d %s", rec.Code, rec.Body.String())
	}

	failAPI := newTestAPI(t, a, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
	}, &scriptedLLM{errs: []error{provider.ErrRequest}})
	rec = failAPI.do(t, http.MethodPost, "/api/v1/ai/generate", failAPI.businessID, `{"conversation_id":"`+failAPI.seedConversation.String()+`","message":"halo","persist_incoming":false}`, true)
	if rec.Code != http.StatusBadGateway || bytes.Contains(rec.Body.Bytes(), []byte("Bearer")) || bytes.Contains(rec.Body.Bytes(), []byte("sk-")) {
		t.Fatalf("provider error = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodPatch, "/api/v1/ai/settings", a, `{"temperature":3}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid temperature = %d %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodGet, "/api/v1/ai/settings", a, "", true)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("api_key")) {
		t.Fatalf("settings leaked key = %d %s", rec.Code, rec.Body.String())
	}
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

func TestPromptHTTPIsolation(t *testing.T) {
	ownerA, ownerB := uuid.New(), uuid.New()
	llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "ok"}}}
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionPromptRead, authz.PermissionPromptCreate, authz.PermissionPromptUpdate, authz.PermissionPromptDelete},
		ownerB: {authz.PermissionPromptRead, authz.PermissionPromptCreate},
	}
	a, b := uuid.New(), uuid.New()
	api := newTestAPI(t, a, perms, map[uuid.UUID]tenant.Access{
		a: {Business: tenant.Business{ID: a}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		b: {Business: tenant.Business{ID: b}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	}, llm)

	rec := api.do(t, http.MethodPost, "/api/v1/ai/prompts", a, `{"name":"customer_service","system_prompt":"Be helpful."}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rec = api.do(t, http.MethodGet, "/api/v1/ai/prompts/"+payload.Data.ID, b, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant prompt = %d %s", rec.Code, rec.Body.String())
	}
}
