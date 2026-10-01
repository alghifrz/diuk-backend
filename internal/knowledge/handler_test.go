package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestKnowledgeEndpointsAuthorizationAndIsolation(t *testing.T) {
	ownerA := uuid.New()
	staffA := uuid.New()
	ownerB := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		ownerA: {authz.PermissionKnowledgeRead, authz.PermissionKnowledgeCreate, authz.PermissionKnowledgeUpdate, authz.PermissionKnowledgeDelete},
		staffA: {authz.PermissionKnowledgeRead},
		ownerB: {authz.PermissionKnowledgeRead, authz.PermissionKnowledgeCreate, authz.PermissionKnowledgeUpdate, authz.PermissionKnowledgeDelete},
	}
	api := newKnowledgeAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: ownerA, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: ownerB, Name: tenant.RoleOwner}},
	})
	rec := api.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents", businessA, `{"title":"Cancellation","content":"Cancel before 17:00.","source_type":"TEXT"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create A = %d %s", rec.Code, rec.Body.String())
	}
	idA := decodeDataID(t, rec)

	rec = api.doJSON(t, http.MethodGet, "/api/v1/knowledge/documents/"+idA, businessB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant get = %d %s", rec.Code, rec.Body.String())
	}

	staffAPI := newKnowledgeAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	staffAPI.svc = api.svc
	staffAPI.handler = rebuildKnowledgeHandler(t, staffAPI.svc, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: staffA, Name: tenant.RoleStaff}},
	})
	rec = staffAPI.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents", businessA, `{"title":"Staff","content":"Nope","source_type":"TEXT"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff create = %d %s", rec.Code, rec.Body.String())
	}
	rec = staffAPI.doJSON(t, http.MethodGet, "/api/v1/knowledge/documents", businessA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("staff list = %d %s", rec.Code, rec.Body.String())
	}
	rec = staffAPI.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents/"+idA+"/reprocess", businessA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff reprocess = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents", businessA, `{"title":"","content":"x","source_type":"TEXT"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid = %d %s", rec.Code, rec.Body.String())
	}

	body, ctype := knowledgeMultipart(t, map[string]string{"title": "PDF fail", "source_type": "PDF"}, []byte("not-pdf"), "file.pdf", "application/pdf")
	rec = api.do(t, http.MethodPost, "/api/v1/knowledge/documents", businessA, body, ctype)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusBadRequest {
		t.Fatalf("pdf upload = %d %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeStatusAndSearchEndpoints(t *testing.T) {
	owner := uuid.New()
	businessA := uuid.New()
	businessB := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{
		owner: {authz.PermissionKnowledgeRead, authz.PermissionKnowledgeCreate},
	}
	api := newKnowledgeAPI(t, perms, map[uuid.UUID]tenant.Access{
		businessA: {Business: tenant.Business{ID: businessA, Name: "A"}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
		businessB: {Business: tenant.Business{ID: businessB, Name: "B"}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
	})

	rec := api.doJSON(t, http.MethodGet, "/api/v1/knowledge/status", businessA, "")
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/search", businessA, `{"query":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank query = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents", businessA, `{"title":"Cancellation","content":"Cancel before 17:00 to avoid a fee.","source_type":"TEXT"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	docID := uuid.MustParse(decodeDataID(t, rec))
	if err := api.svc.ProcessDocument(context.Background(), businessA, docID); err != nil {
		t.Fatalf("process: %v", err)
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/search", businessA, `{"query":"cancel fee"}`)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(docID.String())) {
		t.Fatalf("search A = %d %s", rec.Code, rec.Body.String())
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/search", businessB, `{"query":"cancel fee"}`)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte(docID.String())) {
		t.Fatalf("search B leaked = %d %s", rec.Code, rec.Body.String())
	}
}

type fakeAnswerer struct {
	reply string
	err   error
	got   string
}

func (f *fakeAnswerer) Answer(_ context.Context, _, user string) (string, error) {
	f.got = user
	return f.reply, f.err
}

func TestKnowledgeAskEndpoint(t *testing.T) {
	owner := uuid.New()
	business := uuid.New()
	perms := map[uuid.UUID][]authz.Permission{owner: {authz.PermissionKnowledgeRead, authz.PermissionKnowledgeCreate}}
	api := newKnowledgeAPI(t, perms, map[uuid.UUID]tenant.Access{
		business: {Business: tenant.Business{ID: business, Name: "A"}, Role: tenant.Role{ID: owner, Name: tenant.RoleOwner}},
	})
	answerer := &fakeAnswerer{reply: "Yes, small dogs are welcome."}
	api.svc.SetAnswerer(answerer)

	rec := api.doJSON(t, http.MethodPost, "/api/v1/knowledge/ask", business, `{"question":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank question = %d %s", rec.Code, rec.Body.String())
	}

	// Nothing ingested yet: no model call, honest "not found".
	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/ask", business, `{"question":"can I bring a dog?"}`)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"found":false`)) || answerer.got != "" {
		t.Fatalf("empty kb = %d %s (prompt %q)", rec.Code, rec.Body.String(), answerer.got)
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/documents", business, `{"title":"Pets","content":"Small dogs are welcome on the terrace.","source_type":"TEXT"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	docID := uuid.MustParse(decodeDataID(t, rec))
	if err := api.svc.ProcessDocument(context.Background(), business, docID); err != nil {
		t.Fatalf("process: %v", err)
	}

	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/ask", business, `{"question":"small dogs terrace"}`)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"found":true`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte("small dogs are welcome")) || !bytes.Contains(rec.Body.Bytes(), []byte(docID.String())) {
		t.Fatalf("ask = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(answerer.got, "Small dogs are welcome on the terrace.") {
		t.Fatalf("prompt missing excerpt: %q", answerer.got)
	}

	// The model declining to answer must surface as found=false with no answer text.
	answerer.reply = "NO_ANSWER"
	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/ask", business, `{"question":"small dogs terrace"}`)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"found":false`)) || bytes.Contains(rec.Body.Bytes(), []byte(`"answer"`)) {
		t.Fatalf("no answer = %d %s", rec.Code, rec.Body.String())
	}

	answerer.err = errors.New("boom")
	rec = api.doJSON(t, http.MethodPost, "/api/v1/knowledge/ask", business, `{"question":"small dogs terrace"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("provider error = %d %s", rec.Code, rec.Body.String())
	}
}

type knowledgeAPI struct {
	handler http.Handler
	svc     *Service
	access  map[uuid.UUID]tenant.Access
}

func newKnowledgeAPI(t *testing.T, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) *knowledgeAPI {
	t.Helper()
	svc, _, _, _, _ := testService(true)
	return &knowledgeAPI{
		handler: rebuildKnowledgeHandler(t, svc, perms, access),
		svc:     svc,
		access:  access,
	}
}

func rebuildKnowledgeHandler(t *testing.T, svc *Service, perms map[uuid.UUID][]authz.Permission, access map[uuid.UUID]tenant.Access) http.Handler {
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
	protected.GET("/knowledge/status", authorizer.RequirePermission(authz.PermissionKnowledgeRead), h.Status)
	protected.POST("/knowledge/ask", authorizer.RequirePermission(authz.PermissionKnowledgeRead), h.Ask)
	protected.POST("/knowledge/search", authorizer.RequirePermission(authz.PermissionKnowledgeRead), h.Search)
	kb := protected.Group("/knowledge/documents")
	kb.GET("", authorizer.RequirePermission(authz.PermissionKnowledgeRead), h.ListDocuments)
	kb.POST("", authorizer.RequirePermission(authz.PermissionKnowledgeCreate), h.CreateDocument)
	kb.GET("/:id", authorizer.RequirePermission(authz.PermissionKnowledgeRead), h.GetDocument)
	kb.PATCH("/:id", authorizer.RequirePermission(authz.PermissionKnowledgeUpdate), h.UpdateDocument)
	kb.DELETE("/:id", authorizer.RequirePermission(authz.PermissionKnowledgeDelete), h.DeleteDocument)
	kb.POST("/:id/reprocess", authorizer.RequirePermission(authz.PermissionKnowledgeUpdate), h.ReprocessDocument)
	return r
}

func (a *knowledgeAPI) do(t *testing.T, method, path string, businessID uuid.UUID, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
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

func (a *knowledgeAPI) doJSON(t *testing.T, method, path string, businessID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return a.do(t, method, path, businessID, bytes.NewBufferString(body), "application/json")
}

func knowledgeMultipart(t *testing.T, fields map[string]string, file []byte, filename, fileType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if file != nil {
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
			"Content-Type":        {fileType},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(file)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func decodeDataID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload.Data.ID == "" {
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
