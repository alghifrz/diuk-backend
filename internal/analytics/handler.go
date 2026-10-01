package analytics

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

type Handler struct {
	log *slog.Logger
	svc *Service
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) Overview(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Overview(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, overviewJSON{
		TotalConversations: out.TotalConversations, OpenConversations: out.OpenConversations, ClosedConversations: out.ClosedConversations,
		AIActiveConversations: out.AIActiveConversations, HumanRequestedConversations: out.HumanRequestedConversations,
		HumanActiveConversations: out.HumanActiveConversations, ResolvedHumanConversations: out.ResolvedHumanConversations,
		TotalCustomers: out.TotalCustomers, NewCustomers: out.NewCustomers,
		TotalReservations: out.TotalReservations, ConfirmedReservations: out.ConfirmedReservations,
		CancelledReservations: out.CancelledReservations, CompletedReservations: out.CompletedReservations,
		AIGeneratedMessages: out.AIGeneratedMessages, HumanMessages: out.HumanMessages,
		InboundMessages: out.InboundMessages, OutboundMessages: out.OutboundMessages,
		AutomationRuns: out.AutomationRuns, AutomationSuccesses: out.AutomationSuccesses, AutomationFailures: out.AutomationFailures,
	}, nil)})
}

func (h *Handler) Conversations(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Conversations(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, conversationSummaryJSON{
		Total: out.Total, Open: out.Open, Closed: out.Closed,
		ByChannel: namedCountsJSON(out.ByChannel), ByHandlingState: namedCountsJSON(out.ByHandlingState),
		InboundMessages: out.InboundMessages, OutboundMessages: out.OutboundMessages,
		AIHandled: out.AIHandled, HumanHandled: out.HumanHandled, HumanRequested: out.HumanRequested,
		AverageConversationSeconds: out.AverageConversationSeconds,
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Customers(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Customers(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, customerSummaryJSON{
		TotalCustomers: out.TotalCustomers, NewCustomers: out.NewCustomers, ActiveCustomers: out.ActiveCustomers,
		CustomersWithConversations: out.CustomersWithConversations, CustomersWithReservations: out.CustomersWithReservations,
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Reservations(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Reservations(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	series := make([]reservationDayJSON, 0, len(out.Series))
	for _, day := range out.Series {
		series = append(series, reservationDayJSON{
			Date: day.Date, Total: day.Total, Confirmed: day.Confirmed, Cancelled: day.Cancelled, Completed: day.Completed,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, reservationSummaryJSON{
		Total: out.Total, Pending: out.Pending, Confirmed: out.Confirmed, Cancelled: out.Cancelled, Completed: out.Completed,
		ConfirmationRate: out.ConfirmationRate, CancellationRate: out.CancellationRate, CompletionRate: out.CompletionRate,
		TotalPartySize: out.TotalPartySize, AveragePartySize: out.AveragePartySize,
		ByArea: areaReservationJSONList(out.ByArea), ByTable: tableReservationJSONList(out.ByTable),
		ByHour: hourReservationJSONList(out.ByHour), ByWeekday: weekdayReservationJSONList(out.ByWeekday),
	}, series)})
}

func (h *Handler) Conversion(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Conversion(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	series := make([]conversionDayJSON, 0, len(out.Series))
	for _, day := range out.Series {
		series = append(series, conversionDayJSON{
			Date: day.Date, Conversations: day.Conversations, Reservations: day.Reservations,
			Confirmed: day.Confirmed, Completed: day.Completed, Cancelled: day.Cancelled,
			PaidReservations: day.PaidReservations, PaymentAmount: day.PaymentAmount, PaidAmount: day.PaidAmount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, conversionSummaryJSON{
		Conversations: out.Conversations, ConversationsWithReservation: out.ConversationsWithReservation,
		ReservationsCreated: out.ReservationsCreated, ReservationsConfirmed: out.ReservationsConfirmed,
		ReservationsCompleted: out.ReservationsCompleted, ReservationsCancelled: out.ReservationsCancelled,
		ConversationReservationRate: out.ConversationReservationRate, ConfirmationRate: out.ConfirmationRate,
		CompletionRate: out.CompletionRate, CancellationRate: out.CancellationRate,
		ReservationsRequiringPayment: out.ReservationsRequiringPayment, ReservationsPaid: out.ReservationsPaid,
		ReservationsUnpaid: out.ReservationsUnpaid, ReservationsConfirmedPaid: out.ReservationsConfirmedPaid,
		ReservationsCancelledAfterPayment: out.ReservationsCancelledAfterPayment, ReservationsRefunded: out.ReservationsRefunded,
		PaymentCollectionRate: out.PaymentCollectionRate, PaidReservationConfirmationRate: out.PaidReservationConfirmationRate,
	}, series)})
}

func (h *Handler) Revenue(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Revenue(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	series := make([]paymentDayJSON, 0, len(out.Series))
	for _, day := range out.Series {
		series = append(series, paymentDayJSON{
			Date: day.Date, PaymentRecords: day.PaymentRecords, PaidCount: day.PaidCount,
			TotalAmount: day.TotalAmount, PaidAmount: day.PaidAmount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, paymentSummaryJSON{
		PaymentRecords: out.PaymentRecords, PaidCount: out.PaidCount, UnpaidCount: out.UnpaidCount, RefundedCount: out.RefundedCount,
		TotalAmount: out.TotalAmount, PaidAmount: out.PaidAmount, UnpaidAmount: out.UnpaidAmount, RefundedAmount: out.RefundedAmount,
		ReservationsRequiringPayment: out.ReservationsRequiringPayment, PaymentCollectionRate: out.PaymentCollectionRate,
	}, series)})
}

func (h *Handler) AI(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.AI(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	providers := make([]aiProviderJSON, 0, len(out.ByProvider))
	for _, item := range out.ByProvider {
		providers = append(providers, aiProviderJSON{
			Provider: item.Provider, Model: item.Model, GenerationCount: item.GenerationCount,
			FailureCount: item.FailureCount, AverageLatencyMS: item.AverageLatencyMS,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, aiSummaryJSON{
		TotalGenerations: out.TotalGenerations, SuccessfulGenerations: out.SuccessfulGenerations,
		FailedGenerations: out.FailedGenerations, SkippedGenerations: out.SkippedGenerations,
		AIMessagesGenerated: out.AIMessagesGenerated, AverageLatencyMS: out.AverageLatencyMS, ByProvider: providers,
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Handover(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Handover(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, handoverSummaryJSON{
		HandoverCount: out.HandoverCount, CustomerRequestedCount: out.CustomerRequestedCount,
		AIUnableCount: out.AIUnableCount, ManualHandoffCount: out.ManualHandoffCount,
		HumanStartedCount: out.HumanStartedCount, HumanResolvedCount: out.HumanResolvedCount,
		AverageTimeToHumanSeconds: out.AverageTimeToHumanSeconds, AverageHumanHandlingSeconds: out.AverageHumanHandlingSeconds,
		ByReason: namedCountsJSON(out.ByReason), ByChannel: namedCountsJSON(out.ByChannel),
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Automation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Automation(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]automationBreakdownJSON, 0, len(out.ByAutomation))
	for _, item := range out.ByAutomation {
		items = append(items, automationBreakdownJSON{
			AutomationID: item.AutomationID, AutomationName: item.AutomationName, ActionType: item.ActionType,
			TotalRuns: item.TotalRuns, SuccessfulRuns: item.SuccessfulRuns, FailedRuns: item.FailedRuns, SkippedRuns: item.SkippedRuns,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, automationSummaryJSON{
		TotalRuns: out.TotalRuns, SuccessfulRuns: out.SuccessfulRuns, FailedRuns: out.FailedRuns, SkippedRuns: out.SkippedRuns,
		SuccessRate: out.SuccessRate, FailureRate: out.FailureRate, ByAutomation: items, ByAction: namedCountsJSON(out.ByAction),
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Messaging(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Messaging(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, messagingSummaryJSON{
		InboundMessages: out.InboundMessages, OutboundMessages: out.OutboundMessages,
		Queued: out.Queued, Sent: out.Sent, Delivered: out.Delivered, Read: out.Read, Failed: out.Failed,
		ByChannel: namedCountsJSON(out.ByChannel), ByDirection: namedCountsJSON(out.ByDirection), ByStatus: namedCountsJSON(out.ByStatus),
	}, seriesJSONList(out.Series))})
}

func (h *Handler) Campaigns(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	r, out, err := h.svc.Campaigns(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rangeEnvelope(r, campaignSummaryJSON{
		TotalCampaigns: out.TotalCampaigns, TotalRuns: out.TotalRuns, CompletedRuns: out.CompletedRuns, CancelledRuns: out.CancelledRuns,
		TotalRecipients: out.TotalRecipients, Queued: out.Queued, Sent: out.Sent, Delivered: out.Delivered,
		Read: out.Read, Failed: out.Failed, Skipped: out.Skipped,
		ByCampaignStatus: namedCountsJSON(out.ByCampaignStatus), ByRecipientStatus: namedCountsJSON(out.ByRecipientStatus),
	}, seriesJSONList(out.Series))})
}

func tenantContext(c *gin.Context) (auth.Context, bool) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Context{}, false
	}
	return ac, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	if !errors.Is(err, ErrInvalid) {
		h.log.Error("analytics handler failed", "error", err)
	}
	writeServiceError(c, err)
}
