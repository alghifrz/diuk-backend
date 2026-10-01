package knowledge

import (
	"context"
	"unicode/utf8"

	"github.com/google/uuid"
)

type SearchResult struct {
	Available bool
	Reason    string
	Results   []SearchHit
}

func (s *Service) SearchKnowledge(ctx context.Context, businessID uuid.UUID, query string, limit int) (*SearchResult, error) {
	return s.search(ctx, businessID, query, limit, s.settings.MinSimilarity)
}

func (s *Service) search(ctx context.Context, businessID uuid.UUID, query string, limit int, minSimilarity float64) (*SearchResult, error) {
	query, err := normalizeQuery(query)
	if err != nil {
		return nil, err
	}
	if !s.settings.Enabled || s.embedder == nil {
		return &SearchResult{Available: false, Reason: "knowledge retrieval is not configured", Results: []SearchHit{}}, nil
	}
	limit = normalizeTopK(limit, s.settings.TopK, DefaultMaxTopK)
	vectors, _, err := EmbedBatches(ctx, s.embedder, []string{query}, 1)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, ErrEmbeddingFailed
	}
	hits, err := s.store.SearchChunks(ctx, businessID, vectors[0], minSimilarity, limit)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].Content = truncateRunes(hits[i].Content, s.settings.MaxResultChars)
		hits[i].Metadata = nil
	}
	if hits == nil {
		hits = []SearchHit{}
	}
	return &SearchResult{Available: true, Results: hits}, nil
}

func truncateRunes(text string, max int) string {
	if max < 1 || utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max])
}
