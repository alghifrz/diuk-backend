package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestCreateConversationRules(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	other := uuid.New()
	active := store.addCustomer(biz, "John Doe", "ACTIVE", ptr("+628123456789"), ptr("john@example.com"))
	inactive := store.addCustomer(biz, "Old", "INACTIVE", nil, nil)
	foreign := store.addCustomer(other, "Jane", "ACTIVE", nil, nil)
	ident := store.addIdentity(biz, active, ChannelWhatsApp, "6281", "ACTIVE")
	ig := store.addIdentity(biz, active, ChannelInstagram, "ig-1", "ACTIVE")
	foreignIdent := store.addIdentity(other, foreign, ChannelWhatsApp, "6281", "ACTIVE")

	created, isNew, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "whatsapp", CustomerIdentityID: ptr(ident.String()),
	})
	if err != nil || !isNew || created.Channel != ChannelWhatsApp || created.Status != StatusOpen {
		t.Fatalf("create = %+v new=%v err=%v", created, isNew, err)
	}
	reused, isNew, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(ident.String()),
	})
	if err != nil || isNew || reused.ID != created.ID {
		t.Fatalf("reuse = %+v new=%v err=%v", reused, isNew, err)
	}

	website, isNew, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "WEBSITE",
	})
	if err != nil || !isNew || website.CustomerIdentityID != nil {
		t.Fatalf("website = %+v new=%v err=%v", website, isNew, err)
	}

	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: inactive.String(), Channel: "WEBSITE",
	}); !errors.Is(err, ErrCustomerInactive) {
		t.Fatalf("inactive customer = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(ig.String()),
	}); !errors.Is(err, ErrChannelMismatch) {
		t.Fatalf("channel mismatch = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(uuid.New().String()),
	}); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("missing identity = %v", err)
	}
	otherCust := store.addCustomer(biz, "Pat", "ACTIVE", nil, nil)
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: otherCust.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(ident.String()),
	}); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("identity mismatch = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), other, CreateConversationInput{
		CustomerID: active.String(), Channel: "WEBSITE",
	}); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("cross-tenant customer = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(foreignIdent.String()),
	}); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("cross-tenant identity = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: "not-a-uuid", Channel: "WEBSITE",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad customer id = %v", err)
	}
	if _, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{
		CustomerID: active.String(), Channel: "SMS",
	}); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("bad channel = %v", err)
	}
}

func TestConversationStatusAssignmentAndList(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	other := uuid.New()
	agentID := uuid.New()
	inactiveAgent := uuid.New()
	outsider := uuid.New()
	svc := NewService(store, stubUsers{users: map[uuid.UUID]*tenant.User{
		agentID:       {ID: agentID, FullName: "Agent", Status: tenant.StatusActive},
		inactiveAgent: {ID: inactiveAgent, FullName: "Gone", Status: tenant.StatusInactive},
		outsider:      {ID: outsider, FullName: "Other", Status: tenant.StatusActive},
	}}, stubMembers{ok: map[string]struct{}{
		agentID.String() + "|" + biz.String():    {},
		outsider.String() + "|" + other.String(): {},
	}})
	cust := store.addCustomer(biz, "Jane", "ACTIVE", ptr("0811"), ptr("jane@example.com"))
	conv, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	closed, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{Status: statusPtr(StatusClosed)})
	if err != nil || closed.Status != StatusClosed {
		t.Fatalf("close = %+v err=%v", closed, err)
	}
	opened, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{Status: statusPtr(StatusOpen)})
	if err != nil || opened.Status != StatusOpen {
		t.Fatalf("reopen = %+v err=%v", opened, err)
	}

	assigned, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{AssignedUserID: &agentID})
	if err != nil || assigned.AssignedUserID == nil || *assigned.AssignedUserID != agentID {
		t.Fatalf("assign = %+v err=%v", assigned, err)
	}
	if _, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{AssignedUserID: &inactiveAgent}); !errors.Is(err, ErrAssignedUserInactive) {
		t.Fatalf("inactive agent = %v", err)
	}
	if _, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{AssignedUserID: &outsider}); !errors.Is(err, ErrAssignedUserNotMember) {
		t.Fatalf("outsider = %v", err)
	}
	if _, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{AssignedUserID: uuidPtr(uuid.New())}); !errors.Is(err, ErrAssignedUserNotFound) {
		t.Fatalf("missing user = %v", err)
	}
	unassigned, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{ClearAssignedUser: true})
	if err != nil || unassigned.AssignedUserID != nil {
		t.Fatalf("unassign = %+v err=%v", unassigned, err)
	}

	listed, err := svc.ListConversations(context.Background(), biz, "", "", "", "", "jane", "", "", "")
	if err != nil || len(listed) != 1 {
		t.Fatalf("search = %d err=%v", len(listed), err)
	}
	if _, err := svc.GetConversation(context.Background(), other, conv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}
	if _, err := svc.ListConversations(context.Background(), biz, "ARCHIVED", "", "", "", "", "", "", ""); !errors.Is(err, ErrInvalidConversationStatus) {
		t.Fatalf("bad status filter = %v", err)
	}
}

func TestMessageLifecycle(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	other := uuid.New()
	cust := store.addCustomer(biz, "John", "ACTIVE", nil, nil)
	conv, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: cust.String(), Channel: "WEBSITE"})
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}

	inbound, created, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", MessageType: "TEXT", Content: ptr("Halo"),
	})
	if err != nil || !created || inbound.Status != MessageReceived {
		t.Fatalf("inbound = %+v created=%v err=%v", inbound, created, err)
	}
	afterIn, _ := svc.GetConversation(context.Background(), biz, conv.ID)
	if afterIn.UnreadCount != 1 || afterIn.LastMessageAt == nil {
		t.Fatalf("unread/last after inbound = %+v", afterIn)
	}

	outbound, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "USER", Content: ptr("Ada yang bisa kami bantu?"),
	})
	if err != nil || outbound.Status != MessageQueued {
		t.Fatalf("outbound = %+v err=%v", outbound, err)
	}
	afterOut, _ := svc.GetConversation(context.Background(), biz, conv.ID)
	if afterOut.UnreadCount != 1 {
		t.Fatalf("outbound incremented unread = %d", afterOut.UnreadCount)
	}

	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "AI", Content: ptr("AI reply"),
	}); err != nil {
		t.Fatalf("ai: %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "SYSTEM", MessageType: "SYSTEM", Content: ptr("closed hours"),
	}); err != nil {
		t.Fatalf("system: %v", err)
	}

	if _, err := svc.UpdateConversation(context.Background(), biz, conv.ID, UpdateConversationInput{Status: statusPtr(StatusClosed)}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("masih buka?"),
	}); err != nil {
		t.Fatalf("reopen on message: %v", err)
	}
	reopened, _ := svc.GetConversation(context.Background(), biz, conv.ID)
	if reopened.Status != StatusOpen || reopened.UnreadCount != 2 {
		t.Fatalf("reopened = %+v", reopened)
	}

	read, err := svc.MarkRead(context.Background(), biz, conv.ID)
	if err != nil || read.UnreadCount != 0 {
		t.Fatalf("mark read = %+v err=%v", read, err)
	}
	afterRead, err := svc.ListMessages(context.Background(), biz, conv.ID, "50", "0", "", "")
	if err != nil {
		t.Fatalf("list after read: %v", err)
	}
	for _, msg := range afterRead {
		if msg.Direction == DirectionInbound && msg.SenderType == SenderCustomer && msg.Status != MessageRead {
			t.Fatalf("inbound after read = %+v", msg)
		}
	}

	ext := "wa-1"
	first, created, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("dup"), ExternalID: &ext,
	})
	if err != nil || !created {
		t.Fatalf("first external: created=%v err=%v", created, err)
	}
	again, created, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("dup"), ExternalID: &ext,
	})
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("duplicate external = %+v created=%v err=%v", again, created, err)
	}

	if _, _, err := svc.CreateMessage(context.Background(), other, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("x"),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant message = %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "CUSTOMER", Content: ptr("x"),
	}); !errors.Is(err, ErrInvalidSenderType) {
		t.Fatalf("invalid combo = %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "CAMPAIGN", Content: ptr("promo"),
	}); err == nil {
		t.Fatal("HTTP/default CreateMessage must not accept CAMPAIGN sender")
	}
	campaignMsg, created, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "CAMPAIGN", Content: ptr("promo"), AllowCampaign: true, Status: string(MessageQueued),
	})
	if err != nil || !created || campaignMsg.SenderType != SenderCampaign || campaignMsg.Status != MessageQueued {
		t.Fatalf("campaign message = %+v created=%v err=%v", campaignMsg, created, err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", MessageType: "TEXT",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("text without content = %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", MessageType: "IMAGE",
	}); err != nil {
		t.Fatalf("image without content: %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "SIDEWAYS", SenderType: "CUSTOMER", Content: ptr("x"),
	}); !errors.Is(err, ErrInvalidDirection) {
		t.Fatalf("direction = %v", err)
	}
	if _, err := svc.ListMessages(context.Background(), other, conv.ID, "", "", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant list messages = %v", err)
	}
	if _, err := svc.ListRecentMessages(context.Background(), other, conv.ID, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant recent messages = %v", err)
	}

	msgs, err := svc.ListMessages(context.Background(), biz, conv.ID, "100", "0", "", "")
	if err != nil || len(msgs) < 6 {
		t.Fatalf("list messages = %d err=%v", len(msgs), err)
	}
	if !msgs[0].CreatedAt.Before(msgs[len(msgs)-1].CreatedAt) && msgs[0].CreatedAt != msgs[len(msgs)-1].CreatedAt {
		t.Fatalf("message order is not chronological")
	}
	recent, err := svc.ListRecentMessages(context.Background(), biz, conv.ID, 2)
	if err != nil || len(recent) != 2 {
		t.Fatalf("recent messages = %d err=%v", len(recent), err)
	}
	if recent[0].ID != msgs[len(msgs)-2].ID || recent[1].ID != msgs[len(msgs)-1].ID {
		t.Fatalf("recent window is not the latest chronological pair")
	}
	if _, err := svc.ListConversations(context.Background(), biz, "", "", "", "", "", "", "0", ""); err == nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid pagination = %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("x"), ExternalID: ptr(strings.Repeat("e", 256)),
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long external = %v", err)
	}
}

func TestApplyDeliveryStatusProgression(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	customerID := store.addCustomer(biz, "Pat", "ACTIVE", nil, nil)
	conv, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: customerID.String(), Channel: "WEBSITE"})
	if err != nil {
		t.Fatalf("conv: %v", err)
	}
	msg, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "OUTBOUND", SenderType: "AI", Content: ptr("hi"), Status: "QUEUED", ExternalID: ptr("wamid.1"),
	})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	updated, changed, err := svc.ApplyDeliveryStatus(context.Background(), biz, msg.ID, "SENT", nil)
	if err != nil || !changed || updated.Status != MessageSent {
		t.Fatalf("sent = %+v changed=%v err=%v", updated, changed, err)
	}
	updated, changed, err = svc.ApplyDeliveryStatus(context.Background(), biz, msg.ID, "READ", nil)
	if err != nil || !changed || updated.Status != MessageRead {
		t.Fatalf("read = %+v err=%v", updated, err)
	}
	updated, changed, err = svc.ApplyDeliveryStatus(context.Background(), biz, msg.ID, "SENT", nil)
	if err != nil || changed || updated.Status != MessageRead {
		t.Fatalf("downgrade = %+v changed=%v err=%v", updated, changed, err)
	}

	inbound, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("seen"), ExternalID: ptr("wamid.in"),
	})
	if err != nil {
		t.Fatalf("inbound: %v", err)
	}
	updated, changed, err = svc.ApplyDeliveryStatus(context.Background(), biz, inbound.ID, "READ", nil)
	if err != nil || !changed || updated.Status != MessageRead {
		t.Fatalf("inbound read = %+v changed=%v err=%v", updated, changed, err)
	}
}

func TestMarkReadNotifiesLatestInbound(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, stubUsers{}, stubMembers{})
	biz := uuid.New()
	customerID := store.addCustomer(biz, "Pat", "ACTIVE", nil, nil)
	conv, _, err := svc.CreateConversation(context.Background(), biz, CreateConversationInput{CustomerID: customerID.String(), Channel: "WEBSITE"})
	if err != nil {
		t.Fatalf("conv: %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("one"), ExternalID: ptr("wamid.one"),
	}); err != nil {
		t.Fatalf("one: %v", err)
	}
	if _, _, err := svc.CreateMessage(context.Background(), biz, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("two"), ExternalID: ptr("wamid.two"),
	}); err != nil {
		t.Fatalf("two: %v", err)
	}

	var got []string
	svc.SetOnInboundRead(func(_ context.Context, businessID, conversationID uuid.UUID, ids []string) error {
		if businessID != biz || conversationID != conv.ID {
			t.Fatalf("hook ids = %s %s", businessID, conversationID)
		}
		got = ids
		return nil
	})
	if _, err := svc.MarkRead(context.Background(), biz, conv.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if len(got) != 2 || got[0] != "wamid.one" || got[1] != "wamid.two" {
		t.Fatalf("external ids = %v", got)
	}
}

type stubUsers struct {
	users map[uuid.UUID]*tenant.User
}

func (s stubUsers) GetByID(_ context.Context, id uuid.UUID) (*tenant.User, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, tenant.ErrNotFound
	}
	copy := *user
	return &copy, nil
}

type stubMembers struct {
	ok map[string]struct{}
}

func (s stubMembers) GetActiveAccess(_ context.Context, userID, businessID uuid.UUID) (*tenant.Access, error) {
	if _, exists := s.ok[userID.String()+"|"+businessID.String()]; !exists {
		return nil, tenant.ErrNotFound
	}
	return &tenant.Access{Membership: tenant.BusinessUser{UserID: userID, BusinessID: businessID}}, nil
}

func statusPtr(s Status) *Status { return &s }

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }
