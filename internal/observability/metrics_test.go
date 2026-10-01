package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsLabelsAreLowCardinality(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	m.ObserveHTTP("GET", "/api/v1/analytics/overview", 200, 10*time.Millisecond)
	m.ObserveJob("AI_GENERATE", "ai", "success", 20*time.Millisecond)
	m.ObserveJob("AI_GENERATE", "ai", "retried", 5*time.Millisecond)
	m.ObserveJob("MESSAGE_DELIVERY", "delivery", "dead_lettered", 8*time.Millisecond)
	m.ObserveAI("openai", "success", "success", 30*time.Millisecond)
	m.ObserveAI("openai", "failure", "timeout", 40*time.Millisecond)
	m.ObserveDelivery("whatsapp", "send", "success", 12*time.Millisecond)
	m.ObserveDelivery("whatsapp", "send", "failure", 15*time.Millisecond)
	m.ObserveCampaignRun("WHATSAPP", "started")
	m.ObserveCampaignRecipient("WHATSAPP", "FAILED", "failure")
	m.ObserveCampaignRecipient("WHATSAPP", "SKIPPED", "opt_out")
	m.ObserveCampaignDispatch("WHATSAPP", 8*time.Millisecond)
	m.ObserveUsage("AI_RESPONSES", "consumed")
	m.ObserveUsage("AI_RESPONSES", "exceeded")
	m.SetQueueDepth("ai", "diuk:stream:ai", "ai-workers", 3)

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	dump := ""
	for _, fam := range families {
		dump += fam.String()
	}
	forbidden := []string{"business_id", "customer_id", "conversation_id", "Bearer ", "sk-", "prompt"}
	for _, item := range forbidden {
		if strings.Contains(dump, item) {
			t.Fatalf("metrics leaked %q", item)
		}
	}
	if testutil.ToFloat64(m.JobsRetried.WithLabelValues("AI_GENERATE", "ai")) != 1 {
		t.Fatal("retry metric missing")
	}
	if testutil.ToFloat64(m.JobsDeadLettered.WithLabelValues("MESSAGE_DELIVERY", "delivery")) != 1 {
		t.Fatal("dlq metric missing")
	}
	if testutil.ToFloat64(m.AIFailed.WithLabelValues("openai", "timeout")) != 1 {
		t.Fatal("ai failed metric missing")
	}
	if testutil.ToFloat64(m.UsageConsumption.WithLabelValues("AI_RESPONSES", "consumed")) != 1 {
		t.Fatal("usage consumption metric missing")
	}
	if testutil.ToFloat64(m.UsageQuotaExceeded.WithLabelValues("AI_RESPONSES", "exceeded")) != 1 {
		t.Fatal("usage quota exceeded metric missing")
	}
}
