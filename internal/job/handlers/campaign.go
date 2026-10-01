package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/campaign"
	"github.com/alghifrz/diuk-backend/internal/job"
)

type CampaignProcessor interface {
	StartRun(ctx context.Context, businessID, runID uuid.UUID) error
	DispatchRun(ctx context.Context, businessID, runID uuid.UUID) error
}

type CampaignHandler struct {
	log  *slog.Logger
	proc CampaignProcessor
}

func NewCampaignHandler(log *slog.Logger, proc CampaignProcessor) *CampaignHandler {
	if log == nil {
		log = slog.Default()
	}
	return &CampaignHandler{log: log, proc: proc}
}

func (h *CampaignHandler) Handle(ctx context.Context, j job.Job) error {
	switch j.Type {
	case job.TypeCampaignStart:
		payload, err := job.ParseCampaignStart(j.Payload)
		if err != nil {
			return job.Permanent(err)
		}
		err = h.proc.StartRun(ctx, payload.BusinessID, payload.RunID)
		return h.classify(err, j, payload.BusinessID)
	case job.TypeCampaignDispatch:
		payload, err := job.ParseCampaignDispatch(j.Payload)
		if err != nil {
			return job.Permanent(err)
		}
		err = h.proc.DispatchRun(ctx, payload.BusinessID, payload.RunID)
		return h.classify(err, j, payload.BusinessID)
	default:
		return job.Permanent(job.ErrUnknownJob)
	}
}

func (h *CampaignHandler) classify(err error, j job.Job, businessID uuid.UUID) error {
	if err == nil {
		h.log.Info("campaign job completed",
			"job_id", j.ID, "job_type", j.Type, "business_id", businessID,
			"duration_ms", time.Since(j.CreatedAt).Milliseconds(), "result", "ok",
		)
		return nil
	}
	if errors.Is(err, campaign.ErrNotFound) || errors.Is(err, campaign.ErrRunNotFound) ||
		errors.Is(err, campaign.ErrBusinessInactive) || errors.Is(err, campaign.ErrInvalid) ||
		errors.Is(err, campaign.ErrInvalidTransition) {
		return job.Permanent(err)
	}
	return err
}
