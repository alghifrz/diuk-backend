package handlers

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/job"
)

type Deliverer interface {
	DeliverMessage(ctx context.Context, businessID, messageID uuid.UUID) error
}

type DeliveryHandler struct {
	log     *slog.Logger
	deliver Deliverer
}

func NewDeliveryHandler(log *slog.Logger, deliver Deliverer) *DeliveryHandler {
	if log == nil {
		log = slog.Default()
	}
	return &DeliveryHandler{log: log, deliver: deliver}
}

func (h *DeliveryHandler) Handle(ctx context.Context, j job.Job) error {
	if j.Type != job.TypeMessageDelivery {
		return job.Permanent(job.ErrUnknownJob)
	}
	payload, err := job.ParseMessageDelivery(j.Payload)
	if err != nil {
		return job.Permanent(err)
	}
	if err := h.deliver.DeliverMessage(ctx, payload.BusinessID, payload.MessageID); err != nil {
		return err
	}
	h.log.Info("delivery job completed",
		"job_id", j.ID,
		"job_type", j.Type,
		"business_id", payload.BusinessID,
		"message_id", payload.MessageID,
		"result", "ok",
	)
	return nil
}
