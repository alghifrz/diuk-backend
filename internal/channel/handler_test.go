package channel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

type testAPI struct {
	handler    http.Handler
	store      *memoryStore
	businessID uuid.UUID
	svc        *Service
}

func newTestAPI(t *testing.T, businessID uuid.UUID, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *testAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newMemoryStore()
	customers := newFakeCustomers()
	convs := newFakeConversations()
	svc := newTestService(store, customers, convs, &fakeAI{}, &fakeSender{})
	h := NewHandler(log, svc)
	userID := uuid.New()
	authUserID := uuid.New()
	guard := auth.NewMiddleware(log, stubVerifier{id: authUserID}, stubAuthUsers{user: &tenant.User{
		ID: userID, AuthUserID: authUserID, FullName: "Admin", Email: "admin@example.com",
	}}, stubAccess{byBusiness: access, userID: userID})
	authorizer := authz.NewMiddleware(log, mapChecker{allowed: perms})

	r := gin.New()
	r.GET("/api/webhooks/whatsapp", h.VerifyWhatsApp)
	r.POST("/api/webhooks/whatsapp", h.ReceiveWhatsApp)
	protected := r.Group("/api/v1")
	protected.Use(guard.Authenticate(), guard.RequireTenant())
	ch := protected.Group("/channels")
	ch.GET("", authorizer.RequirePermission(authz.PermissionChannelRead), h.ListChannels)
	ch.POST("", authorizer.RequirePermission(authz.PermissionChannelCreate), h.CreateChannel)
	ch.GET("/:id", authorizer.RequirePermission(authz.PermissionChannelRead), h.GetChannel)
	ch.PATCH("/:id", authorizer.RequirePermission(authz.PermissionChannelUpdate), h.UpdateChannel)
	ch.DELETE("/:id", authorizer.RequirePermission(authz.PermissionChannelDelete), h.DeleteChannel)
	return &testAPI{handler: r, store: store, businessID: businessID, svc: svc}
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

func TestWebhookVerification(t *testing.T) {
	api := newTestAPI(t, uuid.New(), nil, nil)
	rec := httptest.NewRecorder()
	api.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify&hub.challenge=abc123", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "abc123" {
		t.Fatalf("valid verify = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	api.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=nope&hub.challenge=abc123", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invalid token = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	api.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/webhooks/whatsapp?hub.mode=unsubscribe&hub.verify_token=verify&hub.challenge=abc123", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invalid mode = %d", rec.Code)
	}
}

func TestWebhookRejectsOversizedBody(t *testing.T) {
	api := newTestAPI(t, uuid.New(), nil, nil)
	api.svc.meta.MaxWebhookBody = 64
	body := bytes.Repeat([]byte("a"), 128)
	rec := postWebhook(api, body, signBody("secret", body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized webhook = %d", rec.Code)
	}
}

func TestWebhookSignatureAndInbound(t *testing.T) {
	biz := uuid.New()
	api := newTestAPI(t, biz, nil, nil)
	_, _ = api.store.Create(context.Background(), BusinessChannel{
		BusinessID: biz, Channel: ChannelWhatsApp, Status: StatusActive, ExternalPhoneNumberID: ptr("pn-1"),
	})
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"pn-1"},"contacts":[{"wa_id":"6281","profile":{"name":"Budi"}}],"messages":[{"id":"wamid.1","from":"6281","type":"text","text":{"body":"Halo"}}]}}]}]}`)

	rec := postWebhook(api, body, signBody("secret", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid inbound = %d %s", rec.Code, rec.Body.String())
	}
	rec = postWebhook(api, body, "sha256=deadbeef")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid sig = %d", rec.Code)
	}
	rec = postWebhook(api, body, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing sig = %d", rec.Code)
	}
	rec = postWebhook(api, []byte(`{`), signBody("secret", []byte(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed = %d", rec.Code)
	}
}

func TestChannelRBAC(t *testing.T) {
	owner, manager, staff := uuid.New(), uuid.New(), uuid.New()
	biz := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner:   {authz.PermissionChannelRead, authz.PermissionChannelCreate, authz.PermissionChannelUpdate, authz.PermissionChannelDelete},
		manager: {authz.PermissionChannelRead, authz.PermissionChannelCreate, authz.PermissionChannelUpdate, authz.PermissionChannelDelete},
		staff:   {authz.PermissionConversationUpdate},
	}
	access := map[uuid.UUID]tenant.Access{
		biz: {Business: tenant.Business{ID: biz}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
	}
	api := newTestAPI(t, biz, perms, access)
	rec := api.do(t, http.MethodGet, "/api/v1/channels", biz, "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", rec.Code)
	}
	rec = api.do(t, http.MethodPost, "/api/v1/channels", biz, `{"channel":"WHATSAPP","external_phone_number_id":"pn-1"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner create = %d %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("token")) || bytes.Contains(rec.Body.Bytes(), []byte("secret")) {
		t.Fatalf("response leaked secret: %s", rec.Body.String())
	}

	staffAPI := newTestAPI(t, biz, perms, map[uuid.UUID]tenant.Access{
		biz: {Business: tenant.Business{ID: biz}, Role: tenant.Role{ID: staff, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.do(t, http.MethodGet, "/api/v1/channels", biz, "", true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff denied = %d", rec.Code)
	}

	mgrAPI := newTestAPI(t, biz, perms, map[uuid.UUID]tenant.Access{
		biz: {Business: tenant.Business{ID: biz}, Role: tenant.Role{ID: manager, Name: tenant.RoleManager}},
	})
	rec = mgrAPI.do(t, http.MethodPost, "/api/v1/channels", biz, `{"channel":"WHATSAPP","external_phone_number_id":"pn-2"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manager create = %d %s", rec.Code, rec.Body.String())
	}
}

func postWebhook(api *testAPI, body []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", bytes.NewReader(body))
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)
	return rec
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
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

func TestChannelJSONHasNoSecrets(t *testing.T) {
	raw, _ := json.Marshal(channelResponse(&BusinessChannel{ID: uuid.New(), Channel: ChannelWhatsApp, Status: StatusActive}))
	if bytes.Contains(raw, []byte("access_token")) || bytes.Contains(raw, []byte("app_secret")) {
		t.Fatalf("channel json leaked secret: %s", raw)
	}
}
