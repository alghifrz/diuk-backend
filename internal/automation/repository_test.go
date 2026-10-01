package automation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAutomationRepositoryTenantAndIdempotency(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping automation integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	var businessID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM businesses LIMIT 1`).Scan(&businessID); err != nil {
		t.Skip("no business row available for automation repository test")
	}

	repo := NewRepository(pool)
	item, err := repo.CreateAutomation(ctx, Automation{
		BusinessID:    businessID,
		Name:          "Repo Welcome",
		Status:        StatusActive,
		TriggerType:   TriggerCustomerCreated,
		TriggerConfig: json.RawMessage(`{}`),
		Conditions:    json.RawMessage(`[]`),
		ActionType:    ActionSendMessage,
		ActionConfig:  json.RawMessage(`{"template":"Halo {{customer.first_name}}"}`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var eventIDs []uuid.UUID
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM automation_runs WHERE automation_id = $1`, item.ID)
		if len(eventIDs) > 0 {
			_, _ = pool.Exec(ctx, `DELETE FROM automation_events WHERE id = ANY($1)`, eventIDs)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM automations WHERE id = $1`, item.ID); err != nil {
			_, _ = pool.Exec(ctx, `UPDATE automations SET status = 'ARCHIVED' WHERE id = $1`, item.ID)
		}
	})
	if _, err := repo.GetAutomation(ctx, uuid.New(), item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant get = %v", err)
	}
	event, created, err := repo.InsertEvent(ctx, Event{
		BusinessID: businessID, Type: EventCustomerCreated, SubjectType: SubjectCustomer,
		SubjectID: uuid.New(), IdempotencyKey: "repo-test-" + uuid.NewString(), Payload: map[string]any{"status": "ACTIVE"}, OccurredAt: time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("insert event = %+v created=%v err=%v", event, created, err)
	}
	eventIDs = append(eventIDs, event.ID)
	_, created, err = repo.InsertEvent(ctx, *event)
	if err != nil || created {
		t.Fatalf("duplicate event created=%v err=%v", created, err)
	}
	run, created, err := repo.InsertRun(ctx, Run{BusinessID: businessID, AutomationID: item.ID, EventID: &event.ID, SubjectType: SubjectCustomer, SubjectID: event.SubjectID, Status: RunPending})
	if err != nil || !created {
		t.Fatalf("insert run = %+v created=%v err=%v", run, created, err)
	}
	_, created, err = repo.InsertRun(ctx, Run{BusinessID: businessID, AutomationID: item.ID, EventID: &event.ID, Status: RunPending})
	if err != nil || created {
		t.Fatalf("duplicate run created=%v err=%v", created, err)
	}
	claimed, err := repo.ClaimRun(ctx, businessID, run.ID)
	if err != nil || claimed.Status != RunRunning {
		t.Fatalf("claim = %+v err=%v", claimed, err)
	}
	if _, err := repo.ClaimRun(ctx, businessID, run.ID); !errors.Is(err, ErrRunNotClaimed) {
		t.Fatalf("second claim = %v", err)
	}
}
