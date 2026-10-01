package server

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

	"github.com/alghifrz/diuk-backend/pkg/config"
)

// stubDB reports a fixed database state to the health handler.
type stubDB struct {
	err error
}

func (s stubDB) Ping(context.Context) error { return s.err }

func testServer(t *testing.T, db DatabasePinger) http.Handler {
	t.Helper()

	cfg := &config.Config{
		AppEnv:             "test",
		AppPort:            "8080",
		AppName:            "diuk-backend",
		DatabaseURL:        "postgresql://user:password@localhost:5432/diuk",
		RedisURL:           "redis://localhost:6379",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	return New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{DB: db}).http.Handler
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name string
		path string
		want map[string]string
	}{
		{
			name: "root",
			path: "/",
			want: map[string]string{"service": "diuk-backend", "message": "DIUK backend is running"},
		},
		{
			name: "health",
			path: "/api/v1/health",
			want: map[string]string{"status": "ok", "service": "diuk-backend"},
		},
		{
			name: "database health",
			path: "/api/v1/health/db",
			want: map[string]string{"status": "ok", "service": "diuk-backend", "database": "ok"},
		},
	}

	handler := testServer(t, stubDB{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			for key, want := range tt.want {
				if got[key] != want {
					t.Errorf("body[%q] = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}

func TestHealthStillWorksWhenDatabaseIsDown(t *testing.T) {
	handler := testServer(t, stubDB{err: errors.New("connection refused: host 10.0.0.1 password secret")})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDatabaseHealthReportsFailure(t *testing.T) {
	dbErr := errors.New("connection refused: host 10.0.0.1 password secret")
	handler := testServer(t, stubDB{err: dbErr})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/db", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["database"] != "unavailable" {
		t.Errorf("body[\"database\"] = %q, want %q", got["database"], "unavailable")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("response leaks database error details: %s", rec.Body.String())
	}
}

func TestRequestIDGeneratedAndPreserved(t *testing.T) {
	handler := testServer(t, stubDB{})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected generated X-Request-ID")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", "keep-me")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") != "keep-me" {
		t.Fatalf("preserved = %q", rec.Header().Get("X-Request-ID"))
	}
}

func TestReadyRequiresDatabase(t *testing.T) {
	handler := testServer(t, stubDB{err: errors.New("down")})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("ready response missing X-Request-ID")
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	handler := testServer(t, stubDB{})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	handler := testServer(t, stubDB{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff: %v", rec.Header())
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing frame denial")
	}
}

func TestMaxJSONBodyRejectsOversized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(maxJSONBody(32))
	r.POST("/x", func(c *gin.Context) {
		var body map[string]string
		if err := c.ShouldBindJSON(&body); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"0123456789012345678901234567890123456789"}`)))
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized = %d", rec.Code)
	}
}

func TestWebhookRateLimit(t *testing.T) {
	limiter := newRateLimiters(true, 1, 0, 0, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/hook", limiter.middleware("webhook"), func(c *gin.Context) { c.Status(http.StatusOK) })
	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/hook", nil))
	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if first.Code != http.StatusOK || second.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit first=%d second=%d", first.Code, second.Code)
	}
}
