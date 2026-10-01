package conversation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestHumanHandoverStateMachineAndIdempotency(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	other := uuid.New()
	cust := store.addCustomer(biz, "Budi", "ACTIVE", ptr("0811"), nil)
	conv, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if conv.HandlingState != HandlingAIActive {
		t.Fatalf("new conversation handling = %s", conv.HandlingState)
	}

	if _, err := svc.TakeoverConversation(context.Background(), biz, conv.ID); !errors.Is(err, ErrInvalidHandlingTransition) {
		t.Fatalf("takeover from AI = %v", err)
	}
	if _, err := svc.ResolveConversation(context.Background(), biz, conv.ID); !errors.Is(err, ErrInvalidHandlingTransition) {
		t.Fatalf("resolve from AI = %v", err)
	}

	first, err := svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonCustomerRequested))
	if err != nil || first.HandlingState != HandlingHumanRequested || first.HandoverReason == nil || first.HandoverRequestedAt == nil {
		t.Fatalf("handover = %+v err=%v", first, err)
	}
	again, err := svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonManualHandoff))
	if err != nil || again.HandlingState != HandlingHumanRequested || again.HandoverRequestedAt == nil || !again.HandoverRequestedAt.Equal(*first.HandoverRequestedAt) {
		t.Fatalf("duplicate handover = %+v err=%v", again, err)
	}

	active, err := svc.TakeoverConversation(context.Background(), biz, conv.ID)
	if err != nil || active.HandlingState != HandlingHumanActive || active.HumanStartedAt == nil {
		t.Fatalf("takeover = %+v err=%v", active, err)
	}
	started := *active.HumanStartedAt
	againTakeover, err := svc.TakeoverConversation(context.Background(), biz, conv.ID)
	if err != nil || againTakeover.HandlingState != HandlingHumanActive || againTakeover.HumanStartedAt == nil || !againTakeover.HumanStartedAt.Equal(started) {
		t.Fatalf("duplicate takeover = %+v err=%v", againTakeover, err)
	}

	resolved, err := svc.ResolveConversation(context.Background(), biz, conv.ID)
	if err != nil || resolved.HandlingState != HandlingResolved || resolved.ResolvedAt == nil {
		t.Fatalf("resolve = %+v err=%v", resolved, err)
	}
	againResolve, err := svc.ResolveConversation(context.Background(), biz, conv.ID)
	if err != nil || againResolve.HandlingState != HandlingResolved || againResolve.ResolvedAt == nil || !againResolve.ResolvedAt.Equal(*resolved.ResolvedAt) {
		t.Fatalf("duplicate resolve = %+v err=%v", againResolve, err)
	}

	resumed, err := svc.ResumeAI(context.Background(), biz, conv.ID)
	if err != nil || resumed.HandlingState != HandlingAIActive {
		t.Fatalf("resume = %+v err=%v", resumed, err)
	}
	againResume, err := svc.ResumeAI(context.Background(), biz, conv.ID)
	if err != nil || againResume.HandlingState != HandlingAIActive {
		t.Fatalf("duplicate resume = %+v err=%v", againResume, err)
	}

	if _, err := svc.RequestHumanHandover(context.Background(), other, conv.ID, string(ReasonManualHandoff)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant handover = %v", err)
	}
}

func TestHumanInboxSharedAndTenantScoped(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	a, b := uuid.New(), uuid.New()
	custA := store.addCustomer(a, "Siti", "ACTIVE", ptr("0812"), nil)
	custB := store.addCustomer(b, "Andi", "ACTIVE", ptr("0813"), nil)
	convA, _, _ := svc.CreateConversation(context.Background(), a, CreateConversationInput{CustomerID: custA.String(), Channel: "WEBSITE"})
	convB, _, _ := svc.CreateConversation(context.Background(), b, CreateConversationInput{CustomerID: custB.String(), Channel: "WEBSITE"})
	_, _, _ = svc.CreateMessage(context.Background(), a, convA.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("I want an admin"),
	})
	_, _ = svc.RequestHumanHandover(context.Background(), a, convA.ID, string(ReasonCustomerRequested))
	_, _ = svc.RequestHumanHandover(context.Background(), b, convB.ID, string(ReasonAIUnableToAnswer))

	inboxA, err := svc.ListHumanInbox(context.Background(), a, "", "", "")
	if err != nil || len(inboxA) != 1 || inboxA[0].ID != convA.ID {
		t.Fatalf("inbox A = %+v err=%v", inboxA, err)
	}
	if inboxA[0].LastMessage == nil || inboxA[0].LastMessage.Content == nil || *inboxA[0].LastMessage.Content != "I want an admin" {
		t.Fatalf("inbox preview = %+v", inboxA[0].LastMessage)
	}
	inboxB, err := svc.ListHumanInbox(context.Background(), b, "", "", "")
	if err != nil || len(inboxB) != 1 || inboxB[0].ID != convB.ID {
		t.Fatalf("inbox B = %+v err=%v", inboxB, err)
	}

	aiOnly, _, _ := svc.CreateConversation(context.Background(), a, CreateConversationInput{CustomerID: custA.String(), Channel: "INSTAGRAM"})
	inboxDefault, _ := svc.ListHumanInbox(context.Background(), a, "", "", "")
	for _, item := range inboxDefault {
		if item.ID == aiOnly.ID {
			t.Fatal("AI_ACTIVE conversation leaked into human inbox")
		}
	}

	_, _ = svc.ResolveConversation(context.Background(), a, convA.ID)
	activeOnly, _ := svc.ListHumanInbox(context.Background(), a, "", "", "")
	if len(activeOnly) != 0 {
		t.Fatalf("resolved conversation still in default inbox: %+v", activeOnly)
	}
	resolved, _ := svc.ListHumanInbox(context.Background(), a, "RESOLVED", "", "")
	if len(resolved) != 1 || resolved[0].ID != convA.ID {
		t.Fatalf("resolved filter = %+v", resolved)
	}
}

func TestHumanReplyActivatesAndEnqueuesDelivery(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	var queued []uuid.UUID
	svc.SetOnQueuedOutbound(func(_ context.Context, _, messageID uuid.UUID) error {
		queued = append(queued, messageID)
		return nil
	})
	biz := uuid.New()
	other := uuid.New()
	userID := uuid.New()
	cust := store.addCustomer(biz, "Budi", "ACTIVE", nil, nil)
	conv, _, _ := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})

	if _, err := svc.Reply(context.Background(), biz, conv.ID, userID, "too early"); !errors.Is(err, ErrInvalidHandlingTransition) {
		t.Fatalf("reply while AI_ACTIVE = %v", err)
	}

	_, _ = svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonManualHandoff))
	msg, err := svc.Reply(context.Background(), biz, conv.ID, userID, "Saya bantu ya")
	if err != nil || msg.Direction != DirectionOutbound || msg.SenderType != SenderUser || msg.Status != MessageQueued {
		t.Fatalf("reply = %+v err=%v", msg, err)
	}
	if len(queued) != 1 || queued[0] != msg.ID {
		t.Fatalf("delivery hook = %v", queued)
	}
	after, _ := svc.GetConversation(context.Background(), biz, conv.ID)
	if after.HandlingState != HandlingHumanActive {
		t.Fatalf("reply did not activate human mode: %s", after.HandlingState)
	}
	claimed, err := svc.ClaimForDelivery(context.Background(), biz, msg.ID, 0)
	if err != nil || claimed == nil || claimed.Status != MessageSending {
		t.Fatalf("claim reply = %+v err=%v", claimed, err)
	}
	if _, err := svc.Reply(context.Background(), other, conv.ID, userID, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant reply = %v", err)
	}
}

func TestInternalNotesStayPrivate(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	var queued int
	svc.SetOnQueuedOutbound(func(context.Context, uuid.UUID, uuid.UUID) error {
		queued++
		return nil
	})
	biz := uuid.New()
	other := uuid.New()
	userID := uuid.New()
	cust := store.addCustomer(biz, "Siti", "ACTIVE", nil, nil)
	conv, _, _ := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	_, _, _ = svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("refund"),
	})

	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "USER", MessageType: "INTERNAL_NOTE", Content: ptr("secret"),
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("generic create must reject notes = %v", err)
	}

	note, err := svc.CreateNote(context.Background(), biz, conv.ID, userID, "Need to verify the refund manually")
	if err != nil || note.MessageType != MessageTypeInternalNote || note.Status != MessageInternal {
		t.Fatalf("note = %+v err=%v", note, err)
	}
	if queued != 0 {
		t.Fatalf("note entered delivery hook: %d", queued)
	}
	claimed, err := svc.ClaimForDelivery(context.Background(), biz, note.ID, 0)
	if err != nil || claimed != nil {
		t.Fatalf("note claimed for delivery = %+v err=%v", claimed, err)
	}

	_, _ = svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonManualHandoff))
	inbox, _ := svc.ListHumanInbox(context.Background(), biz, "", "", "")
	if len(inbox) != 1 || inbox[0].LastMessage == nil || inbox[0].LastMessage.Content == nil || *inbox[0].LastMessage.Content != "refund" {
		t.Fatalf("inbox leaked note: %+v", inbox)
	}
	if _, err := svc.CreateNote(context.Background(), other, conv.ID, userID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant note = %v", err)
	}
}

func TestResumeFromHumanRequested(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	cust := store.addCustomer(biz, "Andi", "ACTIVE", nil, nil)
	conv, _, _ := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	_, _ = svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonAIUnableToAnswer))
	resumed, err := svc.ResumeAI(context.Background(), biz, conv.ID)
	if err != nil || resumed.HandlingState != HandlingAIActive {
		t.Fatalf("resume from requested = %+v err=%v", resumed, err)
	}
}

func TestResolveFromHumanRequested(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	cust := store.addCustomer(biz, "Andi", "ACTIVE", nil, nil)
	conv, _, _ := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	_, _ = svc.RequestHumanHandover(context.Background(), biz, conv.ID, string(ReasonManualHandoff))
	resolved, err := svc.ResolveConversation(context.Background(), biz, conv.ID)
	if err != nil || resolved.HandlingState != HandlingResolved {
		t.Fatalf("resolve from requested = %+v err=%v", resolved, err)
	}
}
