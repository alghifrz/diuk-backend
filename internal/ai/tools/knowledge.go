package tools

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type SearchKnowledge struct {
	Search KnowledgeSearch
}

func (t SearchKnowledge) Name() string { return "search_knowledge" }

func (t SearchKnowledge) Description() string {
	return "Search the business's uploaded documents: address and location, opening hours, facilities, FAQ, policies, cancellation rules, and service descriptions. Also use it when get_business_info has an empty field. Do not use this for live availability, reservations, current menu prices, or customer lookup. If no relevant result is returned, say the information is unavailable rather than inventing it."
}

func (t SearchKnowledge) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"query": stringSchema("The customer question or topic to search in the tenant knowledge base", ""),
	}, []string{"query"})
}

func (t SearchKnowledge) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	if exec.BusinessID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "business context is missing")
	}
	query := strings.TrimSpace(stringArg(input, "query"))
	if query == "" {
		return fail("AI_INVALID_ARGUMENTS", "query is required")
	}
	if t.Search == nil {
		return success(map[string]any{
			"available": false,
			"reason":    "knowledge retrieval is not configured",
			"results":   []any{},
		})
	}
	out, err := t.Search.SearchKnowledge(ctx, exec.BusinessID, query, 0)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid") || strings.Contains(strings.ToLower(err.Error()), "too long") {
			return fail("AI_INVALID_ARGUMENTS", "query is invalid")
		}
		return fail("AI_TOOL_EXECUTION_FAILED", "knowledge retrieval is unavailable")
	}
	if out == nil {
		return success(map[string]any{"available": false, "reason": "knowledge retrieval is not configured", "results": []any{}})
	}
	if !out.Available {
		return success(map[string]any{
			"available": false,
			"reason":    emptyReason(out.Reason),
			"results":   []any{},
		})
	}
	results := make([]map[string]any, 0, len(out.Results))
	for _, hit := range out.Results {
		results = append(results, map[string]any{
			"document_id": hit.DocumentID,
			"title":       hit.Title,
			"content":     hit.Content,
			"similarity":  hit.Similarity,
		})
	}
	return success(map[string]any{"available": true, "results": results})
}

func emptyReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "no relevant knowledge was found"
	}
	return reason
}
