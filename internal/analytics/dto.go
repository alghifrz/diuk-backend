package analytics

import "time"

type envelope struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
	Summary  any    `json:"summary"`
	Series   any    `json:"series,omitempty"`
}

type namedCountJSON struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type seriesJSON struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

type overviewJSON struct {
	TotalConversations          int64 `json:"total_conversations"`
	OpenConversations           int64 `json:"open_conversations"`
	ClosedConversations         int64 `json:"closed_conversations"`
	AIActiveConversations       int64 `json:"ai_active_conversations"`
	HumanRequestedConversations int64 `json:"human_requested_conversations"`
	HumanActiveConversations    int64 `json:"human_active_conversations"`
	ResolvedHumanConversations  int64 `json:"resolved_human_conversations"`
	TotalCustomers              int64 `json:"total_customers"`
	NewCustomers                int64 `json:"new_customers"`
	TotalReservations           int64 `json:"total_reservations"`
	ConfirmedReservations       int64 `json:"confirmed_reservations"`
	CancelledReservations       int64 `json:"cancelled_reservations"`
	CompletedReservations       int64 `json:"completed_reservations"`
	AIGeneratedMessages         int64 `json:"ai_generated_messages"`
	HumanMessages               int64 `json:"human_messages"`
	InboundMessages             int64 `json:"inbound_messages"`
	OutboundMessages            int64 `json:"outbound_messages"`
	AutomationRuns              int64 `json:"automation_runs"`
	AutomationSuccesses         int64 `json:"automation_successes"`
	AutomationFailures          int64 `json:"automation_failures"`
}

type conversationSummaryJSON struct {
	Total                      int64            `json:"total"`
	Open                       int64            `json:"open"`
	Closed                     int64            `json:"closed"`
	ByChannel                  []namedCountJSON `json:"by_channel"`
	ByHandlingState            []namedCountJSON `json:"by_handling_state"`
	InboundMessages            int64            `json:"inbound_messages"`
	OutboundMessages           int64            `json:"outbound_messages"`
	AIHandled                  int64            `json:"ai_handled"`
	HumanHandled               int64            `json:"human_handled"`
	HumanRequested             int64            `json:"human_requested"`
	AverageConversationSeconds *float64         `json:"average_conversation_duration"`
}

type customerSummaryJSON struct {
	TotalCustomers             int64 `json:"total_customers"`
	NewCustomers               int64 `json:"new_customers"`
	ActiveCustomers            int64 `json:"active_customers"`
	CustomersWithConversations int64 `json:"customers_with_conversations"`
	CustomersWithReservations  int64 `json:"customers_with_reservations"`
}

type reservationSummaryJSON struct {
	Total            int64                    `json:"total"`
	Pending          int64                    `json:"pending"`
	Confirmed        int64                    `json:"confirmed"`
	Cancelled        int64                    `json:"cancelled"`
	Completed        int64                    `json:"completed"`
	ConfirmationRate float64                  `json:"confirmation_rate"`
	CancellationRate float64                  `json:"cancellation_rate"`
	CompletionRate   float64                  `json:"completion_rate"`
	TotalPartySize   int64                    `json:"total_party_size"`
	AveragePartySize float64                  `json:"average_party_size"`
	ByArea           []areaReservationJSON    `json:"by_area"`
	ByTable          []tableReservationJSON   `json:"by_table"`
	ByHour           []hourReservationJSON    `json:"by_hour"`
	ByWeekday        []weekdayReservationJSON `json:"by_weekday"`
}

type areaReservationJSON struct {
	AreaID           string `json:"area_id"`
	AreaName         string `json:"area_name"`
	ReservationCount int64  `json:"reservation_count"`
	ConfirmedCount   int64  `json:"confirmed_count"`
	CompletedCount   int64  `json:"completed_count"`
	CancelledCount   int64  `json:"cancelled_count"`
}

type tableReservationJSON struct {
	TableID          string `json:"table_id"`
	TableName        string `json:"table_name"`
	AreaName         string `json:"area_name"`
	ReservationCount int64  `json:"reservation_count"`
	ConfirmedCount   int64  `json:"confirmed_count"`
	CompletedCount   int64  `json:"completed_count"`
	CancelledCount   int64  `json:"cancelled_count"`
}

type hourReservationJSON struct {
	Hour             int   `json:"hour"`
	ReservationCount int64 `json:"reservation_count"`
	ConfirmedCount   int64 `json:"confirmed_count"`
	CompletedCount   int64 `json:"completed_count"`
	CancelledCount   int64 `json:"cancelled_count"`
}

type weekdayReservationJSON struct {
	DayOfWeek        int   `json:"day_of_week"`
	ReservationCount int64 `json:"reservation_count"`
	ConfirmedCount   int64 `json:"confirmed_count"`
	CompletedCount   int64 `json:"completed_count"`
	CancelledCount   int64 `json:"cancelled_count"`
}

type conversionSummaryJSON struct {
	Conversations                     int64   `json:"conversations"`
	ConversationsWithReservation      int64   `json:"conversations_with_reservation"`
	ReservationsCreated               int64   `json:"reservations_created"`
	ReservationsConfirmed             int64   `json:"reservations_confirmed"`
	ReservationsCompleted             int64   `json:"reservations_completed"`
	ReservationsCancelled             int64   `json:"reservations_cancelled"`
	ConversationReservationRate       float64 `json:"conversation_reservation_rate"`
	ConfirmationRate                  float64 `json:"confirmation_rate"`
	CompletionRate                    float64 `json:"completion_rate"`
	CancellationRate                  float64 `json:"cancellation_rate"`
	ReservationsRequiringPayment      int64   `json:"reservations_requiring_payment"`
	ReservationsPaid                  int64   `json:"reservations_paid"`
	ReservationsUnpaid                int64   `json:"reservations_unpaid"`
	ReservationsConfirmedPaid         int64   `json:"reservations_confirmed_paid"`
	ReservationsCancelledAfterPayment int64   `json:"reservations_cancelled_after_payment"`
	ReservationsRefunded              int64   `json:"reservations_refunded"`
	PaymentCollectionRate             float64 `json:"payment_collection_rate"`
	PaidReservationConfirmationRate   float64 `json:"paid_reservation_confirmation_rate"`
}

type conversionDayJSON struct {
	Date             string `json:"date"`
	Conversations    int64  `json:"conversations"`
	Reservations     int64  `json:"reservations"`
	Confirmed        int64  `json:"confirmed"`
	Completed        int64  `json:"completed"`
	Cancelled        int64  `json:"cancelled"`
	PaidReservations int64  `json:"paid_reservations"`
	PaymentAmount    any    `json:"payment_amount"`
	PaidAmount       any    `json:"paid_amount"`
}

type paymentSummaryJSON struct {
	PaymentRecords               int64   `json:"payment_records"`
	PaidCount                    int64   `json:"paid_count"`
	UnpaidCount                  int64   `json:"unpaid_count"`
	RefundedCount                int64   `json:"refunded_count"`
	TotalAmount                  any     `json:"total_amount"`
	PaidAmount                   any     `json:"paid_amount"`
	UnpaidAmount                 any     `json:"unpaid_amount"`
	RefundedAmount               any     `json:"refunded_amount"`
	ReservationsRequiringPayment int64   `json:"reservations_requiring_payment"`
	PaymentCollectionRate        float64 `json:"payment_collection_rate"`
}

type paymentDayJSON struct {
	Date           string `json:"date"`
	PaymentRecords int64  `json:"payment_records"`
	PaidCount      int64  `json:"paid_count"`
	TotalAmount    any    `json:"total_amount"`
	PaidAmount     any    `json:"paid_amount"`
}

type reservationDayJSON struct {
	Date      string `json:"date"`
	Total     int64  `json:"reservations"`
	Confirmed int64  `json:"confirmed"`
	Cancelled int64  `json:"cancelled"`
	Completed int64  `json:"completed"`
}

type aiSummaryJSON struct {
	TotalGenerations      int64            `json:"total_ai_generations"`
	SuccessfulGenerations int64            `json:"successful_generations"`
	FailedGenerations     int64            `json:"failed_generations"`
	SkippedGenerations    int64            `json:"skipped_generations"`
	AIMessagesGenerated   int64            `json:"ai_messages_generated"`
	AverageLatencyMS      *float64         `json:"average_ai_generation_latency"`
	ByProvider            []aiProviderJSON `json:"by_provider"`
}

type aiProviderJSON struct {
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	GenerationCount  int64    `json:"generation_count"`
	FailureCount     int64    `json:"failure_count"`
	AverageLatencyMS *float64 `json:"average_latency_ms"`
}

type handoverSummaryJSON struct {
	HandoverCount               int64            `json:"handover_count"`
	CustomerRequestedCount      int64            `json:"customer_requested_count"`
	AIUnableCount               int64            `json:"ai_unable_count"`
	ManualHandoffCount          int64            `json:"manual_handoff_count"`
	HumanStartedCount           int64            `json:"human_started_count"`
	HumanResolvedCount          int64            `json:"human_resolved_count"`
	AverageTimeToHumanSeconds   *float64         `json:"average_time_to_human"`
	AverageHumanHandlingSeconds *float64         `json:"average_human_handling_duration"`
	ByReason                    []namedCountJSON `json:"by_reason"`
	ByChannel                   []namedCountJSON `json:"by_channel"`
}

type automationSummaryJSON struct {
	TotalRuns      int64                     `json:"total_runs"`
	SuccessfulRuns int64                     `json:"successful_runs"`
	FailedRuns     int64                     `json:"failed_runs"`
	SkippedRuns    int64                     `json:"skipped_runs"`
	SuccessRate    float64                   `json:"success_rate"`
	FailureRate    float64                   `json:"failure_rate"`
	ByAutomation   []automationBreakdownJSON `json:"by_automation"`
	ByAction       []namedCountJSON          `json:"by_action_type"`
}

type automationBreakdownJSON struct {
	AutomationID   string `json:"automation_id"`
	AutomationName string `json:"automation_name"`
	ActionType     string `json:"action_type"`
	TotalRuns      int64  `json:"total_runs"`
	SuccessfulRuns int64  `json:"successful_runs"`
	FailedRuns     int64  `json:"failed_runs"`
	SkippedRuns    int64  `json:"skipped_runs"`
}

type messagingSummaryJSON struct {
	InboundMessages  int64            `json:"inbound_messages"`
	OutboundMessages int64            `json:"outbound_messages"`
	Queued           int64            `json:"queued"`
	Sent             int64            `json:"sent"`
	Delivered        int64            `json:"delivered"`
	Read             int64            `json:"read"`
	Failed           int64            `json:"failed"`
	ByChannel        []namedCountJSON `json:"by_channel"`
	ByDirection      []namedCountJSON `json:"by_direction"`
	ByStatus         []namedCountJSON `json:"by_status"`
}

type campaignSummaryJSON struct {
	TotalCampaigns    int64            `json:"total_campaigns"`
	TotalRuns         int64            `json:"total_runs"`
	CompletedRuns     int64            `json:"completed_runs"`
	CancelledRuns     int64            `json:"cancelled_runs"`
	TotalRecipients   int64            `json:"total_recipients"`
	Queued            int64            `json:"queued"`
	Sent              int64            `json:"sent"`
	Delivered         int64            `json:"delivered"`
	Read              int64            `json:"read"`
	Failed            int64            `json:"failed"`
	Skipped           int64            `json:"skipped"`
	ByCampaignStatus  []namedCountJSON `json:"by_campaign_status"`
	ByRecipientStatus []namedCountJSON `json:"by_recipient_status"`
}

func rangeEnvelope(r Range, summary any, series any) envelope {
	return envelope{
		From:     r.From.UTC().Format(time.RFC3339),
		To:       r.To.UTC().Format(time.RFC3339),
		Timezone: r.Timezone,
		Summary:  summary,
		Series:   series,
	}
}

func namedCountsJSON(items []NamedCount) []namedCountJSON {
	out := make([]namedCountJSON, 0, len(items))
	for _, item := range items {
		out = append(out, namedCountJSON{Key: item.Key, Count: item.Count})
	}
	return out
}

func areaReservationJSONList(items []AreaReservationStats) []areaReservationJSON {
	out := make([]areaReservationJSON, 0, len(items))
	for _, item := range items {
		out = append(out, areaReservationJSON{
			AreaID: item.AreaID, AreaName: item.AreaName, ReservationCount: item.ReservationCount,
			ConfirmedCount: item.ConfirmedCount, CompletedCount: item.CompletedCount, CancelledCount: item.CancelledCount,
		})
	}
	return out
}

func tableReservationJSONList(items []TableReservationStats) []tableReservationJSON {
	out := make([]tableReservationJSON, 0, len(items))
	for _, item := range items {
		out = append(out, tableReservationJSON{
			TableID: item.TableID, TableName: item.TableName, AreaName: item.AreaName,
			ReservationCount: item.ReservationCount, ConfirmedCount: item.ConfirmedCount,
			CompletedCount: item.CompletedCount, CancelledCount: item.CancelledCount,
		})
	}
	return out
}

func hourReservationJSONList(items []HourReservationStats) []hourReservationJSON {
	out := make([]hourReservationJSON, 0, len(items))
	for _, item := range items {
		out = append(out, hourReservationJSON{
			Hour: item.Hour, ReservationCount: item.ReservationCount, ConfirmedCount: item.ConfirmedCount,
			CompletedCount: item.CompletedCount, CancelledCount: item.CancelledCount,
		})
	}
	return out
}

func weekdayReservationJSONList(items []WeekdayReservationStats) []weekdayReservationJSON {
	out := make([]weekdayReservationJSON, 0, len(items))
	for _, item := range items {
		out = append(out, weekdayReservationJSON{
			DayOfWeek: item.DayOfWeek, ReservationCount: item.ReservationCount, ConfirmedCount: item.ConfirmedCount,
			CompletedCount: item.CompletedCount, CancelledCount: item.CancelledCount,
		})
	}
	return out
}

func seriesJSONList(items []SeriesPoint) []seriesJSON {
	out := make([]seriesJSON, 0, len(items))
	for _, item := range items {
		out = append(out, seriesJSON{Date: item.Date, Value: item.Value})
	}
	return out
}
