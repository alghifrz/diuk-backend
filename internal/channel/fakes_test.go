package channel

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/channel/provider"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeCustomers struct {
	mu         sync.Mutex
	identities map[string]*customer.Identity
	customers  map[uuid.UUID]*customer.Customer
	creates    int
}

func newFakeCustomers() *fakeCustomers {
	return &fakeCustomers{identities: map[string]*customer.Identity{}, customers: map[uuid.UUID]*customer.Customer{}}
}

func (f *fakeCustomers) key(businessID uuid.UUID, channel, external string) string {
	return businessID.String() + "|" + channel + "|" + external
}

func (f *fakeCustomers) EnsureIdentity(_ context.Context, businessID uuid.UUID, in customer.EnsureIdentityInput) (*customer.Identity, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.key(businessID, in.Channel, in.ExternalID)
	if item := f.identities[key]; item != nil {
		copy := *item
		return &copy, false, nil
	}
	cust := &customer.Customer{ID: uuid.New(), BusinessID: businessID, Name: in.Name, Phone: in.Phone, Status: tenant.StatusActive}
	f.customers[cust.ID] = cust
	item := &customer.Identity{
		ID: uuid.New(), BusinessID: businessID, CustomerID: cust.ID,
		Channel: customer.Channel(in.Channel), ExternalID: in.ExternalID, DisplayName: in.DisplayName, Status: tenant.StatusActive,
	}
	f.identities[key] = item
	f.creates++
	copy := *item
	return &copy, true, nil
}

type fakeConversations struct {
	mu            sync.Mutex
	conversations map[uuid.UUID]*conversation.Conversation
	messages      map[uuid.UUID]*conversation.Message
}

func newFakeConversations() *fakeConversations {
	return &fakeConversations{
		conversations: map[uuid.UUID]*conversation.Conversation{},
		messages:      map[uuid.UUID]*conversation.Message{},
	}
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

func (f *fakeConversations) CreateConversation(_ context.Context, businessID uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	custID, _ := uuid.Parse(in.CustomerID)
	var ident *uuid.UUID
	if in.CustomerIdentityID != nil {
		id, _ := uuid.Parse(*in.CustomerIdentityID)
		ident = &id
	}
	for _, conv := range f.conversations {
		if conv.BusinessID == businessID && conv.CustomerID == custID && string(conv.Channel) == in.Channel && conv.Status == conversation.StatusOpen && sameIdent(conv.CustomerIdentityID, ident) {
			copy := *conv
			return &copy, false, nil
		}
	}
	item := &conversation.Conversation{
		ID: uuid.New(), BusinessID: businessID, CustomerID: custID, Channel: conversation.Channel(in.Channel),
		CustomerIdentityID: ident, Status: conversation.StatusOpen, HandlingState: conversation.HandlingAIActive,
	}
	if ident != nil {
		item.Identity = &conversation.IdentitySummary{ID: *ident, Channel: conversation.Channel(in.Channel)}
	}
	f.conversations[item.ID] = item
	copy := *item
	return &copy, true, nil
}

func (f *fakeConversations) LatestConversation(_ context.Context, businessID, customerID uuid.UUID, channelRaw string, identityID *uuid.UUID) (*conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *conversation.Conversation
	for _, conv := range f.conversations {
		if conv.BusinessID != businessID || conv.CustomerID != customerID || string(conv.Channel) != channelRaw {
			continue
		}
		if !sameIdent(conv.CustomerIdentityID, identityID) {
			continue
		}
		if best == nil || conv.UpdatedAt.After(best.UpdatedAt) {
			copy := *conv
			best = &copy
		}
	}
	return best, nil
}

func (f *fakeConversations) CreateMessage(_ context.Context, businessID, conversationID uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	conv, ok := f.conversations[conversationID]
	if !ok || conv.BusinessID != businessID {
		return nil, false, conversation.ErrNotFound
	}
	if in.ExternalID != nil {
		for _, msg := range f.messages {
			if msg.BusinessID == businessID && msg.ExternalID != nil && *msg.ExternalID == *in.ExternalID {
				copy := *msg
				return &copy, false, nil
			}
		}
	}
	msg := &conversation.Message{
		ID: uuid.New(), BusinessID: businessID, ConversationID: conversationID,
		Direction: conversation.Direction(in.Direction), SenderType: conversation.SenderType(in.SenderType),
		MessageType: conversation.MessageType(in.MessageType), Content: in.Content, ExternalID: in.ExternalID,
		Status: conversation.MessageStatus(in.Status), CreatedAt: time.Now().UTC(),
	}
	if conv.Status == conversation.StatusClosed {
		conv.Status = conversation.StatusOpen
	}
	f.messages[msg.ID] = msg
	copy := *msg
	return &copy, true, nil
}

func (f *fakeConversations) GetMessage(_ context.Context, businessID, messageID uuid.UUID) (*conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msg, ok := f.messages[messageID]
	if !ok || msg.BusinessID != businessID {
		return nil, conversation.ErrMessageNotFound
	}
	copy := *msg
	return &copy, nil
}

func (f *fakeConversations) FindMessageByExternalID(_ context.Context, businessID uuid.UUID, externalID string) (*conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, msg := range f.messages {
		if msg.BusinessID == businessID && msg.ExternalID != nil && *msg.ExternalID == externalID {
			copy := *msg
			return &copy, nil
		}
	}
	return nil, nil
}

func (f *fakeConversations) ClaimForDelivery(_ context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msg, ok := f.messages[messageID]
	if !ok || msg.BusinessID != businessID {
		return nil, conversation.ErrMessageNotFound
	}
	if msg.MessageType == conversation.MessageTypeInternalNote {
		return nil, nil
	}
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	switch msg.Status {
	case conversation.MessageQueued:
	case conversation.MessageSending:
		if !msg.UpdatedAt.IsZero() && time.Since(msg.UpdatedAt) < staleAfter {
			return nil, nil
		}
	default:
		return nil, nil
	}
	msg.Status = conversation.MessageSending
	msg.UpdatedAt = time.Now().UTC()
	copy := *msg
	return &copy, nil
}

func (f *fakeConversations) ApplyDeliveryStatus(_ context.Context, businessID, messageID uuid.UUID, statusRaw string, externalID *string) (*conversation.Message, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msg, ok := f.messages[messageID]
	if !ok || msg.BusinessID != businessID {
		return nil, false, conversation.ErrMessageNotFound
	}
	next, changed := applyRank(msg.Status, conversation.MessageStatus(statusRaw))
	if !changed {
		copy := *msg
		return &copy, false, nil
	}
	msg.Status = next
	if externalID != nil {
		msg.ExternalID = externalID
	}
	copy := *msg
	return &copy, true, nil
}

func applyRank(current, incoming conversation.MessageStatus) (conversation.MessageStatus, bool) {
	if incoming == current {
		return current, false
	}
	if current == conversation.MessageFailed || current == conversation.MessageRead {
		return current, false
	}
	switch incoming {
	case conversation.MessageFailed:
		if current == conversation.MessageQueued || current == conversation.MessageSending || current == conversation.MessageSent {
			return conversation.MessageFailed, true
		}
	case conversation.MessageQueued:
		if current == conversation.MessageSending {
			return conversation.MessageQueued, true
		}
	case conversation.MessageSending:
		if current == conversation.MessageQueued {
			return conversation.MessageSending, true
		}
	case conversation.MessageSent:
		if current == conversation.MessageQueued || current == conversation.MessageSending {
			return conversation.MessageSent, true
		}
	case conversation.MessageDelivered:
		if current == conversation.MessageQueued || current == conversation.MessageSending || current == conversation.MessageSent {
			return conversation.MessageDelivered, true
		}
	case conversation.MessageRead:
		if current == conversation.MessageQueued || current == conversation.MessageSending || current == conversation.MessageSent || current == conversation.MessageDelivered {
			return conversation.MessageRead, true
		}
	}
	return current, false
}

func sameIdent(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

type fakeAI struct {
	mu      sync.Mutex
	calls   int
	replyID uuid.UUID
	err     error
}

func (f *fakeAI) GenerateReply(_ context.Context, _ uuid.UUID, _ ai.GenerateReplyInput) (*ai.GenerateReplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.replyID == uuid.Nil {
		f.replyID = uuid.New()
	}
	return &ai.GenerateReplyResult{Reply: "ok", MessageID: f.replyID}, nil
}

type fakeSender struct {
	mu        sync.Mutex
	calls     int
	readCalls int
	last      provider.SendTextInput
	lastRead  provider.MarkReadInput
	id        string
	err       error
	readErr   error
	seenTok   bool
}

func (f *fakeSender) SendText(_ context.Context, input provider.SendTextInput) (provider.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = input
	f.seenTok = input.AccessToken != ""
	if f.err != nil {
		return provider.SendResult{}, f.err
	}
	if f.id == "" {
		f.id = "wamid.1"
	}
	return provider.SendResult{ProviderMessageID: f.id}, nil
}

func (f *fakeSender) SendTemplate(_ context.Context, input provider.SendTemplateInput) (provider.SendResult, error) {
	return f.SendText(context.Background(), provider.SendTextInput{
		PhoneNumberID: input.PhoneNumberID, To: input.To, Body: input.TemplateName,
		AccessToken: input.AccessToken, BaseURL: input.BaseURL, APIVersion: input.APIVersion,
	})
}

func (f *fakeSender) MarkRead(_ context.Context, input provider.MarkReadInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastRead = input
	f.readCalls++
	return f.readErr
}

func newTestService(store Store, customers CustomerAPI, convs ConversationAPI, ai AIService, sender provider.MessagingProvider) *Service {
	svc := NewService(testLogger(), store, customers, convs, ai, sender, MetaConfig{
		AccessToken: "token", AppSecret: "secret", VerifyToken: "verify", APIVersion: "v21.0", BaseURL: "https://graph.facebook.com", Timeout: time.Second,
	})
	svc.SetTrigger(SyncTrigger{Service: svc})
	return svc
}
