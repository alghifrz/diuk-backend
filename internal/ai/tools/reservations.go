package tools

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/payment"
	"github.com/alghifrz/diuk-backend/internal/reservation"
)

type CheckAvailability struct {
	Reservations ReservationAPI
}

func (t CheckAvailability) Name() string { return "check_availability" }

func (t CheckAvailability) Description() string {
	return "Check live table availability for a date, start time, duration, and party size. Always call this before claiming a table is free. Never invent availability. After this succeeds, call get_menu and ask area preference AND menu pre-order together — do not ask payment or final confirm yet. Once area and items are known, continue with quote_reservation."
}

func (t CheckAvailability) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"date":             stringSchema("Local date YYYY-MM-DD", "date"),
		"start_time":       stringSchema("Local start time HH:MM", "time"),
		"duration_minutes": intSchema(1, "Reservation length in minutes"),
		"party_size":       intSchema(1, "Number of guests"),
	}, []string{"date", "start_time", "duration_minutes", "party_size"})
}

func (t CheckAvailability) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	result, err := t.Reservations.Availability(
		ctx,
		exec.BusinessID,
		stringArg(input, "date"),
		stringArg(input, "start_time"),
		strconv.Itoa(intArg(input, "duration_minutes")),
		strconv.Itoa(intArg(input, "party_size")),
	)
	if err != nil {
		return reservationFailure(err)
	}
	tables := make([]map[string]any, 0, len(result.Tables))
	for _, table := range result.Tables {
		tables = append(tables, map[string]any{
			"table_id": table.ID.String(),
			"name":     table.Name,
			"capacity": table.Capacity,
			"area":     table.AreaName,
		})
	}
	return success(map[string]any{
		"date":       result.Date,
		"start_at":   result.StartAt.Format(time.RFC3339),
		"end_at":     result.EndAt.Format(time.RFC3339),
		"party_size": result.PartySize,
		"available":  result.Available,
		"tables":     tables,
	})
}

type CreateReservation struct {
	Reservations ReservationAPI
	Handover     HandoverAPI
	Business     BusinessLookup
	// Quotes enforces the booking SOP. When nil the legacy direct-create path is used.
	Quotes QuoteStore
}

func (t CreateReservation) Name() string { return "create_reservation" }

func (t CreateReservation) Description() string {
	if t.Quotes != nil {
		return "FINAL step of the booking SOP: record the PENDING reservation in the database and hand the chat to staff. Call it as soon as the guest answers the payment question you asked together with the overview: pass payment_type FULL or DEPOSIT as they chose (no other details needed). If there is no deposit option and the guest confirmed the overview, call it with no arguments. Do NOT re-send the overview and do NOT call quote_reservation again. It fails if the overview/question was only sent in this same turn. Returns amount_due and bank transfer details — reply MUST tell the guest to transfer amount_due to that bank account and that staff will confirm. Never invent bank details."
	}
	return "Create a PENDING reservation after the guest chose FULL or DEPOSIT. Prefer date+start_time+duration_minutes (same as check_availability) plus party_size, items, payment_type, and optional table_id or area preference in notes. Returns amount_due, full_amount, deposit_amount, and bank transfer details — reply MUST tell the guest to transfer amount_due to that bank account, then staff handover follows. Never invent bank details."
}

func (t CreateReservation) InputSchema() Schema {
	if t.Quotes != nil {
		return objectSchema(map[string]Schema{
			"payment_type": stringSchema("FULL or DEPOSIT — the method the guest chose in reply to the payment question. Omit only when deposit is not enabled.", ""),
			"quote_id":     stringSchema("Optional. Normally omit it: the server already holds the booking the guest was shown.", ""),
		}, nil)
	}
	return objectSchema(map[string]Schema{
		"customer_id":      stringSchema("Ignored unless it matches the current customer", ""),
		"date":             stringSchema("Local date YYYY-MM-DD (preferred with start_time)", "date"),
		"start_time":       stringSchema("Local start time HH:MM (preferred with date)", "time"),
		"duration_minutes": intSchema(1, "Reservation length in minutes (preferred with date)"),
		"start_at":         stringSchema("RFC3339 start timestamp (fallback if date/start_time omitted)", ""),
		"end_at":           stringSchema("RFC3339 end timestamp (fallback if date/start_time omitted)", ""),
		"party_size":       intSchema(1, "Number of guests"),
		"table_id":         stringSchema("Optional table id from check_availability", ""),
		"notes":            stringSchema("Optional guest notes / area preference e.g. Outdoor", ""),
		"payment_type":     stringSchema("FULL or DEPOSIT. Required when deposit is enabled.", ""),
		"items": arraySchema(
			objectSchema(map[string]Schema{
				"menu_item_id": stringSchema("menu_item_id from get_menu", ""),
				"quantity":     intSchema(1, "Quantity"),
				"notes":        stringSchema("Optional line note", ""),
			}, []string{"menu_item_id", "quantity"}),
			"Pre-order lines from get_menu (required)",
		),
	}, []string{"party_size", "items"})
}

func (t CreateReservation) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	if exec.CustomerID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "customer context is missing")
	}
	if t.Quotes != nil {
		return t.executeFromQuote(ctx, exec, input)
	}
	if raw := strings.TrimSpace(stringArg(input, "customer_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id != exec.CustomerID {
			return fail("AI_TOOL_INVALID_ARGUMENTS", "customer_id does not match the current conversation")
		}
	}
	in := reservation.CreateReservationInput{
		CustomerID:  exec.CustomerID.String(),
		StartAt:     stringArg(input, "start_at"),
		EndAt:       stringArg(input, "end_at"),
		PartySize:   intArg(input, "party_size"),
		Notes:       optionalString(input, "notes"),
		Items:       parseReservationItems(input["items"]),
		PaymentType: strings.ToUpper(strings.TrimSpace(stringArg(input, "payment_type"))),
	}
	if len(in.Items) == 0 {
		return fail("AI_TOOL_INVALID_ARGUMENTS", "items are required — ask the guest to choose menu items via get_menu before create_reservation")
	}
	if date := strings.TrimSpace(stringArg(input, "date")); date != "" {
		startTime := strings.TrimSpace(stringArg(input, "start_time"))
		duration := intArg(input, "duration_minutes")
		if startTime == "" || duration < 1 {
			return fail("AI_TOOL_INVALID_ARGUMENTS", "date requires start_time and duration_minutes")
		}
		avail, err := t.Reservations.Availability(ctx, exec.BusinessID, date, startTime, strconv.Itoa(duration), strconv.Itoa(in.PartySize))
		if err != nil {
			return reservationFailure(err)
		}
		if avail == nil || !avail.Available {
			return fail("RESERVATION_NO_TABLE_AVAILABLE", "no table is available for that slot")
		}
		in.StartAt = avail.StartAt.Format(time.RFC3339)
		in.EndAt = avail.EndAt.Format(time.RFC3339)
		if raw := strings.TrimSpace(stringArg(input, "table_id")); raw != "" {
			if _, err := uuid.Parse(raw); err != nil {
				return fail("AI_TOOL_INVALID_ARGUMENTS", "table_id is invalid")
			}
			in.TableID = &raw
		} else if in.Notes != nil {
			if matched := reservation.MatchPreferredTable(*in.Notes, avail.Tables); matched != nil {
				id := matched.ID.String()
				in.TableID = &id
			} else if len(avail.Tables) > 0 {
				pref := strings.ToLower(strings.TrimSpace(*in.Notes))
				chosen := avail.Tables[0]
				for _, table := range avail.Tables {
					if strings.Contains(pref, strings.ToLower(table.AreaName)) {
						chosen = table
						break
					}
				}
				id := chosen.ID.String()
				in.TableID = &id
			}
		}
	}
	if strings.TrimSpace(in.StartAt) == "" || strings.TrimSpace(in.EndAt) == "" {
		return fail("AI_TOOL_INVALID_ARGUMENTS", "provide date+start_time+duration_minutes or start_at+end_at")
	}
	if in.TableID == nil {
		if raw := strings.TrimSpace(stringArg(input, "table_id")); raw != "" {
			if _, err := uuid.Parse(raw); err != nil {
				return fail("AI_TOOL_INVALID_ARGUMENTS", "table_id is invalid")
			}
			in.TableID = &raw
		}
	}
	// If the model only wrote "Indoor, Meja 4" into notes, resolve a real table_id
	// from live availability so staff see an assigned table instead of free text.
	if in.TableID == nil && in.Notes != nil && strings.TrimSpace(*in.Notes) != "" {
		if resolved := resolveTableIDFromNotes(ctx, t.Reservations, exec.BusinessID, in); resolved != "" {
			in.TableID = &resolved
		}
	}
	return t.createAndHandover(ctx, exec, in)
}

// executeFromQuote is the SOP-enforced path: the reservation parameters come
// from the stored quote, never from the model, and the quote must have been
// shown to the guest in an earlier turn.
func (t CreateReservation) executeFromQuote(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	if exec.ConversationID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "conversation context is missing")
	}
	q, ok := t.Quotes.Get(exec.ConversationID)
	if !ok || q.CustomerID != exec.CustomerID || q.BusinessID != exec.BusinessID {
		return fail("AI_QUOTE_NOT_FOUND", "no booking overview is waiting for confirmation — call quote_reservation, send the overview, and wait for the guest to confirm")
	}
	// quote_id is optional (the model rarely remembers it across turns), but if
	// one is given it must match the stored quote.
	if raw := strings.TrimSpace(stringArg(input, "quote_id")); raw != "" {
		quoteID, err := uuid.Parse(raw)
		if err != nil || q.ID != quoteID {
			return fail("AI_QUOTE_NOT_FOUND", "quote_id does not match the booking waiting for confirmation — call create_reservation without arguments")
		}
	}
	if q.TurnID == exec.TurnID {
		return fail("AI_CONFIRMATION_REQUIRED", "the booking overview was just sent in this turn — the guest must answer in their next message before create_reservation")
	}
	if q.Stage == QuoteStagePaymentChoice {
		// The guest saw the overview with the payment question; their answer is the
		// go-ahead, so the chosen payment_type completes the booking right away.
		chosen := strings.ToUpper(strings.TrimSpace(stringArg(input, "payment_type")))
		if chosen != payment.PaymentTypeFull && chosen != payment.PaymentTypeDeposit {
			return fail("AI_PAYMENT_CHOICE_REQUIRED", "pass payment_type FULL or DEPOSIT as the guest chose; if they have not chosen yet, ask which one")
		}
		q.PaymentType = chosen
	} else if q.Stage != QuoteStageAwaitingConfirmation {
		return fail("AI_PAYMENT_CHOICE_REQUIRED", "the guest has not chosen FULL or DEPOSIT yet")
	}
	in := reservation.CreateReservationInput{
		CustomerID:  exec.CustomerID.String(),
		StartAt:     q.StartAt.Format(time.RFC3339),
		EndAt:       q.EndAt.Format(time.RFC3339),
		PartySize:   q.PartySize,
		TableID:     q.TableID,
		Notes:       q.Notes,
		PaymentType: q.PaymentType,
	}
	if in.PaymentType == payment.PaymentTypeNone {
		in.PaymentType = ""
	}
	for _, item := range q.Items {
		in.Items = append(in.Items, reservation.ReservationItemInput{MenuItemID: item.MenuItemID, Quantity: item.Quantity, Notes: item.Notes})
	}
	result, execErr := t.createAndHandover(ctx, exec, in)
	if execErr == nil && result.Error == "" {
		t.Quotes.Delete(exec.ConversationID)
	}
	return result, execErr
}

func (t CreateReservation) createAndHandover(ctx context.Context, exec ExecContext, in reservation.CreateReservationInput) (Result, error) {
	item, err := t.Reservations.Create(ctx, exec.BusinessID, in)
	if err != nil {
		return reservationFailure(err)
	}
	view := reservationView(*item)
	view["payment_type"] = in.PaymentType
	attachPaymentAmounts(ctx, t.Business, exec.BusinessID, item, in.PaymentType, view)
	// Payment bookings need staff to settle payment — hand over after create so the
	// model can still send one reply with amount_due / full / deposit from this result.
	if t.Handover != nil && exec.ConversationID != uuid.Nil {
		pt := strings.ToUpper(strings.TrimSpace(in.PaymentType))
		if pt == "FULL" || pt == "DEPOSIT" {
			if _, handErr := t.Handover.RequestHumanHandover(ctx, exec.BusinessID, exec.ConversationID, string(conversation.ReasonManualHandoff)); handErr == nil {
				view["handover_requested"] = true
				view["customer_message_hint"] = "Reservation is recorded PENDING. Reply MUST: (1) short booking summary, (2) tell guest to transfer amount_due (currency) to bank_name / bank_account_number a.n. bank_account_holder — never invent bank details, (3) also state full_amount and deposit_amount, (4) say a team member will confirm payment. If bank fields are missing, say staff will send transfer details."
			} else {
				view["handover_requested"] = false
				view["handover_error"] = "handover failed — call request_human_handover only AFTER create succeeded"
			}
		}
	}
	return success(view)
}

func attachPaymentAmounts(ctx context.Context, biz BusinessLookup, businessID uuid.UUID, item *reservation.Reservation, paymentType string, view map[string]any) {
	if item == nil {
		return
	}
	orderTotal := money.Amount(0)
	for _, line := range item.Items {
		unit, err := money.Parse(line.UnitPrice)
		if err != nil {
			continue
		}
		orderTotal += money.Amount(int64(unit) * int64(line.Quantity))
	}
	view["order_total"] = orderTotal.String()
	currency := "IDR"
	settings := payment.DepositSettings{Enabled: false, Type: payment.DepositFixed, Currency: currency}
	var fnb *business.FNBSettings
	if biz != nil {
		if got, err := biz.FNBSettings(ctx, businessID); err == nil && got != nil {
			fnb = got
			currency = fnb.Currency
			settings = payment.DepositSettings{
				Enabled:  fnb.DepositEnabled,
				Type:     fnb.DepositType,
				Value:    fnb.DepositValue,
				Currency: fnb.Currency,
			}
		}
	}
	view["currency"] = currency
	if fnb != nil {
		if fnb.BankName != nil {
			view["bank_name"] = *fnb.BankName
		}
		if fnb.BankAccountNumber != nil {
			view["bank_account_number"] = *fnb.BankAccountNumber
		}
		if fnb.BankAccountHolder != nil {
			view["bank_account_holder"] = *fnb.BankAccountHolder
		}
	}
	if !settings.Enabled {
		return
	}
	if full, _, err := payment.AmountDue(settings, payment.PaymentTypeFull, orderTotal); err == nil {
		view["full_amount"] = full.String()
	}
	if dep, _, err := payment.AmountDue(settings, payment.PaymentTypeDeposit, orderTotal); err == nil {
		view["deposit_amount"] = dep.String()
	}
	pt := strings.ToUpper(strings.TrimSpace(paymentType))
	if due, _, err := payment.AmountDue(settings, pt, orderTotal); err == nil {
		view["amount_due"] = due.String()
	}
}

func resolveTableIDFromNotes(ctx context.Context, api ReservationAPI, businessID uuid.UUID, in reservation.CreateReservationInput) string {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(in.StartAt))
	if err != nil {
		return ""
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(in.EndAt))
	if err != nil {
		return ""
	}
	duration := int(end.Sub(start).Minutes())
	if duration <= 0 {
		return ""
	}
	avail, err := api.Availability(
		ctx,
		businessID,
		start.Format("2006-01-02"),
		start.Format("15:04"),
		strconv.Itoa(duration),
		strconv.Itoa(in.PartySize),
	)
	if err != nil || avail == nil || !avail.Available {
		return ""
	}
	matched := reservation.MatchPreferredTable(*in.Notes, avail.Tables)
	if matched == nil {
		return ""
	}
	return matched.ID.String()
}

func parseReservationItems(raw any) []reservation.ReservationItemInput {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]reservation.ReservationItemInput, 0, len(arr))
	for _, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(stringArg(obj, "menu_item_id"))
		qty := intArg(obj, "quantity")
		if id == "" || qty < 1 {
			continue
		}
		out = append(out, reservation.ReservationItemInput{
			MenuItemID: id,
			Quantity:   qty,
			Notes:      optionalString(obj, "notes"),
		})
	}
	return out
}

type ConfirmReservation struct {
	Reservations ReservationAPI
}

func (t ConfirmReservation) Name() string { return "confirm_reservation" }

func (t ConfirmReservation) Description() string {
	return "Confirm a PENDING reservation for the current customer. The reservation service assigns a table and enforces availability. Never claim confirmation unless this tool succeeds."
}

func (t ConfirmReservation) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"reservation_id": stringSchema("Reservation to confirm", ""),
	}, []string{"reservation_id"})
}

func (t ConfirmReservation) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	item, result, err := loadOwnedReservation(ctx, t.Reservations, exec, stringArg(input, "reservation_id"))
	if err != nil || result.Error != "" {
		return result, err
	}
	confirmed, confirmErr := t.Reservations.Confirm(ctx, exec.BusinessID, item.ID)
	if confirmErr != nil {
		return reservationFailure(confirmErr)
	}
	return success(reservationView(*confirmed))
}

type CancelReservation struct {
	Reservations ReservationAPI
}

func (t CancelReservation) Name() string { return "cancel_reservation" }

func (t CancelReservation) Description() string {
	return "Cancel a reservation for the current customer. Optional reason. Cannot cancel another customer's booking."
}

func (t CancelReservation) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"reservation_id": stringSchema("Reservation to cancel", ""),
		"reason":         stringSchema("Optional cancellation reason", ""),
	}, []string{"reservation_id"})
}

func (t CancelReservation) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	item, result, err := loadOwnedReservation(ctx, t.Reservations, exec, stringArg(input, "reservation_id"))
	if err != nil || result.Error != "" {
		return result, err
	}
	cancelled, cancelErr := t.Reservations.Cancel(ctx, exec.BusinessID, item.ID, optionalString(input, "reason"))
	if cancelErr != nil {
		return reservationFailure(cancelErr)
	}
	return success(reservationView(*cancelled))
}

func loadOwnedReservation(ctx context.Context, api ReservationAPI, exec ExecContext, rawID string) (*reservation.Reservation, Result, error) {
	if exec.CustomerID == uuid.Nil {
		result, err := fail("AI_CONTEXT_ERROR", "customer context is missing")
		return nil, result, err
	}
	id, parseErr := uuid.Parse(strings.TrimSpace(rawID))
	if parseErr != nil {
		result, err := fail("AI_TOOL_INVALID_ARGUMENTS", "reservation_id is invalid")
		return nil, result, err
	}
	item, err := api.Get(ctx, exec.BusinessID, id)
	if err != nil {
		result, failErr := reservationFailure(err)
		return nil, result, failErr
	}
	if item.CustomerID != exec.CustomerID {
		result, failErr := fail("AI_TOOL_EXECUTION_FAILED", "reservation not found")
		return nil, result, failErr
	}
	return item, Result{}, nil
}

func reservationView(item reservation.Reservation) map[string]any {
	out := map[string]any{
		"reservation_id":        item.ID.String(),
		"date":                  item.StartAt.Format("2006-01-02"),
		"start_time":            item.StartAt.Format("15:04"),
		"customer_display_time": item.StartAt.Format("15:04"),
		"start_at":              item.StartAt.Format(time.RFC3339),
		"end_at":                item.EndAt.Format(time.RFC3339),
		"party_size":            item.PartySize,
		"status":                string(item.Status),
		"notes":                 item.Notes,
		"customer_message_hint": "Tell the customer only start_time (or customer_display_time). Do not mention end_at or any end time range.",
	}
	if item.Table != nil {
		out["table"] = map[string]any{
			"name":     item.Table.Name,
			"capacity": item.Table.Capacity,
			"area":     item.Table.AreaName,
		}
	}
	if len(item.Items) > 0 {
		lines := make([]map[string]any, 0, len(item.Items))
		for _, line := range item.Items {
			lines = append(lines, map[string]any{
				"menu_item_id":   line.MenuItemID.String(),
				"menu_item_name": line.MenuItemName,
				"quantity":       line.Quantity,
				"unit_price":     line.UnitPrice,
			})
		}
		out["items"] = lines
	}
	return out
}

func reservationFailure(err error) (Result, error) {
	switch {
	case errors.Is(err, reservation.ErrDisabled):
		return fail("RESERVATION_DISABLED", "reservations are disabled")
	case errors.Is(err, reservation.ErrInvalidStatus), errors.Is(err, reservation.ErrCannotUpdate):
		return fail("RESERVATION_INVALID_STATUS", "reservation status cannot change")
	case errors.Is(err, reservation.ErrOutsideHours):
		return fail("RESERVATION_OUTSIDE_OPERATING_HOURS", "requested time is outside operating hours")
	case errors.Is(err, reservation.ErrOutsideAdvance):
		return fail("RESERVATION_OUTSIDE_ADVANCE_WINDOW", "requested time is outside the advance booking window (too soon before min_advance_minutes, or beyond max_advance_days). Recheck date using current_local_date in the business timezone.")
	case errors.Is(err, reservation.ErrPartySize):
		return fail("RESERVATION_PARTY_SIZE_INVALID", "party size is invalid")
	case errors.Is(err, reservation.ErrNoTableAvailable), errors.Is(err, reservation.ErrTableUnavailable), errors.Is(err, reservation.ErrConflict):
		return fail("RESERVATION_NO_TABLE_AVAILABLE", "no table is available")
	case errors.Is(err, reservation.ErrCapacity):
		return fail("RESERVATION_TABLE_CAPACITY_INSUFFICIENT", "table capacity is insufficient")
	case errors.Is(err, reservation.ErrNotFound):
		return fail("RESERVATION_NOT_FOUND", "reservation not found")
	case errors.Is(err, reservation.ErrCustomerNotFound):
		return fail("RESERVATION_CUSTOMER_NOT_FOUND", "customer not found")
	case errors.Is(err, reservation.ErrMenuItemNotFound):
		return fail("RESERVATION_MENU_ITEM_NOT_FOUND", "menu item not found")
	case errors.Is(err, reservation.ErrMenuItemInactive):
		return fail("RESERVATION_MENU_ITEM_INACTIVE", "menu item is not active")
	case errors.Is(err, reservation.ErrInvalidMenuItem):
		return fail("AI_TOOL_INVALID_ARGUMENTS", "reservation menu item is invalid")
	case errors.Is(err, reservation.ErrInvalid), errors.Is(err, reservation.ErrInvalidTimeRange), errors.Is(err, reservation.ErrCrossMidnight):
		return fail("AI_TOOL_INVALID_ARGUMENTS", "reservation request is invalid")
	default:
		return fail("AI_TOOL_EXECUTION_FAILED", "reservation action failed")
	}
}
