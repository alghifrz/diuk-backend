package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/automation"
	"github.com/alghifrz/diuk-backend/internal/job"
)

type stubAutomation struct {
	evaluate error
	execute  error
}

func (s stubAutomation) EvaluateEvent(context.Context, uuid.UUID, uuid.UUID) error { return s.evaluate }
func (s stubAutomation) ExecuteRun(context.Context, uuid.UUID, uuid.UUID) error    { return s.execute }

func TestAutomationHandlerEvaluateAndRun(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewAutomationHandler(log, stubAutomation{})
	eval, err := job.NewAutomationEvaluate(uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), eval); err != nil {
		t.Fatal(err)
	}
	run, err := job.NewAutomationRun(uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

func TestAutomationHandlerPermanentAndRetryable(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewAutomationHandler(log, stubAutomation{execute: automation.ErrRecipientMissing})
	j, _ := job.NewAutomationRun(uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), j); !errors.Is(err, automation.ErrRecipientMissing) || job.Classify(err).Retryable {
		t.Fatalf("permanent = %v", err)
	}
	h = NewAutomationHandler(log, stubAutomation{evaluate: automation.ErrDisabled})
	e, _ := job.NewAutomationEvaluate(uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), e); !errors.Is(err, automation.ErrDisabled) || !job.Classify(err).Retryable {
		t.Fatalf("retryable = %v", err)
	}
}
