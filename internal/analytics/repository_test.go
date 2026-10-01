package analytics

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryTenantAndNotes(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	bizRepo := tenant.NewBusinessRepository(tx)
	custSvc := customer.NewService(customer.NewRepository(tx))
	convSvc := conversation.NewService(conversation.NewRepository(tx), tenant.NewUserRepository(tx), tenant.NewBusinessUserRepository(tx))

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "A", Slug: "an-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "B", Slug: "bn-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	custA, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "Ada"})
	if err != nil {
		t.Fatalf("cust A: %v", err)
	}
	if _, err := custSvc.CreateCustomer(ctx, b.ID, customer.CreateCustomerInput{Name: "Bob"}); err != nil {
		t.Fatalf("cust B: %v", err)
	}
	conv, _, err := convSvc.CreateConversation(ctx, a.ID, conversation.CreateConversationInput{
		CustomerID: custA.ID.String(), Channel: "WEBSITE",
	})
	if err != nil {
		t.Fatalf("conv: %v", err)
	}
	if _, _, err := convSvc.CreateMessage(ctx, a.ID, conv.ID, conversation.CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("hello"),
	}); err != nil {
		t.Fatalf("inbound: %v", err)
	}
	if _, err := convSvc.CreateNote(ctx, a.ID, conv.ID, uuid.New(), "private"); err != nil {
		t.Fatalf("note: %v", err)
	}

	repo := NewRepository(tx)
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	overA, err := repo.Overview(ctx, a.ID, from, to)
	if err != nil {
		t.Fatalf("overview A: %v", err)
	}
	if overA.TotalConversations != 1 || overA.InboundMessages != 1 || overA.HumanMessages != 0 {
		t.Fatalf("overview A = %+v", overA)
	}
	overB, err := repo.Overview(ctx, b.ID, from, to)
	if err != nil {
		t.Fatalf("overview B: %v", err)
	}
	if overB.TotalConversations != 0 || overB.InboundMessages != 0 || overB.NewCustomers != 1 {
		t.Fatalf("overview B leaked A: %+v", overB)
	}
	msg, err := repo.Messaging(ctx, a.ID, from, to, "UTC")
	if err != nil {
		t.Fatalf("messaging: %v", err)
	}
	if msg.InboundMessages != 1 || msg.OutboundMessages != 0 {
		t.Fatalf("notes counted as messages: %+v", msg)
	}

	var tableID uuid.UUID
	if err := tx.QueryRow(ctx, `
		WITH area AS (
			INSERT INTO areas (business_id, name) VALUES ($1, 'Main') RETURNING id
		)
		INSERT INTO tables (area_id, name, capacity)
		SELECT id, 'T1', 4 FROM area
		RETURNING id`, a.ID).Scan(&tableID); err != nil {
		t.Fatalf("table: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status)
		VALUES
			($1, $2, NULL, 2, now() + interval '1 hour', now() + interval '2 hours', 'PENDING'),
			($1, $2, $3, 2, now() + interval '3 hours', now() + interval '4 hours', 'CONFIRMED'),
			($1, $2, NULL, 2, now() + interval '5 hours', now() + interval '6 hours', 'CANCELLED'),
			($1, $2, NULL, 2, now() + interval '7 hours', now() + interval '8 hours', 'COMPLETED')`,
		a.ID, custA.ID, tableID); err != nil {
		t.Fatalf("reservations: %v", err)
	}
	resA, err := repo.Reservations(ctx, a.ID, from, to, "UTC")
	if err != nil {
		t.Fatalf("reservations A: %v", err)
	}
	if resA.Total != 4 || resA.Pending != 1 || resA.Confirmed != 1 || resA.Cancelled != 1 || resA.Completed != 1 {
		t.Fatalf("reservation counts = %+v", resA)
	}
	if resA.ConfirmationRate != 0.25 || resA.CancellationRate != 0.25 || resA.CompletionRate != 0.25 {
		t.Fatalf("reservation rates = %+v", resA)
	}
	resB, err := repo.Reservations(ctx, b.ID, from, to, "UTC")
	if err != nil || resB.Total != 0 || resB.ConfirmationRate != 0 {
		t.Fatalf("reservation leak B = %+v err=%v", resB, err)
	}

	var inboundIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		msg, _, err := convSvc.CreateMessage(ctx, a.ID, conv.ID, conversation.CreateMessageInput{
			Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("follow-up"),
		})
		if err != nil {
			t.Fatalf("ai inbound: %v", err)
		}
		inboundIDs = append(inboundIDs, msg.ID)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE conversations
		SET handover_requested_at = now(),
		    handover_reason = 'CUSTOMER_REQUESTED_HUMAN',
		    human_started_at = now() + interval '2 minutes',
		    resolved_at = now() + interval '12 minutes',
		    handling_state = 'RESOLVED'
		WHERE id = $1`, conv.ID); err != nil {
		t.Fatalf("handover update: %v", err)
	}
	hand, err := repo.Handover(ctx, a.ID, from, to, "UTC")
	if err != nil {
		t.Fatalf("handover: %v", err)
	}
	if hand.HandoverCount != 1 || hand.CustomerRequestedCount != 1 || hand.HumanStartedCount != 1 || hand.HumanResolvedCount != 1 {
		t.Fatalf("handover = %+v", hand)
	}
	if hand.AverageTimeToHumanSeconds == nil || *hand.AverageTimeToHumanSeconds < 60 {
		t.Fatalf("time to human = %v", hand.AverageTimeToHumanSeconds)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_generations (inbound_message_id, business_id, conversation_id, status, provider, model, latency_ms, started_at, completed_at)
		VALUES
			($3, $1, $2, 'SUCCESS', 'openai', 'gpt-4o-mini', 120, now(), now()),
			($4, $1, $2, 'FAILED', 'openai', 'gpt-4o-mini', 80, now(), now()),
			($5, $1, $2, 'SKIPPED', 'openai', 'gpt-4o-mini', NULL, now(), now())`,
		a.ID, conv.ID, inboundIDs[0], inboundIDs[1], inboundIDs[2]); err != nil {
		t.Fatalf("ai generations: %v", err)
	}
	aiA, err := repo.AI(ctx, a.ID, from, to, "UTC")
	if err != nil {
		t.Fatalf("ai: %v", err)
	}
	if aiA.TotalGenerations != 3 || aiA.SuccessfulGenerations != 1 || aiA.FailedGenerations != 1 || aiA.SkippedGenerations != 1 {
		t.Fatalf("ai = %+v", aiA)
	}
	if aiA.AverageLatencyMS == nil {
		t.Fatal("expected latency average")
	}
	aiB, err := repo.AI(ctx, b.ID, from, to, "UTC")
	if err != nil || aiB.TotalGenerations != 0 {
		t.Fatalf("ai leak B = %+v err=%v", aiB, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO automations (business_id, name, status, trigger_type, action_type)
		VALUES ($1, 'welcome', 'ACTIVE', 'CUSTOMER_CREATED', 'SEND_MESSAGE')
		RETURNING id`, a.ID); err != nil {
		t.Fatalf("automation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO automation_runs (business_id, automation_id, status)
		SELECT $1, id, 'COMPLETED' FROM automations WHERE business_id = $1
		UNION ALL
		SELECT $1, id, 'FAILED' FROM automations WHERE business_id = $1
		UNION ALL
		SELECT $1, id, 'CANCELLED' FROM automations WHERE business_id = $1`, a.ID); err != nil {
		t.Fatalf("automation runs: %v", err)
	}
	auto, err := repo.Automation(ctx, a.ID, from, to, "UTC")
	if err != nil {
		t.Fatalf("automation: %v", err)
	}
	if auto.TotalRuns != 3 || auto.SuccessfulRuns != 1 || auto.FailedRuns != 1 || auto.SkippedRuns != 1 {
		t.Fatalf("automation = %+v", auto)
	}
	if auto.SuccessRate != 1.0/3.0 || auto.FailureRate != 1.0/3.0 {
		t.Fatalf("automation rates = %+v", auto)
	}

	oldFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	oldTo := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	emptyRange, err := repo.Overview(ctx, a.ID, oldFrom, oldTo)
	if err != nil || emptyRange.TotalConversations != 0 || emptyRange.NewCustomers != 0 || emptyRange.TotalReservations != 0 {
		t.Fatalf("half-open empty range = %+v err=%v", emptyRange, err)
	}
	cust, err := repo.Customers(ctx, a.ID, oldFrom, oldTo, "UTC")
	if err != nil || cust.NewCustomers != 0 || cust.TotalCustomers != 1 || cust.CustomersWithConversations != 1 || cust.CustomersWithReservations != 1 {
		t.Fatalf("customer snapshot = %+v err=%v", cust, err)
	}
}

func TestRepositoryConversionRevenueAndTimezone(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	bizRepo := tenant.NewBusinessRepository(tx)
	custSvc := customer.NewService(customer.NewRepository(tx))
	convSvc := conversation.NewService(conversation.NewRepository(tx), tenant.NewUserRepository(tx), tenant.NewBusinessUserRepository(tx))

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Rev A", Slug: "rva-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Rev B", Slug: "rvb-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("biz B: %v", err)
	}
	user, err := tenant.NewUserRepository(tx).Create(ctx, tenant.CreateUserParams{
		AuthUserID: uuid.New(), FullName: "Pay Admin", Email: "rev." + uuid.NewString()[:8] + "@example.com",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	custA, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "Ada"})
	if err != nil {
		t.Fatalf("cust A: %v", err)
	}
	custNone, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "NoRes"})
	if err != nil {
		t.Fatalf("cust none: %v", err)
	}
	custB, err := custSvc.CreateCustomer(ctx, b.ID, customer.CreateCustomerInput{Name: "Bob"})
	if err != nil {
		t.Fatalf("cust B: %v", err)
	}
	convA, _, err := convSvc.CreateConversation(ctx, a.ID, conversation.CreateConversationInput{
		CustomerID: custA.ID.String(), Channel: "WEBSITE",
	})
	if err != nil {
		t.Fatalf("conv A: %v", err)
	}
	convNone, _, err := convSvc.CreateConversation(ctx, a.ID, conversation.CreateConversationInput{
		CustomerID: custNone.ID.String(), Channel: "WEBSITE",
	})
	if err != nil {
		t.Fatalf("conv none: %v", err)
	}

	var tableID uuid.UUID
	if err := tx.QueryRow(ctx, `
		WITH area AS (
			INSERT INTO areas (business_id, name) VALUES ($1, 'Indoor') RETURNING id
		)
		INSERT INTO tables (area_id, name, capacity)
		SELECT id, 'T1', 4 FROM area
		RETURNING id`, a.ID).Scan(&tableID); err != nil {
		t.Fatalf("table: %v", err)
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 10, 3, 0, 0, 0, 0, loc)
	included := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	if _, err := tx.Exec(ctx, `UPDATE conversations SET created_at = $1 WHERE id IN ($2, $3)`, included, convA.ID, convNone.ID); err != nil {
		t.Fatalf("backdate conversations: %v", err)
	}
	middle := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dinner := time.Date(2026, 10, 1, 19, 0, 0, 0, loc)
	excluded := to
	before := time.Date(2026, 9, 30, 23, 0, 0, 0, loc)

	insertRes := func(businessID, customerID uuid.UUID, table any, party int, status string, created, start time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO reservations (business_id, customer_id, table_id, party_size, start_at, end_at, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`, businessID, customerID, table, party, start, start.Add(90*time.Minute), status, created,
		).Scan(&id); err != nil {
			t.Fatalf("reservation %s: %v", status, err)
		}
		return id
	}
	pendingID := insertRes(a.ID, custA.ID, nil, 2, "PENDING", included, dinner)
	confirmedID := insertRes(a.ID, custA.ID, tableID, 4, "CONFIRMED", middle, dinner)
	completedID := insertRes(a.ID, custA.ID, nil, 2, "COMPLETED", middle, dinner)
	cancelledID := insertRes(a.ID, custA.ID, nil, 3, "CANCELLED", middle, dinner)
	_ = insertRes(a.ID, custA.ID, nil, 2, "PENDING", excluded, dinner)
	_ = insertRes(a.ID, custA.ID, nil, 2, "PENDING", before, dinner)
	otherID := insertRes(b.ID, custB.ID, nil, 2, "COMPLETED", middle, dinner)

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		SET completed_at = $1
		WHERE id IN ($2, $3)`, middle, completedID, otherID); err != nil {
		t.Fatalf("set completed_at: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO reservation_payments (business_id, reservation_id, status, amount, currency, confirmed_at, confirmed_by, refunded_at, refunded_by, created_at)
		VALUES
			($1, $2, 'UNPAID', 100000, 'IDR', NULL, NULL, NULL, NULL, $8),
			($1, $3, 'PAID', 50000, 'IDR', $8, $7, NULL, NULL, $8),
			($1, $4, 'PAID', 75000, 'IDR', $8, $7, NULL, NULL, $8),
			($1, $5, 'REFUNDED', 25000, 'IDR', NULL, NULL, $8, $7, $8),
			($6, $9, 'PAID', 999999, 'IDR', $8, $7, NULL, NULL, $8)`,
		a.ID, pendingID, confirmedID, completedID, cancelledID, b.ID, user.ID, middle, otherID,
	); err != nil {
		t.Fatalf("payments: %v", err)
	}

	repo := NewRepository(tx)
	empty, err := repo.Conversion(ctx, a.ID, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), "Asia/Jakarta")
	if err != nil || empty.Conversations != 0 || empty.ReservationsCreated != 0 || empty.ConversationReservationRate != 0 || empty.PaymentCollectionRate != 0 {
		t.Fatalf("empty conversion = %+v err=%v", empty, err)
	}
	emptyPay, err := repo.Revenue(ctx, a.ID, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), "Asia/Jakarta")
	if err != nil || emptyPay.PaymentRecords != 0 || emptyPay.TotalAmount != 0 || emptyPay.PaymentCollectionRate != 0 {
		t.Fatalf("empty revenue = %+v err=%v", emptyPay, err)
	}

	conv, err := repo.Conversion(ctx, a.ID, from.UTC(), to.UTC(), "Asia/Jakarta")
	if err != nil {
		t.Fatalf("conversion: %v", err)
	}
	if conv.Conversations != 2 || conv.ConversationsWithReservation != 1 || conv.ConversationReservationRate != 0.5 {
		t.Fatalf("funnel conversations = %+v", conv)
	}
	if conv.ReservationsCreated != 4 || conv.ReservationsConfirmed != 1 || conv.ReservationsCompleted != 1 || conv.ReservationsCancelled != 1 {
		t.Fatalf("funnel reservations = %+v", conv)
	}
	if conv.ReservationsRequiringPayment != 4 || conv.ReservationsPaid != 2 || conv.ReservationsUnpaid != 1 {
		t.Fatalf("funnel payments = %+v", conv)
	}
	if conv.ReservationsConfirmedPaid != 2 || conv.ReservationsCancelledAfterPayment != 1 || conv.ReservationsRefunded != 1 {
		t.Fatalf("funnel paid outcomes = %+v", conv)
	}
	if conv.PaymentCollectionRate != 0.5 || conv.PaidReservationConfirmationRate != 1 {
		t.Fatalf("funnel rates = %+v", conv)
	}
	if len(conv.Series) == 0 || conv.Series[0].Date != "2026-10-01" {
		t.Fatalf("conversion series = %+v", conv.Series)
	}

	convB, err := repo.Conversion(ctx, b.ID, from.UTC(), to.UTC(), "Asia/Jakarta")
	if err != nil || convB.Conversations != 0 || convB.ReservationsCreated != 1 || convB.ReservationsPaid != 1 {
		t.Fatalf("conversion leak B = %+v err=%v", convB, err)
	}

	res, err := repo.Reservations(ctx, a.ID, from.UTC(), to.UTC(), "Asia/Jakarta")
	if err != nil {
		t.Fatalf("reservations: %v", err)
	}
	if res.Total != 4 || res.TotalPartySize != 11 || res.AveragePartySize != 2.75 {
		t.Fatalf("party size = %+v", res)
	}
	if len(res.ByHour) != 24 || res.ByHour[19].ReservationCount != 4 {
		t.Fatalf("by hour = %+v", res.ByHour)
	}
	if len(res.ByWeekday) != 7 || res.ByWeekday[4].ReservationCount != 4 {
		t.Fatalf("by weekday = %+v", res.ByWeekday)
	}
	foundTable := false
	for _, item := range res.ByTable {
		if item.TableName == "T1" && item.ReservationCount == 1 && item.ConfirmedCount == 1 {
			foundTable = true
		}
	}
	if !foundTable {
		t.Fatalf("by table = %+v", res.ByTable)
	}

	pay, err := repo.Revenue(ctx, a.ID, from.UTC(), to.UTC(), "Asia/Jakarta")
	if err != nil {
		t.Fatalf("revenue: %v", err)
	}
	// Revenue comes from fully PAID payments (50k confirmed + 75k completed), no
	// matter whether the reservation is completed yet. The UNPAID row belongs to a
	// PENDING reservation (outstanding); the REFUNDED row is reported separately.
	if pay.PaymentRecords != 4 || pay.PaidCount != 2 || pay.UnpaidCount != 1 || pay.RefundedCount != 1 {
		t.Fatalf("payment counts = %+v", pay)
	}
	if pay.TotalAmount != money.FromMajor(250000) || pay.PaidAmount != money.FromMajor(125000) ||
		pay.UnpaidAmount != money.FromMajor(100000) || pay.RefundedAmount != money.FromMajor(25000) {
		t.Fatalf("payment amounts = %+v", pay)
	}
	if pay.ReservationsRequiringPayment != 4 || pay.PaymentCollectionRate != 0.5 {
		t.Fatalf("collection = %+v", pay)
	}
	if len(pay.Series) != 1 || pay.Series[0].Date != "2026-10-01" || pay.Series[0].PaidAmount != money.FromMajor(125000) {
		t.Fatalf("revenue series = %+v", pay.Series)
	}
	payB, err := repo.Revenue(ctx, b.ID, from.UTC(), to.UTC(), "Asia/Jakarta")
	if err != nil || payB.PaymentRecords != 1 || payB.PaidAmount != money.FromMajor(999999) {
		t.Fatalf("revenue leak B = %+v err=%v", payB, err)
	}
}

func ptr(v string) *string { return &v }

func newTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping analytics integration tests")
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
