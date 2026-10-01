package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubKnowledge struct {
	available bool
	reason    string
	results   []KnowledgeHit
	err       error
	lastBiz   uuid.UUID
	lastQuery string
}

func (s *stubKnowledge) SearchKnowledge(_ context.Context, businessID uuid.UUID, query string, _ int) (*KnowledgeSearchResult, error) {
	s.lastBiz = businessID
	s.lastQuery = query
	if s.err != nil {
		return nil, s.err
	}
	return &KnowledgeSearchResult{Available: s.available, Reason: s.reason, Results: s.results}, nil
}

func TestSearchKnowledgeTool(t *testing.T) {
	biz := uuid.New()
	tool := SearchKnowledge{Search: &stubKnowledge{
		available: true,
		results: []KnowledgeHit{{
			DocumentID: uuid.NewString(), Title: "Cancellation Policy", Content: "Cancel before 17:00.", Similarity: 0.82,
		}},
	}}
	schema, _ := json.Marshal(tool.InputSchema())
	if _, err := Validate(tool.InputSchema(), json.RawMessage(`{"query":"cancel"}`)); err != nil {
		t.Fatalf("schema: %v raw=%s", err, schema)
	}
	if _, err := Validate(tool.InputSchema(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("query required")
	}

	result, err := tool.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{"query": "cancel policy"})
	if err != nil || result.Error != "" {
		t.Fatalf("relevant = %+v err=%v", result, err)
	}
	data, _ := result.Data.(map[string]any)
	if data["available"] != true {
		t.Fatalf("available = %+v", data)
	}

	none := SearchKnowledge{Search: &stubKnowledge{available: true, results: nil}}
	result, err = none.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{"query": "unknown topic"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = result.Data.(map[string]any)
	results, _ := data["results"].([]map[string]any)
	if results == nil {
		if raw, ok := data["results"].([]any); !ok || len(raw) != 0 {
			t.Fatalf("empty results = %+v", data)
		}
	}

	disabled := SearchKnowledge{}
	result, err = disabled.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{"query": "hours"})
	if err != nil || result.Error != "" {
		t.Fatalf("disabled = %+v err=%v", result, err)
	}

	failing := SearchKnowledge{Search: &stubKnowledge{err: errors.New("provider down")}}
	result, err = failing.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{"query": "hours"})
	if err != nil || result.Error == "" {
		t.Fatalf("provider failure = %+v err=%v", result, err)
	}

	injected := uuid.New()
	searcher := &stubKnowledge{available: true}
	tool = SearchKnowledge{Search: searcher}
	_, _ = tool.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{
		"query": "x", "business_id": injected.String(),
	})
	if searcher.lastBiz != biz {
		t.Fatalf("tool used injected tenant %s", searcher.lastBiz)
	}

	result, err = tool.Execute(context.Background(), ExecContext{}, map[string]any{"query": "x"})
	if err != nil || result.Code != "AI_CONTEXT_ERROR" {
		t.Fatalf("missing business = %+v err=%v", result, err)
	}
	result, err = tool.Execute(context.Background(), ExecContext{BusinessID: biz}, map[string]any{"query": "  "})
	if err != nil || result.Code != "AI_INVALID_ARGUMENTS" {
		t.Fatalf("invalid query = %+v err=%v", result, err)
	}
}
