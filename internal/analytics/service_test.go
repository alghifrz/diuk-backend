package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceTenantIsolationAndEmpty(t *testing.T) {
	store := newMemoryStore()
	a, b := uuid.New(), uuid.New()
	store.overview[a] = Overview{TotalConversations: 4, NewCustomers: 2}
	store.overview[b] = Overview{TotalConversations: 9}
	svc := NewService(store, stubTZ{tz: "UTC"})
	svc.now = func() time.Time { return time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC) }

	_, got, err := svc.Overview(context.Background(), a, "2026-09-01", "2026-09-24")
	if err != nil || got.TotalConversations != 4 || got.NewCustomers != 2 {
		t.Fatalf("A = %+v err=%v", got, err)
	}
	_, other, err := svc.Overview(context.Background(), b, "2026-09-01", "2026-09-24")
	if err != nil || other.TotalConversations != 9 {
		t.Fatalf("B = %+v err=%v", other, err)
	}
	_, empty, err := svc.Conversations(context.Background(), uuid.New(), "2026-09-01", "2026-09-24")
	if err != nil || empty.Total != 0 || empty.Series != nil && len(empty.Series) != 0 {
		t.Fatalf("empty = %+v err=%v", empty, err)
	}
	_, conv, err := svc.Conversion(context.Background(), uuid.New(), "2026-09-01", "2026-09-24")
	if err != nil || conv.Conversations != 0 || conv.ConversationReservationRate != 0 || conv.PaymentCollectionRate != 0 {
		t.Fatalf("empty conversion = %+v err=%v", conv, err)
	}
	_, pay, err := svc.Revenue(context.Background(), uuid.New(), "2026-09-01", "2026-09-24")
	if err != nil || pay.PaymentRecords != 0 || pay.TotalAmount != 0 || pay.PaymentCollectionRate != 0 {
		t.Fatalf("empty revenue = %+v err=%v", pay, err)
	}
}

func TestServiceInvalidRange(t *testing.T) {
	svc := NewService(newMemoryStore(), stubTZ{tz: "UTC"})
	if _, _, err := svc.Overview(context.Background(), uuid.New(), "2026-09-24", "2026-09-01"); err == nil {
		t.Fatal("expected invalid range")
	}
}
