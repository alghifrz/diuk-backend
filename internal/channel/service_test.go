package channel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
)

func seedWhatsApp(t *testing.T, store *memoryStore, businessID uuid.UUID, phoneID string) *BusinessChannel {
	t.Helper()
	item, err := store.Create(context.Background(), BusinessChannel{
		BusinessID: businessID, Channel: ChannelWhatsApp, Status: StatusActive, ExternalPhoneNumberID: &phoneID,
	})
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	return item
}

func TestHandleIncomingNewAndExisting(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	convs := newFakeConversations()
	ai := &fakeAI{}
	svc := newTestService(store, customers, convs, ai, &fakeSender{})

	msg := IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "628111",
		ExternalMessageID: "wamid.in.1", CustomerName: "Budi", CustomerPhone: "628111",
		MessageType: MessageTypeText, Text: "Halo", ReceivedAt: time.Now().UTC(),
	}
	first, err := svc.HandleIncoming(context.Background(), msg)
	if err != nil || !first.Created || first.BusinessID != biz {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	if ai.calls != 1 {
		t.Fatalf("ai calls = %d", ai.calls)
	}
	if customers.creates != 1 {
		t.Fatalf("identity creates = %d", customers.creates)
	}

	second, err := svc.HandleIncoming(context.Background(), msg)
	if err != nil || !second.Duplicate || second.Created {
		t.Fatalf("duplicate = %+v err=%v", second, err)
	}
	if ai.calls != 1 {
		t.Fatalf("duplicate must not trigger AI again, calls=%d", ai.calls)
	}
	if customers.creates != 1 {
		t.Fatalf("duplicate identity create = %d", customers.creates)
	}
}

func TestHandleIncomingReusesClosedConversation(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	ident := &customer.Identity{ID: uuid.New(), BusinessID: biz, CustomerID: uuid.New(), Channel: customer.ChannelWhatsApp, ExternalID: "628111"}
	customers.identities[customers.key(biz, ChannelWhatsApp, "628111")] = ident
	convs := newFakeConversations()
	closed := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, CustomerID: ident.CustomerID, Channel: conversation.ChannelWhatsApp,
		CustomerIdentityID: &ident.ID, Status: conversation.StatusClosed, UpdatedAt: time.Now().UTC(),
	}
	convs.conversations[closed.ID] = closed
	svc := newTestService(store, customers, convs, &fakeAI{}, &fakeSender{})

	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "628111",
		ExternalMessageID: "wamid.in.2", MessageType: MessageTypeText, Text: "lagi",
	})
	if err != nil || got.ConversationID != closed.ID {
		t.Fatalf("reopen = %+v err=%v", got, err)
	}
	if convs.conversations[closed.ID].Status != conversation.StatusOpen {
		t.Fatal("closed conversation was not reopened")
	}
}

func TestHandleIncomingSkipsAIWhenHumanHandling(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	ident := &customer.Identity{ID: uuid.New(), BusinessID: biz, CustomerID: uuid.New(), Channel: customer.ChannelWhatsApp, ExternalID: "628111"}
	customers.identities[customers.key(biz, ChannelWhatsApp, "628111")] = ident
	convs := newFakeConversations()
	conv := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, CustomerID: ident.CustomerID, Channel: conversation.ChannelWhatsApp,
		CustomerIdentityID: &ident.ID, Status: conversation.StatusOpen, HandlingState: conversation.HandlingHumanRequested,
		Identity: &conversation.IdentitySummary{ID: ident.ID, Channel: conversation.ChannelWhatsApp, ExternalID: ident.ExternalID},
	}
	convs.conversations[conv.ID] = conv
	ai := &fakeAI{}
	svc := newTestService(store, customers, convs, ai, &fakeSender{})

	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "628111",
		ExternalMessageID: "wamid.human.1", MessageType: MessageTypeText, Text: "admin please",
	})
	if err != nil || !got.Created {
		t.Fatalf("inbound = %+v err=%v", got, err)
	}
	if ai.calls != 0 {
		t.Fatalf("human handling must not enqueue AI, calls=%d", ai.calls)
	}
}

func TestDeliverMessageSkipsInternalNotes(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	convs := newFakeConversations()
	conv := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, Channel: conversation.ChannelWhatsApp,
		Identity: &conversation.IdentitySummary{ExternalID: "6281"},
	}
	convs.conversations[conv.ID] = conv
	noteID := uuid.New()
	convs.messages[noteID] = &conversation.Message{
		ID: noteID, BusinessID: biz, ConversationID: conv.ID, Direction: conversation.DirectionOutbound,
		MessageType: conversation.MessageTypeInternalNote, Content: ptr("private"), Status: conversation.MessageInternal,
	}
	sender := &fakeSender{}
	svc := newTestService(store, newFakeCustomers(), convs, &fakeAI{}, sender)
	if err := svc.DeliverMessage(context.Background(), biz, noteID); err != nil {
		t.Fatalf("note delivery = %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("internal note was sent to WhatsApp, calls=%d", sender.calls)
	}
	if convs.messages[noteID].Status != conversation.MessageInternal {
		t.Fatalf("note status changed = %s", convs.messages[noteID].Status)
	}
}

func TestHandleIncomingTenantIsolation(t *testing.T) {
	store := newMemoryStore()
	a, b := uuid.New(), uuid.New()
	seedWhatsApp(t, store, a, "pn-a")
	seedWhatsApp(t, store, b, "pn-b")
	customers := newFakeCustomers()
	convs := newFakeConversations()
	svc := newTestService(store, customers, convs, &fakeAI{}, &fakeSender{})

	if _, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-unknown", ExternalIdentityID: "1",
		ExternalMessageID: "x", MessageType: MessageTypeText, Text: "hi",
	}); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("unknown phone = %v", err)
	}

	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-a", ExternalIdentityID: "6281",
		ExternalMessageID: "m1", MessageType: MessageTypeText, Text: "hi",
	})
	if err != nil || got.BusinessID != a {
		t.Fatalf("tenant A = %+v err=%v", got, err)
	}
	if _, ok := customers.identities[customers.key(b, ChannelWhatsApp, "6281")]; ok {
		t.Fatal("identity leaked to business B")
	}
}

func TestHandleIncomingUnsupportedAndDisabled(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	ch := seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	convs := newFakeConversations()
	ai := &fakeAI{}
	svc := newTestService(store, customers, convs, ai, &fakeSender{})

	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "1",
		ExternalMessageID: "img", MessageType: MessageTypeUnsupported,
	})
	if err != nil || !got.Unsupported || ai.calls != 0 {
		t.Fatalf("unsupported = %+v err=%v ai=%d", got, err, ai.calls)
	}

	ch.Status = StatusInactive
	_, _ = store.Update(context.Background(), biz, *ch)
	if _, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "1",
		ExternalMessageID: "t", MessageType: MessageTypeText, Text: "hi",
	}); !errors.Is(err, ErrChannelNotFound) && !errors.Is(err, ErrChannelDisabled) {
		t.Fatalf("disabled = %v", err)
	}
}

func TestHandleIncomingAIFailureIsSafe(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	convs := newFakeConversations()
	svc := newTestService(store, customers, convs, &fakeAI{err: errors.New("provider down")}, &fakeSender{})
	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "6281",
		ExternalMessageID: "m-ai", MessageType: MessageTypeText, Text: "hi",
	})
	if err != nil || !got.Created {
		t.Fatalf("inbound must persist even if AI fails: %+v err=%v", got, err)
	}
}

func TestDeliverQueuedMessage(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	convs := newFakeConversations()
	identID := uuid.New()
	conv := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, CustomerID: uuid.New(), Channel: conversation.ChannelWhatsApp,
		CustomerIdentityID: &identID, Status: conversation.StatusOpen,
		Identity: &conversation.IdentitySummary{ID: identID, Channel: conversation.ChannelWhatsApp, ExternalID: "628111"},
	}
	convs.conversations[conv.ID] = conv
	queued := &conversation.Message{
		ID: uuid.New(), BusinessID: biz, ConversationID: conv.ID, Direction: conversation.DirectionOutbound,
		SenderType: conversation.SenderAI, MessageType: conversation.MessageTypeText, Content: ptr("halo"),
		Status: conversation.MessageQueued,
	}
	convs.messages[queued.ID] = queued
	sender := &fakeSender{id: "wamid.out.1"}
	svc := newTestService(store, newFakeCustomers(), convs, &fakeAI{}, sender)

	if err := svc.DeliverMessage(context.Background(), biz, queued.ID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if sender.calls != 1 || sender.last.To != "628111" || sender.last.PhoneNumberID != "pn-1" {
		t.Fatalf("send = %+v calls=%d", sender.last, sender.calls)
	}
	if convs.messages[queued.ID].Status != conversation.MessageSent || *convs.messages[queued.ID].ExternalID != "wamid.out.1" {
		t.Fatalf("message = %+v", convs.messages[queued.ID])
	}
	if err := svc.DeliverMessage(context.Background(), biz, queued.ID); err != nil {
		t.Fatalf("resend already sent: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("must not resend SENT, calls=%d", sender.calls)
	}
}

func TestDeliverProviderErrors(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	convs := newFakeConversations()
	conv := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, Channel: conversation.ChannelWhatsApp,
		Identity: &conversation.IdentitySummary{ExternalID: "6281"},
	}
	convs.conversations[conv.ID] = conv
	makeQueued := func() uuid.UUID {
		id := uuid.New()
		convs.messages[id] = &conversation.Message{
			ID: id, BusinessID: biz, ConversationID: conv.ID, Direction: conversation.DirectionOutbound,
			MessageType: conversation.MessageTypeText, Content: ptr("x"), Status: conversation.MessageQueued,
		}
		return id
	}

	svc := newTestService(store, newFakeCustomers(), convs, &fakeAI{}, &fakeSender{err: errors.New("http 400")})
	id := makeQueued()
	if err := svc.DeliverMessage(context.Background(), biz, id); !errors.Is(err, ErrProviderRejected) {
		t.Fatalf("4xx = %v", err)
	}
	if convs.messages[id].Status != conversation.MessageFailed {
		t.Fatalf("status = %s", convs.messages[id].Status)
	}

	svc = newTestService(store, newFakeCustomers(), convs, &fakeAI{}, &fakeSender{err: whatsapp.ErrUnavailable})
	id = makeQueued()
	if err := svc.DeliverMessage(context.Background(), biz, id); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("5xx = %v", err)
	}
	if convs.messages[id].Status != conversation.MessageQueued {
		t.Fatalf("transient failure must return to QUEUED, status=%s", convs.messages[id].Status)
	}
}

func TestStatusProgression(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	convs := newFakeConversations()
	msg := &conversation.Message{ID: uuid.New(), BusinessID: biz, Status: conversation.MessageSent, ExternalID: ptr("wamid.s")}
	convs.messages[msg.ID] = msg
	svc := newTestService(store, newFakeCustomers(), convs, &fakeAI{}, &fakeSender{})

	if err := svc.HandleStatus(context.Background(), StatusUpdate{Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalMessageID: "wamid.s", Status: "DELIVERED"}); err != nil {
		t.Fatalf("delivered: %v", err)
	}
	if convs.messages[msg.ID].Status != conversation.MessageDelivered {
		t.Fatalf("status = %s", convs.messages[msg.ID].Status)
	}
	if err := svc.HandleStatus(context.Background(), StatusUpdate{Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalMessageID: "wamid.s", Status: "READ"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := svc.HandleStatus(context.Background(), StatusUpdate{Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalMessageID: "wamid.s", Status: "SENT"}); err != nil {
		t.Fatalf("late sent: %v", err)
	}
	if convs.messages[msg.ID].Status != conversation.MessageRead {
		t.Fatalf("late sent must not downgrade, status=%s", convs.messages[msg.ID].Status)
	}
	if err := svc.HandleStatus(context.Background(), StatusUpdate{Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalMessageID: "wamid.s", Status: "READ"}); err != nil {
		t.Fatalf("duplicate read: %v", err)
	}
}

func TestNotifyInboundReadSendsLatest(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	convs := newFakeConversations()
	conv := &conversation.Conversation{
		ID: uuid.New(), BusinessID: biz, Channel: conversation.ChannelWhatsApp,
	}
	convs.conversations[conv.ID] = conv
	sender := &fakeSender{}
	svc := newTestService(store, newFakeCustomers(), convs, &fakeAI{}, sender)

	if err := svc.NotifyInboundRead(context.Background(), biz, conv.ID, []string{"wamid.one", "wamid.two"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if sender.readCalls != 1 || sender.lastRead.MessageID != "wamid.two" || sender.lastRead.PhoneNumberID != "pn-1" {
		t.Fatalf("mark read = %+v calls=%d", sender.lastRead, sender.readCalls)
	}
}

func TestConcurrentIdentityCreate(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	seedWhatsApp(t, store, biz, "pn-1")
	customers := newFakeCustomers()
	existing := &customer.Identity{ID: uuid.New(), BusinessID: biz, CustomerID: uuid.New(), Channel: customer.ChannelWhatsApp, ExternalID: "6281"}
	customers.identities[customers.key(biz, ChannelWhatsApp, "6281")] = existing
	svc := newTestService(store, customers, newFakeConversations(), &fakeAI{}, &fakeSender{})
	got, err := svc.HandleIncoming(context.Background(), IncomingMessage{
		Channel: ChannelWhatsApp, ExternalAccountID: "pn-1", ExternalIdentityID: "6281",
		ExternalMessageID: "m", MessageType: MessageTypeText, Text: "hi",
	})
	if err != nil || got.Created == false && customers.creates != 0 {
		t.Fatalf("existing identity create count=%d err=%v", customers.creates, err)
	}
	if customers.creates != 0 {
		t.Fatalf("must reuse identity, creates=%d", customers.creates)
	}
}
