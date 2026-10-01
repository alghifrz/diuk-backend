package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alghifrz/diuk-backend/internal/channel/provider"
)

func TestClientSendText(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		sawAuth = true
		if !strings.Contains(r.URL.Path, "/v21.0/pn-1/messages") {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "secret-token") {
			t.Error("token leaked into body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "wamid.1"}}})
	}))
	defer server.Close()

	client := NewClientWithHTTP(server.Client())
	got, err := client.SendText(context.Background(), provider.SendTextInput{
		PhoneNumberID: "pn-1", To: "628111", Body: "halo", AccessToken: "secret-token",
		BaseURL: server.URL, APIVersion: "v21.0",
	})
	if err != nil || got.ProviderMessageID != "wamid.1" || !sawAuth {
		t.Fatalf("send = %+v err=%v", got, err)
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"token secret-token","code":400}}`))
	}))
	defer fail.Close()
	_, err = client.SendText(context.Background(), provider.SendTextInput{
		PhoneNumberID: "pn-1", To: "628111", Body: "halo", AccessToken: "secret-token",
		BaseURL: fail.URL, APIVersion: "v21.0",
	})
	if err == nil || !strings.Contains(err.Error(), "http 400") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("4xx = %v", err)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()
	_, err = client.SendText(context.Background(), provider.SendTextInput{
		PhoneNumberID: "pn-1", To: "628111", Body: "halo", AccessToken: "x",
		BaseURL: down.URL, APIVersion: "v21.0",
	})
	if err != ErrUnavailable {
		t.Fatalf("5xx = %v", err)
	}

	timeout := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer timeout.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := client.SendText(ctx, provider.SendTextInput{
		PhoneNumberID: "pn-1", To: "628111", Body: "halo", AccessToken: "x",
		BaseURL: timeout.URL, APIVersion: "v21.0",
	}); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestClientMarkRead(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	client := NewClientWithHTTP(server.Client())
	if err := client.MarkRead(context.Background(), provider.MarkReadInput{
		PhoneNumberID: "pn-1", MessageID: "wamid.in", AccessToken: "secret-token",
		BaseURL: server.URL, APIVersion: "v21.0",
	}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if !strings.Contains(body, `"status":"read"`) || !strings.Contains(body, "wamid.in") || strings.Contains(body, "secret-token") {
		t.Fatalf("body = %s", body)
	}
}

func TestClientLiveOptional(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_WHATSAPP_LIVE")) != "true" {
		t.Skip("TEST_WHATSAPP_LIVE is not true")
	}
	token := strings.TrimSpace(os.Getenv("META_WHATSAPP_ACCESS_TOKEN"))
	phone := strings.TrimSpace(os.Getenv("META_WHATSAPP_PHONE_NUMBER_ID"))
	to := strings.TrimSpace(os.Getenv("META_WHATSAPP_TO"))
	if token == "" || phone == "" || to == "" {
		t.Skip("live WhatsApp env is incomplete")
	}
	client := NewClient(15 * time.Second)
	if _, err := client.SendText(context.Background(), provider.SendTextInput{
		PhoneNumberID: phone, To: to, Body: "DIUK live test", AccessToken: token,
		BaseURL: os.Getenv("META_WHATSAPP_BASE_URL"), APIVersion: os.Getenv("META_WHATSAPP_API_VERSION"),
	}); err != nil {
		t.Fatalf("live send: %v", err)
	}
}
