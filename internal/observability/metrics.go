package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	HTTPRequests          *prometheus.CounterVec
	HTTPDuration          *prometheus.HistogramVec
	JobsProcessed         *prometheus.CounterVec
	JobsFailed            *prometheus.CounterVec
	JobsRetried           *prometheus.CounterVec
	JobsDeadLettered      *prometheus.CounterVec
	JobDuration           *prometheus.HistogramVec
	QueuePending          *prometheus.GaugeVec
	AIGenerations         *prometheus.CounterVec
	AIFailed              *prometheus.CounterVec
	AIDuration            *prometheus.HistogramVec
	Deliveries            *prometheus.CounterVec
	DeliveryFailed        *prometheus.CounterVec
	DeliveryDuration      *prometheus.HistogramVec
	CampaignRuns          *prometheus.CounterVec
	CampaignRecipients    *prometheus.CounterVec
	CampaignRecipientFail *prometheus.CounterVec
	CampaignRecipientSkip *prometheus.CounterVec
	CampaignDispatchDur   *prometheus.HistogramVec
	UsageConsumption      *prometheus.CounterVec
	UsageQuotaExceeded    *prometheus.CounterVec
	CalendarBlocks        *prometheus.CounterVec
	ReservationReschedule *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)
	return &Metrics{
		HTTPRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests completed.",
		}, []string{"method", "route", "status"}),
		HTTPDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"method", "route", "status"}),
		JobsProcessed: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "job_processed_total",
			Help: "Jobs processed by workers.",
		}, []string{"job_type", "queue", "result"}),
		JobsFailed: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "job_failed_total",
			Help: "Jobs that ended in a failure classification.",
		}, []string{"job_type", "queue"}),
		JobsRetried: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "job_retried_total",
			Help: "Jobs scheduled for retry.",
		}, []string{"job_type", "queue"}),
		JobsDeadLettered: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "job_dead_lettered_total",
			Help: "Jobs moved to the dead-letter queue.",
		}, []string{"job_type", "queue"}),
		JobDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "queue_processing_duration_seconds",
			Help:    "Worker processing duration.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 15, 30, 60},
		}, []string{"job_type", "queue"}),
		QueuePending: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "queue_depth",
			Help: "Redis stream consumer-group pending count.",
		}, []string{"queue", "stream", "consumer_group"}),
		AIGenerations: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_generation_total",
			Help: "AI provider generation attempts.",
		}, []string{"provider", "result", "error_category"}),
		AIFailed: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_generation_failed_total",
			Help: "AI provider generation failures.",
		}, []string{"provider", "error_category"}),
		AIDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ai_generation_duration_seconds",
			Help:    "AI provider generation duration.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 45},
		}, []string{"provider", "result"}),
		Deliveries: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "message_delivery_total",
			Help: "Outbound message delivery attempts.",
		}, []string{"channel", "operation", "result"}),
		DeliveryFailed: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "message_delivery_failed_total",
			Help: "Outbound message delivery failures.",
		}, []string{"channel", "operation"}),
		DeliveryDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "message_delivery_duration_seconds",
			Help:    "Outbound message delivery duration.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"channel", "operation", "result"}),
		CampaignRuns: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "campaign_run_total",
			Help: "Campaign run lifecycle events.",
		}, []string{"channel", "result"}),
		CampaignRecipients: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "campaign_recipient_total",
			Help: "Campaign recipient outcomes.",
		}, []string{"channel", "status", "result"}),
		CampaignRecipientFail: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "campaign_recipient_failed_total",
			Help: "Campaign recipients marked failed.",
		}, []string{"channel", "error_category"}),
		CampaignRecipientSkip: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "campaign_recipient_skipped_total",
			Help: "Campaign recipients skipped.",
		}, []string{"channel", "result"}),
		CampaignDispatchDur: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "campaign_dispatch_duration_seconds",
			Help:    "Campaign dispatch batch duration.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"channel"}),
		UsageConsumption: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "usage_consumption_total",
			Help: "Usage consumption attempts.",
		}, []string{"feature", "result"}),
		UsageQuotaExceeded: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "usage_quota_exceeded_total",
			Help: "Quota checks or consumes that exceeded the plan limit.",
		}, []string{"feature", "result"}),
		CalendarBlocks: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "calendar_block_create_total",
			Help: "Calendar block create attempts.",
		}, []string{"result"}),
		ReservationReschedule: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "reservation_reschedule_total",
			Help: "Reservation reschedule attempts.",
		}, []string{"result"}),
	}
}

func (m *Metrics) ObserveHTTP(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	statusLabel := strconv.Itoa(status)
	m.HTTPRequests.WithLabelValues(method, route, statusLabel).Inc()
	m.HTTPDuration.WithLabelValues(method, route, statusLabel).Observe(duration.Seconds())
}

func (m *Metrics) ObserveJob(jobType, queue, result string, duration time.Duration) {
	if m == nil {
		return
	}
	m.JobsProcessed.WithLabelValues(jobType, queue, result).Inc()
	m.JobDuration.WithLabelValues(jobType, queue).Observe(duration.Seconds())
	switch result {
	case "failed":
		m.JobsFailed.WithLabelValues(jobType, queue).Inc()
	case "retried":
		m.JobsRetried.WithLabelValues(jobType, queue).Inc()
	case "dead_lettered":
		m.JobsFailed.WithLabelValues(jobType, queue).Inc()
		m.JobsDeadLettered.WithLabelValues(jobType, queue).Inc()
	}
}

func (m *Metrics) SetQueueDepth(queue, stream, group string, pending float64) {
	if m == nil {
		return
	}
	m.QueuePending.WithLabelValues(queue, stream, group).Set(pending)
}

func (m *Metrics) ObserveAI(provider, result, category string, duration time.Duration) {
	if m == nil {
		return
	}
	if provider == "" {
		provider = "unknown"
	}
	if category == "" {
		category = "unknown"
	}
	m.AIGenerations.WithLabelValues(provider, result, category).Inc()
	m.AIDuration.WithLabelValues(provider, result).Observe(duration.Seconds())
	if result == "failure" {
		m.AIFailed.WithLabelValues(provider, category).Inc()
	}
}

func (m *Metrics) ObserveDelivery(channel, operation, result string, duration time.Duration) {
	if m == nil {
		return
	}
	if channel == "" {
		channel = "unknown"
	}
	if operation == "" {
		operation = "send"
	}
	m.Deliveries.WithLabelValues(channel, operation, result).Inc()
	m.DeliveryDuration.WithLabelValues(channel, operation, result).Observe(duration.Seconds())
	if result == "failure" {
		m.DeliveryFailed.WithLabelValues(channel, operation).Inc()
	}
}

func (m *Metrics) ObserveCampaignRun(channel, result string) {
	if m == nil {
		return
	}
	if channel == "" {
		channel = "unknown"
	}
	if result == "" {
		result = "unknown"
	}
	m.CampaignRuns.WithLabelValues(strings.ToLower(channel), result).Inc()
}

func (m *Metrics) ObserveCampaignRecipient(channel, status, result string) {
	if m == nil {
		return
	}
	if channel == "" {
		channel = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	if result == "" {
		result = "unknown"
	}
	m.CampaignRecipients.WithLabelValues(strings.ToLower(channel), status, result).Inc()
	if status == "FAILED" {
		m.CampaignRecipientFail.WithLabelValues(strings.ToLower(channel), result).Inc()
	}
	if status == "SKIPPED" {
		m.CampaignRecipientSkip.WithLabelValues(strings.ToLower(channel), result).Inc()
	}
}

func (m *Metrics) ObserveUsage(feature, result string) {
	if m == nil {
		return
	}
	if feature == "" {
		feature = "unknown"
	}
	if result == "" {
		result = "unknown"
	}
	m.UsageConsumption.WithLabelValues(feature, result).Inc()
	if result == "exceeded" {
		m.UsageQuotaExceeded.WithLabelValues(feature, result).Inc()
	}
}

func (m *Metrics) ObserveCalendarBlock(result string) {
	if m == nil {
		return
	}
	if result == "" {
		result = "unknown"
	}
	m.CalendarBlocks.WithLabelValues(result).Inc()
}

func (m *Metrics) ObserveReservationReschedule(result string) {
	if m == nil {
		return
	}
	if result == "" {
		result = "unknown"
	}
	m.ReservationReschedule.WithLabelValues(result).Inc()
}

func (m *Metrics) ObserveCampaignDispatch(channel string, duration time.Duration) {
	if m == nil {
		return
	}
	if channel == "" {
		channel = "unknown"
	}
	m.CampaignDispatchDur.WithLabelValues(strings.ToLower(channel)).Observe(duration.Seconds())
}
