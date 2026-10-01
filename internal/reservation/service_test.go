package reservation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
)

func testWorld(t *testing.T) (*Service, *memoryStore, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := newMemoryStore()
	biz := uuid.New()
	store.seedContext(biz, "Asia/Jakarta", true)
	svc := NewService(store)
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, loc) }
	if _, err := svc.ReplaceHours(context.Background(), biz, []HourInput{
		{DayOfWeek: 4, SortOrder: 0, OpenTime: "09:00", CloseTime: "14:00"},
		{DayOfWeek: 4, SortOrder: 1, OpenTime: "16:00", CloseTime: "22:00"},
		{DayOfWeek: 1, SortOrder: 0, OpenTime: "09:00", CloseTime: "22:00"},
	}); err != nil {
		t.Fatalf("hours: %v", err)
	}
	cust := store.addCustomer(biz, "John Doe", "ACTIVE", ptr("+6281"))
	table := store.addTable(biz, "T4", "Indoor", 4, "ACTIVE", "ACTIVE")
	return svc, store, biz, cust, table
}

func slot(hour, minute, duration int) (string, string) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	start := time.Date(2026, 10, 1, hour, minute, 0, 0, loc)
	end := start.Add(time.Duration(duration) * time.Minute)
	return start.Format(time.RFC3339), end.Format(time.RFC3339)
}

func TestReservationValidation(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	start, end := slot(19, 0, 90)

	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 20,
	}); !errors.Is(err, ErrPartySize) {
		t.Fatalf("party size = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: end, EndAt: start, PartySize: 2,
	}); !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("end before start = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: start, PartySize: 2,
	}); !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("equal range = %v", err)
	}
	crossStart := time.Date(2026, 10, 1, 23, 0, 0, 0, mustLoc()).Format(time.RFC3339)
	crossEnd := time.Date(2026, 10, 2, 1, 0, 0, 0, mustLoc()).Format(time.RFC3339)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: crossStart, EndAt: crossEnd, PartySize: 2,
	}); !errors.Is(err, ErrCrossMidnight) {
		t.Fatalf("cross midnight = %v", err)
	}
	badStart, badEnd := slot(13, 30, 90)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: badStart, EndAt: badEnd, PartySize: 2,
	}); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("outside hours = %v", err)
	}
	gapStart, gapEnd := slot(14, 0, 120)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: gapStart, EndAt: gapEnd, PartySize: 2,
	}); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("gap hours = %v", err)
	}
	early, _ := slot(19, 0, 90)
	svc.now = func() time.Time { return time.Date(2026, 10, 1, 18, 30, 0, 0, mustLoc()) }
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: early, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrOutsideAdvance) {
		t.Fatalf("advance = %v", err)
	}

	store.seedContext(biz, "Asia/Jakarta", false)
	svc.now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, mustLoc()) }
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled = %v", err)
	}
}

func TestReservationCustomerAndTableRules(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	other := uuid.New()
	store.seedContext(other, "Asia/Jakarta", true)
	foreignCust := store.addCustomer(other, "Jane", "ACTIVE", nil)
	inactive := store.addCustomer(biz, "Old", "INACTIVE", nil)
	inactiveTable := store.addTable(biz, "T2", "Indoor", 4, "INACTIVE", "ACTIVE")
	inactiveArea := store.addTable(biz, "T3", "Patio", 4, "ACTIVE", "INACTIVE")
	small := store.addTable(biz, "T1", "Indoor", 2, "ACTIVE", "ACTIVE")
	start, end := slot(19, 0, 90)

	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: uuid.NewString(), StartAt: start, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("missing customer = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: inactive.String(), StartAt: start, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrCustomerInactive) {
		t.Fatalf("inactive customer = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: foreignCust.String(), StartAt: start, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("cross-tenant customer = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(inactiveTable.String()),
	}); !errors.Is(err, ErrTableInactive) {
		t.Fatalf("inactive table = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(inactiveArea.String()),
	}); !errors.Is(err, ErrAreaInactive) {
		t.Fatalf("inactive area = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 4, TableID: ptr(small.String()),
	}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(uuid.NewString()),
	}); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("missing table = %v", err)
	}

	first, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	}); !errors.Is(err, ErrTableUnavailable) {
		t.Fatalf("overlap = %v", err)
	}
	adjStart, adjEnd := slot(20, 30, 60)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: adjStart, EndAt: adjEnd, PartySize: 2, TableID: ptr(table.String()),
	}); err != nil {
		t.Fatalf("adjacent should be allowed: %v", err)
	}
	if _, err := svc.Get(context.Background(), other, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}
}

func TestReservationLifecycle(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	start, end := slot(19, 0, 90)
	pending, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2,
	})
	if err != nil || pending.Status != StatusPending || pending.TableID != nil {
		t.Fatalf("create pending = %+v err=%v", pending, err)
	}
	if _, err := svc.Complete(context.Background(), biz, pending.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("pending complete = %v", err)
	}
	confirmed, err := svc.Confirm(context.Background(), biz, pending.ID)
	if err != nil || confirmed.Status != StatusConfirmed || confirmed.TableID == nil || *confirmed.TableID != table {
		t.Fatalf("auto confirm = %+v err=%v", confirmed, err)
	}

	cancelled, err := svc.Cancel(context.Background(), biz, confirmed.ID, ptr("guest cancelled"))
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel confirmed = %+v err=%v", cancelled, err)
	}
	if _, err := svc.Confirm(context.Background(), biz, cancelled.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("cancel→confirm = %v", err)
	}
	if _, err := svc.Complete(context.Background(), biz, cancelled.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("cancel→complete = %v", err)
	}

	start2, end2 := slot(17, 0, 60)
	second, _ := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start2, EndAt: end2, PartySize: 2, TableID: ptr(table.String()),
	})
	conf2, err := svc.Confirm(context.Background(), biz, second.ID)
	if err != nil {
		t.Fatalf("confirm 2: %v", err)
	}
	done, err := svc.Complete(context.Background(), biz, conf2.ID)
	if err != nil || done.Status != StatusCompleted {
		t.Fatalf("complete = %+v err=%v", done, err)
	}
	if _, err := svc.Cancel(context.Background(), biz, done.ID, ptr("no")); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("complete→cancel = %v", err)
	}
	if _, err := svc.Confirm(context.Background(), biz, done.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("complete→confirm = %v", err)
	}
	if _, err := svc.Update(context.Background(), biz, done.ID, UpdateReservationInput{Notes: ptr("x")}); !errors.Is(err, ErrCannotUpdate) {
		t.Fatalf("update completed = %v", err)
	}

	pendingOnly, _ := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2,
	})
	if _, err := svc.Cancel(context.Background(), biz, pendingOnly.ID, nil); err != nil {
		t.Fatalf("cancel pending: %v", err)
	}

	_ = store
}

func TestAvailabilityAndAutoAssignSmallestTable(t *testing.T) {
	svc, store, biz, cust, _ := testWorld(t)
	_ = store.addTable(biz, "T2", "Indoor", 2, "ACTIVE", "ACTIVE")
	_ = store.addTable(biz, "T8", "Indoor", 8, "ACTIVE", "ACTIVE")

	avail, err := svc.Availability(context.Background(), biz, "2026-10-01", "19:00", "90", "3")
	if err != nil || !avail.Available || len(avail.Tables) < 2 {
		t.Fatalf("availability = %+v err=%v", avail, err)
	}
	if avail.Tables[0].Capacity != 4 {
		t.Fatalf("expected smallest sufficient first, got %+v", avail.Tables)
	}

	start, end := slot(19, 0, 90)
	pending, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	confirmed, err := svc.Confirm(context.Background(), biz, pending.ID)
	if err != nil || confirmed.Table == nil || confirmed.Table.Capacity != 4 {
		t.Fatalf("auto assign = %+v err=%v", confirmed, err)
	}

	left, err := svc.Availability(context.Background(), biz, "2026-10-01", "19:00", "90", "3")
	if err != nil || !left.Available || len(left.Tables) != 1 || left.Tables[0].Capacity != 8 {
		t.Fatalf("remaining tables = %+v err=%v", left, err)
	}
	large, _ := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 8,
	})
	if _, err := svc.Confirm(context.Background(), biz, large.ID); err != nil {
		t.Fatalf("confirm large: %v", err)
	}
	none, err := svc.Availability(context.Background(), biz, "2026-10-01", "19:00", "90", "3")
	if err != nil || none.Available || len(none.Tables) != 0 {
		t.Fatalf("no tables = %+v err=%v", none, err)
	}
}

func TestScheduleExceptions(t *testing.T) {
	svc, _, biz, cust, _ := testWorld(t)
	start, end := slot(19, 0, 90)
	closed, err := svc.CreateException(context.Background(), biz, ExceptionInput{
		ExceptionDate: "2026-10-01", IsClosed: true, Note: ptr("Holiday"),
	})
	if err != nil || !closed.IsClosed {
		t.Fatalf("closed exception = %+v err=%v", closed, err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("closed day = %v", err)
	}

	openNote := "short day"
	if _, err := svc.UpdateException(context.Background(), biz, closed.ID, UpdateExceptionInput{
		IsClosed: boolPtr(false), OpenTime: ptr("10:00"), CloseTime: ptr("16:00"), Note: &openNote,
	}); err != nil {
		t.Fatalf("update exception: %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2,
	}); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("exception hours override = %v", err)
	}
	okStart, okEnd := slot(11, 0, 60)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: okStart, EndAt: okEnd, PartySize: 2,
	}); err != nil {
		t.Fatalf("inside exception hours: %v", err)
	}
	if err := svc.DeleteException(context.Background(), biz, closed.ID); err != nil {
		t.Fatalf("delete exception: %v", err)
	}
	if _, err := svc.Get(context.Background(), biz, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing reservation = %v", err)
	}
}

func TestReservationDepositConfirmation(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	store.seedDeposit(biz, true, "FIXED", money.FromMajor(100000))
	menuID := store.addMenuItem(biz, "Latte", "150000", "ACTIVE")
	start, end := slot(19, 0, 90)
	pending, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
		PaymentType: payment.PaymentTypeDeposit,
		Items:       []ReservationItemInput{{MenuItemID: menuID.String(), Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pay := store.paymentFor(pending.ID)
	if pay == nil || pay.Status != payment.StatusUnpaid || pay.Amount != money.FromMajor(100000) || pay.Currency != "IDR" {
		t.Fatalf("initial payment = %+v", pay)
	}
	if pay.PaymentType != payment.PaymentTypeDeposit {
		t.Fatalf("payment type = %s", pay.PaymentType)
	}
	if _, err := svc.Confirm(context.Background(), biz, pending.ID); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("unpaid confirm = %v", err)
	}
	store.setPaymentStatus(pending.ID, payment.StatusDeposit)
	confirmed, err := svc.Confirm(context.Background(), biz, pending.ID)
	if err != nil || confirmed.Status != StatusConfirmed {
		t.Fatalf("paid confirm = %+v err=%v", confirmed, err)
	}
	if pay := store.paymentFor(pending.ID); pay.Status != payment.StatusDeposit {
		t.Fatalf("confirm mutated payment = %+v", pay)
	}

	// Deposit alone is not enough to complete: the balance must be paid in full.
	if _, err := svc.Complete(context.Background(), biz, pending.ID); !errors.Is(err, ErrFullPaymentRequired) {
		t.Fatalf("complete with deposit only = %v", err)
	}
	store.setPaymentStatus(pending.ID, payment.StatusPaid)
	done, err := svc.Complete(context.Background(), biz, pending.ID)
	if err != nil || done.Status != StatusCompleted {
		t.Fatalf("complete after full payment = %+v err=%v", done, err)
	}

	start2, end2 := slot(17, 0, 60)
	refundedRes, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start2, EndAt: end2, PartySize: 2, TableID: ptr(table.String()),
		PaymentType: payment.PaymentTypeFull,
		Items:       []ReservationItemInput{{MenuItemID: menuID.String(), Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	store.setPaymentStatus(refundedRes.ID, payment.StatusRefunded)
	if _, err := svc.Confirm(context.Background(), biz, refundedRes.ID); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("refunded confirm = %v", err)
	}
}

func TestReservationCancelDoesNotRefundPayment(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	start, end := slot(19, 0, 90)
	pending, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.setPaymentStatus(pending.ID, payment.StatusPaid)
	cancelled, err := svc.Cancel(context.Background(), biz, pending.ID, ptr("guest cancelled"))
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	if pay := store.paymentFor(pending.ID); pay == nil || pay.Status != payment.StatusPaid {
		t.Fatalf("cancel refunded payment = %+v", pay)
	}
}

func TestCalendarBlocksAffectAvailabilityAndCreate(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	start, end := slot(19, 0, 90)
	loc := mustLoc()
	blockStart := time.Date(2026, 10, 1, 18, 0, 0, 0, loc)
	blockEnd := time.Date(2026, 10, 1, 20, 0, 0, 0, loc)
	store.seedBlock(biz, nil, nil, blockStart, blockEnd)

	avail, err := svc.Availability(context.Background(), biz, "2026-10-01", "19:00", "90", "2")
	if err != nil || avail.Available {
		t.Fatalf("business block availability = %+v err=%v", avail, err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	}); !errors.Is(err, ErrCalendarBlocked) {
		t.Fatalf("business block create = %v", err)
	}

	store.blocks = nil
	other := store.addTable(biz, "T8", "Patio", 8, "ACTIVE", "ACTIVE")
	area := store.tables[table].AreaID
	store.seedBlock(biz, &area, nil, blockStart, blockEnd)
	avail, err = svc.Availability(context.Background(), biz, "2026-10-01", "19:00", "90", "2")
	if err != nil || !avail.Available {
		t.Fatalf("area block should leave other area = %+v err=%v", avail, err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	}); !errors.Is(err, ErrCalendarBlocked) {
		t.Fatalf("area block create = %v", err)
	}
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(other.String()),
	}); err != nil {
		t.Fatalf("other area should book: %v", err)
	}

	store.blocks = nil
	store.seedBlock(biz, &area, &table, blockStart, blockEnd)
	if _, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	}); !errors.Is(err, ErrCalendarBlocked) {
		t.Fatalf("table block create = %v", err)
	}
}

func TestReservationReschedule(t *testing.T) {
	svc, store, biz, cust, table := testWorld(t)
	start, end := slot(19, 0, 90)
	pending, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	})
	if err != nil {
		t.Fatal(err)
	}
	nextStart, nextEnd := slot(17, 0, 60)
	moved, err := svc.Reschedule(context.Background(), biz, pending.ID, RescheduleInput{StartAt: nextStart, EndAt: nextEnd})
	if err != nil || !moved.StartAt.Equal(mustRFC(nextStart)) {
		t.Fatalf("reschedule = %+v err=%v", moved, err)
	}
	if pay := store.paymentFor(pending.ID); pay == nil || pay.Status != payment.StatusUnpaid {
		t.Fatalf("payment mutated = %+v", pay)
	}
	store.setPaymentStatus(pending.ID, payment.StatusPaid)
	if _, err := svc.Reschedule(context.Background(), biz, pending.ID, RescheduleInput{StartAt: start, EndAt: end}); err != nil {
		t.Fatalf("reschedule paid: %v", err)
	}
	if pay := store.paymentFor(pending.ID); pay.Status != payment.StatusPaid {
		t.Fatalf("paid reset = %+v", pay)
	}

	cancelled, err := svc.Cancel(context.Background(), biz, pending.ID, ptr("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reschedule(context.Background(), biz, cancelled.ID, RescheduleInput{StartAt: start, EndAt: end}); !errors.Is(err, ErrRescheduleNotAllowed) {
		t.Fatalf("cancel reschedule = %v", err)
	}

	done, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(context.Background(), biz, done.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Complete(context.Background(), biz, done.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reschedule(context.Background(), biz, done.ID, RescheduleInput{StartAt: nextStart, EndAt: nextEnd}); !errors.Is(err, ErrRescheduleNotAllowed) {
		t.Fatalf("complete reschedule = %v", err)
	}

	other, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: start, EndAt: end, PartySize: 2, TableID: ptr(table.String()),
	})
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := svc.Create(context.Background(), biz, CreateReservationInput{
		CustomerID: cust.String(), StartAt: nextStart, EndAt: nextEnd, PartySize: 2, TableID: ptr(table.String()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reschedule(context.Background(), biz, other.ID, RescheduleInput{StartAt: nextStart, EndAt: nextEnd}); !errors.Is(err, ErrRescheduleConflict) {
		t.Fatalf("overlap reschedule = %v", err)
	}
	if _, err := svc.Cancel(context.Background(), biz, blocker.ID, ptr("free slot")); err != nil {
		t.Fatal(err)
	}

	loc := mustLoc()
	store.seedBlock(biz, nil, nil, time.Date(2026, 10, 1, 16, 0, 0, 0, loc), time.Date(2026, 10, 1, 18, 0, 0, 0, loc))
	if _, err := svc.Reschedule(context.Background(), biz, other.ID, RescheduleInput{StartAt: nextStart, EndAt: nextEnd}); !errors.Is(err, ErrCalendarBlocked) {
		t.Fatalf("block reschedule = %v", err)
	}
}

func mustRFC(raw string) time.Time {
	t, _ := time.Parse(time.RFC3339, raw)
	return t
}

func TestOvernightHoursRejected(t *testing.T) {
	svc, _, biz, _, _ := testWorld(t)
	if _, err := svc.ReplaceHours(context.Background(), biz, []HourInput{
		{DayOfWeek: 1, SortOrder: 0, OpenTime: "18:00", CloseTime: "02:00"},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overnight = %v", err)
	}
}

func mustLoc() *time.Location {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return loc
}

func boolPtr(v bool) *bool { return &v }
