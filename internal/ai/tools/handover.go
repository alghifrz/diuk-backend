package tools

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
)

// RequestHumanHandover moves the conversation into HUMAN_REQUESTED so staff
// see it in the human inbox and AI replies stop.
type RequestHumanHandover struct {
	Handover HandoverAPI
}

func (t RequestHumanHandover) Name() string { return "request_human_handover" }

func (t RequestHumanHandover) Description() string {
	return "Hand the conversation to human staff (HUMAN_REQUESTED inbox). Do NOT call this for the booking path — create_reservation already hands over after recording the reservation. Only call when the customer asks for a human, is upset, tools fail, or information is unavailable. Never call this instead of quote_reservation / create_reservation."
}

func (t RequestHumanHandover) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"reason": stringSchema("CUSTOMER_REQUESTED_HUMAN, AI_UNABLE_TO_ANSWER, or MANUAL_HANDOFF (use MANUAL_HANDOFF after payment booking)", ""),
	}, nil)
}

func (t RequestHumanHandover) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	return requestHandover(ctx, t.Handover, exec, stringArg(input, "reason"))
}

func requestHandover(ctx context.Context, api HandoverAPI, exec ExecContext, reasonRaw string) (Result, error) {
	if api == nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "handover is unavailable")
	}
	if exec.ConversationID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "conversation context is missing")
	}
	reason := strings.TrimSpace(strings.ToUpper(reasonRaw))
	if reason == "" {
		reason = string(conversation.ReasonManualHandoff)
	}
	updated, err := api.RequestHumanHandover(ctx, exec.BusinessID, exec.ConversationID, reason)
	if err != nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "could not request human handover")
	}
	state := string(conversation.HandlingAIActive)
	if updated != nil {
		state = string(updated.HandlingState)
	}
	return success(map[string]any{
		"handover_requested":    true,
		"handling_state":        state,
		"reason":                reason,
		"customer_message_hint": "Tell the customer briefly that a team member will continue helping with payment. Do not discuss payment links or amounts beyond what tools already returned.",
	})
}
