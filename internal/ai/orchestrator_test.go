package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

func TestGenerateSimpleReply(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "Meja masih ada, mau saya booking?"}}}
	svc := newTestService(store, llm, biz, customers, menus, convs, reservations)

	out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, UserID: uuid.New(), Message: "Ada meja jam 7?", PersistIncoming: true,
	})
	if err != nil || out.Reply != "Meja masih ada, mau saya booking?" || out.ToolCalls != 0 {
		t.Fatalf("generate = %+v err=%v", out, err)
	}
	if len(convs.messages) != 2 {
		t.Fatalf("persisted messages = %d", len(convs.messages))
	}
	if convs.messages[0].SenderType != conversation.SenderCustomer || convs.messages[1].SenderType != conversation.SenderAI {
		t.Fatalf("senders = %+v", convs.messages)
	}
	if convs.messages[1].Direction != conversation.DirectionOutbound || convs.messages[1].Status != conversation.MessageQueued {
		t.Fatalf("ai message = %+v", convs.messages[1])
	}
}

func TestGenerateOneToolCall(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	llm := &scriptedLLM{steps: []provider.GenerateResponse{
		{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "get_business_info", Arguments: []byte(`{}`)}}},
		{Content: "Kami buka dengan reservasi aktif."},
	}}
	svc := newTestService(store, llm, biz, customers, menus, convs, reservations)

	out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, Message: "Info bisnis?", PersistIncoming: true,
	})
	if err != nil || out.ToolRounds != 1 || out.ToolCalls != 1 {
		t.Fatalf("generate = %+v err=%v", out, err)
	}
	if out.Reply != "Kami buka dengan reservasi aktif." {
		t.Fatalf("reply = %q", out.Reply)
	}
}

func TestGenerateMultipleToolCallsAndFailure(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	reservations.availErr = errors.New("database boom sql: fail")
	llm := &scriptedLLM{steps: []provider.GenerateResponse{
		{ToolCalls: []provider.ToolCall{
			{ID: "a", Name: "get_menu", Arguments: []byte(`{}`)},
			{ID: "b", Name: "check_availability", Arguments: jsonArgs(map[string]any{"date": "2026-10-01", "start_time": "19:00", "duration_minutes": 90, "party_size": 4})},
		}},
		{Content: "Menu ada, ketersediaan sedang dicek ulang."},
	}}
	svc := newTestService(store, llm, biz, customers, menus, convs, reservations)

	out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, Message: "Menu dan meja?", PersistIncoming: false,
	})
	if err != nil || out.ToolCalls != 2 {
		t.Fatalf("generate = %+v err=%v", out, err)
	}
	if reservations.availCalls != 1 {
		t.Fatalf("availability calls = %d", reservations.availCalls)
	}
	last := llm.requests[1]
	foundToolError := false
	for _, msg := range last.Messages {
		if msg.Role == provider.RoleTool && containsFold(msg.Content, "reservation action failed") {
			foundToolError = true
		}
		if containsFold(msg.Content, "sql:") || containsFold(msg.Content, "database boom") {
			t.Fatalf("raw database error leaked: %s", msg.Content)
		}
	}
	if !foundToolError {
		t.Fatal("expected controlled tool failure")
	}
}

func TestGenerateUnknownToolAndLimits(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)

	t.Run("unknown tool", func(t *testing.T) {
		llm := &scriptedLLM{steps: []provider.GenerateResponse{
			{ToolCalls: []provider.ToolCall{{ID: "x", Name: "drop_database", Arguments: []byte(`{}`)}}},
			{Content: "Saya tidak bisa menjalankan tool itu."},
		}}
		svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
		out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{ConversationID: conversationID, Message: "hack", PersistIncoming: false})
		if err != nil || out.Reply == "" {
			t.Fatalf("unknown tool = %+v err=%v", out, err)
		}
		if !containsFold(llm.requests[1].Messages[len(llm.requests[1].Messages)-1].Content, "unknown tool") {
			t.Fatalf("unknown tool result = %+v", llm.requests[1].Messages)
		}
	})

	t.Run("tool rounds", func(t *testing.T) {
		steps := make([]provider.GenerateResponse, 0, MaxToolRounds+1)
		for i := 0; i < MaxToolRounds+1; i++ {
			steps = append(steps, provider.GenerateResponse{ToolCalls: []provider.ToolCall{{
				ID: uuid.NewString(), Name: "get_business_info", Arguments: []byte(`{}`),
			}}})
		}
		svc := newTestService(store, &scriptedLLM{steps: steps}, biz, customers, menus, convs, reservations)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{ConversationID: conversationID, Message: "loop", PersistIncoming: false}); !errors.Is(err, ErrToolLimit) {
			t.Fatalf("round limit = %v", err)
		}
	})

	t.Run("tool calls", func(t *testing.T) {
		calls := make([]provider.ToolCall, 0, MaxToolCalls+1)
		for i := 0; i < MaxToolCalls+1; i++ {
			calls = append(calls, provider.ToolCall{ID: uuid.NewString(), Name: "get_business_info", Arguments: []byte(`{}`)})
		}
		svc := newTestService(store, &scriptedLLM{steps: []provider.GenerateResponse{{ToolCalls: calls}}}, biz, customers, menus, convs, reservations)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{ConversationID: conversationID, Message: "storm", PersistIncoming: false}); !errors.Is(err, ErrToolLimit) {
			t.Fatalf("call limit = %v", err)
		}
	})
}

func TestGenerateProviderAndDisabled(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)

	t.Run("provider error", func(t *testing.T) {
		svc := newTestService(store, &scriptedLLM{errs: []error{provider.ErrRequest}}, biz, customers, menus, convs, reservations)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{ConversationID: conversationID, Message: "hi", PersistIncoming: false}); !errors.Is(err, ErrProvider) {
			t.Fatalf("provider = %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		svc := newTestService(store, &scriptedLLM{steps: []provider.GenerateResponse{{Content: "hi"}}}, biz, customers, menus, convs, reservations)
		if _, err := svc.UpdateSettings(context.Background(), businessID, UpdateSettingsInput{Enabled: ptr(false)}); err != nil {
			t.Fatalf("disable: %v", err)
		}
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{ConversationID: conversationID, Message: "hi", PersistIncoming: false}); !errors.Is(err, ErrDisabled) {
			t.Fatalf("disabled = %v", err)
		}
	})
}

func TestGenerateReplyRespectsHandlingState(t *testing.T) {
	for _, state := range []conversation.HandlingState{
		conversation.HandlingHumanRequested, conversation.HandlingHumanActive, conversation.HandlingResolved,
	} {
		t.Run(string(state), func(t *testing.T) {
			store := newMemoryStore()
			businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
			biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
			convs.conversations[conversationID].HandlingState = state
			llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "should not send"}}}
			svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
			if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
				ConversationID: conversationID, Message: "help", PersistIncoming: false,
			}); !errors.Is(err, ErrHumanHandling) {
				t.Fatalf("state %s = %v", state, err)
			}
			if len(llm.requests) != 0 {
				t.Fatalf("LLM was called for %s", state)
			}
			for _, msg := range convs.messages {
				if msg.SenderType == conversation.SenderAI {
					t.Fatalf("AI outbound persisted for %s", state)
				}
			}
		})
	}

	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	convs.conversations[conversationID].HandlingState = conversation.HandlingAIActive
	svc := newTestService(store, &scriptedLLM{steps: []provider.GenerateResponse{{Content: "ok"}}}, biz, customers, menus, convs, reservations)
	out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, Message: "hi", PersistIncoming: false,
	})
	if err != nil || out.Reply != "ok" {
		t.Fatalf("AI_ACTIVE generate = %+v err=%v", out, err)
	}
}

func TestGenerateQuotaAndMetering(t *testing.T) {
	t.Run("quota exceeded does not call provider", func(t *testing.T) {
		store := newMemoryStore()
		businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
		biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
		llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "should not send"}}}
		svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
		svc.SetUsageMeter(&scriptedMeter{checkErr: usage.ErrQuotaExceeded})
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "hi", PersistIncoming: false, UsageSourceID: uuid.New(),
		}); !errors.Is(err, ErrQuotaExceeded) {
			t.Fatalf("quota = %v", err)
		}
		if len(llm.requests) != 0 {
			t.Fatal("LLM was called after quota exceeded")
		}
		for _, msg := range convs.messages {
			if msg.SenderType == conversation.SenderAI {
				t.Fatal("AI outbound persisted after quota exceeded")
			}
		}
	})

	t.Run("successful generation consumes once and retry is idempotent", func(t *testing.T) {
		store := newMemoryStore()
		businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
		biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
		meter := &scriptedMeter{}
		llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "halo"}}}
		svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
		svc.SetUsageMeter(meter)
		source := uuid.New()
		out, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "hi", PersistIncoming: false, UsageSourceID: source,
		})
		if err != nil || out.Reply != "halo" {
			t.Fatalf("generate = %+v err=%v", out, err)
		}
		if meter.consumeCalls != 1 || meter.checkCalls != 1 {
			t.Fatalf("meter calls check=%d consume=%d", meter.checkCalls, meter.consumeCalls)
		}
		retry, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "again", PersistIncoming: false, UsageSourceID: source,
		})
		if err != nil || retry.MessageID != out.MessageID {
			t.Fatalf("retry = %+v err=%v", retry, err)
		}
		if meter.consumeCalls != 1 || meter.checkCalls != 1 {
			t.Fatalf("retry must not remeter check=%d consume=%d", meter.checkCalls, meter.consumeCalls)
		}
		aiCount := 0
		for _, msg := range convs.messages {
			if msg.SenderType == conversation.SenderAI {
				aiCount++
			}
		}
		if aiCount != 1 {
			t.Fatalf("ai messages = %d", aiCount)
		}
	})

	t.Run("provider failure consumes zero", func(t *testing.T) {
		store := newMemoryStore()
		businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
		biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
		meter := &scriptedMeter{}
		svc := newTestService(store, &scriptedLLM{errs: []error{provider.ErrRequest}}, biz, customers, menus, convs, reservations)
		svc.SetUsageMeter(meter)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "hi", PersistIncoming: false, UsageSourceID: uuid.New(),
		}); !errors.Is(err, ErrProvider) {
			t.Fatalf("provider = %v", err)
		}
		if meter.consumeCalls != 0 {
			t.Fatalf("failed generation consumed %d", meter.consumeCalls)
		}
	})

	t.Run("human handling consumes zero", func(t *testing.T) {
		store := newMemoryStore()
		businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
		biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
		convs.conversations[conversationID].HandlingState = conversation.HandlingHumanActive
		meter := &scriptedMeter{}
		llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "no"}}}
		svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
		svc.SetUsageMeter(meter)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "hi", PersistIncoming: false, UsageSourceID: uuid.New(),
		}); !errors.Is(err, ErrHumanHandling) {
			t.Fatalf("human = %v", err)
		}
		if meter.checkCalls != 0 || meter.consumeCalls != 0 || len(llm.requests) != 0 {
			t.Fatalf("handover metered or generated check=%d consume=%d llm=%d", meter.checkCalls, meter.consumeCalls, len(llm.requests))
		}
	})

	t.Run("final unit race is enforced at consume", func(t *testing.T) {
		store := newMemoryStore()
		businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
		biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
		meter := &scriptedMeter{consumeErr: usage.ErrQuotaExceeded}
		llm := &scriptedLLM{steps: []provider.GenerateResponse{{Content: "late"}}}
		svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
		svc.SetUsageMeter(meter)
		if _, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
			ConversationID: conversationID, Message: "hi", PersistIncoming: false, UsageSourceID: uuid.New(),
		}); !errors.Is(err, ErrQuotaExceeded) {
			t.Fatalf("final consume = %v", err)
		}
		for _, msg := range convs.messages {
			if msg.SenderType == conversation.SenderAI {
				t.Fatal("unmetered AI response persisted")
			}
		}
	})
}

func TestGenerateReplyOncePerInbound(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	llm := &scriptedLLM{steps: []provider.GenerateResponse{
		{Content: "first"},
		{Content: "second"},
	}}
	svc := newTestService(store, llm, biz, customers, menus, convs, reservations)
	inbound := uuid.New()

	first, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, PersistIncoming: false, UsageSourceID: inbound,
	})
	if err != nil || first.Reply != "first" {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	again, err := svc.GenerateReply(context.Background(), businessID, GenerateReplyInput{
		ConversationID: conversationID, PersistIncoming: false, UsageSourceID: inbound,
	})
	if err != nil || again.MessageID != first.MessageID || again.Reply != "first" {
		t.Fatalf("retry = %+v err=%v", again, err)
	}
	aiCount := 0
	for _, msg := range convs.messages {
		if msg.SenderType == conversation.SenderAI {
			aiCount++
		}
	}
	if aiCount != 1 {
		t.Fatalf("ai messages = %d", aiCount)
	}
}

func TestGenerateTenantIsolation(t *testing.T) {
	store := newMemoryStore()
	businessID, customerID, conversationID := uuid.New(), uuid.New(), uuid.New()
	biz, customers, menus, convs, reservations := sampleWorld(businessID, customerID, conversationID)
	svc := newTestService(store, &scriptedLLM{steps: []provider.GenerateResponse{{Content: "ok"}}}, biz, customers, menus, convs, reservations)
	if _, err := svc.GenerateReply(context.Background(), uuid.New(), GenerateReplyInput{
		ConversationID: conversationID, Message: "hi", PersistIncoming: false,
	}); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("cross-tenant conversation = %v", err)
	}
}
