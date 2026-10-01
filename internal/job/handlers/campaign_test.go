package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/campaign"
	"github.com/alghifrz/diuk-backend/internal/job"
)

func TestCampaignHandlerStartAndDispatch(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewCampaignHandler(log, stubCampaign{})
	start, err := job.NewCampaignStart(uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	dispatch, err := job.NewCampaignDispatch(uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), dispatch); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignHandlerPermanentAndRetryable(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewCampaignHandler(log, stubCampaign{start: campaign.ErrNotFound})
	j, _ := job.NewCampaignStart(uuid.New(), uuid.New(), 3)
	if got := job.Classify(h.Handle(context.Background(), j)); got.Retryable {
		t.Fatal("not found must be permanent")
	}
	h = NewCampaignHandler(log, stubCampaign{dispatch: errors.New("temporary")})
	d, _ := job.NewCampaignDispatch(uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), d); err == nil {
		t.Fatal("retryable error expected")
	}
}

type stubCampaign struct {
	start    error
	dispatch error
}

func (s stubCampaign) StartRun(context.Context, uuid.UUID, uuid.UUID) error    { return s.start }
func (s stubCampaign) DispatchRun(context.Context, uuid.UUID, uuid.UUID) error { return s.dispatch }
