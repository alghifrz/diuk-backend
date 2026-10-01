package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDisabledProvider(t *testing.T) {
	p := New(Config{Enabled: false, APIKey: "sk-test"})
	if _, err := p.Embed(context.Background(), []string{"hello"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("disabled = %v", err)
	}
	p = New(Config{Enabled: true})
	if _, err := p.Embed(context.Background(), []string{"hello"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing key = %v", err)
	}
}

func TestOpenAIEmbedBatch(t *testing.T) {
	var gotInputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth")
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var req embedRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotInputs = req.Input
		_ = json.NewEncoder(w).Encode(embedResponse{Data: []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}{
			{Index: 0, Embedding: []float32{0.1, 0.2}},
			{Index: 1, Embedding: []float32{0.3, 0.4}},
		}})
	}))
	defer srv.Close()

	p := New(Config{Enabled: true, APIKey: "test-key", BaseURL: srv.URL, Model: "text-embedding-3-small", Timeout: time.Second, Dimensions: 2})
	out, err := p.Embed(context.Background(), []string{"a", "b"})
	if err != nil || len(out) != 2 || len(out[1]) != 2 {
		t.Fatalf("embed = %v err=%v", out, err)
	}
	if len(gotInputs) != 2 {
		t.Fatalf("batch size = %d", len(gotInputs))
	}
}

func TestOpenAIRetryableAndPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	p := New(Config{Enabled: true, APIKey: "k", BaseURL: srv.URL, Timeout: time.Second})
	if _, err := p.Embed(context.Background(), []string{"x"}); !errors.Is(err, ErrRequest) {
		t.Fatalf("5xx = %v", err)
	}

	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv4.Close()
	p = New(Config{Enabled: true, APIKey: "k", BaseURL: srv4.URL, Timeout: time.Second})
	if _, err := p.Embed(context.Background(), []string{"x"}); !errors.Is(err, ErrPermanent) {
		t.Fatalf("4xx = %v", err)
	}
}
