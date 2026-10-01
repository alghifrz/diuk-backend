package conversation

import "testing"

func TestAllowsAIGeneration(t *testing.T) {
	if !AllowsAIGeneration("") || !AllowsAIGeneration(HandlingAIActive) {
		t.Fatal("AI_ACTIVE must allow generation")
	}
	for _, state := range []HandlingState{HandlingHumanRequested, HandlingHumanActive, HandlingResolved} {
		if AllowsAIGeneration(state) {
			t.Fatalf("%s must suppress AI", state)
		}
	}
}

func TestHandlingTransitions(t *testing.T) {
	allowed := [][2]HandlingState{
		{HandlingAIActive, HandlingHumanRequested},
		{HandlingHumanRequested, HandlingHumanActive},
		{HandlingHumanRequested, HandlingResolved},
		{HandlingHumanRequested, HandlingAIActive},
		{HandlingHumanActive, HandlingResolved},
		{HandlingHumanActive, HandlingHumanRequested},
		{HandlingResolved, HandlingAIActive},
	}
	for _, pair := range allowed {
		if !canTransition(pair[0], pair[1]) {
			t.Fatalf("expected %s -> %s", pair[0], pair[1])
		}
	}
	blocked := [][2]HandlingState{
		{HandlingAIActive, HandlingHumanActive},
		{HandlingAIActive, HandlingResolved},
		{HandlingResolved, HandlingHumanActive},
		{HandlingResolved, HandlingHumanRequested},
		{HandlingHumanActive, HandlingAIActive},
	}
	for _, pair := range blocked {
		if canTransition(pair[0], pair[1]) {
			t.Fatalf("blocked %s -> %s", pair[0], pair[1])
		}
	}
}

func TestParseHumanInboxStates(t *testing.T) {
	defaults, err := parseHumanInboxStates("")
	if err != nil || len(defaults) != 2 || defaults[0] != HandlingHumanRequested || defaults[1] != HandlingHumanActive {
		t.Fatalf("default inbox = %v err=%v", defaults, err)
	}
	resolved, err := parseHumanInboxStates("RESOLVED")
	if err != nil || len(resolved) != 1 || resolved[0] != HandlingResolved {
		t.Fatalf("resolved filter = %v err=%v", resolved, err)
	}
	if _, err := parseHumanInboxStates("AI_ACTIVE"); err == nil {
		t.Fatal("AI_ACTIVE must not be a human inbox filter")
	}
}
