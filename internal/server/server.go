// Package server wires the HTTP layer of the DIUK backend.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/analytics"
	"github.com/alghifrz/diuk-backend/internal/area"
	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/authz"
	"github.com/alghifrz/diuk-backend/internal/automation"
	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/calendar"
	"github.com/alghifrz/diuk-backend/internal/campaign"
	"github.com/alghifrz/diuk-backend/internal/channel"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
	"github.com/alghifrz/diuk-backend/internal/menu"
	"github.com/alghifrz/diuk-backend/internal/observability"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/usage"
	"github.com/alghifrz/diuk-backend/pkg/config"
)

const defaultShutdownTimeout = 15 * time.Second

// DatabasePinger reports whether the database is reachable. It is the only
// part of the database the HTTP layer needs at this stage.
type DatabasePinger interface {
	Ping(ctx context.Context) error
}

type RedisPinger interface {
	Ping(ctx context.Context) error
}

// Dependencies are the collaborators the HTTP server needs beyond config and
// logging. Tests can substitute fakes for the auth and tenant lookups.
type Dependencies struct {
	DB             DatabasePinger
	Auth           *auth.Middleware
	Authz          *authz.Middleware
	Business       *business.Handler
	Area           *area.Handler
	Menu           *menu.Handler
	Customer       *customer.Handler
	Conversation   *conversation.Handler
	Reservation    *reservation.Handler
	AI             *ai.Handler
	Channel        *channel.Handler
	Knowledge      *knowledge.Handler
	Automation     *automation.Handler
	Campaign       *campaign.Handler
	Analytics      *analytics.Handler
	Usage          *usage.Handler
	Payment        *payment.Handler
	Calendar       *calendar.Handler
	Redis          RedisPinger
	Metrics        *observability.Metrics
	MetricsHandler http.Handler
}

// Server owns the HTTP server and its dependencies.
type Server struct {
	cfg            *config.Config
	log            *slog.Logger
	db             DatabasePinger
	auth           *auth.Middleware
	authz          *authz.Middleware
	business       *business.Handler
	area           *area.Handler
	menu           *menu.Handler
	customer       *customer.Handler
	conversation   *conversation.Handler
	reservation    *reservation.Handler
	ai             *ai.Handler
	channel        *channel.Handler
	knowledge      *knowledge.Handler
	automation     *automation.Handler
	campaign       *campaign.Handler
	analytics      *analytics.Handler
	usage          *usage.Handler
	payment        *payment.Handler
	calendar       *calendar.Handler
	redis          RedisPinger
	metrics        *observability.Metrics
	metricsHandler http.Handler
	limits         *rateLimiters
	http           *http.Server
}

// New builds a Server with middleware and routes already registered.
func New(cfg *config.Config, log *slog.Logger, deps Dependencies) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if err := router.SetTrustedProxies(cfg.HTTPTrustedProxies); err != nil {
		_ = router.SetTrustedProxies(nil)
	}
	router.Use(
		observability.RequestID(),
		securityHeaders(),
		maxJSONBody(cfg.HTTPMaxJSONBytes),
		recovery(log),
		requestLogger(log, deps.Metrics),
		cors(cfg.CORSAllowedOrigins),
	)

	s := &Server{
		cfg:            cfg,
		log:            log,
		db:             deps.DB,
		auth:           deps.Auth,
		authz:          deps.Authz,
		business:       deps.Business,
		area:           deps.Area,
		menu:           deps.Menu,
		customer:       deps.Customer,
		conversation:   deps.Conversation,
		reservation:    deps.Reservation,
		ai:             deps.AI,
		channel:        deps.Channel,
		knowledge:      deps.Knowledge,
		automation:     deps.Automation,
		campaign:       deps.Campaign,
		analytics:      deps.Analytics,
		usage:          deps.Usage,
		payment:        deps.Payment,
		calendar:       deps.Calendar,
		redis:          deps.Redis,
		metrics:        deps.Metrics,
		metricsHandler: deps.MetricsHandler,
		limits:         newRateLimiters(cfg.RateLimitEnabled, cfg.RateLimitWebhookPerMin, cfg.RateLimitAIPerMin, cfg.RateLimitUploadPerMin, cfg.RateLimitCampaignPerMin),
		http: &http.Server{
			Addr:              cfg.Address(),
			Handler:           router,
			ReadHeaderTimeout: durationOr(cfg.HTTPReadHeaderTimeout, 10*time.Second),
			ReadTimeout:       durationOr(cfg.HTTPReadTimeout, 30*time.Second),
			WriteTimeout:      durationOr(cfg.HTTPWriteTimeout, 180*time.Second),
			IdleTimeout:       durationOr(cfg.HTTPIdleTimeout, 60*time.Second),
			MaxHeaderBytes:    intOr(cfg.HTTPMaxHeaderBytes, 1<<20),
		},
	}
	s.registerRoutes(router)

	return s
}

func (s *Server) registerRoutes(router *gin.Engine) {
	router.GET("/", s.handleRoot)

	v1 := router.Group("/api/v1")
	v1.GET("/health", s.handleHealth)
	v1.GET("/health/ready", s.handleReady)
	v1.GET("/health/db", s.handleDatabaseHealth)
	v1.GET("/health/redis", s.handleRedisHealth)
	if s.metricsHandler != nil {
		router.GET("/metrics", gin.WrapH(s.metricsHandler))
	} else if s.metrics != nil {
		router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	if s.channel != nil {
		router.GET("/api/webhooks/whatsapp", s.channel.VerifyWhatsApp)
		router.POST("/api/webhooks/whatsapp", s.limits.middleware("webhook"), s.channel.ReceiveWhatsApp)
	}

	if s.auth != nil {
		protected := v1.Group("")
		protected.Use(s.auth.Authenticate(), s.auth.RequireTenant())
		protected.GET("/me", s.handleMe)
		if s.authz != nil {
			protected.GET("/me/permissions", s.handleMePermissions)
		}
		if s.authz != nil && s.business != nil {
			biz := protected.Group("/business")
			biz.GET("", s.authz.RequirePermission(authz.PermissionBusinessRead), s.business.GetProfile)
			biz.PATCH("", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.business.UpdateProfile)
			biz.GET("/fnb-settings", s.authz.RequirePermission(authz.PermissionBusinessRead), s.business.GetFNBSettings)
			biz.PATCH("/fnb-settings", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.business.UpdateFNBSettings)
			if s.reservation != nil {
				biz.GET("/hours", s.authz.RequirePermission(authz.PermissionBusinessRead), s.reservation.GetHours)
				biz.PUT("/hours", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.reservation.ReplaceHours)
				biz.PATCH("/hours", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.reservation.ReplaceHours)
				biz.GET("/schedule-exceptions", s.authz.RequirePermission(authz.PermissionBusinessRead), s.reservation.ListExceptions)
				biz.POST("/schedule-exceptions", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.reservation.CreateException)
				biz.PATCH("/schedule-exceptions/:id", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.reservation.UpdateException)
				biz.DELETE("/schedule-exceptions/:id", s.authz.RequirePermission(authz.PermissionBusinessUpdate), s.reservation.DeleteException)
			}
		}
		if s.authz != nil && s.area != nil {
			areas := protected.Group("/areas")
			areas.GET("", s.authz.RequirePermission(authz.PermissionAreaRead), s.area.ListAreas)
			areas.POST("", s.authz.RequirePermission(authz.PermissionAreaCreate), s.area.CreateArea)
			areas.GET("/:id", s.authz.RequirePermission(authz.PermissionAreaRead), s.area.GetArea)
			areas.PATCH("/:id", s.authz.RequirePermission(authz.PermissionAreaUpdate), s.area.UpdateArea)
			areas.DELETE("/:id", s.authz.RequirePermission(authz.PermissionAreaDelete), s.area.DeleteArea)
			areas.GET("/:id/tables", s.authz.RequirePermission(authz.PermissionAreaRead), s.area.ListTables)
			areas.POST("/:id/tables", s.authz.RequirePermission(authz.PermissionAreaCreate), s.area.CreateTable)
			areas.GET("/:id/tables/:table_id", s.authz.RequirePermission(authz.PermissionAreaRead), s.area.GetTable)
			areas.PATCH("/:id/tables/:table_id", s.authz.RequirePermission(authz.PermissionAreaUpdate), s.area.UpdateTable)
			areas.DELETE("/:id/tables/:table_id", s.authz.RequirePermission(authz.PermissionAreaDelete), s.area.DeleteTable)
		}
		if s.authz != nil && s.menu != nil {
			cats := protected.Group("/menu/categories")
			cats.GET("", s.authz.RequirePermission(authz.PermissionMenuRead), s.menu.ListCategories)
			cats.POST("", s.authz.RequirePermission(authz.PermissionMenuCreate), s.menu.CreateCategory)
			cats.GET("/:id", s.authz.RequirePermission(authz.PermissionMenuRead), s.menu.GetCategory)
			cats.PATCH("/:id", s.authz.RequirePermission(authz.PermissionMenuUpdate), s.menu.UpdateCategory)
			cats.DELETE("/:id", s.authz.RequirePermission(authz.PermissionMenuDelete), s.menu.DeleteCategory)
			cats.GET("/:id/items", s.authz.RequirePermission(authz.PermissionMenuRead), s.menu.ListCategoryItems)

			items := protected.Group("/menu/items")
			items.GET("", s.authz.RequirePermission(authz.PermissionMenuRead), s.menu.ListItems)
			items.POST("", s.limits.middleware("upload"), s.authz.RequirePermission(authz.PermissionMenuCreate), s.menu.CreateItem)
			items.GET("/:id", s.authz.RequirePermission(authz.PermissionMenuRead), s.menu.GetItem)
			items.PATCH("/:id", s.limits.middleware("upload"), s.authz.RequirePermission(authz.PermissionMenuUpdate), s.menu.UpdateItem)
			items.DELETE("/:id", s.authz.RequirePermission(authz.PermissionMenuDelete), s.menu.DeleteItem)
		}
		if s.authz != nil && s.customer != nil {
			cust := protected.Group("/customers")
			cust.GET("", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.ListCustomers)
			cust.POST("", s.authz.RequirePermission(authz.PermissionCustomerCreate), s.customer.CreateCustomer)
			cust.GET("/:id", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.GetCustomer)
			cust.PATCH("/:id", s.authz.RequirePermission(authz.PermissionCustomerUpdate), s.customer.UpdateCustomer)
			cust.DELETE("/:id", s.authz.RequirePermission(authz.PermissionCustomerDelete), s.customer.DeleteCustomer)
			cust.GET("/:id/identities", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.ListIdentities)
			cust.POST("/:id/identities", s.authz.RequirePermission(authz.PermissionCustomerCreate), s.customer.CreateIdentity)
			cust.PATCH("/:id/identities/:identity_id", s.authz.RequirePermission(authz.PermissionCustomerUpdate), s.customer.UpdateIdentity)
			cust.DELETE("/:id/identities/:identity_id", s.authz.RequirePermission(authz.PermissionCustomerDelete), s.customer.DeleteIdentity)
			cust.GET("/:id/tags", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.ListCustomerTags)
			cust.POST("/:id/tags/:tag_id", s.authz.RequirePermission(authz.PermissionCustomerCreate), s.customer.AssignTag)
			cust.DELETE("/:id/tags/:tag_id", s.authz.RequirePermission(authz.PermissionCustomerDelete), s.customer.UnassignTag)

			tags := protected.Group("/tags")
			tags.GET("", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.ListTags)
			tags.POST("", s.authz.RequirePermission(authz.PermissionCustomerCreate), s.customer.CreateTag)
			tags.GET("/:id", s.authz.RequirePermission(authz.PermissionCustomerRead), s.customer.GetTag)
			tags.PATCH("/:id", s.authz.RequirePermission(authz.PermissionCustomerUpdate), s.customer.UpdateTag)
			tags.DELETE("/:id", s.authz.RequirePermission(authz.PermissionCustomerDelete), s.customer.DeleteTag)
		}
		if s.authz != nil && s.conversation != nil {
			convs := protected.Group("/conversations")
			convs.GET("", s.authz.RequirePermission(authz.PermissionConversationRead), s.conversation.ListConversations)
			convs.POST("", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.CreateConversation)
			convs.GET("/human-inbox", s.authz.RequirePermission(authz.PermissionConversationRead), s.conversation.ListHumanInbox)
			convs.GET("/:id", s.authz.RequirePermission(authz.PermissionConversationRead), s.conversation.GetConversation)
			convs.PATCH("/:id", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.UpdateConversation)
			convs.PATCH("/:id/read", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.MarkRead)
			convs.GET("/:id/messages", s.authz.RequirePermission(authz.PermissionConversationRead), s.conversation.ListMessages)
			convs.POST("/:id/messages", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.CreateMessage)
			convs.POST("/:id/handover", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.RequestHandover)
			convs.POST("/:id/takeover", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.Takeover)
			convs.POST("/:id/resolve", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.Resolve)
			convs.POST("/:id/resume-ai", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.ResumeAI)
			convs.POST("/:id/reply", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.Reply)
			convs.POST("/:id/notes", s.authz.RequirePermission(authz.PermissionConversationUpdate), s.conversation.CreateNote)
		}
		if s.authz != nil && s.reservation != nil {
			res := protected.Group("/reservations")
			res.GET("", s.authz.RequirePermission(authz.PermissionReservationRead), s.reservation.ListReservations)
			res.GET("/availability", s.authz.RequirePermission(authz.PermissionReservationRead), s.reservation.Availability)
			res.POST("", s.authz.RequirePermission(authz.PermissionReservationCreate), s.reservation.CreateReservation)
			res.GET("/:id", s.authz.RequirePermission(authz.PermissionReservationRead), s.reservation.GetReservation)
			res.PATCH("/:id", s.authz.RequirePermission(authz.PermissionReservationUpdate), s.reservation.UpdateReservation)
			res.POST("/:id/confirm", s.authz.RequirePermission(authz.PermissionReservationConfirm), s.reservation.ConfirmReservation)
			res.POST("/:id/cancel", s.authz.RequirePermission(authz.PermissionReservationCancel), s.reservation.CancelReservation)
			res.POST("/:id/complete", s.authz.RequirePermission(authz.PermissionReservationUpdate), s.reservation.CompleteReservation)
			res.PATCH("/:id/reschedule", s.authz.RequirePermission(authz.PermissionReservationUpdate), s.reservation.RescheduleReservation)
			if s.payment != nil {
				res.GET("/:id/payment", s.authz.RequirePermission(authz.PermissionPaymentRead), s.payment.GetByReservation)
				res.POST("/:id/payment", s.authz.RequirePermission(authz.PermissionPaymentUpdate), s.payment.CreateForReservation)
			}
		}
		if s.authz != nil && s.calendar != nil {
			cal := protected.Group("/calendar")
			cal.GET("/events", s.authz.RequirePermission(authz.PermissionCalendarRead), s.calendar.ListEvents)
			cal.GET("/day", s.authz.RequirePermission(authz.PermissionCalendarRead), s.calendar.Day)
			cal.POST("/blocks", s.authz.RequirePermission(authz.PermissionCalendarUpdate), s.calendar.CreateBlock)
			cal.PATCH("/blocks/:id", s.authz.RequirePermission(authz.PermissionCalendarUpdate), s.calendar.UpdateBlock)
			cal.DELETE("/blocks/:id", s.authz.RequirePermission(authz.PermissionCalendarUpdate), s.calendar.DeleteBlock)
		}
		if s.authz != nil && s.payment != nil {
			pays := protected.Group("/reservation-payments")
			pays.GET("", s.authz.RequirePermission(authz.PermissionPaymentRead), s.payment.ListPayments)
			pays.PATCH("/:id", s.authz.RequirePermission(authz.PermissionPaymentUpdate), s.payment.UpdatePayment)
			pays.POST("/:id/confirm", s.authz.RequirePermission(authz.PermissionPaymentUpdate), s.payment.ConfirmPayment)
			pays.POST("/:id/mark-full", s.authz.RequirePermission(authz.PermissionPaymentUpdate), s.payment.MarkFullPayment)
			pays.POST("/:id/refund", s.authz.RequirePermission(authz.PermissionPaymentUpdate), s.payment.RefundPayment)
		}
		if s.authz != nil && s.ai != nil {
			aiGroup := protected.Group("/ai")
			aiGroup.GET("/settings", s.authz.RequirePermission(authz.PermissionPromptRead), s.ai.GetSettings)
			aiGroup.PATCH("/settings", s.authz.RequirePermission(authz.PermissionPromptUpdate), s.ai.UpdateSettings)
			aiGroup.GET("/prompts", s.authz.RequirePermission(authz.PermissionPromptRead), s.ai.ListPrompts)
			aiGroup.POST("/prompts", s.authz.RequirePermission(authz.PermissionPromptCreate), s.ai.CreatePrompt)
			aiGroup.GET("/prompts/:id", s.authz.RequirePermission(authz.PermissionPromptRead), s.ai.GetPrompt)
			aiGroup.PATCH("/prompts/:id", s.authz.RequirePermission(authz.PermissionPromptUpdate), s.ai.UpdatePrompt)
			aiGroup.POST("/prompts/:id/activate", s.authz.RequirePermission(authz.PermissionPromptUpdate), s.ai.ActivatePrompt)
			aiGroup.DELETE("/prompts/:id", s.authz.RequirePermission(authz.PermissionPromptDelete), s.ai.DeletePrompt)
			aiGroup.POST("/generate", s.limits.middleware("ai"), s.authz.RequirePermission(authz.PermissionConversationUpdate), s.ai.Generate)
		}
		if s.authz != nil && s.channel != nil {
			ch := protected.Group("/channels")
			ch.GET("", s.authz.RequirePermission(authz.PermissionChannelRead), s.channel.ListChannels)
			ch.POST("", s.authz.RequirePermission(authz.PermissionChannelCreate), s.channel.CreateChannel)
			ch.GET("/:id", s.authz.RequirePermission(authz.PermissionChannelRead), s.channel.GetChannel)
			ch.PATCH("/:id", s.authz.RequirePermission(authz.PermissionChannelUpdate), s.channel.UpdateChannel)
			ch.DELETE("/:id", s.authz.RequirePermission(authz.PermissionChannelDelete), s.channel.DeleteChannel)
		}
		if s.authz != nil && s.knowledge != nil {
			protected.GET("/knowledge/status", s.authz.RequirePermission(authz.PermissionKnowledgeRead), s.knowledge.Status)
			protected.POST("/knowledge/ask", s.limits.middleware("ai"), s.authz.RequirePermission(authz.PermissionKnowledgeRead), s.knowledge.Ask)
			protected.POST("/knowledge/search", s.limits.middleware("ai"), s.authz.RequirePermission(authz.PermissionKnowledgeRead), s.knowledge.Search)
			kb := protected.Group("/knowledge/documents")
			kb.GET("", s.authz.RequirePermission(authz.PermissionKnowledgeRead), s.knowledge.ListDocuments)
			kb.POST("", s.limits.middleware("upload"), s.authz.RequirePermission(authz.PermissionKnowledgeCreate), s.knowledge.CreateDocument)
			kb.GET("/:id", s.authz.RequirePermission(authz.PermissionKnowledgeRead), s.knowledge.GetDocument)
			kb.PATCH("/:id", s.authz.RequirePermission(authz.PermissionKnowledgeUpdate), s.knowledge.UpdateDocument)
			kb.DELETE("/:id", s.authz.RequirePermission(authz.PermissionKnowledgeDelete), s.knowledge.DeleteDocument)
			kb.POST("/:id/reprocess", s.limits.middleware("upload"), s.authz.RequirePermission(authz.PermissionKnowledgeUpdate), s.knowledge.ReprocessDocument)
		}
		if s.authz != nil && s.automation != nil {
			autos := protected.Group("/automations")
			autos.GET("", s.authz.RequirePermission(authz.PermissionAutomationRead), s.automation.ListAutomations)
			autos.POST("", s.authz.RequirePermission(authz.PermissionAutomationCreate), s.automation.CreateAutomation)
			autos.GET("/:id", s.authz.RequirePermission(authz.PermissionAutomationRead), s.automation.GetAutomation)
			autos.PATCH("/:id", s.authz.RequirePermission(authz.PermissionAutomationUpdate), s.automation.UpdateAutomation)
			autos.DELETE("/:id", s.authz.RequirePermission(authz.PermissionAutomationDelete), s.automation.DeleteAutomation)
			autos.POST("/:id/activate", s.authz.RequirePermission(authz.PermissionAutomationUpdate), s.automation.ActivateAutomation)
			autos.POST("/:id/deactivate", s.authz.RequirePermission(authz.PermissionAutomationUpdate), s.automation.DeactivateAutomation)
			autos.GET("/:id/runs", s.authz.RequirePermission(authz.PermissionAutomationRead), s.automation.ListRuns)
		}
		if s.authz != nil && s.campaign != nil {
			camps := protected.Group("/campaigns")
			camps.POST("/audience/preview", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.PreviewAudience)
			camps.GET("", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.ListCampaigns)
			camps.POST("", s.authz.RequirePermission(authz.PermissionCampaignCreate), s.campaign.CreateCampaign)
			camps.GET("/:id", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.GetCampaign)
			camps.PATCH("/:id", s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.UpdateCampaign)
			camps.DELETE("/:id", s.authz.RequirePermission(authz.PermissionCampaignDelete), s.campaign.DeleteCampaign)
			camps.POST("/:id/schedule", s.limits.middleware("campaign"), s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.ScheduleCampaign)
			camps.POST("/:id/start", s.limits.middleware("campaign"), s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.StartCampaign)
			camps.POST("/:id/pause", s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.PauseCampaign)
			camps.POST("/:id/resume", s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.ResumeCampaign)
			camps.POST("/:id/cancel", s.authz.RequirePermission(authz.PermissionCampaignUpdate), s.campaign.CancelCampaign)
			camps.GET("/:id/runs", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.ListRuns)
			camps.GET("/:id/runs/:run_id", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.GetRun)
			camps.GET("/:id/runs/:run_id/recipients", s.authz.RequirePermission(authz.PermissionCampaignRead), s.campaign.ListRecipients)
		}
		if s.authz != nil && s.usage != nil {
			use := protected.Group("/usage")
			use.GET("", s.authz.RequirePermission(authz.PermissionUsageRead), s.usage.GetUsage)
			use.GET("/history", s.authz.RequirePermission(authz.PermissionUsageRead), s.usage.GetHistory)
		}
		if s.authz != nil && s.analytics != nil {
			an := protected.Group("/analytics")
			an.GET("/overview", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Overview)
			an.GET("/conversations", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Conversations)
			an.GET("/customers", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Customers)
			an.GET("/reservations", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Reservations)
			an.GET("/conversion", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Conversion)
			an.GET("/revenue", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Revenue)
			an.GET("/ai", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.AI)
			an.GET("/handover", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Handover)
			an.GET("/automation", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Automation)
			an.GET("/messaging", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Messaging)
			an.GET("/campaigns", s.authz.RequirePermission(authz.PermissionAnalyticsRead), s.analytics.Campaigns)
		}
	}
}

func (s *Server) handleRoot(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": s.cfg.AppName,
		"message": "DIUK backend is running",
	})
}

// handleHealth reports that the HTTP application is alive. It deliberately
// does not touch PostgreSQL or Redis.
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": s.cfg.AppName,
	})
}

func (s *Server) handleReady(c *gin.Context) {
	dbStatus := "ok"
	if s.db == nil || s.db.Ping(c.Request.Context()) != nil {
		dbStatus = "unavailable"
	}
	redisStatus := "unavailable"
	if s.redis != nil && s.redis.Ping(c.Request.Context()) == nil {
		redisStatus = "ok"
	}
	status := "ok"
	code := http.StatusOK
	if dbStatus != "ok" {
		status = "error"
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, gin.H{
		"status":   status,
		"service":  s.cfg.AppName,
		"database": dbStatus,
		"redis":    redisStatus,
	})
}

// handleDatabaseHealth reports PostgreSQL connectivity. The underlying error
// is logged but never returned to the client.
func (s *Server) handleDatabaseHealth(c *gin.Context) {
	if err := s.db.Ping(c.Request.Context()); err != nil {
		s.log.Error("database health check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "error",
			"service":  s.cfg.AppName,
			"database": "unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"service":  s.cfg.AppName,
		"database": "ok",
	})
}

func (s *Server) handleRedisHealth(c *gin.Context) {
	if s.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"service": s.cfg.AppName,
			"redis":   "unavailable",
		})
		return
	}
	if err := s.redis.Ping(c.Request.Context()); err != nil {
		s.log.Error("redis health check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"service": s.cfg.AppName,
			"redis":   "unavailable",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": s.cfg.AppName,
		"redis":   "ok",
	})
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts the
// server down gracefully so in-flight requests can finish.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info("http server listening", "addr", s.http.Addr, "env", s.cfg.AppEnv)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutdown signal received", "timeout", s.shutdownTimeout().String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout())
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	s.log.Info("http server stopped")
	return <-errCh
}

func (s *Server) shutdownTimeout() time.Duration {
	return durationOr(s.cfg.HTTPShutdownTimeout, defaultShutdownTimeout)
}

func durationOr(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func intOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
