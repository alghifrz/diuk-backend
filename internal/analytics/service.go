package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type BusinessLookup interface {
	Timezone(ctx context.Context, businessID uuid.UUID) (string, error)
}

type Service struct {
	store    Store
	business BusinessLookup
	now      func() time.Time
}

func NewService(store Store, business BusinessLookup) *Service {
	return &Service{store: store, business: business, now: time.Now}
}

func (s *Service) rangeFor(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, error) {
	tz := "UTC"
	if s.business != nil {
		if loaded, err := s.business.Timezone(ctx, businessID); err == nil && loaded != "" {
			tz = loaded
		}
	}
	return parseRange(fromRaw, toRaw, tz, s.now())
}

func (s *Service) Overview(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, Overview, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, Overview{}, err
	}
	out, err := s.store.Overview(ctx, businessID, r.From, r.To)
	return r, out, err
}

func (s *Service) Conversations(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, ConversationReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, ConversationReport{}, err
	}
	out, err := s.store.Conversations(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Customers(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, CustomerReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, CustomerReport{}, err
	}
	out, err := s.store.Customers(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Reservations(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, ReservationReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, ReservationReport{}, err
	}
	out, err := s.store.Reservations(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Conversion(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, ConversionReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, ConversionReport{}, err
	}
	out, err := s.store.Conversion(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Revenue(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, PaymentReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, PaymentReport{}, err
	}
	out, err := s.store.Revenue(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) AI(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, AIReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, AIReport{}, err
	}
	out, err := s.store.AI(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Handover(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, HandoverReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, HandoverReport{}, err
	}
	out, err := s.store.Handover(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Automation(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, AutomationReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, AutomationReport{}, err
	}
	out, err := s.store.Automation(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Messaging(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, MessagingReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, MessagingReport{}, err
	}
	out, err := s.store.Messaging(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}

func (s *Service) Campaigns(ctx context.Context, businessID uuid.UUID, fromRaw, toRaw string) (Range, CampaignReport, error) {
	r, err := s.rangeFor(ctx, businessID, fromRaw, toRaw)
	if err != nil {
		return Range{}, CampaignReport{}, err
	}
	out, err := s.store.Campaigns(ctx, businessID, r.From, r.To, r.Timezone)
	return r, out, err
}
