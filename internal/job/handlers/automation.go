package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/automation"
	"github.com/alghifrz/diuk-backend/internal/job"
)

type AutomationProcessor interface {
	EvaluateEvent(ctx context.Context, businessID, eventID uuid.UUID) error
	ExecuteRun(ctx context.Context, businessID, runID uuid.UUID) error
}

type AutomationHandler struct {
	log  *slog.Logger
	proc AutomationProcessor
}

func NewAutomationHandler(log *slog.Logger, proc AutomationProcessor) *AutomationHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AutomationHandler{log: log, proc: proc}
}

func (h *AutomationHandler) Handle(ctx context.Context, j job.Job) error {
	switch j.Type {
	case job.TypeAutomationEvaluate:
		payload, err := job.ParseAutomationEvaluate(j.Payload)
		if err != nil {
			return job.Permanent(err)
		}
		err = h.proc.EvaluateEvent(ctx, payload.BusinessID, payload.EventID)
		return h.classify(err, j, payload.BusinessID)
	case job.TypeAutomationRun:
		payload, err := job.ParseAutomationRun(j.Payload)
		if err != nil {
			return job.Permanent(err)
		}
		err = h.proc.ExecuteRun(ctx, payload.BusinessID, payload.RunID)
		return h.classify(err, j, payload.BusinessID)
	default:
		return job.Permanent(job.ErrUnknownJob)
	}
}

func (h *AutomationHandler) classify(err error, j job.Job, businessID uuid.UUID) error {
	if err == nil {
		h.log.Info("automation job completed",
			"job_id", j.ID,
			"job_type", j.Type,
			"business_id", businessID,
			"duration_ms", time.Since(j.CreatedAt).Milliseconds(),
			"result", "ok",
		)
		return nil
	}
	if isAutomationPermanent(err) {
		return job.Permanent(err)
	}
	if isAutomationRetryable(err) {
		return job.Retryable(err, 0)
	}
	return job.Permanent(err)
}

func isAutomationPermanent(err error) bool {
	return errors.Is(err, automation.ErrNotFound) ||
		errors.Is(err, automation.ErrEventNotFound) ||
		errors.Is(err, automation.ErrRunNotFound) ||
		errors.Is(err, automation.ErrRecipientMissing) ||
		errors.Is(err, automation.ErrSubjectMissing) ||
		errors.Is(err, automation.ErrSubjectInvalid) ||
		errors.Is(err, automation.ErrInvalidTemplate) ||
		errors.Is(err, automation.ErrUnsupportedAction) ||
		errors.Is(err, automation.ErrInactiveAutomation) ||
		errors.Is(err, automation.ErrBusinessInactive) ||
		errors.Is(err, automation.ErrInvalid)
}

func isAutomationRetryable(err error) bool {
	return errors.Is(err, automation.ErrDisabled)
}
