package calendar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateBlocksAndConflicts(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	actor := uuid.New()
	store.seedBusiness(biz)
	area := store.addArea(biz, "Indoor")
	table := store.addTable(biz, area, "T01")
	svc := NewService(store)

	if _, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Bad", StartAt: "2026-10-01T12:00:00Z", EndAt: "2026-10-01T12:00:00Z",
	}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("equal range = %v", err)
	}
	if _, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Bad scope", StartAt: "2026-10-01T12:00:00Z", EndAt: "2026-10-01T13:00:00Z", TableID: ptr(table.String()),
	}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("table without area = %v", err)
	}

	block, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Private event", StartAt: "2026-10-01T05:00:00Z", EndAt: "2026-10-01T11:00:00Z",
	})
	if err != nil || block.Scope() != ScopeBusiness {
		t.Fatalf("business block = %+v err=%v", block, err)
	}
	if _, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Overlap", StartAt: "2026-10-01T10:00:00Z", EndAt: "2026-10-01T12:00:00Z",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate business block = %v", err)
	}

	areaBlock, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Outdoor closed", StartAt: "2026-10-01T07:00:00Z", EndAt: "2026-10-01T10:00:00Z", AreaID: ptr(area.String()),
	})
	if err != nil || areaBlock.Scope() != ScopeArea {
		t.Fatalf("area block = %+v err=%v", areaBlock, err)
	}
	tableBlock, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "T01 maintenance", StartAt: "2026-10-01T11:00:00Z", EndAt: "2026-10-01T13:00:00Z",
		AreaID: ptr(area.String()), TableID: ptr(table.String()),
	})
	if err != nil || tableBlock.Scope() != ScopeTable {
		t.Fatalf("table block = %+v err=%v", tableBlock, err)
	}

	cancelled, err := svc.CancelBlock(context.Background(), biz, block.ID)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	if _, err := svc.CreateBlock(context.Background(), biz, actor, CreateBlockInput{
		Title: "Reuse", StartAt: "2026-10-01T05:00:00Z", EndAt: "2026-10-01T11:00:00Z",
	}); err != nil {
		t.Fatalf("cancelled slot reuse: %v", err)
	}
	if _, err := svc.GetBlock(context.Background(), uuid.New(), block.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}
}

func TestBlockConflictsWithExistingReservation(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	store.seedBusiness(biz)
	start := time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)
	store.reservations = []ReservationEvent{{
		ID: uuid.New(), StartAt: start, EndAt: start.Add(2 * time.Hour), Status: "CONFIRMED", Title: "John",
	}}
	svc := NewService(store)
	_, err := svc.CreateBlock(context.Background(), biz, uuid.New(), CreateBlockInput{
		Title: "Close", StartAt: "2026-10-01T10:00:00Z", EndAt: "2026-10-01T14:00:00Z",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected reservation conflict, got %v", err)
	}
	var ce conflictError
	if !errors.As(err, &ce) || len(ce.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", err)
	}

	store.reservations[0].Status = "CANCELLED"
	if _, err := svc.CreateBlock(context.Background(), biz, uuid.New(), CreateBlockInput{
		Title: "Close", StartAt: "2026-10-01T10:00:00Z", EndAt: "2026-10-01T14:00:00Z",
	}); err != nil {
		t.Fatalf("cancelled reservation should allow block: %v", err)
	}
}

func TestCalendarEventsAndDay(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	store.seedBusiness(biz)
	area := store.addArea(biz, "Indoor")
	table := store.addTable(biz, area, "T01")
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	resID := uuid.New()
	paid := "PAID"
	store.reservations = []ReservationEvent{{
		ID: resID, ReservationID: resID, Title: "John", StartAt: start, EndAt: start.Add(90 * time.Minute),
		Status: "CONFIRMED", PartySize: 2, CustomerID: uuid.New(), CustomerName: "John",
		TableID: &table, TableName: "T01", AreaID: &area, AreaName: "Indoor", PaymentStatus: &paid,
	}}
	svc := NewService(store)
	if _, err := svc.CreateBlock(context.Background(), biz, uuid.New(), CreateBlockInput{
		Title: "Staff meeting", StartAt: "2026-10-01T08:00:00Z", EndAt: "2026-10-01T09:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	events, err := svc.Events(context.Background(), biz, "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "", "", "")
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %+v err=%v", events, err)
	}
	if events[0].Type != TypeBusinessBlock || events[1].Type != TypeReservation {
		t.Fatalf("order = %+v", events)
	}
	if events[1].PaymentStatus == nil || *events[1].PaymentStatus != "PAID" {
		t.Fatalf("payment = %+v", events[1])
	}
	if _, err := svc.Events(context.Background(), biz, "2020-01-01T00:00:00Z", "2035-01-01T00:00:00Z", "", "", ""); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("huge range = %v", err)
	}

	day, err := svc.Day(context.Background(), biz, "2026-10-01", "", "")
	if err != nil || day.Timezone != "Asia/Jakarta" || day.Closed || len(day.Hours) == 0 {
		t.Fatalf("day = %+v err=%v", day, err)
	}
	if len(day.Events) != 2 {
		t.Fatalf("day events = %+v", day.Events)
	}
}

func ptr(v string) *string { return &v }
