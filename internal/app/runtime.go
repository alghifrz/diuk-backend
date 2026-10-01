package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/ai/embedding"
	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/analytics"
	"github.com/alghifrz/diuk-backend/internal/area"
	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/automation"
	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/calendar"
	"github.com/alghifrz/diuk-backend/internal/campaign"
	"github.com/alghifrz/diuk-backend/internal/channel"
	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/database"
	"github.com/alghifrz/diuk-backend/internal/job"
	"github.com/alghifrz/diuk-backend/internal/job/handlers"
	redisq "github.com/alghifrz/diuk-backend/internal/job/redis"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
	"github.com/alghifrz/diuk-backend/internal/menu"
	"github.com/alghifrz/diuk-backend/internal/observability"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/server"
	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/internal/usage"
	"github.com/alghifrz/diuk-backend/pkg/config"
	"github.com/alghifrz/diuk-backend/pkg/storage"
)

type Runtime struct {
	cfg       *config.Config
	log       *slog.Logger
	db        *database.DB
	redis     *redisq.Client
	queue     job.Queue
	server    *server.Server
	workers   []*job.Worker
	publish   *job.Publisher
	scheduler *automation.Service
	campaigns *campaign.Service
	closeFn   func()
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Runtime, error) {
	db, err := database.Connect(ctx, cfg.DatabaseURL, database.PoolSettings{
		MaxConns:          cfg.DBMaxConns,
		MinConns:          cfg.DBMinConns,
		MaxConnLifetime:   cfg.DBMaxConnLifetime,
		MaxConnIdleTime:   cfg.DBMaxConnIdleTime,
		HealthCheckPeriod: cfg.DBHealthCheckPeriod,
	})
	if err != nil {
		return nil, err
	}

	redisClient, err := redisq.Connect(ctx, redisq.Config{
		URL: cfg.RedisURL, Password: cfg.RedisPassword, DB: cfg.RedisDB, Timeout: cfg.RedisTimeout,
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	verifier, err := auth.NewVerifier(ctx, cfg.SupabaseURL)
	if err != nil {
		_ = redisClient.Close()
		db.Close()
		return nil, err
	}

	queue := redisq.NewQueue(redisClient, cfg.RedisQueuePrefix, cfg.JobDLQRetention)
	jobStore := job.NewRepository(db.Pool())
	outbox := job.NewOutbox(log, jobStore, queue, cfg.JobMaxAttempts)

	guard := auth.NewMiddleware(
		log,
		verifier,
		tenant.NewUserRepository(db.Pool()),
		tenant.NewBusinessUserRepository(db.Pool()),
	)
	authorizer := authz.NewMiddleware(log, authz.NewService(authz.NewRepository(db.Pool())))
	objects := storage.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.SupabaseStorageMenuBucket)
	conversationService := conversation.NewService(
		conversation.NewRepository(db.Pool()),
		tenant.NewUserRepository(db.Pool()),
		tenant.NewBusinessUserRepository(db.Pool()),
	)
	reservationService := reservation.NewService(reservation.NewRepository(db.Pool()))
	paymentService := payment.NewService(payment.NewRepository(db.Pool()))
	calendarService := calendar.NewService(calendar.NewRepository(db.Pool()))
	businessService := business.NewService(business.NewRepository(db.Pool()))
	customerService := customer.NewService(customer.NewRepository(db.Pool()))
	menuService := menu.NewService(menu.NewRepository(db.Pool()), objects, log)
	knowledgeService := knowledge.NewService(
		log,
		knowledge.NewRepository(db.Pool()),
		objects,
		embedding.New(embedding.Config{
			Enabled:    cfg.AIEmbeddingEnabled,
			APIKey:     cfg.AIEmbeddingAPIKey,
			BaseURL:    cfg.AIEmbeddingBaseURL,
			Model:      cfg.AIEmbeddingModel,
			Timeout:    cfg.AIEmbeddingTimeout,
			Dimensions: cfg.AIEmbeddingDimensions,
		}),
		outbox,
		knowledge.Settings{
			Enabled:             cfg.AIEmbeddingEnabled,
			ChunkSize:           cfg.KnowledgeChunkSize,
			ChunkOverlap:        cfg.KnowledgeChunkOverlap,
			TopK:                cfg.KnowledgeTopK,
			MinSimilarity:       cfg.KnowledgeMinSimilarity,
			MaxFileBytes:        int64(cfg.KnowledgeMaxFileSizeMB) * 1024 * 1024,
			EmbeddingModel:      cfg.AIEmbeddingModel,
			EmbeddingDimensions: cfg.AIEmbeddingDimensions,
			EmbeddingBatchSize:  cfg.AIEmbeddingBatchSize,
			StaleProcessing:     cfg.KnowledgeWorkerTimeout,
		},
	)
	aiService := ai.NewService(
		log,
		ai.NewRepository(db.Pool()),
		ai.Defaults{
			Provider: cfg.AIProvider, Model: cfg.AIModel, APIKey: cfg.AIAPIKey,
			BaseURL: cfg.AIBaseURL, Timeout: cfg.AITimeout,
		},
		provider.NewOpenAI(provider.Config{APIKey: cfg.AIAPIKey, BaseURL: cfg.AIBaseURL, Timeout: cfg.AITimeout}),
		nil,
		businessService, customerService, menuService, conversationService, reservationService,
		ai.NewKnowledgeSearch(knowledgeService),
	)
	metrics := observability.NewMetrics(nil)
	usageService := usage.NewService(log, usage.NewRepository(db.Pool()))
	usageService.SetMetrics(metrics)
	calendarService.SetMetrics(metrics)
	reservationService.SetMetrics(metrics)
	aiService.SetMetrics(metrics)
	aiService.SetUsageMeter(usageService)
	knowledgeService.SetUsageMeter(usageService)
	knowledgeService.SetAnswerer(ai.NewKnowledgeAnswerer(
		provider.NewOpenAI(provider.Config{APIKey: cfg.AIAPIKey, BaseURL: cfg.AIBaseURL, Timeout: cfg.AITimeout}),
		cfg.AIModel,
	))
	conversationService.SetOnQueuedOutbound(outbox.EnqueueDelivery)

	channelService := channel.NewService(
		log,
		channel.NewRepository(db.Pool()),
		customerService,
		conversationService,
		aiService,
		whatsapp.NewClient(cfg.MetaWhatsAppTimeout),
		channel.MetaConfig{
			AccessToken:    cfg.MetaWhatsAppAccessToken,
			AppSecret:      cfg.MetaWhatsAppAppSecret,
			VerifyToken:    cfg.MetaWhatsAppVerifyToken,
			APIVersion:     cfg.MetaWhatsAppAPIVersion,
			BaseURL:        cfg.MetaWhatsAppBaseURL,
			Timeout:        cfg.MetaWhatsAppTimeout,
			MaxWebhookBody: cfg.WebhookMaxBodyBytes,
		},
	)
	channelService.SetTrigger(channel.QueueTrigger{Enqueue: outbox, Log: log})
	channelService.SetMetrics(metrics)
	channelService.SetUsageMeter(usageService)
	analyticsService := analytics.NewService(analytics.NewRepository(db.Pool()), businessTZ{svc: businessService})

	automationService := automation.NewService(
		log,
		automation.NewRepository(db.Pool()),
		automation.QueuePublisher{Evaluate: outbox.EnqueueAutomationEvaluate, Run: outbox.EnqueueAutomationRun},
		outbox,
		conversationService,
		customerService,
		reservationService,
		businessService,
		aiService,
		automation.Settings{
			Enabled:            cfg.AutomationEnabled,
			SchedulerInterval:  cfg.AutomationSchedulerInterval,
			MaxRunsPerTick:     cfg.AutomationMaxRunsPerTick,
			DefaultInactiveMin: cfg.AutomationDefaultInactiveMin,
		},
	)
	automationService.SetUsageMeter(usageService)
	reservationService.SetEvents(automationService)
	conversationService.SetEvents(automationService)
	customerService.SetEvents(automationService)

	campaignService := campaign.NewService(
		log,
		campaign.NewRepository(db.Pool()),
		outbox,
		campaignBusiness{svc: businessService},
		conversationService,
		campaign.Settings{
			BatchSize:         cfg.CampaignBatchSize,
			DispatchInterval:  cfg.CampaignDispatchInterval,
			SchedulerInterval: cfg.CampaignSchedulerInterval,
			MaxStartsPerTick:  cfg.CampaignMaxStartsPerTick,
		},
	)
	campaignService.SetMetrics(metrics)
	campaignService.SetUsageMeter(usageService)
	conversationService.SetOnDeliveryStatus(func(ctx context.Context, businessID, messageID uuid.UUID, status conversation.MessageStatus) error {
		return campaignService.OnMessageDelivery(ctx, businessID, messageID, string(status))
	})
	conversationService.SetOnInboundRead(func(ctx context.Context, businessID, conversationID uuid.UUID, externalIDs []string) error {
		return channelService.NotifyInboundRead(ctx, businessID, conversationID, externalIDs)
	})

	policy := job.RetryPolicy{MaxAttempts: cfg.JobMaxAttempts, Base: cfg.JobRetryBase, Cap: cfg.JobRetryCap}
	workers := []*job.Worker{
		job.NewWorker(log, queue, handlers.NewAIHandler(log, aiService, conversationService, jobStore), jobStore, job.WorkerConfig{
			Name: "ai-1", Queue: "ai", Concurrency: cfg.WorkerAIConcurrency,
			Timeout: cfg.AIWorkerTimeout, IdleClaim: cfg.JobClaimIdle, Policy: policy,
		}),
		job.NewWorker(log, queue, handlers.NewDeliveryHandler(log, channelService), jobStore, job.WorkerConfig{
			Name: "delivery-1", Queue: "delivery", Concurrency: cfg.WorkerDeliveryConcurrency,
			Timeout: cfg.DeliveryWorkerTimeout, IdleClaim: cfg.JobClaimIdle, Policy: policy,
		}),
		job.NewWorker(log, queue, handlers.NewKnowledgeHandler(log, knowledgeService), jobStore, job.WorkerConfig{
			Name: "knowledge-1", Queue: "knowledge", Concurrency: cfg.WorkerKnowledgeConcurrency,
			Timeout: cfg.KnowledgeWorkerTimeout, IdleClaim: cfg.JobClaimIdle, Policy: policy,
		}),
		job.NewWorker(log, queue, handlers.NewAutomationHandler(log, automationService), jobStore, job.WorkerConfig{
			Name: "automation-1", Queue: "automation", Concurrency: cfg.WorkerAutomationConcurrency,
			Timeout: cfg.AutomationWorkerTimeout, IdleClaim: cfg.JobClaimIdle, Policy: policy,
		}),
		job.NewWorker(log, queue, handlers.NewCampaignHandler(log, campaignService), jobStore, job.WorkerConfig{
			Name: "campaign-1", Queue: "campaign", Concurrency: cfg.CampaignMaxConcurrency,
			Timeout: cfg.CampaignWorkerTimeout, IdleClaim: cfg.JobClaimIdle, Policy: policy,
		}),
	}
	for _, w := range workers {
		w.SetTelemetry(metrics)
	}

	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, stat := range queue.Stats(r.Context(), []string{"ai", "delivery", "knowledge", "automation", "campaign"}) {
			metrics.SetQueueDepth(stat.Queue, stat.Stream, stat.Group, float64(stat.Pending))
		}
		promhttp.Handler().ServeHTTP(w, r)
	})

	httpServer := server.New(cfg, log, server.Dependencies{
		DB:             db,
		Redis:          redisClient,
		Auth:           guard,
		Authz:          authorizer,
		Business:       business.NewHandler(log, businessService),
		Area:           area.NewHandler(log, area.NewService(area.NewRepository(db.Pool()))),
		Menu:           menu.NewHandler(log, menuService),
		Customer:       customer.NewHandler(log, customerService),
		Conversation:   conversation.NewHandler(log, conversationService),
		Reservation:    reservation.NewHandler(log, reservationService),
		AI:             ai.NewHandler(log, aiService),
		Channel:        channel.NewHandler(log, channelService),
		Knowledge:      knowledge.NewHandler(log, knowledgeService),
		Automation:     automation.NewHandler(log, automationService),
		Campaign:       campaign.NewHandler(log, campaignService),
		Analytics:      analytics.NewHandler(log, analyticsService),
		Usage:          usage.NewHandler(log, usageService),
		Payment:        payment.NewHandler(log, paymentService),
		Calendar:       calendar.NewHandler(log, calendarService),
		Metrics:        metrics,
		MetricsHandler: metricsHandler,
	})

	return &Runtime{
		cfg: cfg, log: log, db: db, redis: redisClient, queue: queue,
		server: httpServer, workers: workers, publish: job.NewPublisher(log, jobStore, queue, cfg.OutboxPollInterval),
		scheduler: automationService,
		campaigns: campaignService,
		closeFn: func() {
			verifier.Close()
			_ = queue.Close()
			db.Close()
		},
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 4)

	if r.cfg.ServesAPI() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.server.Run(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	if r.cfg.ServesWorkers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.publish.Run(ctx)
		}()
		for i := range r.workers {
			w := r.workers[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := w.Run(ctx); err != nil && ctx.Err() == nil {
					errCh <- err
				}
			}()
		}
		if r.scheduler != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.scheduler.RunScheduler(ctx)
			}()
		}
		if r.campaigns != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.campaigns.RunScheduler(ctx)
			}()
		}
		r.log.Info("workers started",
			"ai_concurrency", r.cfg.WorkerAIConcurrency,
			"delivery_concurrency", r.cfg.WorkerDeliveryConcurrency,
			"knowledge_concurrency", r.cfg.WorkerKnowledgeConcurrency,
			"automation_concurrency", r.cfg.WorkerAutomationConcurrency,
			"campaign_concurrency", r.cfg.CampaignMaxConcurrency,
			"automation_enabled", r.cfg.AutomationEnabled,
		)
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timeout := r.shutdownWait()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		r.log.Error("shutdown wait timed out", "timeout", timeout.String())
		return nil
	}
}

func (r *Runtime) shutdownWait() time.Duration {
	max := r.cfg.HTTPShutdownTimeout
	for _, d := range []time.Duration{
		r.cfg.AIWorkerTimeout,
		r.cfg.DeliveryWorkerTimeout,
		r.cfg.KnowledgeWorkerTimeout,
		r.cfg.AutomationWorkerTimeout,
		r.cfg.CampaignWorkerTimeout,
	} {
		if d > max {
			max = d
		}
	}
	if max <= 0 {
		return 15 * time.Second
	}
	return max + 2*time.Second
}

func (r *Runtime) Close() {
	if r.closeFn != nil {
		r.closeFn()
	}
}

func (r *Runtime) String() string {
	return fmt.Sprintf("diuk runtime mode=%s", r.cfg.AppMode)
}

type businessTZ struct {
	svc *business.Service
}

func (b businessTZ) Timezone(ctx context.Context, businessID uuid.UUID) (string, error) {
	profile, err := b.svc.Profile(ctx, businessID)
	if err != nil {
		return "UTC", err
	}
	if profile == nil || strings.TrimSpace(profile.Timezone) == "" {
		return "UTC", nil
	}
	return profile.Timezone, nil
}

type campaignBusiness struct {
	svc *business.Service
}

func (b campaignBusiness) ActiveTimezone(ctx context.Context, businessID uuid.UUID) (string, error) {
	profile, err := b.svc.Profile(ctx, businessID)
	if err != nil {
		return "", err
	}
	if profile == nil || profile.Status != tenant.StatusActive {
		return "", campaign.ErrBusinessInactive
	}
	if strings.TrimSpace(profile.Timezone) == "" {
		return "UTC", nil
	}
	return profile.Timezone, nil
}
