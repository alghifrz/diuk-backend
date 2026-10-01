package ai

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/ai/tools"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

type Defaults struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
}

type Service struct {
	log           *slog.Logger
	store         Store
	defaults      Defaults
	llm           provider.LLMProvider
	tools         *tools.Registry
	business      BusinessLookup
	customers     CustomerLookup
	menu          MenuLookup
	conversations ConversationAPI
	reservations  ReservationAPI
	knowledge     tools.KnowledgeSearch
	onQueued      func(ctx context.Context, businessID, messageID uuid.UUID) error
	metrics       AIMetrics
	meter         usage.Meter
}

type AIMetrics interface {
	ObserveAI(provider, result, category string, duration time.Duration)
}

func NewService(
	log *slog.Logger,
	store Store,
	defaults Defaults,
	llm provider.LLMProvider,
	registry *tools.Registry,
	business BusinessLookup,
	customers CustomerLookup,
	menu MenuLookup,
	conversations ConversationAPI,
	reservations ReservationAPI,
	knowledge tools.KnowledgeSearch,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if defaults.Timeout <= 0 {
		defaults.Timeout = 30 * time.Second
	}
	if registry == nil {
		registry = DefaultRegistry(business, customers, menu, reservations, knowledge, conversations)
	}
	return &Service{
		log:           log,
		store:         store,
		defaults:      defaults,
		llm:           llm,
		tools:         registry,
		business:      business,
		customers:     customers,
		menu:          menu,
		conversations: conversations,
		reservations:  reservations,
		knowledge:     knowledge,
	}
}

func DefaultRegistry(business BusinessLookup, customers CustomerLookup, menu MenuLookup, reservations ReservationAPI, knowledge tools.KnowledgeSearch, handover tools.HandoverAPI) *tools.Registry {
	registry := tools.NewRegistry()
	quotes := tools.NewMemoryQuoteStore()
	registry.SetQuotes(quotes)
	businessInfo := tools.BusinessInfo{Business: business}
	if hours, ok := reservations.(tools.HoursLookup); ok {
		businessInfo.Hours = hours
	}
	_ = registry.Register(businessInfo)
	_ = registry.Register(tools.Menu{Menu: menu})
	_ = registry.Register(tools.SearchCustomer{Customers: customers})
	_ = registry.Register(tools.CustomerReservations{Reservations: reservations})
	_ = registry.Register(tools.CheckAvailability{Reservations: reservations})
	_ = registry.Register(tools.QuoteReservation{Reservations: reservations, Business: business, Menu: menu, Quotes: quotes})
	_ = registry.Register(tools.CreateReservation{Reservations: reservations, Handover: handover, Business: business, Quotes: quotes})
	_ = registry.Register(tools.ConfirmReservation{Reservations: reservations})
	_ = registry.Register(tools.CancelReservation{Reservations: reservations})
	_ = registry.Register(tools.RequestHumanHandover{Handover: handover})
	_ = registry.Register(tools.SearchKnowledge{Search: knowledge})
	return registry
}

func (s *Service) SetOnQueuedOutbound(hook func(ctx context.Context, businessID, messageID uuid.UUID) error) {
	s.onQueued = hook
}

func (s *Service) SetMetrics(metrics AIMetrics) {
	s.metrics = metrics
}

func (s *Service) SetUsageMeter(meter usage.Meter) {
	s.meter = meter
}

func (s *Service) GenerateReply(ctx context.Context, businessID uuid.UUID, in GenerateReplyInput) (*GenerateReplyResult, error) {
	started := time.Now()
	out, err := s.generate(ctx, businessID, in)
	if s.metrics != nil && !errors.Is(err, ErrHumanHandling) && !errors.Is(err, ErrDisabled) && !errors.Is(err, ErrConversationNotFound) && !errors.Is(err, ErrQuotaExceeded) {
		result, category := "success", "success"
		if err != nil {
			result = "failure"
			category = generationCategory(err)
		}
		providerName := s.defaults.Provider
		if out != nil && out.Provider != "" {
			providerName = out.Provider
		}
		s.metrics.ObserveAI(providerName, result, category, time.Since(started))
	}
	return out, err
}
