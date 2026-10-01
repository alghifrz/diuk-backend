package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/job"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
)

type KnowledgeProcessor interface {
	ProcessDocument(ctx context.Context, businessID, documentID uuid.UUID) error
}

type KnowledgeHandler struct {
	log  *slog.Logger
	proc KnowledgeProcessor
}

func NewKnowledgeHandler(log *slog.Logger, proc KnowledgeProcessor) *KnowledgeHandler {
	if log == nil {
		log = slog.Default()
	}
	return &KnowledgeHandler{log: log, proc: proc}
}

func (h *KnowledgeHandler) Handle(ctx context.Context, j job.Job) error {
	if j.Type != job.TypeKnowledgeIngest {
		return job.Permanent(job.ErrUnknownJob)
	}
	payload, err := job.ParseKnowledgeIngest(j.Payload)
	if err != nil {
		return job.Permanent(err)
	}
	err = h.proc.ProcessDocument(ctx, payload.BusinessID, payload.DocumentID)
	if err != nil {
		if isKnowledgePermanent(err) {
			return job.Permanent(err)
		}
		if isKnowledgeRetryable(err) {
			return job.Retryable(err, 0)
		}
		return job.Permanent(err)
	}
	h.log.Info("knowledge ingest job completed",
		"job_id", j.ID,
		"job_type", j.Type,
		"business_id", payload.BusinessID,
		"document_id", payload.DocumentID,
		"duration_ms", time.Since(j.CreatedAt).Milliseconds(),
		"result", "ok",
	)
	return nil
}

func isKnowledgePermanent(err error) bool {
	return errors.Is(err, knowledge.ErrDocumentNotFound) ||
		errors.Is(err, knowledge.ErrUnsupportedDocumentType) ||
		errors.Is(err, knowledge.ErrDocumentEmpty) ||
		errors.Is(err, knowledge.ErrDocumentTooLarge) ||
		errors.Is(err, knowledge.ErrEmbeddingNotConfigured) ||
		errors.Is(err, knowledge.ErrInvalid) ||
		errors.Is(err, knowledge.ErrDocumentProcessingFailed)
}

func isKnowledgeRetryable(err error) bool {
	return errors.Is(err, knowledge.ErrEmbeddingFailed) ||
		errors.Is(err, knowledge.ErrStorageFailed)
}
