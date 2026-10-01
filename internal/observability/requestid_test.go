package observability

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDGeneratedAndPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, RequestIDFromGin(c))
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if rec.Header().Get(HeaderRequestID) == "" {
		t.Fatal("missing generated request id")
	}
	if rec.Body.String() != rec.Header().Get(HeaderRequestID) {
		t.Fatal("context id mismatch")
	}

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, "req-abc")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderRequestID) != "req-abc" || rec.Body.String() != "req-abc" {
		t.Fatalf("preserve = %s %s", rec.Header().Get(HeaderRequestID), rec.Body.String())
	}
	_ = slog.New(slog.NewTextHandler(io.Discard, nil))
}
