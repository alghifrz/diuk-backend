package ai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/menu"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeBusiness struct {
	profile  *business.Profile
	settings *business.FNBSettings
	err      error
}

func (f fakeBusiness) Profile(context.Context, uuid.UUID) (*business.Profile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}

func (f fakeBusiness) FNBSettings(context.Context, uuid.UUID) (*business.FNBSettings, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.settings, nil
}

type fakeCustomers struct {
	items []customer.Customer
	err   error
}

func (f fakeCustomers) GetCustomer(_ context.Context, businessID, customerID uuid.UUID) (*customer.Customer, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.items {
		if f.items[i].BusinessID == businessID && f.items[i].ID == customerID {
			copy := f.items[i]
			return &copy, nil
		}
	}
	return nil, customer.ErrCustomerNotFound
}

func (f fakeCustomers) ListCustomers(_ context.Context, businessID uuid.UUID, _, search, _, _ string) ([]customer.Customer, error) {
	if f.err != nil {
		return nil, f.err
	}
	term := strings.ToLower(strings.TrimSpace(search))
	out := make([]customer.Customer, 0)
	for _, item := range f.items {
		if item.BusinessID != businessID {
			continue
		}
		if term != "" &&
			!strings.Contains(strings.ToLower(item.Name), term) &&
			!strings.Contains(strings.ToLower(deref(item.Phone)), term) &&
			!strings.Contains(strings.ToLower(deref(item.Email)), term) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

type fakeMenu struct {
	categories []menu.Category
	items      []menu.Item
	err        error
}

func (f fakeMenu) ListCategories(_ context.Context, businessID uuid.UUID, _ string) ([]menu.Category, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]menu.Category, 0)
	for _, item := range f.categories {
		if item.BusinessID == businessID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f fakeMenu) ListItems(_ context.Context, businessID uuid.UUID, _ string, _ *uuid.UUID) ([]menu.Item, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]menu.Item, 0)
	for _, item := range f.items {
		if item.BusinessID == businessID {
			out = append(out, item)
		}
	}
	return out, nil
}

type fakeConversations struct {
	mu            sync.Mutex
	conversations map[uuid.UUID]*conversation.Conversation
	messages      []conversation.Message
	createErr     error
}

func newFakeConversations() *fakeConversations {
	return &fakeConversations{conversations: map[uuid.UUID]*conversation.Conversation{}}
}

func (f *fakeConversations) add(conv conversation.Conversation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := conv
	f.conversations[conv.ID] = &copy
}

func (f *fakeConversations) GetConversation(_ context.Context, businessID, conversationID uuid.UUID) (*conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.conversations[conversationID]
	if !ok || item.BusinessID != businessID {
		return nil, conversation.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeConversations) ListRecentMessages(_ context.Context, businessID, conversationID uuid.UUID, limit int) ([]conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.conversations[conversationID]; !ok {
		return nil, conversation.ErrNotFound
	}
	out := make([]conversation.Message, 0)
	for _, msg := range f.messages {
		if msg.BusinessID == businessID && msg.ConversationID == conversationID {
			out = append(out, msg)
		}
	}
	if limit < len(out) {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (f *fakeConversations) FindMessageByExternalID(_ context.Context, businessID uuid.UUID, externalID string) (*conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.messages {
		msg := f.messages[i]
		if msg.BusinessID == businessID && msg.ExternalID != nil && *msg.ExternalID == externalID {
			copy := msg
			return &copy, nil
		}
	}
	return nil, nil
}

func (f *fakeConversations) CreateMessage(_ context.Context, businessID, conversationID uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error) {
	if f.createErr != nil {
		return nil, false, f.createErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.conversations[conversationID]; !ok || f.conversations[conversationID].BusinessID != businessID {
		return nil, false, conversation.ErrNotFound
	}
	if in.ExternalID != nil {
		for i := range f.messages {
			msg := f.messages[i]
			if msg.BusinessID == businessID && msg.ExternalID != nil && *msg.ExternalID == *in.ExternalID {
				copy := msg
				return &copy, false, nil
			}
		}
	}
	msg := conversation.Message{
		ID:             uuid.New(),
		BusinessID:     businessID,
		ConversationID: conversationID,
		Direction:      conversation.Direction(in.Direction),
		SenderType:     conversation.SenderType(in.SenderType),
		MessageType:    conversation.MessageType(in.MessageType),
		Content:        in.Content,
		ExternalID:     in.ExternalID,
		Status:         conversation.MessageStatus(in.Status),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	f.messages = append(f.messages, msg)
	return &msg, true, nil
}

func (f *fakeConversations) RequestHumanHandover(_ context.Context, businessID, conversationID uuid.UUID, reasonRaw string) (*conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.conversations[conversationID]
	if !ok || item.BusinessID != businessID {
		return nil, conversation.ErrNotFound
	}
	now := time.Now().UTC()
	reason := conversation.HandoverReason(strings.TrimSpace(strings.ToUpper(reasonRaw)))
	if reason == "" {
		reason = conversation.ReasonManualHandoff
	}
	item.HandlingState = conversation.HandlingHumanRequested
	item.HandoverReason = &reason
	item.HandoverRequestedAt = &now
	copy := *item
	return &copy, nil
}

type fakeReservations struct {
	mu           sync.Mutex
	availability *reservation.AvailabilityResult
	items        []reservation.Reservation
	availCalls   int
	createCalls  int
	confirmCalls int
	cancelCalls  int
	lastAvail    [4]string
	availErr     error
	createErr    error
	confirmErr   error
	cancelErr    error
}

func (f *fakeReservations) Availability(_ context.Context, _ uuid.UUID, dateRaw, startRaw, durationRaw, partyRaw string) (*reservation.AvailabilityResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.availCalls++
	f.lastAvail = [4]string{dateRaw, startRaw, durationRaw, partyRaw}
	if f.availErr != nil {
		return nil, f.availErr
	}
	if f.availability != nil {
		copy := *f.availability
		return &copy, nil
	}
	party, _ := strconv.Atoi(partyRaw)
	return &reservation.AvailabilityResult{Date: dateRaw, PartySize: party, Available: false}, nil
}

func (f *fakeReservations) Create(_ context.Context, businessID uuid.UUID, in reservation.CreateReservationInput) (*reservation.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	start, _ := time.Parse(time.RFC3339, in.StartAt)
	end, _ := time.Parse(time.RFC3339, in.EndAt)
	custID, _ := uuid.Parse(in.CustomerID)
	item := reservation.Reservation{
		ID: uuid.New(), BusinessID: businessID, CustomerID: custID, PartySize: in.PartySize,
		StartAt: start, EndAt: end, Status: reservation.StatusPending, Notes: in.Notes,
	}
	f.items = append(f.items, item)
	return &item, nil
}

func (f *fakeReservations) Confirm(_ context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirmCalls++
	if f.confirmErr != nil {
		return nil, f.confirmErr
	}
	for i := range f.items {
		if f.items[i].BusinessID == businessID && f.items[i].ID == reservationID {
			f.items[i].Status = reservation.StatusConfirmed
			copy := f.items[i]
			return &copy, nil
		}
	}
	return nil, reservation.ErrNotFound
}

func (f *fakeReservations) Cancel(_ context.Context, businessID, reservationID uuid.UUID, _ *string) (*reservation.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls++
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	for i := range f.items {
		if f.items[i].BusinessID == businessID && f.items[i].ID == reservationID {
			f.items[i].Status = reservation.StatusCancelled
			copy := f.items[i]
			return &copy, nil
		}
	}
	return nil, reservation.ErrNotFound
}

func (f *fakeReservations) Get(_ context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items {
		if f.items[i].BusinessID == businessID && f.items[i].ID == reservationID {
			copy := f.items[i]
			return &copy, nil
		}
	}
	return nil, reservation.ErrNotFound
}

func (f *fakeReservations) List(_ context.Context, businessID uuid.UUID, _, customerRaw, _, _, _, _, _, _ string) ([]reservation.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	custID, _ := uuid.Parse(customerRaw)
	out := make([]reservation.Reservation, 0)
	for _, item := range f.items {
		if item.BusinessID == businessID && (custID == uuid.Nil || item.CustomerID == custID) {
			out = append(out, item)
		}
	}
	return out, nil
}

type scriptedLLM struct {
	mu       sync.Mutex
	steps    []provider.GenerateResponse
	errs     []error
	i        int
	requests []provider.GenerateRequest
}

func (s *scriptedLLM) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if s.i < len(s.errs) && s.errs[s.i] != nil {
		err := s.errs[s.i]
		s.i++
		return provider.GenerateResponse{}, err
	}
	if s.i >= len(s.steps) {
		return provider.GenerateResponse{Content: "done"}, nil
	}
	step := s.steps[s.i]
	s.i++
	return step, nil
}

type scriptedMeter struct {
	mu           sync.Mutex
	checkErr     error
	consumeErr   error
	checkCalls   int
	consumeCalls int
	sources      map[uuid.UUID]struct{}
}

func (m *scriptedMeter) Check(context.Context, uuid.UUID, usage.FeatureKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCalls++
	return m.checkErr
}

func (m *scriptedMeter) Consume(_ context.Context, _ uuid.UUID, in usage.ConsumeInput) (*usage.ConsumeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consumeCalls++
	if m.sources == nil {
		m.sources = map[uuid.UUID]struct{}{}
	}
	m.sources[in.SourceID] = struct{}{}
	if m.consumeErr != nil {
		return nil, m.consumeErr
	}
	return &usage.ConsumeResult{Used: int64(len(m.sources))}, nil
}

func (m *scriptedMeter) CheckResource(context.Context, uuid.UUID, usage.FeatureKey) error {
	return m.Check(context.Background(), uuid.Nil, "")
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func sampleWorld(businessID, customerID, conversationID uuid.UUID) (fakeBusiness, fakeCustomers, fakeMenu, *fakeConversations, *fakeReservations) {
	phone := "+628111"
	biz := fakeBusiness{
		profile: &business.Profile{
			ID: businessID, Name: "Kasiko", Timezone: "Asia/Jakarta", Description: ptr("Coffee shop"), Phone: &phone,
		},
		settings: &business.FNBSettings{
			BusinessID: businessID, Currency: "IDR", ReservationEnabled: true,
			ReservationSlotDurationMinutes: 90, ReservationMinPartySize: 1, ReservationMaxPartySize: 8,
			ReservationMinAdvanceMinutes: 60, ReservationMaxAdvanceDays: 30,
		},
	}
	customers := fakeCustomers{items: []customer.Customer{{
		ID: customerID, BusinessID: businessID, Name: "Budi", Phone: &phone, Email: ptr("budi@example.com"),
		Status: tenant.StatusActive, Tags: []customer.Tag{{Name: "VIP"}},
	}}}
	menus := fakeMenu{
		categories: []menu.Category{{BusinessID: businessID, Name: "Coffee", Status: tenant.StatusActive}},
		items: []menu.Item{{
			BusinessID: businessID, CategoryName: "Coffee", Name: "Latte", Price: "35000",
			Description: ptr("Hot latte"), Status: tenant.StatusActive,
		}},
	}
	convs := newFakeConversations()
	convs.add(conversation.Conversation{
		ID: conversationID, BusinessID: businessID, CustomerID: customerID, Channel: conversation.ChannelWebsite, Status: conversation.StatusOpen,
		Customer: conversation.CustomerSummary{ID: customerID, Name: "Budi", Phone: &phone},
	})
	reservations := &fakeReservations{
		availability: &reservation.AvailabilityResult{
			Date: "2026-10-01", StartAt: time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC),
			EndAt: time.Date(2026, 10, 1, 20, 30, 0, 0, time.UTC), PartySize: 4, Available: true,
			Tables: []reservation.TableSummary{{ID: uuid.New(), Name: "T1", Capacity: 4, AreaName: "Indoor"}},
		},
	}
	return biz, customers, menus, convs, reservations
}

func jsonArgs(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
