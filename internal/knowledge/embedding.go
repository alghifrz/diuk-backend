package knowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/alghifrz/diuk-backend/internal/ai/embedding"
)

func EmbedBatches(ctx context.Context, provider EmbeddingProvider, texts []string, batchSize int) ([][]float32, int, error) {
	if provider == nil {
		return nil, 0, ErrEmbeddingNotConfigured
	}
	if batchSize < 1 {
		batchSize = DefaultEmbeddingBatchSize
	}
	out := make([][]float32, 0, len(texts))
	batches := 0
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := provider.Embed(ctx, texts[start:end])
		if err != nil {
			return nil, batches, mapEmbeddingError(err)
		}
		if len(vectors) != end-start {
			return nil, batches, fmt.Errorf("%w: batch size mismatch", ErrEmbeddingFailed)
		}
		out = append(out, vectors...)
		batches++
	}
	return out, batches, nil
}

func mapEmbeddingError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, embedding.ErrNotConfigured) {
		return ErrEmbeddingNotConfigured
	}
	if errors.Is(err, embedding.ErrPermanent) {
		return fmt.Errorf("%w: %s", ErrDocumentProcessingFailed, publicProcessingMessage(ErrEmbeddingFailed))
	}
	if errors.Is(err, embedding.ErrRequest) || errors.Is(err, embedding.ErrResponse) {
		return fmt.Errorf("%w: provider", ErrEmbeddingFailed)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: timeout", ErrEmbeddingFailed)
	}
	return fmt.Errorf("%w: %s", ErrEmbeddingFailed, publicProcessingMessage(err))
}

func encodeVector(values []float32) string {
	if len(values) == 0 {
		return "[]"
	}
	var b []byte
	b = append(b, '[')
	for i, v := range values {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, strconvFormatFloat(v)...)
	}
	b = append(b, ']')
	return string(b)
}

func strconvFormatFloat(v float32) string {
	return trimFloat(fmt.Sprintf("%.8f", v))
}

func trimFloat(s string) string {
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s += "0"
	}
	return s
}
