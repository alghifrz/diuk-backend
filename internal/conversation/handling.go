package conversation

import "strings"

func AllowsAIGeneration(state HandlingState) bool {
	return normalizeHandling(state) == HandlingAIActive
}

func normalizeHandling(state HandlingState) HandlingState {
	if state == "" {
		return HandlingAIActive
	}
	return state
}

func canTransition(from, to HandlingState) bool {
	from = normalizeHandling(from)
	to = normalizeHandling(to)
	if from == to {
		return true
	}
	switch from {
	case HandlingAIActive:
		return to == HandlingHumanRequested
	case HandlingHumanRequested:
		return to == HandlingHumanActive || to == HandlingResolved || to == HandlingAIActive
	case HandlingHumanActive:
		return to == HandlingResolved || to == HandlingHumanRequested
	case HandlingResolved:
		return to == HandlingAIActive
	default:
		return false
	}
}

func parseHandlingState(value string) (HandlingState, error) {
	state := HandlingState(strings.ToUpper(strings.TrimSpace(value)))
	switch state {
	case HandlingAIActive, HandlingHumanRequested, HandlingHumanActive, HandlingResolved:
		return state, nil
	default:
		return "", invalid("handling_state is invalid")
	}
}

func parseHandoverReason(value string) (HandoverReason, error) {
	reason := HandoverReason(strings.ToUpper(strings.TrimSpace(value)))
	switch reason {
	case ReasonCustomerRequested, ReasonAIUnableToAnswer, ReasonManualHandoff:
		return reason, nil
	default:
		return "", invalid("handover_reason is invalid")
	}
}

func parseHumanInboxStates(raw string) ([]HandlingState, error) {
	if strings.TrimSpace(raw) == "" {
		return []HandlingState{HandlingHumanRequested, HandlingHumanActive}, nil
	}
	state, err := parseHandlingState(raw)
	if err != nil {
		return nil, err
	}
	switch state {
	case HandlingHumanRequested, HandlingHumanActive, HandlingResolved:
		return []HandlingState{state}, nil
	default:
		return nil, invalid("human inbox state is invalid")
	}
}
