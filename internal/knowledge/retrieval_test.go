package knowledge

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRetrievalThresholdAndLimit(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	other := uuid.New()
	ready := &Document{ID: uuid.New(), BusinessID: biz, Title: "Policy", Status: StatusReady}
	pending := &Document{ID: uuid.New(), BusinessID: biz, Title: "Draft", Status: StatusPending}
	foreign := &Document{ID: uuid.New(), BusinessID: other, Title: "Other", Status: StatusReady}
	_, _ = store.CreateDocument(context.Background(), ready)
	_, _ = store.CreateDocument(context.Background(), pending)
	_, _ = store.CreateDocument(context.Background(), foreign)
	_ = store.ReplaceChunks(context.Background(), biz, ready.ID, []Chunk{
		{Embedding: []float32{1, 0, 0, 0}, Content: "high", ChunkIndex: 0},
		{Embedding: []float32{0.2, 0.8, 0, 0}, Content: "low", ChunkIndex: 1},
	})
	_ = store.ReplaceChunks(context.Background(), biz, pending.ID, []Chunk{
		{Embedding: []float32{1, 0, 0, 0}, Content: "pending", ChunkIndex: 0},
	})
	_ = store.ReplaceChunks(context.Background(), other, foreign.ID, []Chunk{
		{Embedding: []float32{1, 0, 0, 0}, Content: "foreign", ChunkIndex: 0},
	})

	hits, err := store.SearchChunks(context.Background(), biz, []float32{1, 0, 0, 0}, 0.7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != "high" {
		t.Fatalf("threshold hits = %+v", hits)
	}

	hits, err = store.SearchChunks(context.Background(), biz, []float32{1, 0, 0, 0}, 0.01, 1)
	if err != nil || len(hits) != 1 {
		t.Fatalf("top k = %+v err=%v", hits, err)
	}

	foreignHits, err := store.SearchChunks(context.Background(), other, []float32{1, 0, 0, 0}, 0.01, 10)
	if err != nil || len(foreignHits) != 1 || foreignHits[0].Content != "foreign" {
		t.Fatalf("tenant B = %+v err=%v", foreignHits, err)
	}
}
