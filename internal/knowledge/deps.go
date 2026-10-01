package knowledge

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	CreateDocument(ctx context.Context, doc *Document) (*Document, error)
	GetDocument(ctx context.Context, businessID, documentID uuid.UUID) (*Document, error)
	ListDocuments(ctx context.Context, businessID uuid.UUID, status string) ([]Document, error)
	UpdateDocumentMeta(ctx context.Context, businessID, documentID uuid.UUID, title string, description *string, updatedBy *uuid.UUID) (*Document, error)
	UpdateDocumentStorage(ctx context.Context, businessID, documentID uuid.UUID, path, mime string) (*Document, error)
	MarkPending(ctx context.Context, businessID, documentID uuid.UUID, updatedBy *uuid.UUID) (*Document, error)
	ReleaseProcessing(ctx context.Context, businessID, documentID uuid.UUID) error
	ClaimProcessing(ctx context.Context, businessID, documentID uuid.UUID, staleAfter time.Duration) (*Document, error)
	MarkReady(ctx context.Context, businessID, documentID uuid.UUID) (*Document, error)
	MarkFailed(ctx context.Context, businessID, documentID uuid.UUID, processingError string) (*Document, error)
	DeleteDocument(ctx context.Context, businessID, documentID uuid.UUID) error
	ReplaceChunks(ctx context.Context, businessID, documentID uuid.UUID, chunks []Chunk) error
	SearchChunks(ctx context.Context, businessID uuid.UUID, embedding []float32, minSimilarity float64, limit int) ([]SearchHit, error)
	CountChunks(ctx context.Context, businessID, documentID uuid.UUID) (int, error)
}

type ObjectStore interface {
	Upload(ctx context.Context, path string, contentType string, data io.Reader) error
	Download(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
}

type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// AnswerGenerator writes a short answer from supplied excerpts. The AI package
// supplies the implementation so this package stays free of provider types.
type AnswerGenerator interface {
	Answer(ctx context.Context, system, user string) (string, error)
}

type IngestEnqueuer interface {
	EnqueueKnowledgeIngest(ctx context.Context, businessID, documentID uuid.UUID) error
}
