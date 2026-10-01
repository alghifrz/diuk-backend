package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestCampaignRepositoryTenantSnapshotAndClaim(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	repo := NewRepository(tx)
	bizRepo := tenant.NewBusinessRepository(tx)
	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Camp A", Slug: "camp-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Camp B", Slug: "camp-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("biz B: %v", err)
	}

	custA := insertCustomer(t, tx, a.ID, "Pat", true)
	custB := insertCustomer(t, tx, b.ID, "Other", true)
	insertIdentity(t, tx, a.ID, custA, "628111")
	insertIdentity(t, tx, b.ID, custB, "628222")
	optOut := insertCustomer(t, tx, a.ID, "NoMail", false)
	insertIdentity(t, tx, a.ID, optOut, "628333")

	tpl, lang := "hello", "id"
	created, err := repo.CreateCampaign(ctx, Campaign{
		BusinessID: a.ID, Name: "Promo", Status: StatusDraft, Channel: ChannelWhatsApp, MessageKind: KindTemplate,
		TemplateName: &tpl, TemplateLanguage: &lang, AudienceType: AudienceFilter,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.GetCampaign(ctx, b.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant campaign = %v", err)
	}

	run, createdRun, err := repo.CreateRun(ctx, Run{BusinessID: a.ID, CampaignID: created.ID, Status: RunPending})
	if err != nil || !createdRun {
		t.Fatalf("run = %+v created=%v err=%v", run, createdRun, err)
	}
	again, createdRun, err := repo.CreateRun(ctx, Run{BusinessID: a.ID, CampaignID: created.ID, Status: RunPending})
	if err != nil || createdRun || again.ID != run.ID {
		t.Fatalf("duplicate run created=%v id=%s err=%v", createdRun, again.ID, err)
	}

	status := "ACTIVE"
	n, err := repo.SnapshotRecipients(ctx, a.ID, run.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &status})
	if err != nil || n != 2 {
		t.Fatalf("snapshot = %d err=%v", n, err)
	}
	n, err = repo.SnapshotRecipients(ctx, a.ID, run.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &status})
	if err != nil || n != 0 {
		t.Fatalf("retry snapshot = %d err=%v", n, err)
	}
	recs, err := repo.ListRecipients(ctx, a.ID, run.ID, "", 50, 0)
	if err != nil || len(recs) != 2 {
		t.Fatalf("list recipients = %d err=%v", len(recs), err)
	}
	for _, rec := range recs {
		if rec.CustomerID == optOut && (rec.Status != RecipientSkipped || rec.FailureCode == nil || *rec.FailureCode != SkipMarketingOptOut) {
			t.Fatalf("opt-out recipient = %+v", rec)
		}
		if rec.CustomerID == custB {
			t.Fatal("other tenant customer leaked into snapshot")
		}
	}
	if _, err := repo.GetRun(ctx, b.ID, created.ID, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("cross tenant run = %v", err)
	}

	claimed, err := repo.ClaimPendingRecipients(ctx, a.ID, run.ID, 50)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d err=%v", len(claimed), err)
	}
	dup, err := repo.ClaimPendingRecipients(ctx, a.ID, run.ID, 50)
	if err != nil || len(dup) != 0 {
		t.Fatalf("duplicate claim = %d err=%v", len(dup), err)
	}

	preview, err := repo.PreviewAudience(ctx, a.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &status})
	if err != nil || preview.EstimatedCount != 2 {
		t.Fatalf("preview = %+v err=%v", preview, err)
	}
	otherPreview, err := repo.PreviewAudience(ctx, b.ID, ChannelWhatsApp, AudienceDefinition{CustomerStatus: &status})
	if err != nil || otherPreview.EstimatedCount != 1 {
		t.Fatalf("other preview = %+v err=%v", otherPreview, err)
	}

	raw, _ := json.Marshal(AudienceDefinition{CustomerStatus: &status})
	_ = raw
	due := time.Now().UTC().Add(-time.Minute)
	created.Status = StatusScheduled
	created.ScheduledAt = &due
	if _, err := repo.UpdateCampaign(ctx, *created); err != nil {
		t.Fatal(err)
	}
	listed, err := repo.ListDueScheduled(ctx, time.Now().UTC(), 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range listed {
		if item.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("due scheduled campaign missing")
	}
}

func insertCustomer(t *testing.T, tx pgx.Tx, businessID uuid.UUID, name string, optIn bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := tx.QueryRow(context.Background(), `
		INSERT INTO customers (business_id, name, status, marketing_opt_in)
		VALUES ($1, $2, 'ACTIVE', $3) RETURNING id`, businessID, name, optIn).Scan(&id); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	return id
}

func insertIdentity(t *testing.T, tx pgx.Tx, businessID, customerID uuid.UUID, external string) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO customer_identities (business_id, customer_id, channel, external_id, status)
		VALUES ($1, $2, 'WHATSAPP', $3, 'ACTIVE')`, businessID, customerID, external); err != nil {
		t.Fatalf("insert identity: %v", err)
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping campaign integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
