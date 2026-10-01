package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// The "AI Preview" contact created by the /prompt "Try it" sandbox is internal
// test data, not a real guest, so guest metrics leave it out.
const (
	sandboxNote = "Internal sandbox customer for /prompt Try it. Not a WhatsApp contact."

	// realCustomer filters an unaliased customers table.
	realCustomer = "NOT (name = 'AI Preview' AND btrim(COALESCE(notes, '')) = '" + sandboxNote + "')"
	// realCustomerCU filters customers joined as "cu".
	realCustomerCU = "NOT (cu.name = 'AI Preview' AND btrim(COALESCE(cu.notes, '')) = '" + sandboxNote + "')"
)

type Store interface {
	Overview(ctx context.Context, businessID uuid.UUID, from, to time.Time) (Overview, error)
	Conversations(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ConversationReport, error)
	Customers(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (CustomerReport, error)
	Reservations(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ReservationReport, error)
	Conversion(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ConversionReport, error)
	Revenue(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (PaymentReport, error)
	AI(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (AIReport, error)
	Handover(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (HandoverReport, error)
	Automation(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (AutomationReport, error)
	Messaging(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (MessagingReport, error)
	Campaigns(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (CampaignReport, error)
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Overview(ctx context.Context, businessID uuid.UUID, from, to time.Time) (Overview, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'OPEN'),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CLOSED'),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND COALESCE(handling_state, 'AI_ACTIVE') = 'AI_ACTIVE'),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND handling_state = 'HUMAN_REQUESTED'),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND handling_state = 'HUMAN_ACTIVE'),
			(SELECT COUNT(*) FROM conversations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND handling_state = 'RESOLVED'),
			(SELECT COUNT(*) FROM customers WHERE business_id = $1 AND ` + realCustomer + `),
			(SELECT COUNT(*) FROM customers WHERE business_id = $1 AND ` + realCustomer + ` AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM reservations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM reservations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CONFIRMED'),
			(SELECT COUNT(*) FROM reservations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CANCELLED'),
			(SELECT COUNT(*) FROM reservations WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'COMPLETED'),
			(SELECT COUNT(*) FROM messages WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE' AND sender_type = 'AI'),
			(SELECT COUNT(*) FROM messages WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE' AND sender_type = 'USER'),
			(SELECT COUNT(*) FROM messages WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE' AND direction = 'INBOUND'),
			(SELECT COUNT(*) FROM messages WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE' AND direction = 'OUTBOUND'),
			(SELECT COUNT(*) FROM automation_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM automation_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'COMPLETED'),
			(SELECT COUNT(*) FROM automation_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'FAILED')`
	var out Overview
	err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.TotalConversations, &out.OpenConversations, &out.ClosedConversations,
		&out.AIActiveConversations, &out.HumanRequestedConversations, &out.HumanActiveConversations, &out.ResolvedHumanConversations,
		&out.TotalCustomers, &out.NewCustomers,
		&out.TotalReservations, &out.ConfirmedReservations, &out.CancelledReservations, &out.CompletedReservations,
		&out.AIGeneratedMessages, &out.HumanMessages, &out.InboundMessages, &out.OutboundMessages,
		&out.AutomationRuns, &out.AutomationSuccesses, &out.AutomationFailures,
	)
	if err != nil {
		return Overview{}, fmt.Errorf("database: %w", err)
	}
	return out, nil
}

func (r *Repository) Conversations(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ConversationReport, error) {
	const summary = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'OPEN'),
			COUNT(*) FILTER (WHERE status = 'CLOSED'),
			COUNT(*) FILTER (WHERE handover_requested_at IS NULL),
			COUNT(*) FILTER (WHERE human_started_at IS NOT NULL),
			COUNT(*) FILTER (WHERE handover_requested_at IS NOT NULL),
			AVG(EXTRACT(EPOCH FROM (COALESCE(last_message_at, updated_at) - created_at)))
				FILTER (WHERE status = 'CLOSED')
		FROM conversations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3`
	var out ConversationReport
	if err := r.db.QueryRow(ctx, summary, businessID, from, to).Scan(
		&out.Total, &out.Open, &out.Closed, &out.AIHandled, &out.HumanHandled, &out.HumanRequested, &out.AverageConversationSeconds,
	); err != nil {
		return ConversationReport{}, fmt.Errorf("database: %w", err)
	}
	channels, err := r.namedCounts(ctx, `
		SELECT channel, COUNT(*) FROM conversations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY channel ORDER BY channel`, businessID, from, to)
	if err != nil {
		return ConversationReport{}, err
	}
	out.ByChannel = channels
	states, err := r.namedCounts(ctx, `
		SELECT COALESCE(handling_state, 'AI_ACTIVE'), COUNT(*) FROM conversations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to)
	if err != nil {
		return ConversationReport{}, err
	}
	out.ByHandlingState = states
	if err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE direction = 'INBOUND'),
			COUNT(*) FILTER (WHERE direction = 'OUTBOUND')
		FROM messages
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE'`,
		businessID, from, to,
	).Scan(&out.InboundMessages, &out.OutboundMessages); err != nil {
		return ConversationReport{}, fmt.Errorf("database: %w", err)
	}
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM conversations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return ConversationReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Customers(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (CustomerReport, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM customers WHERE business_id = $1 AND ` + realCustomer + `),
			(SELECT COUNT(*) FROM customers WHERE business_id = $1 AND ` + realCustomer + ` AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM customers WHERE business_id = $1 AND ` + realCustomer + ` AND status = 'ACTIVE'),
			(SELECT COUNT(DISTINCT c.customer_id) FROM conversations c JOIN customers cu ON cu.id = c.customer_id
				WHERE c.business_id = $1 AND ` + realCustomerCU + `),
			(SELECT COUNT(DISTINCT r.customer_id) FROM reservations r JOIN customers cu ON cu.id = r.customer_id
				WHERE r.business_id = $1 AND ` + realCustomerCU + `)`
	var out CustomerReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.TotalCustomers, &out.NewCustomers, &out.ActiveCustomers, &out.CustomersWithConversations, &out.CustomersWithReservations,
	); err != nil {
		return CustomerReport{}, fmt.Errorf("database: %w", err)
	}
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM customers
		WHERE business_id = $1 AND `+realCustomer+` AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return CustomerReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Reservations(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ReservationReport, error) {
	const query = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'PENDING'),
			COUNT(*) FILTER (WHERE status = 'CONFIRMED'),
			COUNT(*) FILTER (WHERE status = 'CANCELLED'),
			COUNT(*) FILTER (WHERE status = 'COMPLETED'),
			COALESCE(SUM(party_size), 0),
			COALESCE(AVG(party_size), 0)
		FROM reservations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3`
	var out ReservationReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.Total, &out.Pending, &out.Confirmed, &out.Cancelled, &out.Completed,
		&out.TotalPartySize, &out.AveragePartySize,
	); err != nil {
		return ReservationReport{}, fmt.Errorf("database: %w", err)
	}
	out.ConfirmationRate = rate(out.Confirmed, out.Total)
	out.CancellationRate = rate(out.Cancelled, out.Total)
	out.CompletionRate = rate(out.Completed, out.Total)
	rows, err := r.db.Query(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'CONFIRMED'),
		       COUNT(*) FILTER (WHERE status = 'CANCELLED'),
		       COUNT(*) FILTER (WHERE status = 'COMPLETED')
		FROM reservations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return ReservationReport{}, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out.Series = []ReservationDay{}
	for rows.Next() {
		var day ReservationDay
		if err := rows.Scan(&day.Date, &day.Total, &day.Confirmed, &day.Cancelled, &day.Completed); err != nil {
			return ReservationReport{}, fmt.Errorf("database: %w", err)
		}
		out.Series = append(out.Series, day)
	}
	if err := rows.Err(); err != nil {
		return ReservationReport{}, err
	}
	areas, err := r.reservationAreas(ctx, businessID, from, to)
	if err != nil {
		return ReservationReport{}, err
	}
	out.ByArea = areas
	tables, err := r.reservationTables(ctx, businessID, from, to)
	if err != nil {
		return ReservationReport{}, err
	}
	out.ByTable = tables
	hours, err := r.reservationHours(ctx, businessID, from, to, tz)
	if err != nil {
		return ReservationReport{}, err
	}
	out.ByHour = hours
	weekdays, err := r.reservationWeekdays(ctx, businessID, from, to, tz)
	if err != nil {
		return ReservationReport{}, err
	}
	out.ByWeekday = weekdays
	return out, nil
}

func (r *Repository) Conversion(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (ConversionReport, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM conversations
			 WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM conversations c
			 WHERE c.business_id = $1 AND c.created_at >= $2 AND c.created_at < $3
			   AND EXISTS (
			       SELECT 1 FROM reservations r
			       WHERE r.business_id = c.business_id
			         AND r.customer_id = c.customer_id
			         AND r.created_at >= $2 AND r.created_at < $3
			   )),
			(SELECT COUNT(*) FROM reservations
			 WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM reservations
			 WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CONFIRMED'),
			(SELECT COUNT(*) FROM reservations
			 WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'COMPLETED'),
			(SELECT COUNT(*) FROM reservations
			 WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CANCELLED'),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3 AND p.amount > 0),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3 AND p.status = 'PAID'),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3 AND p.status = 'UNPAID' AND p.amount > 0),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
			   AND p.status = 'PAID' AND r.status IN ('CONFIRMED', 'COMPLETED')),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
			   AND r.status = 'CANCELLED' AND p.status IN ('PAID', 'REFUNDED')),
			(SELECT COUNT(*) FROM reservations r
			 JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			 WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3 AND p.status = 'REFUNDED')`
	var out ConversionReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.Conversations, &out.ConversationsWithReservation,
		&out.ReservationsCreated, &out.ReservationsConfirmed, &out.ReservationsCompleted, &out.ReservationsCancelled,
		&out.ReservationsRequiringPayment, &out.ReservationsPaid, &out.ReservationsUnpaid,
		&out.ReservationsConfirmedPaid, &out.ReservationsCancelledAfterPayment, &out.ReservationsRefunded,
	); err != nil {
		return ConversionReport{}, fmt.Errorf("database: %w", err)
	}
	out.ConversationReservationRate = rate(out.ConversationsWithReservation, out.Conversations)
	out.ConfirmationRate = rate(out.ReservationsConfirmed, out.ReservationsCreated)
	out.CompletionRate = rate(out.ReservationsCompleted, out.ReservationsCreated)
	out.CancellationRate = rate(out.ReservationsCancelled, out.ReservationsCreated)
	out.PaymentCollectionRate = rate(out.ReservationsPaid, out.ReservationsRequiringPayment)
	out.PaidReservationConfirmationRate = rate(out.ReservationsConfirmedPaid, out.ReservationsPaid)
	series, err := r.conversionSeries(ctx, businessID, from, to, tz)
	if err != nil {
		return ConversionReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Revenue(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (PaymentReport, error) {
	// Revenue is recognized from FULLY PAID payments (status PAID), dated by the
	// moment the payment was confirmed (for deposit bookings: when the balance was
	// recorded via "Pay in full"). It does not wait for the reservation to be
	// completed. Deposit-only payments are not counted as collected revenue.
	// UNPAID rows only count while the reservation is still PENDING/CONFIRMED.
	const paymentEvents = `
		WITH pay AS (
			SELECT COALESCE(p.amount, 0) AS amount,
			       p.status,
			       CASE p.status
			           WHEN 'PAID' THEN COALESCE(p.confirmed_at, p.created_at)
			           WHEN 'REFUNDED' THEN COALESCE(p.refunded_at, p.created_at)
			           ELSE p.created_at
			       END AS event_at
			FROM reservation_payments p
			JOIN reservations r ON r.id = p.reservation_id AND r.business_id = p.business_id
			WHERE p.business_id = $1
			  AND (p.status <> 'UNPAID' OR r.status IN ('PENDING', 'CONFIRMED'))
		)`
	const query = paymentEvents + `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'PAID'),
			COUNT(*) FILTER (WHERE status = 'UNPAID'),
			COUNT(*) FILTER (WHERE status = 'REFUNDED'),
			COALESCE(SUM(amount), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = 'PAID'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = 'UNPAID'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = 'REFUNDED'), 0),
			COUNT(*) FILTER (WHERE amount > 0),
			COUNT(*) FILTER (WHERE status = 'PAID' AND amount > 0)
		FROM pay
		WHERE event_at >= $2 AND event_at < $3`
	var out PaymentReport
	var paidRequiring int64
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.PaymentRecords, &out.PaidCount, &out.UnpaidCount, &out.RefundedCount,
		&out.TotalAmount, &out.PaidAmount, &out.UnpaidAmount, &out.RefundedAmount,
		&out.ReservationsRequiringPayment, &paidRequiring,
	); err != nil {
		return PaymentReport{}, fmt.Errorf("database: %w", err)
	}
	out.PaymentCollectionRate = rate(paidRequiring, out.ReservationsRequiringPayment)
	rows, err := r.db.Query(ctx, paymentEvents+`
		SELECT (event_at AT TIME ZONE $4)::date::text,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'PAID'),
		       COALESCE(SUM(amount), 0),
		       COALESCE(SUM(amount) FILTER (WHERE status = 'PAID'), 0)
		FROM pay
		WHERE event_at >= $2 AND event_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return PaymentReport{}, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out.Series = []PaymentDay{}
	for rows.Next() {
		var day PaymentDay
		if err := rows.Scan(&day.Date, &day.PaymentRecords, &day.PaidCount, &day.TotalAmount, &day.PaidAmount); err != nil {
			return PaymentReport{}, fmt.Errorf("database: %w", err)
		}
		out.Series = append(out.Series, day)
	}
	return out, rows.Err()
}
func (r *Repository) AI(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (AIReport, error) {
	const query = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'SUCCESS' OR (status IS NULL AND outbound_message_id IS NOT NULL AND outbound_message_id <> '00000000-0000-0000-0000-000000000000')),
			COUNT(*) FILTER (WHERE status = 'FAILED'),
			COUNT(*) FILTER (WHERE status = 'SKIPPED' OR outbound_message_id = '00000000-0000-0000-0000-000000000000'),
			AVG(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)
		FROM ai_generations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3`
	var out AIReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.TotalGenerations, &out.SuccessfulGenerations, &out.FailedGenerations, &out.SkippedGenerations, &out.AverageLatencyMS,
	); err != nil {
		return AIReport{}, fmt.Errorf("database: %w", err)
	}
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		  AND message_type <> 'INTERNAL_NOTE' AND sender_type = 'AI'`,
		businessID, from, to,
	).Scan(&out.AIMessagesGenerated); err != nil {
		return AIReport{}, fmt.Errorf("database: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT COALESCE(provider, ''), COALESCE(model, ''), COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'FAILED'),
		       AVG(latency_ms) FILTER (WHERE latency_ms IS NOT NULL)
		FROM ai_generations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1, 2 ORDER BY 1, 2`, businessID, from, to)
	if err != nil {
		return AIReport{}, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out.ByProvider = []AIProviderStats{}
	for rows.Next() {
		var item AIProviderStats
		if err := rows.Scan(&item.Provider, &item.Model, &item.GenerationCount, &item.FailureCount, &item.AverageLatencyMS); err != nil {
			return AIReport{}, fmt.Errorf("database: %w", err)
		}
		out.ByProvider = append(out.ByProvider, item)
	}
	if err := rows.Err(); err != nil {
		return AIReport{}, err
	}
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM ai_generations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return AIReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Handover(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (HandoverReport, error) {
	const query = `
		SELECT
			COUNT(*) FILTER (WHERE handover_requested_at IS NOT NULL),
			COUNT(*) FILTER (WHERE handover_reason = 'CUSTOMER_REQUESTED_HUMAN'),
			COUNT(*) FILTER (WHERE handover_reason = 'AI_UNABLE_TO_ANSWER'),
			COUNT(*) FILTER (WHERE handover_reason = 'MANUAL_HANDOFF'),
			COUNT(*) FILTER (WHERE human_started_at IS NOT NULL),
			COUNT(*) FILTER (WHERE resolved_at IS NOT NULL),
			AVG(EXTRACT(EPOCH FROM (human_started_at - handover_requested_at)))
				FILTER (WHERE human_started_at IS NOT NULL AND handover_requested_at IS NOT NULL AND human_started_at >= handover_requested_at),
			AVG(EXTRACT(EPOCH FROM (resolved_at - human_started_at)))
				FILTER (WHERE resolved_at IS NOT NULL AND human_started_at IS NOT NULL AND resolved_at >= human_started_at)
		FROM conversations
		WHERE business_id = $1
		  AND handover_requested_at IS NOT NULL
		  AND handover_requested_at >= $2
		  AND handover_requested_at < $3`
	var out HandoverReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.HandoverCount, &out.CustomerRequestedCount, &out.AIUnableCount, &out.ManualHandoffCount,
		&out.HumanStartedCount, &out.HumanResolvedCount, &out.AverageTimeToHumanSeconds, &out.AverageHumanHandlingSeconds,
	); err != nil {
		return HandoverReport{}, fmt.Errorf("database: %w", err)
	}
	reasons, err := r.namedCounts(ctx, `
		SELECT COALESCE(handover_reason, ''), COUNT(*)
		FROM conversations
		WHERE business_id = $1 AND handover_requested_at IS NOT NULL
		  AND handover_requested_at >= $2 AND handover_requested_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to)
	if err != nil {
		return HandoverReport{}, err
	}
	out.ByReason = reasons
	channels, err := r.namedCounts(ctx, `
		SELECT channel, COUNT(*)
		FROM conversations
		WHERE business_id = $1 AND handover_requested_at IS NOT NULL
		  AND handover_requested_at >= $2 AND handover_requested_at < $3
		GROUP BY channel ORDER BY channel`, businessID, from, to)
	if err != nil {
		return HandoverReport{}, err
	}
	out.ByChannel = channels
	series, err := r.series(ctx, `
		SELECT (handover_requested_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM conversations
		WHERE business_id = $1 AND handover_requested_at IS NOT NULL
		  AND handover_requested_at >= $2 AND handover_requested_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return HandoverReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Automation(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (AutomationReport, error) {
	const query = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'COMPLETED'),
			COUNT(*) FILTER (WHERE status = 'FAILED'),
			COUNT(*) FILTER (WHERE status = 'CANCELLED')
		FROM automation_runs
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3`
	var out AutomationReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.TotalRuns, &out.SuccessfulRuns, &out.FailedRuns, &out.SkippedRuns,
	); err != nil {
		return AutomationReport{}, fmt.Errorf("database: %w", err)
	}
	out.SuccessRate = rate(out.SuccessfulRuns, out.TotalRuns)
	out.FailureRate = rate(out.FailedRuns, out.TotalRuns)
	rows, err := r.db.Query(ctx, `
		SELECT a.id::text, a.name, a.action_type,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE r.status = 'COMPLETED'),
		       COUNT(*) FILTER (WHERE r.status = 'FAILED'),
		       COUNT(*) FILTER (WHERE r.status = 'CANCELLED')
		FROM automation_runs r
		JOIN automations a ON a.id = r.automation_id AND a.business_id = r.business_id
		WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
		GROUP BY a.id, a.name, a.action_type
		ORDER BY a.name`, businessID, from, to)
	if err != nil {
		return AutomationReport{}, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out.ByAutomation = []AutomationBreakdown{}
	for rows.Next() {
		var item AutomationBreakdown
		if err := rows.Scan(&item.AutomationID, &item.AutomationName, &item.ActionType, &item.TotalRuns, &item.SuccessfulRuns, &item.FailedRuns, &item.SkippedRuns); err != nil {
			return AutomationReport{}, fmt.Errorf("database: %w", err)
		}
		out.ByAutomation = append(out.ByAutomation, item)
	}
	if err := rows.Err(); err != nil {
		return AutomationReport{}, err
	}
	actions, err := r.namedCounts(ctx, `
		SELECT a.action_type, COUNT(*)
		FROM automation_runs r
		JOIN automations a ON a.id = r.automation_id AND a.business_id = r.business_id
		WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
		GROUP BY a.action_type ORDER BY a.action_type`, businessID, from, to)
	if err != nil {
		return AutomationReport{}, err
	}
	out.ByAction = actions
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM automation_runs
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return AutomationReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Messaging(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (MessagingReport, error) {
	const query = `
		SELECT
			COUNT(*) FILTER (WHERE m.direction = 'INBOUND'),
			COUNT(*) FILTER (WHERE m.direction = 'OUTBOUND'),
			COUNT(*) FILTER (WHERE m.status = 'QUEUED'),
			COUNT(*) FILTER (WHERE m.status = 'SENT'),
			COUNT(*) FILTER (WHERE m.status = 'DELIVERED'),
			COUNT(*) FILTER (WHERE m.status = 'READ'),
			COUNT(*) FILTER (WHERE m.status = 'FAILED')
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id AND c.business_id = m.business_id
		WHERE m.business_id = $1 AND m.created_at >= $2 AND m.created_at < $3
		  AND m.message_type <> 'INTERNAL_NOTE'`
	var out MessagingReport
	if err := r.db.QueryRow(ctx, query, businessID, from, to).Scan(
		&out.InboundMessages, &out.OutboundMessages, &out.Queued, &out.Sent, &out.Delivered, &out.Read, &out.Failed,
	); err != nil {
		return MessagingReport{}, fmt.Errorf("database: %w", err)
	}
	channels, err := r.namedCounts(ctx, `
		SELECT c.channel, COUNT(*)
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id AND c.business_id = m.business_id
		WHERE m.business_id = $1 AND m.created_at >= $2 AND m.created_at < $3
		  AND m.message_type <> 'INTERNAL_NOTE'
		GROUP BY c.channel ORDER BY c.channel`, businessID, from, to)
	if err != nil {
		return MessagingReport{}, err
	}
	out.ByChannel = channels
	directions, err := r.namedCounts(ctx, `
		SELECT direction, COUNT(*) FROM messages
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE'
		GROUP BY direction ORDER BY direction`, businessID, from, to)
	if err != nil {
		return MessagingReport{}, err
	}
	out.ByDirection = directions
	statuses, err := r.namedCounts(ctx, `
		SELECT status, COUNT(*) FROM messages
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE'
		GROUP BY status ORDER BY status`, businessID, from, to)
	if err != nil {
		return MessagingReport{}, err
	}
	out.ByStatus = statuses
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM messages
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND message_type <> 'INTERNAL_NOTE'
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return MessagingReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) Campaigns(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) (CampaignReport, error) {
	const summary = `
		SELECT
			(SELECT COUNT(*) FROM campaigns WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM campaign_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM campaign_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'COMPLETED'),
			(SELECT COUNT(*) FROM campaign_runs WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'CANCELLED'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'QUEUED'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'SENT'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'DELIVERED'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'READ'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'FAILED'),
			(SELECT COUNT(*) FROM campaign_recipients WHERE business_id = $1 AND created_at >= $2 AND created_at < $3 AND status = 'SKIPPED')`
	var out CampaignReport
	if err := r.db.QueryRow(ctx, summary, businessID, from, to).Scan(
		&out.TotalCampaigns, &out.TotalRuns, &out.CompletedRuns, &out.CancelledRuns, &out.TotalRecipients,
		&out.Queued, &out.Sent, &out.Delivered, &out.Read, &out.Failed, &out.Skipped,
	); err != nil {
		return CampaignReport{}, fmt.Errorf("database: %w", err)
	}
	statuses, err := r.namedCounts(ctx, `
		SELECT status, COUNT(*) FROM campaigns
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY status ORDER BY status`, businessID, from, to)
	if err != nil {
		return CampaignReport{}, err
	}
	out.ByCampaignStatus = statuses
	recipients, err := r.namedCounts(ctx, `
		SELECT status, COUNT(*) FROM campaign_recipients
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY status ORDER BY status`, businessID, from, to)
	if err != nil {
		return CampaignReport{}, err
	}
	out.ByRecipientStatus = recipients
	series, err := r.series(ctx, `
		SELECT (created_at AT TIME ZONE $4)::date::text, COUNT(*)
		FROM campaign_runs
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return CampaignReport{}, err
	}
	out.Series = series
	return out, nil
}

func (r *Repository) reservationAreas(ctx context.Context, businessID uuid.UUID, from, to time.Time) ([]AreaReservationStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT COALESCE(a.id::text, ''),
		       COALESCE(a.name, 'Unassigned'),
		       COUNT(*),
		       COUNT(*) FILTER (WHERE r.status = 'CONFIRMED'),
		       COUNT(*) FILTER (WHERE r.status = 'COMPLETED'),
		       COUNT(*) FILTER (WHERE r.status = 'CANCELLED')
		FROM reservations r
		LEFT JOIN tables t ON t.id = r.table_id
		LEFT JOIN areas a ON a.id = t.area_id AND a.business_id = r.business_id
		WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
		GROUP BY a.id, a.name
		ORDER BY COALESCE(a.name, 'Unassigned'), COALESCE(a.id::text, '')`, businessID, from, to)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []AreaReservationStats{}
	for rows.Next() {
		var item AreaReservationStats
		if err := rows.Scan(&item.AreaID, &item.AreaName, &item.ReservationCount, &item.ConfirmedCount, &item.CompletedCount, &item.CancelledCount); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) reservationTables(ctx context.Context, businessID uuid.UUID, from, to time.Time) ([]TableReservationStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id::text, t.name, a.name,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE r.status = 'CONFIRMED'),
		       COUNT(*) FILTER (WHERE r.status = 'COMPLETED'),
		       COUNT(*) FILTER (WHERE r.status = 'CANCELLED')
		FROM reservations r
		JOIN tables t ON t.id = r.table_id
		JOIN areas a ON a.id = t.area_id AND a.business_id = r.business_id
		WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
		GROUP BY t.id, t.name, a.name
		ORDER BY a.name, t.name, t.id`, businessID, from, to)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []TableReservationStats{}
	for rows.Next() {
		var item TableReservationStats
		if err := rows.Scan(&item.TableID, &item.TableName, &item.AreaName, &item.ReservationCount, &item.ConfirmedCount, &item.CompletedCount, &item.CancelledCount); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) reservationHours(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) ([]HourReservationStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT EXTRACT(HOUR FROM (start_at AT TIME ZONE $4))::int,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'CONFIRMED'),
		       COUNT(*) FILTER (WHERE status = 'COMPLETED'),
		       COUNT(*) FILTER (WHERE status = 'CANCELLED')
		FROM reservations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1`, businessID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	byHour := map[int]HourReservationStats{}
	for rows.Next() {
		var item HourReservationStats
		if err := rows.Scan(&item.Hour, &item.ReservationCount, &item.ConfirmedCount, &item.CompletedCount, &item.CancelledCount); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		byHour[item.Hour] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]HourReservationStats, 0, 24)
	for hour := 0; hour < 24; hour++ {
		item := byHour[hour]
		item.Hour = hour
		out = append(out, item)
	}
	return out, nil
}

func (r *Repository) reservationWeekdays(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) ([]WeekdayReservationStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT EXTRACT(DOW FROM (start_at AT TIME ZONE $4))::int,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'CONFIRMED'),
		       COUNT(*) FILTER (WHERE status = 'COMPLETED'),
		       COUNT(*) FILTER (WHERE status = 'CANCELLED')
		FROM reservations
		WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1`, businessID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	byDay := map[int]WeekdayReservationStats{}
	for rows.Next() {
		var item WeekdayReservationStats
		if err := rows.Scan(&item.DayOfWeek, &item.ReservationCount, &item.ConfirmedCount, &item.CompletedCount, &item.CancelledCount); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		byDay[item.DayOfWeek] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]WeekdayReservationStats, 0, 7)
	for day := 0; day < 7; day++ {
		item := byDay[day]
		item.DayOfWeek = day
		out = append(out, item)
	}
	return out, nil
}

func (r *Repository) conversionSeries(ctx context.Context, businessID uuid.UUID, from, to time.Time, tz string) ([]ConversionDay, error) {
	rows, err := r.db.Query(ctx, `
		WITH conv AS (
			SELECT (created_at AT TIME ZONE $4)::date AS day, COUNT(*) AS conversations
			FROM conversations
			WHERE business_id = $1 AND created_at >= $2 AND created_at < $3
			GROUP BY 1
		),
		res AS (
			SELECT (r.created_at AT TIME ZONE $4)::date AS day,
			       COUNT(*) AS reservations,
			       COUNT(*) FILTER (WHERE r.status = 'CONFIRMED') AS confirmed,
			       COUNT(*) FILTER (WHERE r.status = 'COMPLETED') AS completed,
			       COUNT(*) FILTER (WHERE r.status = 'CANCELLED') AS cancelled,
			       COUNT(*) FILTER (WHERE p.status = 'PAID') AS paid_reservations,
			       COALESCE(SUM(p.amount), 0) AS payment_amount,
			       COALESCE(SUM(p.amount) FILTER (WHERE p.status = 'PAID'), 0) AS paid_amount
			FROM reservations r
			LEFT JOIN reservation_payments p ON p.reservation_id = r.id AND p.business_id = r.business_id
			WHERE r.business_id = $1 AND r.created_at >= $2 AND r.created_at < $3
			GROUP BY 1
		)
		SELECT COALESCE(c.day, r.day)::text,
		       COALESCE(c.conversations, 0),
		       COALESCE(r.reservations, 0),
		       COALESCE(r.confirmed, 0),
		       COALESCE(r.completed, 0),
		       COALESCE(r.cancelled, 0),
		       COALESCE(r.paid_reservations, 0),
		       COALESCE(r.payment_amount, 0),
		       COALESCE(r.paid_amount, 0)
		FROM conv c
		FULL OUTER JOIN res r ON r.day = c.day
		ORDER BY 1`, businessID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []ConversionDay{}
	for rows.Next() {
		var day ConversionDay
		if err := rows.Scan(
			&day.Date, &day.Conversations, &day.Reservations, &day.Confirmed, &day.Completed, &day.Cancelled,
			&day.PaidReservations, &day.PaymentAmount, &day.PaidAmount,
		); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		out = append(out, day)
	}
	return out, rows.Err()
}

func (r *Repository) namedCounts(ctx context.Context, query string, businessID uuid.UUID, from, to time.Time) ([]NamedCount, error) {
	rows, err := r.db.Query(ctx, query, businessID, from, to)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []NamedCount{}
	for rows.Next() {
		var item NamedCount
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) series(ctx context.Context, query string, businessID uuid.UUID, from, to time.Time, tz string) ([]SeriesPoint, error) {
	rows, err := r.db.Query(ctx, query, businessID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := []SeriesPoint{}
	for rows.Next() {
		var item SeriesPoint
		if err := rows.Scan(&item.Date, &item.Value); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
