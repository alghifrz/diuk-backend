package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// setRequired sets the variables every test case needs, so a test can focus on
// the single variable it exercises.
func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgresql://user:password@db.example.com:5432/postgres")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("TEST_AI_LIVE", "")
	t.Setenv("TEST_WHATSAPP_LIVE", "")
}

func TestLoadAppliesDefaults(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "")
	t.Setenv("APP_PORT", "")
	t.Setenv("APP_NAME", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, "development")
	}
	if cfg.Address() != ":8080" {
		t.Errorf("Address() = %q, want %q", cfg.Address(), ":8080")
	}
	if cfg.AppName != "diuk-backend" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "diuk-backend")
	}
	if want := []string{"http://localhost:3000"}; !slices.Equal(cfg.CORSAllowedOrigins, want) {
		t.Errorf("CORSAllowedOrigins = %v, want %v", cfg.CORSAllowedOrigins, want)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false")
	}
	if cfg.SupabaseStorageMenuBucket != "diuk-menu-images" {
		t.Errorf("SupabaseStorageMenuBucket = %q, want default bucket", cfg.SupabaseStorageMenuBucket)
	}
	if cfg.AIEmbeddingEnabled {
		t.Error("AIEmbeddingEnabled default should be false")
	}
	if cfg.AIEmbeddingModel != "text-embedding-3-small" || cfg.AIEmbeddingDimensions != 1536 || cfg.AIEmbeddingBatchSize != 64 {
		t.Errorf("embedding defaults = %s %d %d", cfg.AIEmbeddingModel, cfg.AIEmbeddingDimensions, cfg.AIEmbeddingBatchSize)
	}
	if cfg.KnowledgeChunkSize != 800 || cfg.KnowledgeTopK != 5 || cfg.KnowledgeMinSimilarity != 0.35 || cfg.KnowledgeMaxFileSizeMB != 10 {
		t.Errorf("knowledge defaults = %d %d %v %d", cfg.KnowledgeChunkSize, cfg.KnowledgeTopK, cfg.KnowledgeMinSimilarity, cfg.KnowledgeMaxFileSizeMB)
	}
	if !cfg.AutomationEnabled {
		t.Error("AutomationEnabled default should be true")
	}
	if cfg.AutomationSchedulerInterval != 30*time.Second || cfg.AutomationMaxRunsPerTick != 50 || cfg.AutomationDefaultInactiveMin != 120 {
		t.Errorf("automation defaults = %s %d %d", cfg.AutomationSchedulerInterval, cfg.AutomationMaxRunsPerTick, cfg.AutomationDefaultInactiveMin)
	}
}

func TestLoadRequiresDatabaseRedisAndSupabaseURLs(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		redisURL    string
		supabaseURL string
		want        []string
	}{
		{name: "database missing", redisURL: "redis://localhost:6379", supabaseURL: "https://example.supabase.co", want: []string{"DATABASE_URL"}},
		{name: "redis missing", databaseURL: "postgresql://localhost:5432/postgres", supabaseURL: "https://example.supabase.co", want: []string{"REDIS_URL"}},
		{name: "supabase missing", databaseURL: "postgresql://localhost:5432/postgres", redisURL: "redis://localhost:6379", want: []string{"SUPABASE_URL"}},
		{name: "all missing", want: []string{"DATABASE_URL", "REDIS_URL", "SUPABASE_URL"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("REDIS_URL", tt.redisURL)
			t.Setenv("SUPABASE_URL", tt.supabaseURL)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want an error about missing variables")
			}

			for _, name := range tt.want {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("error %q does not mention %q", err, name)
				}
			}
		})
	}
}

func TestLoadReadsDatabaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.diuk.id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if want := "postgresql://user:password@db.example.com:5432/postgres"; cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, want)
	}
	if want := "https://example.supabase.co"; cfg.SupabaseURL != want {
		t.Errorf("SupabaseURL = %q, want %q", cfg.SupabaseURL, want)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
}

func TestLoadReadsStorageSettings(t *testing.T) {
	setRequired(t)
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-test")
	t.Setenv("SUPABASE_STORAGE_MENU_BUCKET", "custom-menu")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SupabaseServiceRoleKey != "service-role-test" {
		t.Errorf("SupabaseServiceRoleKey = %q", cfg.SupabaseServiceRoleKey)
	}
	if cfg.SupabaseStorageMenuBucket != "custom-menu" {
		t.Errorf("SupabaseStorageMenuBucket = %q", cfg.SupabaseStorageMenuBucket)
	}
}

func TestLoadTrimsSupabaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("SUPABASE_URL", "https://example.supabase.co/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SupabaseURL != "https://example.supabase.co" {
		t.Errorf("SupabaseURL = %q, want trimmed project URL", cfg.SupabaseURL)
	}
}

func TestLoadProductionRejectsInsecureCORS(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected production to require CORS_ALLOWED_ORIGINS")
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	if _, err := Load(); err == nil {
		t.Fatal("expected production to reject wildcard CORS")
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.diuk.id")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid production CORS: %v", err)
	}
	if cfg.HTTPReadTimeout != 30*time.Second || cfg.DBMaxConns != 10 || !cfg.RateLimitEnabled {
		t.Fatalf("hardening defaults = timeout=%s conns=%d ratelimit=%t", cfg.HTTPReadTimeout, cfg.DBMaxConns, cfg.RateLimitEnabled)
	}
}

func TestLoadProductionRejectsTestFlags(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.diuk.id")
	t.Setenv("TEST_AI_LIVE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected production to reject TEST_AI_LIVE")
	}
}

func TestLoadParsesCORSOrigins(t *testing.T) {
	setRequired(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, https://app.diuk.id ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"http://localhost:3000", "https://app.diuk.id"}
	if !slices.Equal(cfg.CORSAllowedOrigins, want) {
		t.Errorf("CORSAllowedOrigins = %v, want %v", cfg.CORSAllowedOrigins, want)
	}
}
