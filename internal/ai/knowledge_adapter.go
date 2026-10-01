package ai

import (
	"context"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/tools"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
)

type knowledgeAdapter struct {
	svc *knowledge.Service
}

func NewKnowledgeSearch(svc *knowledge.Service) tools.KnowledgeSearch {
	if svc == nil {
		return nil
	}
	return knowledgeAdapter{svc: svc}
}

func (a knowledgeAdapter) SearchKnowledge(ctx context.Context, businessID uuid.UUID, query string, limit int) (*tools.KnowledgeSearchResult, error) {
	out, err := a.svc.SearchKnowledge(ctx, businessID, query, limit)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &tools.KnowledgeSearchResult{Available: false, Reason: "knowledge retrieval is not configured", Results: []tools.KnowledgeHit{}}, nil
	}
	hits := make([]tools.KnowledgeHit, 0, len(out.Results))
	for _, hit := range out.Results {
		hits = append(hits, tools.KnowledgeHit{
			DocumentID: hit.DocumentID.String(),
			Title:      hit.Title,
			Content:    hit.Content,
			Similarity: hit.Similarity,
		})
	}
	return &tools.KnowledgeSearchResult{Available: out.Available, Reason: out.Reason, Results: hits}, nil
}
