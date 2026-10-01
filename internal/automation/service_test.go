package automation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func testService(store *memoryStore, enabled bool) (*Service, *fakeDeps) {
	deps := &fakeDeps{
		publisher:    &fakePublisher{},
		delivery:     &fakeDelivery{},
		convs:        &fakeConversations{byID: map[uuid.UUID]*conversation.Conversation{}},
		customers:    &fakeCustomers{items: map[uuid.UUID]*customer.Customer{}},
		reservations: &fakeReservations{items: map[uuid.UUID]*reservation.Reservation{}},
		businesses:   &fakeBusinesses{status: tenant.StatusActive, name: "Warung"},
		ai:           &fakeAI{},
	}
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store, deps.publisher, deps.delivery, deps.convs, deps.customers, deps.reservations, deps.businesses, deps.ai, Settings{
		Enabled: enabled, SchedulerInterval: time.Second, MaxRunsPerTick: 20, DefaultInactiveMin: 120,
	})
	return svc, deps
}

func TestCRUDActivateArchiveAndValidation(t *testing.T) {
	store := newMemoryStore()
	svc, _ := testService(store, true)
	biz := uuid.New()
	actor := uuid.New()
	item, err := svc.Create(context.Background(), biz, actor, CreateAutomationInput{
		Name:         "Welcome",
		TriggerType:  TriggerCustomerCreated,
		ActionType:   ActionSendMessage,
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if err != nil || item.Status != StatusInactive {
		t.Fatalf("create = %+v err=%v", item, err)
	}
	if _, err := svc.Get(context.Background(), uuid.New(), item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant get = %v", err)
	}
	active, err := svc.Activate(context.Background(), biz, item.ID, actor)
	if err != nil || active.Status != StatusActive {
		t.Fatalf("activate = %+v err=%v", active, err)
	}
	inactive, err := svc.Deactivate(context.Background(), biz, item.ID, actor)
	if err != nil || inactive.Status != StatusInactive {
		t.Fatalf("deactivate = %+v err=%v", inactive, err)
	}
	archived, err := svc.Archive(context.Background(), biz, item.ID, actor)
	if err != nil || archived.Status != StatusArchived {
		t.Fatalf("archive = %+v err=%v", archived, err)
	}
	if _, err := svc.Create(context.Background(), biz, actor, CreateAutomationInput{Name: "", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid create = %v", err)
	}
}

func TestPublishEvaluateAndIdempotentRuns(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	custID := uuid.New()
	auto, err := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Welcome", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	event := EventFromCustomer(&customer.Customer{ID: custID, BusinessID: biz, Status: tenant.StatusActive})
	if err := svc.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if deps.publisher.evaluate.Load() != 1 {
		t.Fatalf("evaluate jobs = %d", deps.publisher.evaluate.Load())
	}
	if err := svc.EvaluateEvent(context.Background(), biz, event.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.EvaluateEvent(context.Background(), biz, event.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := svc.ListRuns(context.Background(), biz, auto.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %+v err=%v", runs, err)
	}
	if deps.publisher.runs.Load() != 1 {
		t.Fatalf("run jobs = %d", deps.publisher.runs.Load())
	}
}

func TestDisabledDoesNotExecute(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, false)
	biz := uuid.New()
	event := EventFromCustomer(&customer.Customer{ID: uuid.New(), BusinessID: biz, Status: tenant.StatusActive})
	if err := svc.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if deps.publisher.evaluate.Load() != 0 {
		t.Fatal("disabled publish must not enqueue")
	}
}

func TestExecuteSendMessageUsesDeliveryNotProvider(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	custID := uuid.New()
	convID := uuid.New()
	identityID := uuid.New()
	deps.customers.items[custID] = &customer.Customer{
		ID: custID, BusinessID: biz, Name: "Budi", Status: tenant.StatusActive,
		Identities: []customer.Identity{{ID: identityID, CustomerID: custID, Channel: customer.ChannelWhatsApp, Status: tenant.StatusActive}},
	}
	deps.convs.latest = &conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: custID, Channel: conversation.ChannelWhatsApp}
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Welcome", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	event := EventFromCustomer(&customer.Customer{ID: custID, BusinessID: biz, Status: tenant.StatusActive})
	_ = svc.Publish(context.Background(), event)
	_ = svc.EvaluateEvent(context.Background(), biz, event.ID)
	runs, _ := svc.ListRuns(context.Background(), biz, auto.ID)
	if err := svc.ExecuteRun(context.Background(), biz, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	if deps.delivery.calls.Load() != 1 {
		t.Fatalf("delivery = %d", deps.delivery.calls.Load())
	}
	if deps.convs.lastSender != string(conversation.SenderSystem) {
		t.Fatalf("sender = %s", deps.convs.lastSender)
	}
}

func TestExecuteAIUsesExistingOrchestrator(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	convID := uuid.New()
	custID := uuid.New()
	deps.convs.byID[convID] = &conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: custID, Status: conversation.StatusOpen, Channel: conversation.ChannelWhatsApp}
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Follow", TriggerType: TriggerConversationCreated, ActionType: ActionAIGenerateMessage,
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	event := EventFromConversation(&conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: custID, Status: conversation.StatusOpen, Channel: conversation.ChannelWhatsApp})
	_ = svc.Publish(context.Background(), event)
	_ = svc.EvaluateEvent(context.Background(), biz, event.ID)
	runs, _ := svc.ListRuns(context.Background(), biz, auto.ID)
	if err := svc.ExecuteRun(context.Background(), biz, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	if deps.ai.calls.Load() != 1 {
		t.Fatalf("ai calls = %d", deps.ai.calls.Load())
	}
	if !deps.ai.last.DisableMutatingTools || deps.ai.last.PersistIncoming {
		t.Fatalf("ai input = %+v", deps.ai.last)
	}
}

func TestExecuteAISkipsWhenHumanHandling(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	convID := uuid.New()
	custID := uuid.New()
	deps.convs.byID[convID] = &conversation.Conversation{
		ID: convID, BusinessID: biz, CustomerID: custID, Status: conversation.StatusOpen,
		Channel: conversation.ChannelWhatsApp, HandlingState: conversation.HandlingHumanActive,
	}
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Follow", TriggerType: TriggerConversationCreated, ActionType: ActionAIGenerateMessage,
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	event := EventFromConversation(&conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: custID, Status: conversation.StatusOpen, Channel: conversation.ChannelWhatsApp})
	_ = svc.Publish(context.Background(), event)
	_ = svc.EvaluateEvent(context.Background(), biz, event.ID)
	runs, _ := svc.ListRuns(context.Background(), biz, auto.ID)
	if err := svc.ExecuteRun(context.Background(), biz, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	if deps.ai.calls.Load() != 0 {
		t.Fatalf("AI generate must skip while human handling, calls=%d", deps.ai.calls.Load())
	}
}

func TestCancelledReservationSkipsReminder(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	resID := uuid.New()
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Reminder", TriggerType: TriggerScheduled,
		TriggerConfig: json.RawMessage(`{"offset_minutes":-1440,"reference":"reservation.start_at"}`),
		Conditions:    json.RawMessage(`[{"field":"reservation.status","operator":"EQ","value":"CONFIRMED"}]`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	key := ScheduledReservationKey(resID, -1440)
	run, _, err := store.InsertRun(context.Background(), Run{
		BusinessID: biz, AutomationID: auto.ID, ExecutionKey: &key, SubjectType: SubjectReservation, SubjectID: resID, Status: RunPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.reservations.items[resID] = &reservation.Reservation{ID: resID, BusinessID: biz, Status: reservation.StatusCancelled, PartySize: 2, StartAt: time.Now().Add(time.Hour)}
	if err := svc.ExecuteRun(context.Background(), biz, run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRun(context.Background(), biz, run.ID)
	if got.Status != RunCancelled {
		t.Fatalf("status = %s", got.Status)
	}
	if deps.delivery.calls.Load() != 0 {
		t.Fatal("must not send after cancel")
	}
}

func TestInactiveConversationRecheck(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	convID := uuid.New()
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Nudge", TriggerType: TriggerConversationInactive,
		TriggerConfig: json.RawMessage(`{"inactive_minutes":120}`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	deps.convs.byID[convID] = &conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: uuid.New(), Status: conversation.StatusOpen, Channel: conversation.ChannelWhatsApp}
	store.lastInbound[convID] = time.Now().UTC()
	key := ScheduledInactiveKey(convID, time.Now().Add(-3*time.Hour))
	run, _, _ := store.InsertRun(context.Background(), Run{
		BusinessID: biz, AutomationID: auto.ID, ExecutionKey: &key, SubjectType: SubjectConversation, SubjectID: convID, Status: RunPending,
	})
	if err := svc.ExecuteRun(context.Background(), biz, run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRun(context.Background(), biz, run.ID)
	if got.Status != RunCancelled {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestConcurrentClaimProducesOneMessage(t *testing.T) {
	store := newMemoryStore()
	svc, deps := testService(store, true)
	biz := uuid.New()
	custID := uuid.New()
	convID := uuid.New()
	deps.customers.items[custID] = &customer.Customer{
		ID: custID, BusinessID: biz, Name: "Budi", Status: tenant.StatusActive,
		Identities: []customer.Identity{{ID: uuid.New(), CustomerID: custID, Channel: customer.ChannelWhatsApp, Status: tenant.StatusActive}},
	}
	deps.convs.latest = &conversation.Conversation{ID: convID, BusinessID: biz, CustomerID: custID}
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Welcome", TriggerType: TriggerCustomerCreated, ActionType: ActionSendMessage,
		ActionConfig: json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	event := EventFromCustomer(&customer.Customer{ID: custID, BusinessID: biz, Status: tenant.StatusActive})
	_ = svc.Publish(context.Background(), event)
	_ = svc.EvaluateEvent(context.Background(), biz, event.ID)
	runs, _ := svc.ListRuns(context.Background(), biz, auto.ID)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = svc.ExecuteRun(context.Background(), biz, runs[0].ID) }()
	go func() { defer wg.Done(); _ = svc.ExecuteRun(context.Background(), biz, runs[0].ID) }()
	wg.Wait()
	if deps.delivery.calls.Load() != 1 {
		t.Fatalf("delivery = %d", deps.delivery.calls.Load())
	}
}

func TestSchedulerReservationAndInactive(t *testing.T) {
	store := newMemoryStore()
	svc, _ := testService(store, true)
	biz := uuid.New()
	due := uuid.New()
	future := uuid.New()
	auto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Reminder", TriggerType: TriggerScheduled,
		TriggerConfig: json.RawMessage(`{"offset_minutes":-60,"reference":"reservation.start_at"}`),
		Conditions:    json.RawMessage(`[{"field":"reservation.status","operator":"EQ","value":"CONFIRMED"}]`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, auto.ID, uuid.New())
	now := time.Now().UTC()
	store.reservations = []ReservationCandidate{
		{ID: due, BusinessID: biz, CustomerID: uuid.New(), Status: "CONFIRMED", PartySize: 2, StartAt: now.Add(30 * time.Minute)},
		{ID: future, BusinessID: biz, CustomerID: uuid.New(), Status: "CONFIRMED", PartySize: 2, StartAt: now.Add(3 * time.Hour)},
		{ID: uuid.New(), BusinessID: biz, CustomerID: uuid.New(), Status: "CANCELLED", PartySize: 2, StartAt: now.Add(30 * time.Minute)},
	}
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, _ := svc.ListRuns(context.Background(), biz, auto.ID)
	if len(runs) != 1 || runs[0].SubjectID != due {
		t.Fatalf("runs = %+v", runs)
	}

	inactiveAuto, _ := svc.Create(context.Background(), biz, uuid.New(), CreateAutomationInput{
		Name: "Nudge", TriggerType: TriggerConversationInactive,
		TriggerConfig: json.RawMessage(`{"inactive_minutes":120}`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	_, _ = svc.Activate(context.Background(), biz, inactiveAuto.ID, uuid.New())
	oldConv := uuid.New()
	freshConv := uuid.New()
	store.conversations = []ConversationCandidate{
		{ID: oldConv, BusinessID: biz, CustomerID: uuid.New(), Status: "OPEN", Channel: "WHATSAPP", LastInboundAt: now.Add(-3 * time.Hour)},
		{ID: freshConv, BusinessID: biz, CustomerID: uuid.New(), Status: "OPEN", Channel: "WHATSAPP", LastInboundAt: now.Add(-10 * time.Minute)},
	}
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	inactiveRuns, _ := svc.ListRuns(context.Background(), biz, inactiveAuto.ID)
	if len(inactiveRuns) != 1 || inactiveRuns[0].SubjectID != oldConv {
		t.Fatalf("inactive runs = %+v", inactiveRuns)
	}
}

func TestEventMappingAndKeys(t *testing.T) {
	res := &reservation.Reservation{ID: uuid.New(), BusinessID: uuid.New(), CustomerID: uuid.New(), Status: reservation.StatusConfirmed, PartySize: 3, StartAt: time.Now()}
	ev := EventFromReservation(res, EventReservationConfirmed)
	if ev.Type != EventReservationConfirmed || ev.SubjectType != SubjectReservation {
		t.Fatalf("reservation event = %+v", ev)
	}
	if ScheduledReservationKey(res.ID, -1440) == "" || ScheduledInactiveKey(uuid.New(), time.Now()) == "" {
		t.Fatal("keys")
	}
	if !IsInactiveDue(time.Now().Add(-3*time.Hour), 120, time.Now()) {
		t.Fatal("inactive due")
	}
	if IsInactiveDue(time.Now().Add(-10*time.Minute), 120, time.Now()) {
		t.Fatal("not yet inactive")
	}
}

type fakeDeps struct {
	publisher    *fakePublisher
	delivery     *fakeDelivery
	convs        *fakeConversations
	customers    *fakeCustomers
	reservations *fakeReservations
	businesses   *fakeBusinesses
	ai           *fakeAI
}

type fakePublisher struct {
	evaluate atomic.Int32
	runs     atomic.Int32
}

func (f *fakePublisher) EnqueueEvaluate(context.Context, uuid.UUID, uuid.UUID) error {
	f.evaluate.Add(1)
	return nil
}
func (f *fakePublisher) EnqueueRun(context.Context, uuid.UUID, uuid.UUID) error {
	f.runs.Add(1)
	return nil
}

type fakeDelivery struct{ calls atomic.Int32 }

func (f *fakeDelivery) EnqueueDelivery(context.Context, uuid.UUID, uuid.UUID) error {
	f.calls.Add(1)
	return nil
}

type fakeConversations struct {
	byID       map[uuid.UUID]*conversation.Conversation
	latest     *conversation.Conversation
	lastSender string
}

func (f *fakeConversations) GetConversation(_ context.Context, businessID, conversationID uuid.UUID) (*conversation.Conversation, error) {
	if f.byID == nil {
		f.byID = map[uuid.UUID]*conversation.Conversation{}
	}
	item := f.byID[conversationID]
	if item == nil || item.BusinessID != businessID {
		return nil, conversation.ErrNotFound
	}
	return item, nil
}
func (f *fakeConversations) LatestConversation(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID) (*conversation.Conversation, error) {
	return f.latest, nil
}
func (f *fakeConversations) CreateConversation(_ context.Context, businessID uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error) {
	conv := &conversation.Conversation{ID: uuid.New(), BusinessID: businessID, Channel: conversation.Channel(in.Channel)}
	return conv, true, nil
}
func (f *fakeConversations) CreateMessage(_ context.Context, _, _ uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error) {
	f.lastSender = in.SenderType
	return &conversation.Message{ID: uuid.New()}, true, nil
}

type fakeCustomers struct {
	items map[uuid.UUID]*customer.Customer
}

func (f *fakeCustomers) GetCustomer(_ context.Context, businessID, customerID uuid.UUID) (*customer.Customer, error) {
	if f.items == nil {
		f.items = map[uuid.UUID]*customer.Customer{}
	}
	item := f.items[customerID]
	if item == nil || item.BusinessID != businessID {
		return nil, customer.ErrCustomerNotFound
	}
	return item, nil
}

type fakeReservations struct {
	items map[uuid.UUID]*reservation.Reservation
}

func (f *fakeReservations) Get(_ context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error) {
	if f.items == nil {
		f.items = map[uuid.UUID]*reservation.Reservation{}
	}
	item := f.items[reservationID]
	if item == nil || item.BusinessID != businessID {
		return nil, reservation.ErrNotFound
	}
	return item, nil
}

type fakeBusinesses struct {
	status tenant.Status
	name   string
}

func (f *fakeBusinesses) Profile(_ context.Context, businessID uuid.UUID) (*business.Profile, error) {
	return &business.Profile{ID: businessID, Name: f.name, Status: f.status, Timezone: "UTC"}, nil
}

type fakeAI struct {
	calls atomic.Int32
	last  ai.GenerateReplyInput
}

func (f *fakeAI) GenerateReply(_ context.Context, _ uuid.UUID, in ai.GenerateReplyInput) (*ai.GenerateReplyResult, error) {
	f.calls.Add(1)
	f.last = in
	return &ai.GenerateReplyResult{Reply: "ok", MessageID: uuid.New()}, nil
}
