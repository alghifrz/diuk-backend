package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/reservation"
)

// Booking SOP enforced by the server, not by the prompt alone:
//
//  1. quote_reservation without payment_type  -> stage PAYMENT_CHOICE
//     (AI asks FULL vs DEPOSIT; customer answers in the next turn)
//  2. quote_reservation with payment_type     -> stage AWAITING_CONFIRMATION
//     (AI sends the order + reservation overview; customer confirms next turn)
//  3. create_reservation(quote_id)            -> PENDING reservation + handover
//
// Each step must happen in a later turn than the previous one. A turn is one
// GenerateReply call, i.e. one customer message, so the model cannot skip the
// customer's answer. When deposit is disabled step 1 is skipped.

const (
	QuoteStagePaymentChoice        = "PAYMENT_CHOICE"
	QuoteStageAwaitingConfirmation = "AWAITING_CONFIRMATION"

	quoteTTL = 2 * time.Hour
)

type QuoteItem struct {
	MenuItemID string
	Name       string
	Quantity   int
	UnitPrice  money.Amount
	Notes      *string
}

type Quote struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	ConversationID uuid.UUID
	CustomerID     uuid.UUID
	TurnID         uuid.UUID
	Stage          string
	Fingerprint    string
	IssuedAt       time.Time

	Date            string
	StartTime       string
	DurationMinutes int
	StartAt         time.Time
	EndAt           time.Time
	PartySize       int
	TableID         *string
	TableName       string
	AreaName        string
	Notes           *string
	PaymentType     string
	Items           []QuoteItem

	Currency      string
	OrderTotal    money.Amount
	FullAmount    money.Amount
	DepositAmount money.Amount
	AmountDue     money.Amount
	DepositOn     bool
}

func (q Quote) Expired(now time.Time) bool {
	return now.Sub(q.IssuedAt) > quoteTTL
}

// QuoteStore keeps the latest booking quote per conversation.
type QuoteStore interface {
	Get(conversationID uuid.UUID) (Quote, bool)
	Put(q Quote)
	Delete(conversationID uuid.UUID)
}

type MemoryQuoteStore struct {
	mu     sync.Mutex
	quotes map[uuid.UUID]Quote
	now    func() time.Time
}

func NewMemoryQuoteStore() *MemoryQuoteStore {
	return &MemoryQuoteStore{quotes: map[uuid.UUID]Quote{}, now: time.Now}
}

func (m *MemoryQuoteStore) Get(conversationID uuid.UUID) (Quote, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, ok := m.quotes[conversationID]
	if !ok {
		return Quote{}, false
	}
	if q.Expired(m.now()) {
		delete(m.quotes, conversationID)
		return Quote{}, false
	}
	return q, true
}

func (m *MemoryQuoteStore) Put(q Quote) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for id, existing := range m.quotes {
		if existing.Expired(now) {
			delete(m.quotes, id)
		}
	}
	m.quotes[q.ConversationID] = q
}

func (m *MemoryQuoteStore) Delete(conversationID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.quotes, conversationID)
}

// QuoteReservation validates a booking, computes amounts, and records which
// SOP stage the conversation is in. It never writes a reservation.
type QuoteReservation struct {
	Reservations ReservationAPI
	Business     BusinessLookup
	Menu         MenuLookup
	Quotes       QuoteStore
}

func (t QuoteReservation) Name() string { return "quote_reservation" }

func (t QuoteReservation) Description() string {
	return "Required before create_reservation. Validates the slot and pre-order, computes order_total / full_amount / deposit_amount, and tracks the booking SOP. " +
		"Step A: once date, time, party size, area, and menu items with quantities are known, call WITHOUT payment_type -> result stage PAYMENT_CHOICE: ask the guest FULL or DEPOSIT with both amounts, then stop and wait. " +
		"Step B: after the guest answers, call again WITH payment_type -> result stage AWAITING_CONFIRMATION with quote_id: send the full order + reservation overview and ask the guest to confirm, then stop and wait. " +
		"Step C: only after the guest confirms in a later message, call create_reservation with no arguments (do not call this tool again just to confirm). Re-call this tool only when the guest changes a detail."
}

func (t QuoteReservation) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"date":             stringSchema("Local date YYYY-MM-DD", "date"),
		"start_time":       stringSchema("Local start time HH:MM", "time"),
		"duration_minutes": intSchema(1, "Reservation length in minutes (same as check_availability)"),
		"party_size":       intSchema(1, "Number of guests"),
		"table_id":         stringSchema("Optional table id from check_availability", ""),
		"notes":            stringSchema("Area preference and guest notes, e.g. Indoor dekat jendela", ""),
		"payment_type":     stringSchema("FULL or DEPOSIT. Omit on the first call so the guest is asked; set it only after the guest chose.", ""),
		"items": arraySchema(
			objectSchema(map[string]Schema{
				"menu_item_id": stringSchema("menu_item_id from get_menu", ""),
				"quantity":     intSchema(1, "Quantity"),
				"notes":        stringSchema("Optional line note", ""),
			}, []string{"menu_item_id", "quantity"}),
			"Pre-order lines from get_menu (required)",
		),
	}, []string{"date", "start_time", "duration_minutes", "party_size", "items"})
}

func (t QuoteReservation) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	if exec.CustomerID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "customer context is missing")
	}
	if exec.ConversationID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "conversation context is missing")
	}
	if t.Quotes == nil || t.Reservations == nil || t.Menu == nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "booking quote is unavailable")
	}

	lines := parseReservationItems(input["items"])
	if len(lines) == 0 {
		return fail("AI_TOOL_INVALID_ARGUMENTS", "items are required — ask the guest to choose menu items via get_menu first")
	}
	date := strings.TrimSpace(stringArg(input, "date"))
	startTime := strings.TrimSpace(stringArg(input, "start_time"))
	duration := intArg(input, "duration_minutes")
	partySize := intArg(input, "party_size")
	paymentType := strings.ToUpper(strings.TrimSpace(stringArg(input, "payment_type")))
	if paymentType != "" && paymentType != payment.PaymentTypeFull && paymentType != payment.PaymentTypeDeposit {
		return fail("AI_TOOL_INVALID_ARGUMENTS", "payment_type must be FULL or DEPOSIT")
	}
	notes := optionalString(input, "notes")

	avail, err := t.Reservations.Availability(ctx, exec.BusinessID, date, startTime, strconv.Itoa(duration), strconv.Itoa(partySize))
	if err != nil {
		return reservationFailure(err)
	}
	if avail == nil || !avail.Available {
		return fail("RESERVATION_NO_TABLE_AVAILABLE", "no table is available for that slot — offer another time")
	}

	items, orderTotal, bad := t.priceItems(ctx, exec.BusinessID, lines)
	if bad != "" {
		return fail("RESERVATION_MENU_ITEM_NOT_FOUND", "menu item "+bad+" is not on the active menu — use menu_item_id from get_menu")
	}

	tableID, tableName, areaName := pickTable(strings.TrimSpace(stringArg(input, "table_id")), notes, avail.Tables)

	currency := "IDR"
	settings := payment.DepositSettings{Enabled: false, Type: payment.DepositFixed, Currency: currency}
	if t.Business != nil {
		if fnb, err := t.Business.FNBSettings(ctx, exec.BusinessID); err == nil && fnb != nil {
			currency = fnb.Currency
			settings = payment.DepositSettings{Enabled: fnb.DepositEnabled, Type: fnb.DepositType, Value: fnb.DepositValue, Currency: fnb.Currency}
		}
	}

	now := time.Now().UTC()
	previous, hasPrevious := t.Quotes.Get(exec.ConversationID)
	if hasPrevious && previous.CustomerID != exec.CustomerID {
		hasPrevious = false
	}

	q := Quote{
		ID:              uuid.New(),
		BusinessID:      exec.BusinessID,
		ConversationID:  exec.ConversationID,
		CustomerID:      exec.CustomerID,
		TurnID:          exec.TurnID,
		IssuedAt:        now,
		Date:            avail.Date,
		StartTime:       startTime,
		DurationMinutes: duration,
		StartAt:         avail.StartAt,
		EndAt:           avail.EndAt,
		PartySize:       partySize,
		TableID:         tableID,
		TableName:       tableName,
		AreaName:        areaName,
		Notes:           notes,
		Items:           items,
		Currency:        currency,
		OrderTotal:      orderTotal,
		DepositOn:       settings.Enabled,
	}

	if settings.Enabled {
		if full, _, err := payment.AmountDue(settings, payment.PaymentTypeFull, orderTotal); err == nil {
			q.FullAmount = full
		}
		dep, _, err := payment.AmountDue(settings, payment.PaymentTypeDeposit, orderTotal)
		if err != nil {
			return fail("AI_TOOL_EXECUTION_FAILED", "deposit is misconfigured — hand over to staff")
		}
		q.DepositAmount = dep

		if paymentType == "" {
			q.Stage = QuoteStagePaymentChoice
			q.Fingerprint = quoteFingerprint(q)
			if hasPrevious && previous.Stage == QuoteStagePaymentChoice && previous.Fingerprint == q.Fingerprint {
				// Same question already asked earlier; keep the original turn so the
				// customer's answer still counts as a later turn.
				q.TurnID, q.IssuedAt, q.ID = previous.TurnID, previous.IssuedAt, previous.ID
			}
			t.Quotes.Put(q)
			view := quoteView(q)
			view["customer_message_hint"] = fmt.Sprintf(
				"Send ONE message now: a short overview (date, start_time, party_size, area, every item with quantity and price, order_total %s %s) and ask the guest to choose the payment method: FULL %s %s or DEPOSIT %s %s. Then stop. Do NOT call create_reservation in this turn. When the guest answers FULL or DEPOSIT in their next message, call create_reservation with that payment_type — the reservation is recorded and handed to staff right then, there is no second confirmation question.",
				currency, q.OrderTotal.String(), currency, q.FullAmount.String(), currency, q.DepositAmount.String())
			return success(view)
		}

		// payment_type supplied: the question must have been asked in an earlier
		// turn. A previous AWAITING_CONFIRMATION quote already passed that gate.
		if !hasPrevious || (previous.TurnID == exec.TurnID && previous.Stage == QuoteStagePaymentChoice) {
			return fail("AI_PAYMENT_CHOICE_NOT_ASKED", "the guest has not been asked FULL vs DEPOSIT yet — call quote_reservation without payment_type, ask the question, and wait for the guest's answer in their next message")
		}
		q.PaymentType = paymentType
		due, _, err := payment.AmountDue(settings, paymentType, orderTotal)
		if err != nil {
			return fail("AI_TOOL_INVALID_ARGUMENTS", "payment_type cannot be applied to this order")
		}
		q.AmountDue = due
	} else {
		q.PaymentType = payment.PaymentTypeNone
	}

	q.Stage = QuoteStageAwaitingConfirmation
	q.Fingerprint = quoteFingerprint(q)
	if hasPrevious && previous.Stage == QuoteStagePaymentChoice && previous.TurnID != exec.TurnID &&
		detailsFingerprint(previous) == detailsFingerprint(q) {
		// The guest saw this exact overview together with the payment question and
		// has now answered it. That answer is the go-ahead: no extra confirmation.
		q.TurnID, q.IssuedAt, q.ID = previous.TurnID, previous.IssuedAt, previous.ID
		t.Quotes.Put(q)
		view := quoteView(q)
		view["quote_id"] = q.ID.String()
		view["already_shown"] = true
		view["customer_message_hint"] = "The guest already saw this overview and has now chosen the payment method. Do NOT send another overview or confirmation question. Call create_reservation NOW (payment_type " + paymentType + ") and then send the payment instructions."
		return success(view)
	}
	if hasPrevious && previous.Stage == QuoteStageAwaitingConfirmation && previous.Fingerprint == q.Fingerprint {
		q.TurnID, q.IssuedAt, q.ID = previous.TurnID, previous.IssuedAt, previous.ID
		if previous.TurnID != exec.TurnID {
			// The guest already saw this exact overview in an earlier message. Do not
			// tell the model to send it again — that is what caused the repeat loop.
			t.Quotes.Put(q)
			view := quoteView(q)
			view["quote_id"] = q.ID.String()
			view["already_shown"] = true
			view["customer_message_hint"] = "The guest ALREADY received this exact overview in an earlier message. If their latest message confirms it (ya, benar, oke, betul, lanjut, sudah), call create_reservation NOW with no arguments and do not repeat the overview. Only re-send the overview if the guest changed something or did not confirm."
			return success(view)
		}
	}
	t.Quotes.Put(q)

	view := quoteView(q)
	view["quote_id"] = q.ID.String()
	view["customer_message_hint"] = "Send the guest a complete overview now: date, start_time, party_size, area/table preference, every item with quantity and price, order_total" +
		depositHint(q) +
		". End by asking the guest to confirm (e.g. 'Sudah benar? Balas YA atau konfirmasi untuk kami catat'). Do NOT call create_reservation in this turn — only after the guest confirms in their next message (any clear yes), call create_reservation with no arguments. Nothing is saved yet."
	return success(view)
}

// PendingBookingBlock renders the server-side booking state for the system
// prompt. Chat history contains only message text, so without this the model
// forgets in the next turn what it asked and which quote is waiting.
func PendingBookingBlock(q Quote) string {
	var b strings.Builder
	b.WriteString("Pending booking (server state — trust this over your own memory):\n")
	fmt.Fprintf(&b, "- stage: %s\n", q.Stage)
	fmt.Fprintf(&b, "- date: %s, start_time: %s, duration_minutes: %d, party_size: %d\n", q.Date, q.StartAt.Format("15:04"), q.DurationMinutes, q.PartySize)
	if q.AreaName != "" {
		fmt.Fprintf(&b, "- area: %s\n", q.AreaName)
	}
	if q.Notes != nil {
		fmt.Fprintf(&b, "- notes: %s\n", *q.Notes)
	}
	b.WriteString("- items:\n")
	for _, item := range q.Items {
		fmt.Fprintf(&b, "  - menu_item_id=%s name=%q quantity=%d unit_price=%s\n", item.MenuItemID, item.Name, item.Quantity, item.UnitPrice.String())
	}
	fmt.Fprintf(&b, "- order_total: %s %s\n", q.Currency, q.OrderTotal.String())
	if q.DepositOn {
		fmt.Fprintf(&b, "- full_amount: %s, deposit_amount: %s\n", q.FullAmount.String(), q.DepositAmount.String())
	}

	switch q.Stage {
	case QuoteStagePaymentChoice:
		b.WriteString("State: you already sent the guest this overview and asked FULL vs DEPOSIT. If their latest message chooses one (full / deposit / dp / lunas / bayar penuh / bayar sebagian, etc.), your NEXT action MUST be create_reservation with payment_type FULL or DEPOSIT — it records the reservation and hands over to staff immediately. Do NOT send the overview again and do NOT ask for another confirmation. If they only say yes without choosing, ask which one (FULL or DEPOSIT). If they change a detail, call quote_reservation with the updated details. If they ask something else, answer it and keep waiting.\n")
	case QuoteStageAwaitingConfirmation:
		fmt.Fprintf(&b, "- payment_type: %s, amount_due: %s\n", q.PaymentType, q.AmountDue.String())
		b.WriteString("State: you already sent the guest the overview above and asked them to confirm. If their latest message confirms it (ya, benar, oke, betul, lanjut, sudah, siap, etc.), your NEXT action MUST be create_reservation with no arguments — do NOT repeat the overview and do NOT call quote_reservation. If they change anything, call quote_reservation with the updated details. If they ask a question, answer it and keep waiting for confirmation.\n")
	}
	return strings.TrimSpace(b.String())
}

func depositHint(q Quote) string {
	if !q.DepositOn {
		return ""
	}
	return fmt.Sprintf(", payment method %s, and amount_due %s %s", q.PaymentType, q.Currency, q.AmountDue.String())
}

func (t QuoteReservation) priceItems(ctx context.Context, businessID uuid.UUID, lines []reservation.ReservationItemInput) ([]QuoteItem, money.Amount, string) {
	menuItems, err := t.Menu.ListItems(ctx, businessID, "ACTIVE", nil)
	if err != nil {
		return nil, 0, "lookup failed"
	}
	byID := make(map[string]int, len(menuItems))
	for i := range menuItems {
		byID[menuItems[i].ID.String()] = i
	}
	out := make([]QuoteItem, 0, len(lines))
	total := money.Amount(0)
	for _, line := range lines {
		idx, ok := byID[line.MenuItemID]
		if !ok {
			return nil, 0, line.MenuItemID
		}
		unit, err := money.Parse(menuItems[idx].Price)
		if err != nil {
			return nil, 0, line.MenuItemID
		}
		out = append(out, QuoteItem{
			MenuItemID: line.MenuItemID,
			Name:       menuItems[idx].Name,
			Quantity:   line.Quantity,
			UnitPrice:  unit,
			Notes:      line.Notes,
		})
		total += money.Amount(int64(unit) * int64(line.Quantity))
	}
	return out, total, ""
}

func pickTable(rawTableID string, notes *string, tables []reservation.TableSummary) (*string, string, string) {
	if rawTableID != "" {
		if _, err := uuid.Parse(rawTableID); err == nil {
			for _, table := range tables {
				if table.ID.String() == rawTableID {
					id := rawTableID
					return &id, table.Name, table.AreaName
				}
			}
		}
	}
	if notes == nil || strings.TrimSpace(*notes) == "" || len(tables) == 0 {
		return nil, "", ""
	}
	if matched := reservation.MatchPreferredTable(*notes, tables); matched != nil {
		id := matched.ID.String()
		return &id, matched.Name, matched.AreaName
	}
	pref := strings.ToLower(*notes)
	for _, table := range tables {
		if table.AreaName != "" && strings.Contains(pref, strings.ToLower(table.AreaName)) {
			id := table.ID.String()
			return &id, table.Name, table.AreaName
		}
	}
	return nil, "", ""
}

func quoteFingerprint(q Quote) string {
	return fingerprintOf(q, q.Stage, q.PaymentType)
}

// detailsFingerprint identifies the booking content only, ignoring stage and
// payment type, so a PAYMENT_CHOICE quote can be matched to its answer.
func detailsFingerprint(q Quote) string {
	return fingerprintOf(q, "", "")
}

func fingerprintOf(q Quote, stage, paymentType string) string {
	parts := []string{
		stage, q.Date, q.StartTime, strconv.Itoa(q.DurationMinutes), strconv.Itoa(q.PartySize), paymentType, q.AreaName, q.TableName,
	}
	items := make([]string, 0, len(q.Items))
	for _, item := range q.Items {
		items = append(items, item.MenuItemID+"x"+strconv.Itoa(item.Quantity))
	}
	sort.Strings(items)
	parts = append(parts, items...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func quoteView(q Quote) map[string]any {
	items := make([]map[string]any, 0, len(q.Items))
	for _, item := range q.Items {
		items = append(items, map[string]any{
			"menu_item_id": item.MenuItemID,
			"name":         item.Name,
			"quantity":     item.Quantity,
			"unit_price":   item.UnitPrice.String(),
			"subtotal":     money.Amount(int64(item.UnitPrice) * int64(item.Quantity)).String(),
		})
	}
	view := map[string]any{
		"stage":           q.Stage,
		"date":            q.Date,
		"start_time":      q.StartAt.Format("15:04"),
		"party_size":      q.PartySize,
		"items":           items,
		"currency":        q.Currency,
		"order_total":     q.OrderTotal.String(),
		"deposit_enabled": q.DepositOn,
		"saved":           false,
	}
	if q.AreaName != "" {
		view["area"] = q.AreaName
	}
	if q.Notes != nil {
		view["notes"] = *q.Notes
	}
	if q.DepositOn {
		view["full_amount"] = q.FullAmount.String()
		view["deposit_amount"] = q.DepositAmount.String()
		if q.PaymentType != "" {
			view["payment_type"] = q.PaymentType
			view["amount_due"] = q.AmountDue.String()
		}
	}
	return view
}
