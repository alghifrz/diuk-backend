package analytics

import (
	"time"

	"github.com/alghifrz/diuk-backend/internal/money"
)

const (
	DefaultLookbackDays = 30
	MaxRangeDays        = 366
)

type Range struct {
	From     time.Time
	To       time.Time
	Timezone string
}

type NamedCount struct {
	Key   string
	Count int64
}

type SeriesPoint struct {
	Date  string
	Value int64
}

type Overview struct {
	TotalConversations          int64
	OpenConversations           int64
	ClosedConversations         int64
	AIActiveConversations       int64
	HumanRequestedConversations int64
	HumanActiveConversations    int64
	ResolvedHumanConversations  int64
	TotalCustomers              int64
	NewCustomers                int64
	TotalReservations           int64
	ConfirmedReservations       int64
	CancelledReservations       int64
	CompletedReservations       int64
	AIGeneratedMessages         int64
	HumanMessages               int64
	InboundMessages             int64
	OutboundMessages            int64
	AutomationRuns              int64
	AutomationSuccesses         int64
	AutomationFailures          int64
}

type ConversationReport struct {
	Total                      int64
	Open                       int64
	Closed                     int64
	ByChannel                  []NamedCount
	ByHandlingState            []NamedCount
	InboundMessages            int64
	OutboundMessages           int64
	AIHandled                  int64
	HumanHandled               int64
	HumanRequested             int64
	AverageConversationSeconds *float64
	Series                     []SeriesPoint
}

type CustomerReport struct {
	TotalCustomers             int64
	NewCustomers               int64
	ActiveCustomers            int64
	CustomersWithConversations int64
	CustomersWithReservations  int64
	Series                     []SeriesPoint
}

type ReservationReport struct {
	Total            int64
	Pending          int64
	Confirmed        int64
	Cancelled        int64
	Completed        int64
	ConfirmationRate float64
	CancellationRate float64
	CompletionRate   float64
	TotalPartySize   int64
	AveragePartySize float64
	ByArea           []AreaReservationStats
	ByTable          []TableReservationStats
	ByHour           []HourReservationStats
	ByWeekday        []WeekdayReservationStats
	Series           []ReservationDay
}

type ReservationDay struct {
	Date      string
	Total     int64
	Confirmed int64
	Cancelled int64
	Completed int64
}

type AreaReservationStats struct {
	AreaID           string
	AreaName         string
	ReservationCount int64
	ConfirmedCount   int64
	CompletedCount   int64
	CancelledCount   int64
}

type TableReservationStats struct {
	TableID          string
	TableName        string
	AreaName         string
	ReservationCount int64
	ConfirmedCount   int64
	CompletedCount   int64
	CancelledCount   int64
}

type HourReservationStats struct {
	Hour             int
	ReservationCount int64
	ConfirmedCount   int64
	CompletedCount   int64
	CancelledCount   int64
}

type WeekdayReservationStats struct {
	DayOfWeek        int
	ReservationCount int64
	ConfirmedCount   int64
	CompletedCount   int64
	CancelledCount   int64
}

// ConversionReport is observational: a conversation and a reservation that
// share a customer in the same created_at window are associated, not attributed.
type ConversionReport struct {
	Conversations                     int64
	ConversationsWithReservation      int64
	ReservationsCreated               int64
	ReservationsConfirmed             int64
	ReservationsCompleted             int64
	ReservationsCancelled             int64
	ConversationReservationRate       float64
	ConfirmationRate                  float64
	CompletionRate                    float64
	CancellationRate                  float64
	ReservationsRequiringPayment      int64
	ReservationsPaid                  int64
	ReservationsUnpaid                int64
	ReservationsConfirmedPaid         int64
	ReservationsCancelledAfterPayment int64
	ReservationsRefunded              int64
	PaymentCollectionRate             float64
	PaidReservationConfirmationRate   float64
	Series                            []ConversionDay
}

type ConversionDay struct {
	Date             string
	Conversations    int64
	Reservations     int64
	Confirmed        int64
	Completed        int64
	Cancelled        int64
	PaidReservations int64
	PaymentAmount    money.Amount
	PaidAmount       money.Amount
}

// PaymentReport is payment totals for COMPLETED reservations only.
// Each completed reservation contributes its payment row (PAID/DEPOSIT = collected).
// payment_collection_rate = paid rows with amount > 0 / rows with amount > 0.
type PaymentReport struct {
	PaymentRecords               int64
	PaidCount                    int64
	UnpaidCount                  int64
	RefundedCount                int64
	TotalAmount                  money.Amount
	PaidAmount                   money.Amount
	UnpaidAmount                 money.Amount
	RefundedAmount               money.Amount
	ReservationsRequiringPayment int64
	PaymentCollectionRate        float64
	Series                       []PaymentDay
}

type PaymentDay struct {
	Date           string
	PaymentRecords int64
	PaidCount      int64
	TotalAmount    money.Amount
	PaidAmount     money.Amount
}

type AIReport struct {
	TotalGenerations      int64
	SuccessfulGenerations int64
	FailedGenerations     int64
	SkippedGenerations    int64
	AIMessagesGenerated   int64
	AverageLatencyMS      *float64
	ByProvider            []AIProviderStats
	Series                []SeriesPoint
}

type AIProviderStats struct {
	Provider         string
	Model            string
	GenerationCount  int64
	FailureCount     int64
	AverageLatencyMS *float64
}

type HandoverReport struct {
	HandoverCount               int64
	CustomerRequestedCount      int64
	AIUnableCount               int64
	ManualHandoffCount          int64
	HumanStartedCount           int64
	HumanResolvedCount          int64
	AverageTimeToHumanSeconds   *float64
	AverageHumanHandlingSeconds *float64
	ByReason                    []NamedCount
	ByChannel                   []NamedCount
	Series                      []SeriesPoint
}

type AutomationReport struct {
	TotalRuns      int64
	SuccessfulRuns int64
	FailedRuns     int64
	SkippedRuns    int64
	SuccessRate    float64
	FailureRate    float64
	ByAutomation   []AutomationBreakdown
	ByAction       []NamedCount
	Series         []SeriesPoint
}

type AutomationBreakdown struct {
	AutomationID   string
	AutomationName string
	ActionType     string
	TotalRuns      int64
	SuccessfulRuns int64
	FailedRuns     int64
	SkippedRuns    int64
}

type CampaignReport struct {
	TotalCampaigns    int64
	TotalRuns         int64
	CompletedRuns     int64
	CancelledRuns     int64
	TotalRecipients   int64
	Queued            int64
	Sent              int64
	Delivered         int64
	Read              int64
	Failed            int64
	Skipped           int64
	ByCampaignStatus  []NamedCount
	ByRecipientStatus []NamedCount
	Series            []SeriesPoint
}

type MessagingReport struct {
	InboundMessages  int64
	OutboundMessages int64
	Queued           int64
	Sent             int64
	Delivered        int64
	Read             int64
	Failed           int64
	ByChannel        []NamedCount
	ByDirection      []NamedCount
	ByStatus         []NamedCount
	Series           []SeriesPoint
}
