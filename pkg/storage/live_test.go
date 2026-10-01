package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveSupabaseStorage(t *testing.T) {
	if os.Getenv("TEST_STORAGE_ENABLED") != "true" {
		t.Skip("TEST_STORAGE_ENABLED is not true; skipping live Storage tests")
	}
	projectURL := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	bucket := os.Getenv("SUPABASE_STORAGE_MENU_BUCKET")
	if projectURL == "" || key == "" {
		t.Fatal("SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY are required when TEST_STORAGE_ENABLED=true")
	}

	s := NewSupabase(projectURL, key, bucket)
	path := "live-test/menu/probe/original.jpg"
	if err := s.Upload(context.Background(), path, "image/jpeg", strings.NewReader("not-a-real-image")); err != nil {
		t.Fatalf("Upload() = %v", err)
	}
	t.Cleanup(func() { _ = s.Delete(context.Background(), path) })

	url, err := s.CreateSignedURL(context.Background(), path, time.Minute)
	if err != nil {
		t.Fatalf("CreateSignedURL() = %v", err)
	}
	if !strings.HasPrefix(url, "http") {
		t.Fatalf("signed url = %q", url)
	}
	if err := s.Delete(context.Background(), path); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
}
