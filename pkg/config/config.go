// Package config loads the application configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every value the application needs at startup.
type Config struct {
	AppEnv  string
	AppPort string
	AppName string

	DatabaseURL string
	RedisURL    string
	SupabaseURL string

	SupabaseServiceRoleKey    string
	SupabaseStorageMenuBucket string

	CORSAllowedOrigins []string

	AIProvider string
	AIAPIKey   string
	AIModel    string
	AIBaseURL  string
	AITimeout  time.Duration

	MetaWhatsAppAccessToken string
	MetaWhatsAppAppSecret   string
	MetaWhatsAppVerifyToken string
	MetaWhatsAppAPIVersion  string
	MetaWhatsAppBaseURL     string
	MetaWhatsAppTimeout     time.Duration

	AppMode                   string
	RedisPassword             string
	RedisDB                   int
	RedisTimeout              time.Duration
	RedisQueuePrefix          string
	WorkerAIConcurrency       int
	WorkerDeliveryConcurrency int
	AIWorkerTimeout           time.Duration
	DeliveryWorkerTimeout     time.Duration
	JobMaxAttempts            int
	JobRetryBase              time.Duration
	JobRetryCap               time.Duration
	JobDLQRetention           time.Duration
	OutboxPollInterval        time.Duration
	JobClaimIdle              time.Duration

	AIEmbeddingEnabled    bool
	AIEmbeddingProvider   string
	AIEmbeddingAPIKey     string
	AIEmbeddingModel      string
	AIEmbeddingBaseURL    string
	AIEmbeddingDimensions int
	AIEmbeddingBatchSize  int
	AIEmbeddingTimeout    time.Duration

	KnowledgeChunkSize         int
	KnowledgeChunkOverlap      int
	KnowledgeTopK              int
	KnowledgeMinSimilarity     float64
	KnowledgeMaxFileSizeMB     int
	KnowledgeWorkerTimeout     time.Duration
	WorkerKnowledgeConcurrency int

	AutomationEnabled            bool
	AutomationSchedulerInterval  time.Duration
	AutomationMaxRunsPerTick     int
	AutomationDefaultInactiveMin int
	WorkerAutomationConcurrency  int
	AutomationWorkerTimeout      time.Duration

	CampaignBatchSize         int
	CampaignDispatchInterval  time.Duration
	CampaignSchedulerInterval time.Duration
	CampaignMaxStartsPerTick  int
	CampaignMaxConcurrency    int
	CampaignWorkerTimeout     time.Duration

	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPMaxHeaderBytes    int
	HTTPMaxJSONBytes      int64
	HTTPShutdownTimeout   time.Duration
	HTTPTrustedProxies    []string
	WebhookMaxBodyBytes   int64

	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration

	RateLimitEnabled        bool
	RateLimitWebhookPerMin  int
	RateLimitAIPerMin       int
	RateLimitUploadPerMin   int
	RateLimitCampaignPerMin int
}

// Load reads configuration from the environment, falling back to a local .env
// file when one exists. It returns an error listing every missing required
// variable so misconfiguration is obvious on the first run.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	cfg := &Config{
		AppEnv:                       lookup("APP_ENV", "development"),
		AppPort:                      lookup("APP_PORT", "8080"),
		AppName:                      lookup("APP_NAME", "diuk-backend"),
		DatabaseURL:                  os.Getenv("DATABASE_URL"),
		RedisURL:                     os.Getenv("REDIS_URL"),
		SupabaseURL:                  strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/"),
		SupabaseServiceRoleKey:       strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY")),
		SupabaseStorageMenuBucket:    lookup("SUPABASE_STORAGE_MENU_BUCKET", "diuk-menu-images"),
		CORSAllowedOrigins:           splitList(lookup("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		AIProvider:                   strings.ToLower(lookup("AI_PROVIDER", "openai")),
		AIAPIKey:                     strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIModel:                      lookup("AI_MODEL", "gpt-4o-mini"),
		AIBaseURL:                    strings.TrimRight(lookup("AI_BASE_URL", "https://api.openai.com/v1"), "/"),
		AITimeout:                    lookupDurationSeconds("AI_TIMEOUT_SECONDS", 30),
		MetaWhatsAppAccessToken:      strings.TrimSpace(os.Getenv("META_WHATSAPP_ACCESS_TOKEN")),
		MetaWhatsAppAppSecret:        strings.TrimSpace(os.Getenv("META_WHATSAPP_APP_SECRET")),
		MetaWhatsAppVerifyToken:      strings.TrimSpace(os.Getenv("META_WHATSAPP_VERIFY_TOKEN")),
		MetaWhatsAppAPIVersion:       lookup("META_WHATSAPP_API_VERSION", "v21.0"),
		MetaWhatsAppBaseURL:          strings.TrimRight(lookup("META_WHATSAPP_BASE_URL", "https://graph.facebook.com"), "/"),
		MetaWhatsAppTimeout:          lookupDurationSeconds("META_WHATSAPP_TIMEOUT_SECONDS", 10),
		AppMode:                      strings.ToLower(lookup("APP_MODE", "all")),
		RedisPassword:                strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
		RedisDB:                      lookupInt("REDIS_DB", 0),
		RedisTimeout:                 lookupDurationSeconds("REDIS_TIMEOUT_SECONDS", 5),
		RedisQueuePrefix:             lookup("REDIS_QUEUE_PREFIX", "diuk"),
		WorkerAIConcurrency:          lookupBoundedInt("WORKER_AI_CONCURRENCY", 2, 1, 32),
		WorkerDeliveryConcurrency:    lookupBoundedInt("WORKER_DELIVERY_CONCURRENCY", 2, 1, 32),
		AIWorkerTimeout:              lookupDurationSeconds("AI_WORKER_TIMEOUT_SECONDS", 90),
		DeliveryWorkerTimeout:        lookupDurationSeconds("DELIVERY_WORKER_TIMEOUT_SECONDS", 20),
		JobMaxAttempts:               lookupBoundedInt("JOB_MAX_ATTEMPTS", 3, 1, 20),
		JobRetryBase:                 lookupDurationSeconds("JOB_RETRY_BASE_SECONDS", 30),
		JobRetryCap:                  lookupDurationSeconds("JOB_RETRY_CAP_SECONDS", 120),
		JobDLQRetention:              time.Duration(lookupInt("JOB_DLQ_RETENTION_DAYS", 7)) * 24 * time.Hour,
		OutboxPollInterval:           lookupDurationSeconds("OUTBOX_POLL_SECONDS", 2),
		JobClaimIdle:                 lookupDurationSeconds("JOB_CLAIM_IDLE_SECONDS", 60),
		AIEmbeddingProvider:          strings.ToLower(lookup("AI_EMBEDDING_PROVIDER", "openai")),
		AIEmbeddingAPIKey:            firstNonEmpty(os.Getenv("AI_EMBEDDING_API_KEY"), os.Getenv("AI_API_KEY")),
		AIEmbeddingModel:             lookup("AI_EMBEDDING_MODEL", "text-embedding-3-small"),
		AIEmbeddingBaseURL:           strings.TrimRight(lookup("AI_EMBEDDING_BASE_URL", lookup("AI_BASE_URL", "https://api.openai.com/v1")), "/"),
		AIEmbeddingDimensions:        lookupInt("AI_EMBEDDING_DIMENSIONS", 1536),
		AIEmbeddingBatchSize:         lookupInt("AI_EMBEDDING_BATCH_SIZE", 64),
		AIEmbeddingTimeout:           lookupDurationSeconds("AI_EMBEDDING_TIMEOUT_SECONDS", 30),
		KnowledgeChunkSize:           lookupInt("KNOWLEDGE_CHUNK_SIZE", 800),
		KnowledgeChunkOverlap:        lookupInt("KNOWLEDGE_CHUNK_OVERLAP", 120),
		KnowledgeTopK:                lookupInt("KNOWLEDGE_TOP_K", 5),
		KnowledgeMinSimilarity:       lookupFloat("KNOWLEDGE_MIN_SIMILARITY", 0.35),
		KnowledgeMaxFileSizeMB:       lookupBoundedInt("KNOWLEDGE_MAX_FILE_SIZE_MB", 10, 1, 25),
		KnowledgeWorkerTimeout:       lookupDurationSeconds("KNOWLEDGE_WORKER_TIMEOUT_SECONDS", 180),
		WorkerKnowledgeConcurrency:   lookupBoundedInt("WORKER_KNOWLEDGE_CONCURRENCY", 1, 1, 16),
		AutomationSchedulerInterval:  lookupDurationSeconds("AUTOMATION_SCHEDULER_INTERVAL", 30),
		AutomationMaxRunsPerTick:     lookupBoundedInt("AUTOMATION_MAX_RUNS_PER_TICK", 50, 1, 200),
		AutomationDefaultInactiveMin: lookupInt("AUTOMATION_DEFAULT_INACTIVITY_MINUTES", 120),
		WorkerAutomationConcurrency:  lookupBoundedInt("WORKER_AUTOMATION_CONCURRENCY", 2, 1, 16),
		AutomationWorkerTimeout:      lookupDurationSeconds("AUTOMATION_WORKER_TIMEOUT_SECONDS", 60),
		CampaignBatchSize:            lookupBoundedInt("CAMPAIGN_BATCH_SIZE", 50, 1, 200),
		CampaignDispatchInterval:     lookupDurationMillis("CAMPAIGN_DISPATCH_INTERVAL_MS", 200),
		CampaignSchedulerInterval:    lookupDurationSeconds("CAMPAIGN_SCHEDULER_INTERVAL", 30),
		CampaignMaxStartsPerTick:     lookupBoundedInt("CAMPAIGN_MAX_STARTS_PER_TICK", 20, 1, 100),
		CampaignMaxConcurrency:       lookupBoundedInt("CAMPAIGN_MAX_CONCURRENCY", 2, 1, 16),
		CampaignWorkerTimeout:        lookupDurationSeconds("CAMPAIGN_WORKER_TIMEOUT_SECONDS", 60),
		HTTPReadHeaderTimeout:        lookupDurationSeconds("HTTP_READ_HEADER_TIMEOUT_SECONDS", 10),
		HTTPReadTimeout:              lookupDurationSeconds("HTTP_READ_TIMEOUT_SECONDS", 30),
		HTTPWriteTimeout:             lookupDurationSeconds("HTTP_WRITE_TIMEOUT_SECONDS", 180),
		HTTPIdleTimeout:              lookupDurationSeconds("HTTP_IDLE_TIMEOUT_SECONDS", 60),
		HTTPMaxHeaderBytes:           lookupBoundedInt("HTTP_MAX_HEADER_BYTES", 1<<20, 4096, 4<<20),
		HTTPMaxJSONBytes:             int64(lookupBoundedInt("HTTP_MAX_JSON_BYTES", 1<<20, 1024, 8<<20)),
		HTTPShutdownTimeout:          lookupDurationSeconds("HTTP_SHUTDOWN_TIMEOUT_SECONDS", 15),
		HTTPTrustedProxies:           splitList(os.Getenv("HTTP_TRUSTED_PROXIES")),
		WebhookMaxBodyBytes:          int64(lookupBoundedInt("WEBHOOK_MAX_BODY_BYTES", 1<<20, 1024, 8<<20)),
		DBMaxConns:                   int32(lookupBoundedInt("DB_MAX_CONNS", 10, 1, 50)),
		DBMinConns:                   int32(lookupBoundedInt("DB_MIN_CONNS", 2, 0, 50)),
		DBMaxConnLifetime:            lookupDurationSeconds("DB_MAX_CONN_LIFETIME_SECONDS", 3600),
		DBMaxConnIdleTime:            lookupDurationSeconds("DB_MAX_CONN_IDLE_TIME_SECONDS", 1800),
		DBHealthCheckPeriod:          lookupDurationSeconds("DB_HEALTH_CHECK_PERIOD_SECONDS", 60),
		RateLimitWebhookPerMin:       lookupBoundedInt("RATE_LIMIT_WEBHOOK_PER_MIN", 180, 0, 10000),
		RateLimitAIPerMin:            lookupBoundedInt("RATE_LIMIT_AI_PER_MIN", 20, 0, 1000),
		RateLimitUploadPerMin:        lookupBoundedInt("RATE_LIMIT_UPLOAD_PER_MIN", 10, 0, 1000),
		RateLimitCampaignPerMin:      lookupBoundedInt("RATE_LIMIT_CAMPAIGN_PER_MIN", 10, 0, 1000),
	}
	cfg.AIEmbeddingEnabled = lookupBool("AI_EMBEDDING_ENABLED", false)
	cfg.AutomationEnabled = lookupBool("AUTOMATION_ENABLED", true)
	cfg.RateLimitEnabled = lookupBool("RATE_LIMIT_ENABLED", true)
	if cfg.DBMinConns > cfg.DBMaxConns {
		cfg.DBMinConns = cfg.DBMaxConns
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if cfg.SupabaseURL == "" {
		missing = append(missing, "SUPABASE_URL")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if !c.IsProduction() {
		return nil
	}
	rawCORS := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if rawCORS == "" {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS is required in production")
	}
	for _, origin := range c.CORSAllowedOrigins {
		if origin == "*" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS cannot be * in production")
		}
	}
	if lookupBool("TEST_AI_LIVE", false) || lookupBool("TEST_WHATSAPP_LIVE", false) {
		return fmt.Errorf("test-only flags cannot be enabled in production")
	}
	return nil
}

// IsProduction reports whether the application runs in a production environment.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// Address returns the TCP address the HTTP server listens on.
func (c *Config) Address() string {
	return ":" + c.AppPort
}

func (c *Config) ServesAPI() bool {
	return c.AppMode == "api" || c.AppMode == "all" || c.AppMode == ""
}

func (c *Config) ServesWorkers() bool {
	return c.AppMode == "worker" || c.AppMode == "all" || c.AppMode == ""
}

func lookup(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func lookupInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func lookupBoundedInt(key string, fallback, min, max int) int {
	n := lookupInt(key, fallback)
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func lookupDurationMillis(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Millisecond
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return time.Duration(fallback) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}

func lookupDurationSeconds(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(n) * time.Second
}

func lookupBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func lookupFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")

	list := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			list = append(list, trimmed)
		}
	}

	return list
}
