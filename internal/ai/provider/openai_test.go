package provider

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
)

func TestOpenAIGenerateAndErrors(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		sawAuth = true
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "secret-key") {
			t.Error("api key leaked into request body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o-mini",
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "hello",
					"tool_calls": []map[string]any{{
						"id":   "call-1",
						"type": "function",
						"function": map[string]any{
							"name":      "get_menu",
							"arguments": `{"unused":true}`,
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAI(Config{APIKey: "secret-key", BaseURL: server.URL, Timeout: time.Second})
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil || resp.Content != "hello" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_menu" {
		t.Fatalf("generate = %+v err=%v", resp, err)
	}
	if !sawAuth {
		t.Fatal("expected authorization header")
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key sk-secret"}}`))
	}))
	defer fail.Close()
	client = NewOpenAI(Config{APIKey: "sk-secret", BaseURL: fail.URL, Timeout: time.Second})
	_, err = client.Generate(context.Background(), GenerateRequest{Model: "gpt-4o-mini", Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "http 401") {
		t.Fatalf("http error = %v", err)
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("provider error leaked key: %v", err)
	}
}

func TestOpenAILiveOptional(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_AI_LIVE")) != "true" {
		t.Skip("TEST_AI_LIVE is not true")
	}
	key := strings.TrimSpace(os.Getenv("AI_API_KEY"))
	if key == "" {
		t.Skip("AI_API_KEY is not set")
	}
	base := strings.TrimRight(os.Getenv("AI_BASE_URL"), "/")
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	client := NewOpenAI(Config{APIKey: key, BaseURL: base, Timeout: 20 * time.Second})
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:    model,
		Messages: []Message{{Role: RoleUser, Content: "Reply with the word ok only."}},
	})
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		t.Fatalf("live generate = %+v err=%v", resp, err)
	}
}
