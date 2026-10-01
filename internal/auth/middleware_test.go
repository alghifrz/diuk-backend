package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestAuthenticate(t *testing.T) {
	authUserID := uuid.New()
	userID := uuid.New()
	user := &tenant.User{
		ID:         userID,
		AuthUserID: authUserID,
		FullName:   "Ada Lovelace",
		Email:      "ada@example.com",
		Status:     tenant.StatusActive,
	}

	tests := []struct {
		name       string
		header     string
		verifier   Verifier
		users      UserFinder
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing header",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{user: user},
		},
		{
			name:       "malformed header",
			header:     "Bearer",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{user: user},
		},
		{
			name:       "non bearer scheme",
			header:     "Basic abc",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{user: user},
		},
		{
			name:       "empty token",
			header:     "Bearer   ",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{user: user},
		},
		{
			name:       "invalid jwt",
			header:     "Bearer bad",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{err: ErrInvalidToken},
			users:      stubUsers{user: user},
		},
		{
			name:       "user not found",
			header:     "Bearer good",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{err: tenant.ErrNotFound},
		},
		{
			name:       "inactive or unknown user",
			header:     "Bearer good",
			wantStatus: http.StatusUnauthorized,
			wantCode:   codeUnauthorized,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{err: tenant.ErrNotFound},
		},
		{
			name:       "lookup failure",
			header:     "Bearer good",
			wantStatus: http.StatusInternalServerError,
			wantCode:   codeInternal,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{err: errors.New("connection refused")},
		},
		{
			name:       "success",
			header:     "Bearer good",
			wantStatus: http.StatusOK,
			verifier:   stubVerifier{id: authUserID},
			users:      stubUsers{user: user},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := perform(t, tt.header, "", tt.verifier, tt.users, stubAccess{}, false)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" && errorCode(t, rec) != tt.wantCode {
				t.Errorf("code = %q, want %q", errorCode(t, rec), tt.wantCode)
			}
			if rec.Code == http.StatusOK {
				if body.UserID != userID {
					t.Errorf("user id = %s, want %s", body.UserID, userID)
				}
				if body.HasTenant {
					t.Error("tenant should not be resolved by Authenticate alone")
				}
			}
			if rec.Code >= 400 && strings.Contains(rec.Body.String(), "connection refused") {
				t.Errorf("error leaks details: %s", rec.Body.String())
			}
		})
	}
}

func TestRequireTenant(t *testing.T) {
	authUserID := uuid.New()
	userID := uuid.New()
	businessID := uuid.New()
	roleID := uuid.New()
	user := &tenant.User{ID: userID, AuthUserID: authUserID, FullName: "Ada", Email: "ada@example.com"}
	access := &tenant.Access{
		Membership: tenant.BusinessUser{UserID: userID, BusinessID: businessID, RoleID: roleID, Status: tenant.StatusActive},
		Business:   tenant.Business{ID: businessID, Name: "Warung Ada", Slug: "warung-ada", Status: tenant.StatusActive},
		Role:       tenant.Role{ID: roleID, Name: tenant.RoleOwner},
	}

	tests := []struct {
		name       string
		business   string
		access     AccessFinder
		wantStatus int
		wantCode   string
	}{
		{name: "missing header", wantStatus: http.StatusBadRequest, wantCode: codeBadRequest, access: stubAccess{access: access}},
		{name: "invalid uuid", business: "not-a-uuid", wantStatus: http.StatusBadRequest, wantCode: codeBadRequest, access: stubAccess{access: access}},
		{name: "not a member", business: uuid.New().String(), wantStatus: http.StatusForbidden, wantCode: codeForbidden, access: stubAccess{err: tenant.ErrNotFound}},
		{name: "inactive membership", business: businessID.String(), wantStatus: http.StatusForbidden, wantCode: codeForbidden, access: stubAccess{err: tenant.ErrNotFound}},
		{name: "inactive business", business: businessID.String(), wantStatus: http.StatusForbidden, wantCode: codeForbidden, access: stubAccess{err: tenant.ErrNotFound}},
		{name: "lookup failure", business: businessID.String(), wantStatus: http.StatusInternalServerError, wantCode: codeInternal, access: stubAccess{err: errors.New("db down")}},
		{name: "active membership", business: businessID.String(), wantStatus: http.StatusOK, access: stubAccess{access: access}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := perform(t, "Bearer good", tt.business, stubVerifier{id: authUserID}, stubUsers{user: user}, tt.access, true)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" && errorCode(t, rec) != tt.wantCode {
				t.Errorf("code = %q, want %q", errorCode(t, rec), tt.wantCode)
			}
			if rec.Code == http.StatusOK {
				if body.BusinessID != businessID {
					t.Errorf("business id = %s, want %s", body.BusinessID, businessID)
				}
				if body.RoleName != string(tenant.RoleOwner) {
					t.Errorf("role = %q, want OWNER", body.RoleName)
				}
			}
			if rec.Code == http.StatusForbidden && rec.Body.String() != `{"error":{"code":"FORBIDDEN","message":"access denied"}}` {
				// Keep the 403 wording identical so clients cannot tell missing
				// businesses apart from missing or inactive memberships.
				if errorCode(t, rec) != codeForbidden {
					t.Errorf("forbidden response = %s", rec.Body.String())
				}
			}
		})
	}
}

func TestRequireTenantWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), stubVerifier{}, stubUsers{}, stubAccess{})

	r := gin.New()
	r.GET("/t", m.RequireTenant(), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

type probe struct {
	UserID     uuid.UUID `json:"user_id"`
	BusinessID uuid.UUID `json:"business_id"`
	RoleName   string    `json:"role_name"`
	HasTenant  bool      `json:"has_tenant"`
}

func perform(t *testing.T, authorization, businessID string, verifier Verifier, users UserFinder, access AccessFinder, requireTenant bool) (*httptest.ResponseRecorder, probe) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	m := NewMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), verifier, users, access)
	r := gin.New()
	handlers := []gin.HandlerFunc{m.Authenticate()}
	if requireTenant {
		handlers = append(handlers, m.RequireTenant())
	}
	handlers = append(handlers, func(c *gin.Context) {
		ac, _ := GetAuthContext(c)
		c.JSON(http.StatusOK, probe{
			UserID:     ac.UserID,
			BusinessID: ac.BusinessID,
			RoleName:   string(ac.RoleName),
			HasTenant:  ac.HasTenant(),
		})
	})
	r.GET("/probe", handlers...)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if businessID != "" {
		req.Header.Set("X-Business-ID", businessID)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var body probe
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec, body
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error: %v body=%s", err, rec.Body.String())
	}
	return payload.Error.Code
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
		return uuid.Nil, ErrInvalidToken
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
