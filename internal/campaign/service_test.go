package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
)

func TestCampaignTransitions(t *testing.T) {
	valid := [][2]string{
		{StatusDraft, StatusScheduled}, {StatusDraft, StatusCancelled}, {StatusDraft, StatusRunning},
		{StatusScheduled, StatusRunning}, {StatusScheduled, StatusPaused}, {StatusScheduled, StatusCancelled},
		{StatusRunning, StatusPaused}, {StatusRunning, StatusCompleted}, {StatusRunning, StatusCancelled},
		{StatusPaused, StatusRunning}, {StatusPaused, StatusCancelled},
	}
	for _, pair := range valid {
		if !canTransition(pair[0], pair[1]) {
			t.Fatalf("expected %s -> %s", pair[0], pair[1])
		}
	}
	invalid := [][2]string{
		{StatusCompleted, StatusRunning}, {StatusCompleted, StatusPaused},
		{StatusCancelled, StatusRunning}, {StatusCancelled, StatusScheduled},
		{StatusPaused, StatusScheduled}, {StatusDraft, StatusPaused},
	}
	for _, pair := range invalid {
		if canTransition(pair[0], pair[1]) {
			t.Fatalf("did not expect %s -> %s", pair[0], pair[1])
		}
	}
}

func TestCampaignCRUDAndActions(t *testing.T) {
	svc, store, pub := testService(t)
	biz, user := uuid.New(), uuid.New()
	tpl, lang := "promo", "id"
	created, err := svc.Create(context.Background(), biz, user, CreateCampaignInput{
		Name: "Weekend", Channel: "WHATSAPP", MessageKind: "TEMPLATE", TemplateName: &tpl, TemplateLanguage: &lang,
		Audience: json.RawMessage(`{"customer_status":"ACTIVE"}`),
	})
	if err != nil || created.Status != StatusDraft {
		t.Fatalf("create = %+v err=%v", created, err)
	}
	if _, err := svc.Create(context.Background(), biz, user, CreateCampaignInput{Name: " ", MessageKind: "TEMPLATE"}); err == nil {
		t.Fatal("empty name")
	}
	if _, err := svc.Create(context.Background(), biz, user, CreateCampaignInput{
		Name: "X", MessageKind: "TEMPLATE", Audience: json.RawMessage(`{"unknown":"x"}`),
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown audience = %v", err)
	}

	updated, err := svc.Update(context.Background(), biz, created.ID, user, UpdateCampaignInput{Name: ptr("Weekend Promo")})
	if err != nil || updated.Name != "Weekend Promo" {
		t.Fatalf("update = %+v err=%v", updated, err)
	}

	when := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	scheduled, err := svc.Schedule(context.Background(), biz, created.ID, user, when)
	if err != nil || scheduled.Status != StatusScheduled {
		t.Fatalf("schedule = %+v err=%v", scheduled, err)
	}
	again, err := svc.Schedule(context.Background(), biz, created.ID, user, time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339))
	if err != nil || again.Status != StatusScheduled {
		t.Fatalf("reschedule = %+v err=%v", again, err)
	}
	if _, err := svc.Schedule(context.Background(), biz, created.ID, user, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past schedule = %v", err)
	}

	started, err := svc.Start(context.Background(), biz, created.ID, user)
	if err != nil || started.Status != StatusRunning {
		t.Fatalf("start = %+v err=%v", started, err)
	}
	if len(pub.starts) != 1 {
		t.Fatalf("start jobs = %d", len(pub.starts))
	}
	startedAgain, err := svc.Start(context.Background(), biz, created.ID, user)
	if err != nil || startedAgain.Status != StatusRunning {
		t.Fatalf("idempotent start = %+v err=%v", startedAgain, err)
	}

	paused, err := svc.Pause(context.Background(), biz, created.ID, user)
	if err != nil || paused.Status != StatusPaused {
		t.Fatalf("pause = %+v err=%v", paused, err)
	}
	pausedAgain, err := svc.Pause(context.Background(), biz, created.ID, user)
	if err != nil || pausedAgain.Status != StatusPaused {
		t.Fatalf("idempotent pause = %+v err=%v", pausedAgain, err)
	}

	resumed, err := svc.Resume(context.Background(), biz, created.ID, user)
	if err != nil || resumed.Status != StatusRunning {
		t.Fatalf("resume = %+v err=%v", resumed, err)
	}

	cancelled, err := svc.Cancel(context.Background(), biz, created.ID, user)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	cancelledAgain, err := svc.Cancel(context.Background(), biz, created.ID, user)
	if err != nil || cancelledAgain.Status != StatusCancelled {
		t.Fatalf("idempotent cancel = %+v err=%v", cancelledAgain, err)
	}

	other := uuid.New()
	if _, err := svc.Get(context.Background(), other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant get = %v", err)
	}
	if _, err := svc.Update(context.Background(), other, created.ID, user, UpdateCampaignInput{Name: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant update = %v", err)
	}

	completed := *created
	completed.Status = StatusCompleted
	store.campaigns[created.ID] = &completed
	if _, err := svc.Start(context.Background(), biz, created.ID, user); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("start completed = %v", err)
	}
	if _, err := svc.Cancel(context.Background(), biz, created.ID, user); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancel completed = %v", err)
	}
}

func TestAudiencePreviewAndSnapshotIdempotency(t *testing.T) {
	svc, store, _ := testService(t)
	a, b := uuid.New(), uuid.New()
	active, vip := "ACTIVE", "regular"
	_ = seedCustomer(store, a, "Active", "ACTIVE", true, []string{vip}, []string{ChannelWhatsApp})
	optOut := seedCustomer(store, a, "Opted", "ACTIVE", false, []string{vip}, []string{ChannelWhatsApp})
	_ = seedCustomer(store, a, "Inactive", "INACTIVE", true, nil, []string{ChannelWhatsApp})
	_ = seedCustomer(store, b, "Other", "ACTIVE", true, []string{vip}, []string{ChannelWhatsApp})

	preview, err := svc.Preview(context.Background(), a, ChannelWhatsApp, json.RawMessage(`{"customer_status":"ACTIVE","tags":["regular"]}`))
	if err != nil || preview.EstimatedCount != 2 {
		t.Fatalf("preview = %+v err=%v", preview, err)
	}
	otherPreview, err := svc.Preview(context.Background(), b, ChannelWhatsApp, json.RawMessage(`{"customer_status":"ACTIVE"}`))
	if err != nil || otherPreview.EstimatedCount != 1 {
		t.Fatalf("other preview = %+v err=%v", otherPreview, err)
	}

	c := mustCreate(t, svc, a)
	run, _, err := store.CreateRun(context.Background(), Run{BusinessID: a, CampaignID: c.ID, Status: RunPending})
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.SnapshotRecipients(context.Background(), a, run.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &active, Tags: []string{vip}})
	if err != nil || n != 2 {
		t.Fatalf("snapshot = %d err=%v", n, err)
	}
	n, err = store.SnapshotRecipients(context.Background(), a, run.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &active, Tags: []string{vip}})
	if err != nil || n != 0 {
		t.Fatalf("retry snapshot = %d err=%v", n, err)
	}
	recs, err := store.ListRecipients(context.Background(), a, run.ID, "", 50, 0)
	if err != nil || len(recs) != 2 {
		t.Fatalf("recipients = %d err=%v", len(recs), err)
	}
	skipped := 0
	for _, rec := range recs {
		if rec.Status == RecipientSkipped && rec.FailureCode != nil && *rec.FailureCode == SkipMarketingOptOut {
			skipped++
		}
		if rec.CustomerID == optOut && rec.Status != RecipientSkipped {
			t.Fatal("opt-out must be skipped at snapshot")
		}
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d", skipped)
	}

	if err := svc.StartRun(context.Background(), a, run.ID); err != nil {
		t.Fatal(err)
	}
	later := seedCustomer(store, a, "Later", "ACTIVE", true, []string{vip}, []string{ChannelWhatsApp})
	if err := svc.StartRun(context.Background(), a, run.ID); err != nil {
		t.Fatal(err)
	}
	recs, _ = store.ListRecipients(context.Background(), a, run.ID, "", 50, 0)
	for _, rec := range recs {
		if rec.CustomerID == later {
			t.Fatal("CRM change must not alter existing snapshot")
		}
	}
}

func TestDispatchClaimOptOutPauseCancelAndDelivery(t *testing.T) {
	svc, store, _ := testService(t)
	biz := uuid.New()
	cust := seedCustomer(store, biz, "Pat", "ACTIVE", true, nil, []string{ChannelWhatsApp})
	c := mustCreate(t, svc, biz)
	run, _, _ := store.CreateRun(context.Background(), Run{BusinessID: biz, CampaignID: c.ID, Status: RunRunning})
	_, _ = store.SnapshotRecipients(context.Background(), biz, run.ID, ChannelWhatsApp, AudienceDefinition{})
	claimed, err := store.ClaimPendingRecipients(context.Background(), biz, run.ID, 10)
	if err != nil || len(claimed) != 1 || claimed[0].Status != RecipientQueued {
		t.Fatalf("claim = %+v err=%v", claimed, err)
	}
	again, err := store.ClaimPendingRecipients(context.Background(), biz, run.ID, 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("duplicate claim = %+v err=%v", again, err)
	}

	if err := svc.DispatchRun(context.Background(), biz, run.ID); err != nil {
		t.Fatal(err)
	}
	rec, _ := store.GetRecipient(context.Background(), biz, claimed[0].ID)
	if rec.MessageID == nil || rec.Status != RecipientQueued {
		t.Fatalf("after dispatch = %+v", rec)
	}
	if err := svc.DispatchRun(context.Background(), biz, run.ID); err != nil {
		t.Fatal(err)
	}
	rec2, _ := store.GetRecipient(context.Background(), biz, claimed[0].ID)
	if rec2.MessageID == nil || *rec2.MessageID != *rec.MessageID {
		t.Fatal("retry must not create a second campaign message when already linked")
	}

	if err := svc.OnMessageDelivery(context.Background(), biz, *rec.MessageID, "SENT"); err != nil {
		t.Fatal(err)
	}
	sent, _ := store.GetRecipient(context.Background(), biz, rec.ID)
	if sent.Status != RecipientSent {
		t.Fatalf("sent = %s", sent.Status)
	}

	opt := seedCustomer(store, biz, "Out", "ACTIVE", false, []string{"optout-only"}, []string{ChannelWhatsApp})
	c2 := mustCreate(t, svc, biz)
	run2, _, _ := store.CreateRun(context.Background(), Run{BusinessID: biz, CampaignID: c2.ID, Status: RunRunning})
	_, _ = store.SnapshotRecipients(context.Background(), biz, run2.ID, ChannelWhatsApp, AudienceDefinition{Tags: []string{"optout-only"}})
	pending, _ := store.ListRecipients(context.Background(), biz, run2.ID, RecipientPending, 10, 0)
	if len(pending) != 0 {
		t.Fatal("opt-out snapshot should skip")
	}
	_ = opt
	_ = cust

	c3 := mustCreate(t, svc, biz)
	_ = seedCustomer(store, biz, "Hold", "ACTIVE", true, nil, []string{ChannelWhatsApp})
	started, _ := svc.Start(context.Background(), biz, c3.ID, uuid.New())
	if started.Status != StatusRunning {
		t.Fatal(started.Status)
	}
	if err := svc.StartRun(context.Background(), biz, mustActiveRun(t, store, biz, c3.ID)); err != nil {
		t.Fatal(err)
	}
	paused, err := svc.Pause(context.Background(), biz, c3.ID, uuid.New())
	if err != nil || paused.Status != StatusPaused {
		t.Fatalf("pause = %+v err=%v", paused, err)
	}
	if err := svc.DispatchRun(context.Background(), biz, mustActiveRun(t, store, biz, c3.ID)); err != nil {
		t.Fatal(err)
	}
	pendingAfterPause, _ := store.PendingRecipientCount(context.Background(), biz, mustActiveRun(t, store, biz, c3.ID))
	if pendingAfterPause == 0 {
		t.Fatal("pause must leave pending recipients pending")
	}

	c4 := mustCreate(t, svc, biz)
	_ = seedCustomer(store, biz, "SkipMe", "ACTIVE", true, nil, []string{ChannelWhatsApp})
	_, _ = svc.Start(context.Background(), biz, c4.ID, uuid.New())
	_ = svc.StartRun(context.Background(), biz, mustActiveRun(t, store, biz, c4.ID))
	if _, err := svc.Cancel(context.Background(), biz, c4.ID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	recs, _ := store.ListRecipients(context.Background(), biz, mustRun(t, store, biz, c4.ID), RecipientSkipped, 20, 0)
	foundCancel := false
	for _, rec := range recs {
		if rec.FailureCode != nil && *rec.FailureCode == SkipCampaignCancelled {
			foundCancel = true
			break
		}
	}
	if !foundCancel {
		t.Fatalf("cancel skip = %+v", recs)
	}
}

func TestSchedulerStartsOnce(t *testing.T) {
	svc, store, pub := testService(t)
	biz := uuid.New()
	c := mustCreate(t, svc, biz)
	past := time.Now().UTC().Add(-time.Minute)
	c.Status = StatusScheduled
	c.ScheduledAt = &past
	store.campaigns[c.ID] = c
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, _ := store.ListRuns(context.Background(), biz, c.ID, 10, 0)
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	if len(pub.starts) < 1 {
		t.Fatal("expected campaign start job")
	}
}

func TestDispatchRechecksOptOut(t *testing.T) {
	svc, store, _ := testService(t)
	biz := uuid.New()
	cust := seedCustomer(store, biz, "Flip", "ACTIVE", true, nil, []string{ChannelWhatsApp})
	c := mustCreate(t, svc, biz)
	run, _, _ := store.CreateRun(context.Background(), Run{BusinessID: biz, CampaignID: c.ID, Status: RunRunning})
	_, _ = store.SnapshotRecipients(context.Background(), biz, run.ID, ChannelWhatsApp, AudienceDefinition{})
	for i := range store.customers {
		if store.customers[i].ID == cust {
			store.customers[i].MarketingOptIn = false
		}
	}
	if err := svc.DispatchRun(context.Background(), biz, run.ID); err != nil {
		t.Fatal(err)
	}
	recs, _ := store.ListRecipients(context.Background(), biz, run.ID, "", 10, 0)
	if len(recs) != 1 || recs[0].Status != RecipientSkipped || recs[0].FailureCode == nil || *recs[0].FailureCode != SkipMarketingOptOut {
		t.Fatalf("late opt-out = %+v", recs)
	}
}

func TestAudienceCombinedFilters(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	now := time.Now().UTC()
	convAt := now.Add(-24 * time.Hour)
	resAt := now.Add(-48 * time.Hour)
	status := "COMPLETED"
	match := seedCustomer(store, biz, "Match", "ACTIVE", true, []string{"regular"}, []string{ChannelWhatsApp})
	for i := range store.customers {
		if store.customers[i].ID == match {
			store.customers[i].CreatedAt = now.Add(-72 * time.Hour)
			store.customers[i].LastConversationAt = &convAt
			store.customers[i].LastReservationAt = &resAt
			store.customers[i].LastReservationStatus = &status
		}
	}
	_ = seedCustomer(store, biz, "NoTag", "ACTIVE", true, nil, []string{ChannelWhatsApp})
	preview, err := store.PreviewAudience(context.Background(), biz, ChannelWhatsApp, AudienceDefinition{
		CustomerStatus: ptr("ACTIVE"), Tags: []string{"regular"}, Channels: []string{ChannelWhatsApp},
		HasConversation: ptrBool(true), LastReservationStatus: &status,
	})
	if err != nil || preview.EstimatedCount != 1 {
		t.Fatalf("combined = %+v err=%v", preview, err)
	}
	empty, err := store.PreviewAudience(context.Background(), biz, ChannelWhatsApp, AudienceDefinition{Tags: []string{"missing"}})
	if err != nil || empty.EstimatedCount != 0 {
		t.Fatalf("empty = %+v err=%v", empty, err)
	}
}

func testService(t *testing.T) (*Service, *memoryStore, *stubPublisher) {
	t.Helper()
	store := newMemoryStore()
	pub := &stubPublisher{}
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store, pub, stubBusiness{}, stubConversations{store: store}, Settings{
		BatchSize: 50, DispatchInterval: time.Millisecond, SchedulerInterval: time.Hour, MaxStartsPerTick: 20,
	})
	return svc, store, pub
}

func mustCreate(t *testing.T, svc *Service, biz uuid.UUID) *Campaign {
	t.Helper()
	tpl, lang := "hello", "id"
	item, err := svc.Create(context.Background(), biz, uuid.New(), CreateCampaignInput{
		Name: "Promo", Channel: "WHATSAPP", MessageKind: "TEMPLATE", TemplateName: &tpl, TemplateLanguage: &lang,
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func mustActiveRun(t *testing.T, store *memoryStore, biz, campaignID uuid.UUID) uuid.UUID {
	t.Helper()
	run, err := store.GetActiveRun(context.Background(), biz, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	return run.ID
}

func mustRun(t *testing.T, store *memoryStore, biz, campaignID uuid.UUID) uuid.UUID {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), biz, campaignID, 1, 0)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs err=%v", err)
	}
	return runs[0].ID
}

func seedCustomer(store *memoryStore, biz uuid.UUID, name, status string, optIn bool, tags, channels []string) uuid.UUID {
	id := uuid.New()
	ident := uuid.New()
	ext := ""
	if len(channels) > 0 {
		ext = "6281" + id.String()[:8]
	}
	store.customers = append(store.customers, memoryCustomer{
		ID: id, BusinessID: biz, Name: name, Status: status, MarketingOptIn: optIn,
		Tags: tags, Channels: channels, ExternalID: ext, IdentityID: ident, CreatedAt: time.Now().UTC(),
	})
	return id
}

func ptr[T any](v T) *T    { return &v }
func ptrBool(v bool) *bool { return &v }

type stubPublisher struct {
	starts     []uuid.UUID
	dispatches []uuid.UUID
}

func (s *stubPublisher) EnqueueCampaignStart(_ context.Context, _, runID uuid.UUID) error {
	s.starts = append(s.starts, runID)
	return nil
}
func (s *stubPublisher) EnqueueCampaignDispatch(_ context.Context, _, runID uuid.UUID) error {
	s.dispatches = append(s.dispatches, runID)
	return nil
}
func (s *stubPublisher) EnqueueCampaignDispatchAfter(_ context.Context, _, runID uuid.UUID, _ time.Duration) error {
	s.dispatches = append(s.dispatches, runID)
	return nil
}

type stubBusiness struct{}

func (stubBusiness) ActiveTimezone(context.Context, uuid.UUID) (string, error) {
	return "Asia/Jakarta", nil
}

type stubConversations struct{ store *memoryStore }

func (s stubConversations) CreateConversation(_ context.Context, _ uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error) {
	if in.SuppressAutomation != true {
		return nil, false, errors.New("campaign conversations must suppress automation")
	}
	return &conversation.Conversation{ID: uuid.New()}, true, nil
}

func (s stubConversations) CreateMessage(_ context.Context, businessID, _ uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error) {
	if in.SenderType != string(conversation.SenderCampaign) || !in.AllowCampaign {
		return nil, false, errors.New("campaign sender required")
	}
	id := uuid.New()
	if s.store != nil {
		var meta struct {
			RecipientID string `json:"campaign_recipient_id"`
		}
		_ = json.Unmarshal(in.Metadata, &meta)
		if meta.RecipientID != "" {
			s.store.messages[businessID.String()+":"+meta.RecipientID] = id
		}
	}
	return &conversation.Message{ID: id, Status: conversation.MessageQueued, Direction: conversation.DirectionOutbound}, true, nil
}
