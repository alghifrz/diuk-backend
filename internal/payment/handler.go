package payment

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

type Handler struct {
	log *slog.Logger
	svc *Service
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListPayments(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.List(c.Request.Context(), ac.BusinessID, c.Query("status"), c.Query("reservation_id"), c.Query("customer_id"), c.Query("from"), c.Query("to"), c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentsResponse(items)})
}

func (h *Handler) GetByReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetByReservation(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func (h *Handler) CreateForReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Notes *string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.CreateForReservation(c.Request.Context(), ac.BusinessID, id, req.Notes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func (h *Handler) ConfirmPayment(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Notes *string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.Confirm(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.Notes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func (h *Handler) MarkFullPayment(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Notes *string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.MarkFull(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.Notes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func (h *Handler) RefundPayment(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Notes *string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.Refund(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.Notes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func (h *Handler) UpdatePayment(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Notes *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.UpdateNotes(c.Request.Context(), ac.BusinessID, id, req.Notes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": paymentResponse(item)})
}

func tenantContext(c *gin.Context) (auth.Context, bool) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Context{}, false
	}
	return ac, true
}

func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	if !isClientError(err) {
		h.log.Error("payment handler failed", "error", err)
	}
	writeServiceError(c, err)
}
