package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/menu"
	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/reservation"
)

type stubMenu struct{ items []menu.Item }

func (s stubMenu) ListCategories(context.Context, uuid.UUID, string) ([]menu.Category, error) {
	return nil, nil
}
func (s stubMenu) ListItems(context.Context, uuid.UUID, string, *uuid.UUID) ([]menu.Item, error) {
	return s.items, nil
}

type stubBusiness struct{ fnb business.FNBSettings }

func (s stubBusiness) Profile(context.Context, uuid.UUID) (*business.Profile, error) {
	return &business.Profile{}, nil
}
func (s stubBusiness) FNBSettings(context.Context, uuid.UUID) (*business.FNBSettings, error) {
	fnb := s.fnb
	return &fnb, nil
}

type stubHandover struct{ calls int }

func (s *stubHandover) RequestHumanHandover(_ context.Context, _, _ uuid.UUID, _ string) (*conversation.Conversation, error) {
	s.calls++
	return &conversation.Conversation{HandlingState: conversation.HandlingHumanRequested}, nil
}

func bookingArgs(itemID uuid.UUID, paymentType string) map[string]any {
	args := map[string]any{
		"date": "2026-10-02", "start_time": "15:00", "duration_minutes": 90, "party_size": 5,
		"notes": "Indoor dekat jendela",
		"items": []any{map[string]any{"menu_item_id": itemID.String(), "quantity": 2}},
	}
	if paymentType != "" {
		args["payment_type"] = paymentType
	}
	return args
}

func newDepositFixture(cust uuid.UUID, itemID uuid.UUID) (*recordingReservations, *MemoryQuoteStore, *stubHandover, QuoteReservation, CreateReservation) {
	api := &recordingReservations{item: reservation.Reservation{ID: uuid.New(), CustomerID: cust, PartySize: 5, Status: reservation.StatusPending}}
	quotes := NewMemoryQuoteStore()
	handover := &stubHandover{}
	menuStub := stubMenu{items: []menu.Item{{ID: itemID, Name: "Aren Signature", Price: "23000"}}}
	bizStub := stubBusiness{fnb: business.FNBSettings{
		Currency: "IDR", DepositEnabled: true, DepositType: "PERCENTAGE", DepositValue: money.FromMajor(50),
	}}
	quote := QuoteReservation{Reservations: api, Business: bizStub, Menu: menuStub, Quotes: quotes}
	create := CreateReservation{Reservations: api, Handover: handover, Business: bizStub, Quotes: quotes}
	return api, quotes, handover, quote, create
}

// The guest gets ONE message (overview + FULL/DEPOSIT question). Their answer
// records the PENDING reservation and hands over immediately.
func TestBookingSOPPaymentAnswerCreatesAndHandsOver(t *testing.T) {
	biz, cust, conv := uuid.New(), uuid.New(), uuid.New()
	itemID := uuid.New()
	api, quotes, handover, quote, create := newDepositFixture(cust, itemID)
	turn1 := ExecContext{BusinessID: biz, CustomerID: cust, ConversationID: conv, TurnID: uuid.New()}

	// Skipping straight to payment_type in the first turn is rejected.
	res, _ := quote.Execute(context.Background(), turn1, bookingArgs(itemID, "DEPOSIT"))
	if res.Code != "AI_PAYMENT_CHOICE_NOT_ASKED" {
		t.Fatalf("payment before asking = %+v", res)
	}

	// Turn 1: overview + payment question in one message.
	res, _ = quote.Execute(context.Background(), turn1, bookingArgs(itemID, ""))
	data, _ := res.Data.(map[string]any)
	if res.Error != "" || data["stage"] != QuoteStagePaymentChoice {
		t.Fatalf("payment choice = %+v", res)
	}
	if data["full_amount"] != "46000.00" || data["deposit_amount"] != "23000.00" || data["order_total"] != "46000.00" {
		t.Fatalf("amounts = %+v", data)
	}
	if hint, _ := data["customer_message_hint"].(string); !strings.Contains(hint, "overview") || !strings.Contains(hint, "FULL") || !strings.Contains(hint, "no second confirmation") {
		t.Fatalf("hint = %q", hint)
	}

	// The model cannot create in the same turn it asked.
	res, _ = create.Execute(context.Background(), turn1, map[string]any{"payment_type": "DEPOSIT"})
	if res.Code != "AI_CONFIRMATION_REQUIRED" || api.createCalls != 0 {
		t.Fatalf("same-turn create = %+v calls=%d", res, api.createCalls)
	}

	// Turn 2: the guest picked deposit. A missing payment_type is refused...
	turn2 := turn1
	turn2.TurnID = uuid.New()
	res, _ = create.Execute(context.Background(), turn2, map[string]any{})
	if res.Code != "AI_PAYMENT_CHOICE_REQUIRED" || api.createCalls != 0 {
		t.Fatalf("create without payment_type = %+v calls=%d", res, api.createCalls)
	}
	// ...and with payment_type it records the reservation and hands over at once.
	res, _ = create.Execute(context.Background(), turn2, map[string]any{"payment_type": "deposit"})
	if res.Error != "" || api.createCalls != 1 || handover.calls != 1 {
		t.Fatalf("create = %+v calls=%d handover=%d", res, api.createCalls, handover.calls)
	}
	if api.lastCreate.PaymentType != "DEPOSIT" || api.lastCreate.Notes == nil || len(api.lastCreate.Items) != 1 || api.lastCreate.Items[0].Quantity != 2 {
		t.Fatalf("create input = %+v", api.lastCreate)
	}
	if _, still := quotes.Get(conv); still {
		t.Fatal("quote should be consumed after create")
	}

	// Replays create nothing.
	res, _ = create.Execute(context.Background(), turn2, map[string]any{"payment_type": "DEPOSIT"})
	if res.Code != "AI_QUOTE_NOT_FOUND" || api.createCalls != 1 {
		t.Fatalf("replay = %+v calls=%d", res, api.createCalls)
	}
}

// If the model calls quote_reservation with payment_type after the guest
// answered, it must be told to create now rather than re-send the overview.
func TestBookingSOPQuoteWithPaymentTypeAfterAnswerDoesNotRepeatOverview(t *testing.T) {
	biz, cust, conv := uuid.New(), uuid.New(), uuid.New()
	itemID := uuid.New()
	api, quotes, handover, quote, create := newDepositFixture(cust, itemID)
	turn1 := ExecContext{BusinessID: biz, CustomerID: cust, ConversationID: conv, TurnID: uuid.New()}
	if res, _ := quote.Execute(context.Background(), turn1, bookingArgs(itemID, "")); res.Error != "" {
		t.Fatalf("payment choice = %+v", res)
	}

	turn2 := turn1
	turn2.TurnID = uuid.New()
	res, _ := quote.Execute(context.Background(), turn2, bookingArgs(itemID, "DEPOSIT"))
	data, _ := res.Data.(map[string]any)
	if res.Error != "" || data["already_shown"] != true || data["amount_due"] != "23000.00" {
		t.Fatalf("re-quote after answer = %+v", res)
	}
	if hint, _ := data["customer_message_hint"].(string); !strings.Contains(hint, "create_reservation NOW") {
		t.Fatalf("hint = %q", hint)
	}
	// Same turn is fine here: the guest saw the overview in turn 1.
	res, _ = create.Execute(context.Background(), turn2, map[string]any{})
	if res.Error != "" || api.createCalls != 1 || handover.calls != 1 || api.lastCreate.PaymentType != "DEPOSIT" {
		t.Fatalf("create = %+v calls=%d handover=%d input=%+v", res, api.createCalls, handover.calls, api.lastCreate)
	}
	if _, still := quotes.Get(conv); still {
		t.Fatal("quote should be consumed after create")
	}
}

// If the guest changes the order when answering, the new overview must be shown
// and confirmed before anything is recorded.
func TestBookingSOPChangedDetailsNeedNewConfirmation(t *testing.T) {
	biz, cust, conv := uuid.New(), uuid.New(), uuid.New()
	itemID := uuid.New()
	api, _, _, quote, create := newDepositFixture(cust, itemID)
	turn1 := ExecContext{BusinessID: biz, CustomerID: cust, ConversationID: conv, TurnID: uuid.New()}
	if res, _ := quote.Execute(context.Background(), turn1, bookingArgs(itemID, "")); res.Error != "" {
		t.Fatalf("payment choice = %+v", res)
	}

	turn2 := turn1
	turn2.TurnID = uuid.New()
	changed := bookingArgs(itemID, "DEPOSIT")
	changed["items"] = []any{map[string]any{"menu_item_id": itemID.String(), "quantity": 3}}
	res, _ := quote.Execute(context.Background(), turn2, changed)
	data, _ := res.Data.(map[string]any)
	if res.Error != "" || data["already_shown"] == true || data["stage"] != QuoteStageAwaitingConfirmation {
		t.Fatalf("changed quote = %+v", res)
	}
	res, _ = create.Execute(context.Background(), turn2, map[string]any{})
	if res.Code != "AI_CONFIRMATION_REQUIRED" || api.createCalls != 0 {
		t.Fatalf("create on changed order = %+v calls=%d", res, api.createCalls)
	}
	turn3 := turn1
	turn3.TurnID = uuid.New()
	res, _ = create.Execute(context.Background(), turn3, map[string]any{})
	if res.Error != "" || api.createCalls != 1 || api.lastCreate.Items[0].Quantity != 3 {
		t.Fatalf("create after confirm = %+v calls=%d input=%+v", res, api.createCalls, api.lastCreate)
	}
}

func TestPendingBookingBlockTellsModelToCreateOnAnswer(t *testing.T) {
	itemID := uuid.New()
	q := Quote{Stage: QuoteStagePaymentChoice, Date: "2026-10-02", DurationMinutes: 90, PartySize: 5, Currency: "IDR", DepositOn: true,
		Items: []QuoteItem{{MenuItemID: itemID.String(), Name: "Aren Signature", Quantity: 2}}}
	block := PendingBookingBlock(q)
	for _, want := range []string{"PAYMENT_CHOICE", "create_reservation", "payment_type", "Do NOT send the overview again", itemID.String()} {
		if !strings.Contains(block, want) {
			t.Fatalf("block missing %q:\n%s", want, block)
		}
	}
}

func TestBookingSOPWithoutDepositSkipsPaymentQuestion(t *testing.T) {
	biz, cust, conv := uuid.New(), uuid.New(), uuid.New()
	itemID := uuid.New()
	api := &recordingReservations{item: reservation.Reservation{ID: uuid.New(), CustomerID: cust, PartySize: 2, Status: reservation.StatusPending}}
	quotes := NewMemoryQuoteStore()
	menuStub := stubMenu{items: []menu.Item{{ID: itemID, Name: "Latte", Price: "30000"}}}
	bizStub := stubBusiness{fnb: business.FNBSettings{Currency: "IDR", DepositEnabled: false}}
	quote := QuoteReservation{Reservations: api, Business: bizStub, Menu: menuStub, Quotes: quotes}
	create := CreateReservation{Reservations: api, Business: bizStub, Quotes: quotes}

	turn1 := ExecContext{BusinessID: biz, CustomerID: cust, ConversationID: conv, TurnID: uuid.New()}
	res, _ := quote.Execute(context.Background(), turn1, bookingArgs(itemID, ""))
	data, _ := res.Data.(map[string]any)
	if res.Error != "" || data["stage"] != QuoteStageAwaitingConfirmation {
		t.Fatalf("no-deposit quote = %+v", res)
	}
	quoteID, _ := data["quote_id"].(string)

	turn2 := turn1
	turn2.TurnID = uuid.New()
	res, _ = create.Execute(context.Background(), turn2, map[string]any{"quote_id": quoteID})
	if res.Error != "" || api.createCalls != 1 || api.lastCreate.PaymentType != "" {
		t.Fatalf("no-deposit create = %+v calls=%d input=%+v", res, api.createCalls, api.lastCreate)
	}
}

func TestCreateReservationRejectsUnknownOrForeignQuote(t *testing.T) {
	biz, cust, conv := uuid.New(), uuid.New(), uuid.New()
	api := &recordingReservations{item: reservation.Reservation{ID: uuid.New(), CustomerID: cust}}
	quotes := NewMemoryQuoteStore()
	create := CreateReservation{Reservations: api, Quotes: quotes}
	exec := ExecContext{BusinessID: biz, CustomerID: cust, ConversationID: conv, TurnID: uuid.New()}

	// Nothing stored yet: both with and without a quote_id must fail safely.
	res, _ := create.Execute(context.Background(), exec, map[string]any{})
	if res.Code != "AI_QUOTE_NOT_FOUND" {
		t.Fatalf("no quote = %+v", res)
	}
	res, _ = create.Execute(context.Background(), exec, map[string]any{"quote_id": uuid.NewString()})
	if res.Code != "AI_QUOTE_NOT_FOUND" {
		t.Fatalf("unknown = %+v", res)
	}

	// A quote belonging to another customer in the same conversation is never used.
	quotes.Put(Quote{ID: uuid.New(), BusinessID: biz, ConversationID: conv, CustomerID: uuid.New(), Stage: QuoteStageAwaitingConfirmation, TurnID: uuid.New()})
	res, _ = create.Execute(context.Background(), exec, map[string]any{})
	if res.Code != "AI_QUOTE_NOT_FOUND" || api.createCalls != 0 {
		t.Fatalf("foreign customer quote = %+v calls=%d", res, api.createCalls)
	}

	// A mismatching quote_id for the guest's own quote is rejected too.
	quotes.Put(Quote{ID: uuid.New(), BusinessID: biz, ConversationID: conv, CustomerID: cust, Stage: QuoteStageAwaitingConfirmation, TurnID: uuid.New()})
	res, _ = create.Execute(context.Background(), exec, map[string]any{"quote_id": uuid.NewString()})
	if res.Code != "AI_QUOTE_NOT_FOUND" || api.createCalls != 0 {
		t.Fatalf("mismatched quote_id = %+v calls=%d", res, api.createCalls)
	}
}
