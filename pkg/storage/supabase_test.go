package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSupabaseUnconfigured(t *testing.T) {
	s := NewSupabase("", "", "")
	if err := s.Upload(context.Background(), "p", "image/jpeg", strings.NewReader("x")); err != ErrNotConfigured {
		t.Fatalf("Upload() = %v", err)
	}
}

func TestSupabaseUploadSignDelete(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-role" {
			t.Errorf("missing service role authorization")
		}
		if strings.Contains(r.Header.Get("Authorization"), "\n") {
			t.Error("authorization header looks malformed")
		}
		sawAuth = true
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/object/sign/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"path":"biz/menu/item/original.jpg","signedURL":"/storage/v1/object/sign/diuk-menu-images/biz/menu/item/original.jpg?token=abc"}]`))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := NewSupabase(srv.URL, "test-role", "diuk-menu-images")
	if err := s.Upload(context.Background(), "biz/menu/item/original.jpg", "image/jpeg", strings.NewReader("img")); err != nil {
		t.Fatalf("Upload() = %v", err)
	}
	urls, err := s.CreateSignedURLs(context.Background(), []string{"biz/menu/item/original.jpg"}, DefaultSignedURLTTL)
	if err != nil {
		t.Fatalf("CreateSignedURLs() = %v", err)
	}
	got := urls["biz/menu/item/original.jpg"]
	if !strings.HasPrefix(got, srv.URL+"/storage/v1/object/sign/") {
		t.Fatalf("signed url = %q", got)
	}
	if err := s.Delete(context.Background(), "biz/menu/item/original.jpg"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if !sawAuth {
		t.Fatal("storage client did not send authorization")
	}
}

func TestParseSignedURLResponse(t *testing.T) {
	out, err := parseSignedURLResponse("https://example.supabase.co", []byte(`[{"path":"a/b.webp","signedURL":"/object/sign/bucket/a/b.webp?token=1"}]`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out["a/b.webp"] != "https://example.supabase.co/storage/v1/object/sign/bucket/a/b.webp?token=1" {
		t.Fatalf("got %q", out["a/b.webp"])
	}
}

func TestSupabaseUploadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()
	s := NewSupabase(srv.URL, "key", "diuk-menu-images")
	if err := s.Upload(context.Background(), "p", "image/jpeg", strings.NewReader("x")); err != ErrUploadFailed {
		t.Fatalf("Upload() = %v", err)
	}
}

func TestCreateSignedURLSingle(t *testing.T) {
	s := NewSupabase("https://example.supabase.co", "key", "diuk-menu-images")
	s.http = &http.Client{Timeout: time.Millisecond, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("err")), Header: make(http.Header)}, nil
	})}
	if _, err := s.CreateSignedURL(context.Background(), "p", time.Minute); err != ErrSignFailed {
		t.Fatalf("CreateSignedURL() = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
