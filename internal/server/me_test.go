package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/pkg/config"
)

func TestMeRequiresAuthAndTenant(t *testing.T) {
	authUserID := uuid.New()
	userID := uuid.New()
	businessID := uuid.New()
	roleID := uuid.New()

	handler := newMeHandler(t, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID:         userID,
		AuthUserID: authUserID,
		FullName:   "Ada Lovelace",
		Email:      "ada@example.com",
	}}, stubAccess{access: &tenant.Access{
		Business: tenant.Business{ID: businessID, Name: "Warung Ada", Slug: "warung-ada"},
		Role:     tenant.Role{ID: roleID, Name: tenant.RoleOwner},
	}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["user"]["id"] != userID.String() || body["user"]["email"] != "ada@example.com" {
		t.Errorf("user = %+v", body["user"])
	}
	if body["business"]["id"] != businessID.String() || body["business"]["slug"] != "warung-ada" {
		t.Errorf("business = %+v", body["business"])
	}
	if body["role"]["name"] != "OWNER" {
		t.Errorf("role = %+v", body["role"])
	}
	if strings.Contains(rec.Body.String(), "Bearer") || strings.Contains(rec.Body.String(), "password") {
		t.Errorf("response leaked secrets: %s", rec.Body.String())
	}
}

func TestMePermissionsReturnsCurrentRoleSet(t *testing.T) {
	authUserID := uuid.New()
	userID := uuid.New()
	businessID := uuid.New()
	roleID := uuid.New()

	handler := newMeHandler(t, stubVerifier{id: authUserID}, stubUsers{user: &tenant.User{
		ID:         userID,
		AuthUserID: authUserID,
		FullName:   "Ada Lovelace",
		Email:      "ada@example.com",
	}}, stubAccess{access: &tenant.Access{
		Business: tenant.Business{ID: businessID, Name: "Warung Ada", Slug: "warung-ada"},
		Role:     tenant.Role{ID: roleID, Name: tenant.RoleOwner},
	}}, &stubPerms{roleID: roleID, codes: []string{"area.create", "business.read", "business.update"}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/permissions", nil)
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("X-Business-ID", businessID.String())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			BusinessID  string   `json:"business_id"`
			Role        string   `json:"role"`
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.BusinessID != businessID.String() || body.Data.Role != "OWNER" {
		t.Errorf("data = %+v", body.Data)
	}
	if len(body.Data.Permissions) != 3 || body.Data.Permissions[0] != "area.create" {
		t.Errorf("permissions = %v", body.Data.Permissions)
	}
}

func TestHealthDoesNotRequireAuth(t *testing.T) {
	handler := newMeHandler(t, stubVerifier{err: auth.ErrInvalidToken}, stubUsers{err: tenant.ErrNotFound}, stubAccess{err: tenant.ErrNotFound}, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}
}

func newMeHandler(t *testing.T, verifier auth.Verifier, users auth.UserFinder, access auth.AccessFinder, perms authz.Checker) http.Handler {
	t.Helper()

	cfg := &config.Config{
		AppEnv:             "test",
		AppPort:            "8080",
		AppName:            "diuk-backend",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	guard := auth.NewMiddleware(log, verifier, users, access)

	deps := Dependencies{DB: stubDB{}, Auth: guard}
	if perms != nil {
		deps.Authz = authz.NewMiddleware(log, perms)
	}

	return New(cfg, log, deps).http.Handler
}

type stubPerms struct {
	roleID uuid.UUID
	codes  []string
	err    error
}

func (s *stubPerms) HasPermission(context.Context, uuid.UUID, authz.Permission) (bool, error) {
	return false, s.err
}

func (s *stubPerms) List(_ context.Context, roleID uuid.UUID) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	if roleID != s.roleID {
		return nil, nil
	}
	return s.codes, nil
}

type stubVerifier struct {
	id  uuid.UUID
	err error
}

func (s stubVerifier) Verify(token string) (uuid.UUID, error) {
	if s.err != nil {
		return uuid.Nil, s.err
	}
	if token != "good" {
		return uuid.Nil, auth.ErrInvalidToken
	}
	return s.id, nil
}

type stubUsers struct {
	user *tenant.User
	err  error
}

func (s stubUsers) GetActiveByAuthUserID(context.Context, uuid.UUID) (*tenant.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

type stubAccess struct {
	access *tenant.Access
	err    error
}

func (s stubAccess) GetActiveAccess(context.Context, uuid.UUID, uuid.UUID) (*tenant.Access, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.access, nil
}
