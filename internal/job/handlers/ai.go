package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/job"
)

type AIGenerator interface {
	GenerateReply(ctx context.Context, businessID uuid.UUID, in ai.GenerateReplyInput) (*ai.GenerateReplyResult, error)
}

type MessageReader interface {
	GetMessage(ctx context.Context, businessID, messageID uuid.UUID) (*conversation.Message, error)
}

type AIHandler struct {
	log      *slog.Logger
	ai       AIGenerator
	messages MessageReader
	store    job.OutboxStore
}

func NewAIHandler(log *slog.Logger, generator AIGenerator, messages MessageReader, store job.OutboxStore) *AIHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AIHandler{log: log, ai: generator, messages: messages, store: store}
}

func (h *AIHandler) Handle(ctx context.Context, j job.Job) error {
	if j.Type != job.TypeAIGenerate {
		return job.Permanent(job.ErrUnknownJob)
	}
	payload, err := job.ParseAIGenerate(j.Payload)
	if err != nil {
		return job.Permanent(err)
	}
	msg, err := h.messages.GetMessage(ctx, payload.BusinessID, payload.MessageID)
	if err != nil {
		if isMissing(err) {
			return job.Permanent(err)
		}
		return job.Retryable(err, 0)
	}
	if msg.Direction != conversation.DirectionInbound {
		return nil
	}

	claimed, done, err := h.store.ClaimAIGeneration(ctx, payload.BusinessID, payload.ConversationID, payload.MessageID)
	if err != nil {
		return job.Retryable(err, 0)
	}
	if done {
		h.log.Info("ai job already processed",
			"job_id", j.ID, "job_type", j.Type, "business_id", payload.BusinessID,
			"conversation_id", payload.ConversationID, "message_id", payload.MessageID,
		)
		return nil
	}
	if !claimed {
		return nil
	}

	result, err := h.ai.GenerateReply(ctx, payload.BusinessID, ai.GenerateReplyInput{
		ConversationID:  payload.ConversationID,
		PersistIncoming: false,
		UsageSourceID:   payload.MessageID,
	})
	if err != nil {
		if errors.Is(err, ai.ErrHumanHandling) || errors.Is(err, ai.ErrQuotaExceeded) {
			if completeErr := h.store.CompleteAIGeneration(ctx, payload.MessageID, uuid.Nil); completeErr != nil {
				return job.Retryable(completeErr, 0)
			}
			reason := "human handling"
			if errors.Is(err, ai.ErrQuotaExceeded) {
				reason = "quota exceeded"
			}
			h.log.Info("ai job skipped "+reason,
				"job_id", j.ID, "job_type", j.Type, "business_id", payload.BusinessID,
				"conversation_id", payload.ConversationID, "message_id", payload.MessageID,
			)
			return nil
		}
		if !job.Classify(err).Retryable {
			_ = h.store.RecordAIGenerationFailure(ctx, payload.MessageID)
		}
		return err
	}
	if err := h.store.CompleteAIGeneration(ctx, payload.MessageID, result.MessageID); err != nil {
		return job.Retryable(err, 0)
	}
	_ = h.store.RecordAIGenerationMeta(ctx, payload.MessageID, result.Provider, result.Model)
	h.log.Info("ai job completed",
		"job_id", j.ID,
		"job_type", j.Type,
		"business_id", payload.BusinessID,
		"conversation_id", payload.ConversationID,
		"message_id", payload.MessageID,
		"duration_ms", time.Since(j.CreatedAt).Milliseconds(),
		"result", "ok",
	)
	return nil
}

func isMissing(err error) bool {
	return errors.Is(err, conversation.ErrMessageNotFound) ||
		errors.Is(err, conversation.ErrNotFound) ||
		errors.Is(err, ai.ErrConversationNotFound)
}
