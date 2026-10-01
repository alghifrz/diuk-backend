package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/conversation"
)

func newTestService(store Store, llm provider.LLMProvider, biz fakeBusiness, customers fakeCustomers, menus fakeMenu, convs *fakeConversations, reservations *fakeReservations) *Service {
	return NewService(testLogger(), store, Defaults{Provider: ProviderOpenAI, Model: "gpt-4o-mini", APIKey: "test-key", Timeout: 0}, llm, nil, biz, customers, menus, convs, reservations, nil)
}

func TestDefaultRegistryIncludesSearchKnowledge(t *testing.T) {
	reg := DefaultRegistry(nil, nil, nil, nil, nil, nil)
	if _, err := reg.Get("search_knowledge"); err != nil {
		t.Fatalf("search_knowledge missing: %v", err)
	}
	if _, err := reg.Get("get_business_info"); err != nil {
		t.Fatalf("structured tool missing: %v", err)
	}
	if _, err := reg.Get("request_human_handover"); err != nil {
		t.Fatalf("request_human_handover missing: %v", err)
	}
}

func TestPromptVersioningAndActivation(t *testing.T) {
	store := newMemoryStore()
	biz, customers, menus, convs, reservations := sampleWorld(uuid.New(), uuid.New(), uuid.New())
	svc := newTestService(store, &scriptedLLM{}, biz, customers, menus, convs, reservations)
	businessID := biz.profile.ID
	userID := uuid.New()

	first, err := svc.CreatePrompt(context.Background(), businessID, userID, CreatePromptInput{
		Name: "Customer_Service", SystemPrompt: "You are helpful.",
	})
	if err != nil || first.Version != 1 || first.Status != PromptDraft || first.Name != "customer_service" {
		t.Fatalf("create first = %+v err=%v", first, err)
	}
	activated, err := svc.ActivatePrompt(context.Background(), businessID, first.ID)
	if err != nil || activated.Status != PromptActive {
		t.Fatalf("activate first = %+v err=%v", activated, err)
	}

	second, err := svc.CreatePrompt(context.Background(), businessID, userID, CreatePromptInput{
		Name: "customer_service", SystemPrompt: "You are concise.",
	})
	if err != nil || second.Version != 2 || second.Status != PromptDraft {
		t.Fatalf("create second = %+v err=%v", second, err)
	}
	if _, err := svc.UpdatePrompt(context.Background(), businessID, second.ID, UpdatePromptInput{
		SystemPrompt: ptr("You are concise and polite."),
	}); err != nil {
		t.Fatalf("edit draft: %v", err)
	}
	if _, err := svc.UpdatePrompt(context.Background(), businessID, first.ID, UpdatePromptInput{
		SystemPrompt: ptr("rewrite history"),
	}); !errors.Is(err, ErrPromptNotDraft) {
		t.Fatalf("active edit = %v", err)
	}

	secondActive, err := svc.ActivatePrompt(context.Background(), businessID, second.ID)
	if err != nil || secondActive.Status != PromptActive {
		t.Fatalf("activate second = %+v err=%v", secondActive, err)
	}
	archived, err := svc.GetPrompt(context.Background(), businessID, first.ID)
	if err != nil || archived.Status != PromptArchived {
		t.Fatalf("previous active = %+v err=%v", archived, err)
	}
	actives, err := svc.ListPrompts(context.Background(), businessID, "customer_service", PromptActive)
	if err != nil || len(actives) != 1 || actives[0].ID != second.ID {
		t.Fatalf("one active = %+v err=%v", actives, err)
	}

	if _, err := store.CreatePrompt(context.Background(), Prompt{
		BusinessID: businessID, Name: "customer_service", SystemPrompt: "dup", Version: 2, Status: PromptDraft,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate version = %v", err)
	}

	deleted, err := svc.DeletePrompt(context.Background(), businessID, second.ID)
	if err != nil || deleted.Status != PromptArchived {
		t.Fatalf("delete = %+v err=%v", deleted, err)
	}
	other := uuid.New()
	if _, err := svc.GetPrompt(context.Background(), other, first.ID); !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}
}

func TestSettingsValidationAndIsolation(t *testing.T) {
	store := newMemoryStore()
	a, b := uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(a, uuid.New(), uuid.New())
	svc := newTestService(store, &scriptedLLM{}, biz, customers, menus, convs, reservations)

	got, err := svc.GetSettings(context.Background(), a)
	if err != nil || !got.Enabled || got.Provider != ProviderOpenAI {
		t.Fatalf("defaults = %+v err=%v", got, err)
	}
	temp := 0.4
	tokens := 512
	updated, err := svc.UpdateSettings(context.Background(), a, UpdateSettingsInput{
		Provider: ptr(ProviderGroq), Model: ptr("llama-3.1-70b"), Temperature: &temp, MaxOutputTokens: &tokens,
	})
	if err != nil || updated.Provider != ProviderGroq || *updated.Temperature != 0.4 {
		t.Fatalf("update = %+v err=%v", updated, err)
	}
	if _, err := svc.UpdateSettings(context.Background(), a, UpdateSettingsInput{Provider: ptr("unknown")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid provider = %v", err)
	}
	badTemp := 2.5
	if _, err := svc.UpdateSettings(context.Background(), a, UpdateSettingsInput{Temperature: &badTemp}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid temperature = %v", err)
	}
	badTokens := 0
	if _, err := svc.UpdateSettings(context.Background(), a, UpdateSettingsInput{MaxOutputTokens: &badTokens}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid tokens = %v", err)
	}
	hugeTokens := 999999999
	if _, err := svc.UpdateSettings(context.Background(), a, UpdateSettingsInput{MaxOutputTokens: &hugeTokens}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("pathological tokens = %v", err)
	}

	other, err := svc.GetSettings(context.Background(), b)
	if err != nil || other.Provider != ProviderOpenAI || other.Model != "gpt-4o-mini" {
		t.Fatalf("other tenant defaults = %+v err=%v", other, err)
	}
}

func TestContextBuilder(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	older := "first"
	newer := "latest"
	convs.messages = []conversation.Message{
		{ID: uuid.New(), BusinessID: businessID, ConversationID: conversationID, SenderType: conversation.SenderCustomer, MessageType: conversation.MessageTypeText, Content: &older},
		{ID: uuid.New(), BusinessID: businessID, ConversationID: conversationID, SenderType: conversation.SenderAI, MessageType: conversation.MessageTypeText, Content: &newer},
		{ID: uuid.New(), BusinessID: businessID, ConversationID: conversationID, SenderType: conversation.SenderSystem, MessageType: conversation.MessageTypeSystem, Content: ptr("ignore")},
	}
	svc := newTestService(store, &scriptedLLM{}, biz, customers, menus, convs, reservations)

	runtime, err := svc.BuildContext(context.Background(), businessID, conversationID, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !containsFold(runtime.BusinessBlock, "Kasiko") || !containsFold(runtime.CustomerBlock, "Budi") {
		t.Fatalf("context missing facts: %+v", runtime)
	}
	if len(runtime.History) != 3 {
		t.Fatalf("history len = %d", len(runtime.History))
	}
	if messageText(runtime.History[0]) != "first" || messageText(runtime.History[1]) != "latest" {
		t.Fatalf("history order = %+v", runtime.History)
	}
	if _, err := svc.BuildContext(context.Background(), uuid.New(), conversationID, ""); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("tenant isolation = %v", err)
	}
}
