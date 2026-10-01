package knowledge

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/usage"
	"github.com/alghifrz/diuk-backend/pkg/storage"
)

type Service struct {
	log      *slog.Logger
	store    Store
	objects  ObjectStore
	embedder EmbeddingProvider
	enqueue  IngestEnqueuer
	settings Settings
	meter    usage.Meter
	answerer AnswerGenerator
}

func NewService(log *slog.Logger, store Store, objects ObjectStore, embedder EmbeddingProvider, enqueue IngestEnqueuer, settings Settings) *Service {
	if log == nil {
		log = slog.Default()
	}
	if settings.ChunkSize < 1 {
		settings.ChunkSize = DefaultChunkSize
	}
	if settings.TopK < 1 {
		settings.TopK = DefaultTopK
	}
	if settings.MaxFileBytes < 1 {
		settings.MaxFileBytes = int64(DefaultMaxFileSizeMB) * 1024 * 1024
	}
	if settings.EmbeddingBatchSize < 1 {
		settings.EmbeddingBatchSize = DefaultEmbeddingBatchSize
	}
	if settings.MaxResultChars < 1 {
		settings.MaxResultChars = DefaultMaxResultChars
	}
	if settings.StaleProcessing <= 0 {
		settings.StaleProcessing = 15 * time.Minute
	}
	return &Service{log: log, store: store, objects: objects, embedder: embedder, enqueue: enqueue, settings: settings}
}

func (s *Service) Enabled() bool { return s.settings.Enabled }

func (s *Service) SetUsageMeter(m usage.Meter) { s.meter = m }

func (s *Service) SetAnswerer(a AnswerGenerator) { s.answerer = a }

func (s *Service) ListDocuments(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]Document, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListDocuments(ctx, businessID, status)
}

func (s *Service) GetDocument(ctx context.Context, businessID, documentID uuid.UUID) (*Document, error) {
	return s.store.GetDocument(ctx, businessID, documentID)
}

func (s *Service) CreateDocument(ctx context.Context, businessID uuid.UUID, in CreateDocumentInput) (*Document, error) {
	title, err := normalizeTitle(in.Title)
	if err != nil {
		return nil, err
	}
	description, err := normalizeOptionalDescription(in.Description)
	if err != nil {
		return nil, err
	}
	source, err := normalizeSourceType(in.SourceType, in.Filename, in.MimeType)
	if err != nil {
		return nil, err
	}
	if err := validateFile(in.Content, s.settings.MaxFileBytes); err != nil {
		return nil, err
	}
	if err := validateSourceMatch(source, in.Filename, in.MimeType); err != nil {
		return nil, err
	}
	if source == SourceText {
		if _, err := extractPlainText(in.Content); err != nil {
			return nil, err
		}
	}
	if s.meter != nil {
		if err := s.meter.CheckResource(ctx, businessID, usage.FeatureKnowledgeDocuments); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) || errors.Is(err, usage.ErrFeatureDisabled) || errors.Is(err, usage.ErrNotEntitled) || errors.Is(err, usage.ErrPlanInactive) {
				return nil, ErrQuotaExceeded
			}
			return nil, err
		}
	}

	createdBy := parseOptionalUser(in.CreatedBy)
	doc, err := s.store.CreateDocument(ctx, &Document{
		BusinessID:  businessID,
		Title:       title,
		Description: description,
		SourceType:  source,
		Status:      StatusPending,
		Version:     1,
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	})
	if err != nil {
		return nil, err
	}

	path := objectPath(businessID.String(), doc.ID.String(), source)
	mime := strings.TrimSpace(in.MimeType)
	if mime == "" {
		mime = defaultMIME(source)
	}
	if s.objects != nil {
		if err := s.objects.Upload(ctx, path, mime, bytes.NewReader(in.Content)); err != nil {
			_, _ = s.store.MarkFailed(ctx, businessID, doc.ID, "document storage failed")
			s.log.Error("knowledge upload failed", "business_id", businessID, "document_id", doc.ID, "error", err.Error())
			return nil, ErrStorageFailed
		}
	}
	doc, err = s.store.UpdateDocumentStorage(ctx, businessID, doc.ID, path, mime)
	if err != nil {
		return nil, err
	}
	if err := s.enqueueIngest(ctx, businessID, doc.ID); err != nil {
		return nil, err
	}
	s.log.Info("knowledge document created",
		"business_id", businessID, "document_id", doc.ID, "source_type", source, "status", doc.Status)
	return doc, nil
}

func (s *Service) UpdateDocument(ctx context.Context, businessID, documentID uuid.UUID, in UpdateDocumentInput) (*Document, error) {
	current, err := s.store.GetDocument(ctx, businessID, documentID)
	if err != nil {
		return nil, err
	}
	title := current.Title
	if in.Title != nil {
		title, err = normalizeTitle(*in.Title)
		if err != nil {
			return nil, err
		}
	}
	description := current.Description
	if in.Description != nil {
		description, err = normalizeOptionalDescription(in.Description)
		if err != nil {
			return nil, err
		}
	}
	updated, err := s.store.UpdateDocumentMeta(ctx, businessID, documentID, title, description, parseOptionalUser(in.UpdatedBy))
	if err != nil {
		return nil, err
	}
	s.log.Info("knowledge document updated", "business_id", businessID, "document_id", documentID, "status", updated.Status)
	return updated, nil
}

func (s *Service) ReprocessDocument(ctx context.Context, businessID, documentID uuid.UUID, updatedBy *string) (*Document, error) {
	current, err := s.store.GetDocument(ctx, businessID, documentID)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusProcessing {
		return nil, ErrDocumentAlreadyProcessing
	}
	doc, err := s.store.MarkPending(ctx, businessID, documentID, parseOptionalUser(updatedBy))
	if err != nil {
		return nil, err
	}
	if err := s.enqueueIngest(ctx, businessID, documentID); err != nil {
		return nil, err
	}
	s.log.Info("knowledge document reprocess queued", "business_id", businessID, "document_id", documentID, "version", doc.Version)
	return doc, nil
}

func (s *Service) DeleteDocument(ctx context.Context, businessID, documentID uuid.UUID) error {
	doc, err := s.store.GetDocument(ctx, businessID, documentID)
	if err != nil {
		return err
	}
	if doc.StoragePath != nil && s.objects != nil {
		if err := s.objects.Delete(ctx, *doc.StoragePath); err != nil && !errors.Is(err, storage.ErrNotFound) {
			s.log.Error("knowledge storage delete failed", "business_id", businessID, "document_id", documentID, "error", err.Error())
			return ErrStorageFailed
		}
	}
	if err := s.store.DeleteDocument(ctx, businessID, documentID); err != nil {
		return err
	}
	s.log.Info("knowledge document deleted", "business_id", businessID, "document_id", documentID)
	return nil
}

func (s *Service) ProcessDocument(ctx context.Context, businessID, documentID uuid.UUID) error {
	started := time.Now()
	current, err := s.store.GetDocument(ctx, businessID, documentID)
	if err != nil {
		return err
	}
	if current.Status == StatusReady {
		s.log.Info("knowledge ingest skipped", "business_id", businessID, "document_id", documentID, "status", current.Status)
		return nil
	}
	if current.Status == StatusArchived {
		return ErrDocumentNotFound
	}

	claimed, err := s.store.ClaimProcessing(ctx, businessID, documentID, s.settings.StaleProcessing)
	if err != nil {
		if errors.Is(err, ErrDocumentAlreadyProcessing) {
			latest, getErr := s.store.GetDocument(ctx, businessID, documentID)
			if getErr != nil {
				return getErr
			}
			if latest.Status == StatusReady || latest.Status == StatusProcessing {
				s.log.Info("knowledge ingest duplicate skipped",
					"business_id", businessID, "document_id", documentID, "status", latest.Status)
				return nil
			}
			if latest.Status == StatusFailed {
				return ErrDocumentProcessingFailed
			}
			return nil
		}
		return err
	}

	if err := s.ingest(ctx, claimed); err != nil {
		if isPermanentIngest(err) {
			_, _ = s.store.MarkFailed(ctx, businessID, documentID, sanitizeProcessingError(err))
			s.log.Error("knowledge ingest failed",
				"business_id", businessID, "document_id", documentID, "status", StatusFailed,
				"failure_category", publicProcessingMessage(err),
				"duration_ms", time.Since(started).Milliseconds(),
			)
			return err
		}
		_ = s.store.ReleaseProcessing(ctx, businessID, documentID)
		s.log.Error("knowledge ingest retryable",
			"business_id", businessID, "document_id", documentID,
			"failure_category", "retryable",
			"duration_ms", time.Since(started).Milliseconds(),
		)
		return err
	}
	s.log.Info("knowledge ingest completed",
		"business_id", businessID, "document_id", documentID, "status", StatusReady,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return nil
}

func (s *Service) ingest(ctx context.Context, doc *Document) error {
	if !s.settings.Enabled || s.embedder == nil {
		return ErrEmbeddingNotConfigured
	}
	data, err := s.loadSource(ctx, doc)
	if err != nil {
		return err
	}
	text, err := ExtractText(doc.SourceType, data)
	if err != nil {
		return err
	}
	parts, err := ChunkText(text, s.settings.ChunkSize, s.settings.ChunkOverlap)
	if err != nil {
		return err
	}
	texts := make([]string, len(parts))
	for i, part := range parts {
		texts[i] = part.Content
	}
	vectors, batches, err := EmbedBatches(ctx, s.embedder, texts, s.settings.EmbeddingBatchSize)
	if err != nil {
		return err
	}
	if len(vectors) != len(parts) {
		return ErrEmbeddingFailed
	}
	for _, vec := range vectors {
		if s.settings.EmbeddingDimensions > 0 && len(vec) != s.settings.EmbeddingDimensions {
			return invalid("unsupported embedding dimension")
		}
	}

	chunks := make([]Chunk, 0, len(parts))
	for i, part := range parts {
		tokenCount := part.TokenCount
		chunks = append(chunks, Chunk{
			BusinessID: doc.BusinessID,
			DocumentID: doc.ID,
			ChunkIndex: part.Index,
			Content:    part.Content,
			Embedding:  vectors[i],
			TokenCount: &tokenCount,
			Metadata: map[string]any{
				"source_type": doc.SourceType,
				"title":       doc.Title,
			},
		})
	}
	if err := s.store.ReplaceChunks(ctx, doc.BusinessID, doc.ID, chunks); err != nil {
		return err
	}
	if _, err := s.store.MarkReady(ctx, doc.BusinessID, doc.ID); err != nil {
		return err
	}
	s.log.Info("knowledge chunks persisted",
		"business_id", doc.BusinessID,
		"document_id", doc.ID,
		"chunk_count", len(chunks),
		"embedding_batch_count", batches,
	)
	return nil
}

func (s *Service) loadSource(ctx context.Context, doc *Document) ([]byte, error) {
	if doc.StoragePath == nil || *doc.StoragePath == "" {
		return nil, ErrDocumentEmpty
	}
	if s.objects == nil {
		return nil, ErrStorageFailed
	}
	data, err := s.objects.Download(ctx, *doc.StoragePath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrDocumentEmpty
		}
		return nil, ErrStorageFailed
	}
	if err := validateFile(data, s.settings.MaxFileBytes); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) enqueueIngest(ctx context.Context, businessID, documentID uuid.UUID) error {
	if s.enqueue == nil {
		return nil
	}
	return s.enqueue.EnqueueKnowledgeIngest(ctx, businessID, documentID)
}

func parseOptionalUser(raw *string) *uuid.UUID {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	return &id
}

func isPermanentIngest(err error) bool {
	return errors.Is(err, ErrUnsupportedDocumentType) ||
		errors.Is(err, ErrDocumentEmpty) ||
		errors.Is(err, ErrDocumentTooLarge) ||
		errors.Is(err, ErrEmbeddingNotConfigured) ||
		errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrDocumentNotFound) ||
		errors.Is(err, ErrDocumentProcessingFailed)
}
