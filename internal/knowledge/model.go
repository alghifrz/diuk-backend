package knowledge

import (
	"time"

	"github.com/google/uuid"
)

const (
	SourceText = "TEXT"
	SourcePDF  = "PDF"
	SourceDOCX = "DOCX"

	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusReady      = "READY"
	StatusFailed     = "FAILED"
	StatusArchived   = "ARCHIVED"

	DefaultEmbeddingDimensions = 1536
	DefaultChunkSize           = 800
	DefaultChunkOverlap        = 120
	DefaultTopK                = 5
	DefaultMinSimilarity       = 0.35
	DefaultMaxFileSizeMB       = 10
	DefaultEmbeddingBatchSize  = 64
	DefaultMaxResultChars      = 1200
	DefaultMaxQueryRunes       = 2000
	DefaultMaxTitleRunes       = 200
	DefaultMaxDescriptionRunes = 2000
	DefaultMaxTopK             = 20
)

type Document struct {
	ID              uuid.UUID
	BusinessID      uuid.UUID
	Title           string
	Description     *string
	SourceType      string
	SourceURL       *string
	StoragePath     *string
	MimeType        *string
	Status          string
	Version         int
	ProcessingError *string
	CreatedBy       *uuid.UUID
	UpdatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Chunk struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	DocumentID uuid.UUID
	ChunkIndex int
	Content    string
	Embedding  []float32
	TokenCount *int
	Metadata   map[string]any
	CreatedAt  time.Time
}

type SearchHit struct {
	ChunkID    uuid.UUID
	DocumentID uuid.UUID
	Title      string
	Content    string
	Similarity float64
	Metadata   map[string]any
}

type Settings struct {
	Enabled             bool
	ChunkSize           int
	ChunkOverlap        int
	TopK                int
	MinSimilarity       float64
	MaxFileBytes        int64
	EmbeddingModel      string
	EmbeddingDimensions int
	EmbeddingBatchSize  int
	MaxResultChars      int
	StaleProcessing     time.Duration
}

func DefaultSettings() Settings {
	return Settings{
		Enabled:             false,
		ChunkSize:           DefaultChunkSize,
		ChunkOverlap:        DefaultChunkOverlap,
		TopK:                DefaultTopK,
		MinSimilarity:       DefaultMinSimilarity,
		MaxFileBytes:        int64(DefaultMaxFileSizeMB) * 1024 * 1024,
		EmbeddingModel:      "text-embedding-3-small",
		EmbeddingDimensions: DefaultEmbeddingDimensions,
		EmbeddingBatchSize:  DefaultEmbeddingBatchSize,
		MaxResultChars:      DefaultMaxResultChars,
		StaleProcessing:     15 * time.Minute,
	}
}
