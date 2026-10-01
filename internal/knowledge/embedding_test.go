package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/alghifrz/diuk-backend/internal/ai/embedding"
)

func TestEmbedBatches(t *testing.T) {
	provider := &stubEmbedder{dim: 4}
	texts := []string{"one", "two", "three", "four", "five"}
	vectors, batches, err := EmbedBatches(context.Background(), provider, texts, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 5 || batches != 3 {
		t.Fatalf("vectors=%d batches=%d", len(vectors), batches)
	}
	if provider.calls != 3 {
		t.Fatalf("calls = %d", provider.calls)
	}

	if _, _, err := EmbedBatches(context.Background(), nil, texts, 2); !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("nil provider = %v", err)
	}
	provider.err = embedding.ErrRequest
	if _, _, err := EmbedBatches(context.Background(), provider, texts[:1], 2); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("retryable = %v", err)
	}
	provider.err = embedding.ErrNotConfigured
	if _, _, err := EmbedBatches(context.Background(), provider, texts[:1], 2); !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("disabled = %v", err)
	}
}

func TestEncodeVector(t *testing.T) {
	got := encodeVector([]float32{0.5, 1})
	if got != "[0.5,1.0]" && got != "[0.5,1]" {
		t.Fatalf("vector = %s", got)
	}
}
