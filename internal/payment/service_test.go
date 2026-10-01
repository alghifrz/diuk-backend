package payment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
)

func TestCreatePaymentSnapshotsDeposit(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	res := uuid.New()
	cust := uuid.New()
	store.seedSettings(biz, DepositSettings{Enabled: true, Type: DepositFixed, Value: money.FromMajor(100000), Currency: "IDR"})
	store.addReservation(biz, res, cust)
	svc := NewService(store)

	item, err := svc.CreateInitial(context.Background(), biz, res, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Status != StatusUnpaid || item.Amount != money.FromMajor(100000) || item.Currency != "IDR" {
		t.Fatalf("payment = %+v", item)
	}

	store.seedSettings(biz, DepositSettings{Enabled: true, Type: DepositFixed, Value: money.FromMajor(200000), Currency: "SGD"})
	again, err := svc.CreateInitial(context.Background(), biz, res, nil)
	if err != nil {
		t.Fatalf("idempotent create: %v", err)
	}
	if again.ID != item.ID || again.Amount != money.FromMajor(100000) || again.Currency != "IDR" {
		t.Fatalf("snapshot mutated = %+v", again)
	}

	if _, err := svc.CreateForReservation(context.Background(), biz, res, nil); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := svc.CreateForReservation(context.Background(), biz, uuid.New(), nil); !errors.Is(err, ErrReservationMissing) {
		t.Fatalf("missing reservation = %v", err)
	}
	if _, err := svc.Get(context.Background(), uuid.New(), item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}
}

func TestPaymentStateMachine(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	res := uuid.New()
	actor := uuid.New()
	store.seedSettings(biz, DepositSettings{Currency: "IDR", Type: DepositFixed})
	store.addReservation(biz, res, uuid.New())
	svc := NewService(store)
	fixed := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	item, err := svc.CreateInitial(context.Background(), biz, res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refund(context.Background(), biz, item.ID, actor, nil); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unpaid refund = %v", err)
	}

	note := "Transfer BCA checked via screenshot"
	paid, err := svc.Confirm(context.Background(), biz, item.ID, actor, &note)
	if err != nil || paid.Status != StatusPaid || paid.ConfirmedBy == nil || *paid.ConfirmedBy != actor || paid.ConfirmedAt == nil {
		t.Fatalf("confirm = %+v err=%v", paid, err)
	}
	if paid.ConfirmedAt.UTC() != fixed || paid.Notes == nil || *paid.Notes != note {
		t.Fatalf("audit = %+v", paid)
	}

	again, err := svc.Confirm(context.Background(), biz, item.ID, uuid.New(), nil)
	if err != nil || again.Status != StatusPaid || again.ConfirmedBy == nil || *again.ConfirmedBy != actor {
		t.Fatalf("idempotent confirm = %+v err=%v", again, err)
	}

	refunded, err := svc.Refund(context.Background(), biz, item.ID, actor, ptr("manual refund"))
	if err != nil || refunded.Status != StatusRefunded || refunded.RefundedBy == nil {
		t.Fatalf("refund = %+v err=%v", refunded, err)
	}
	if _, err := svc.Refund(context.Background(), biz, item.ID, actor, nil); !errors.Is(err, ErrAlreadyRefunded) {
		t.Fatalf("double refund = %v", err)
	}
	if _, err := svc.Confirm(context.Background(), biz, item.ID, actor, nil); !errors.Is(err, ErrAlreadyRefunded) {
		t.Fatalf("refunded confirm = %v", err)
	}
}

func TestAssertConfirmable(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	res := uuid.New()
	store.addReservation(biz, res, uuid.New())
	svc := NewService(store)

	store.seedSettings(biz, DepositSettings{Enabled: false, Type: DepositFixed, Currency: "IDR"})
	if err := svc.AssertConfirmable(context.Background(), biz, res); err != nil {
		t.Fatalf("deposit disabled: %v", err)
	}

	store.seedSettings(biz, DepositSettings{Enabled: true, Type: DepositFixed, Value: money.FromMajor(100000), Currency: "IDR"})
	if _, err := svc.CreateInitial(context.Background(), biz, res, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertConfirmable(context.Background(), biz, res); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("unpaid = %v", err)
	}
	item, _ := svc.GetByReservation(context.Background(), biz, res)
	if _, err := svc.Confirm(context.Background(), biz, item.ID, uuid.New(), nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertConfirmable(context.Background(), biz, res); err != nil {
		t.Fatalf("paid = %v", err)
	}
	if _, err := svc.Refund(context.Background(), biz, item.ID, uuid.New(), nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertConfirmable(context.Background(), biz, res); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("refunded = %v", err)
	}
}

func TestMarkFullFromDeposit(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	res := uuid.New()
	actor := uuid.New()
	store.seedSettings(biz, DepositSettings{Enabled: true, Type: DepositPercentage, Value: money.FromMajor(50), Currency: "IDR"})
	store.addReservation(biz, res, uuid.New())
	store.orderTotals[res] = money.FromMajor(282000)
	svc := NewService(store)

	item, err := svc.CreateInitialWithOrder(context.Background(), biz, res, CreatePaymentInput{
		PaymentType: PaymentTypeDeposit,
		OrderTotal:  money.FromMajor(282000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(context.Background(), biz, item.ID, actor, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertCompletable(context.Background(), biz, res); !errors.Is(err, ErrFullPaymentRequired) {
		t.Fatalf("deposit blocks complete = %v", err)
	}
	full, err := svc.MarkFull(context.Background(), biz, item.ID, actor, ptr("Balance received"))
	if err != nil || full.Status != StatusPaid || full.PaymentType != PaymentTypeFull || full.Amount != money.FromMajor(282000) {
		t.Fatalf("mark full = %+v err=%v", full, err)
	}
	if err := svc.AssertCompletable(context.Background(), biz, res); err != nil {
		t.Fatalf("paid allows complete = %v", err)
	}
}

func TestConcurrentConfirmIsSafe(t *testing.T) {
	store := newMemoryStore()
	biz := uuid.New()
	res := uuid.New()
	store.seedSettings(biz, DepositSettings{Currency: "IDR", Type: DepositFixed})
	store.addReservation(biz, res, uuid.New())
	svc := NewService(store)
	item, err := svc.CreateInitial(context.Background(), biz, res, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Confirm(context.Background(), biz, item.ID, uuid.New(), nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent confirm: %v", err)
		}
	}
	got, err := svc.Get(context.Background(), biz, item.ID)
	if err != nil || got.Status != StatusPaid || got.ConfirmedBy == nil {
		t.Fatalf("final = %+v err=%v", got, err)
	}
}

func ptr(v string) *string { return &v }
