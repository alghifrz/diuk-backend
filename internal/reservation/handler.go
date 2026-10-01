package reservation

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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

func (h *Handler) ListReservations(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.List(c.Request.Context(), ac.BusinessID, c.Query("status"), c.Query("customer_id"), c.Query("table_id"), c.Query("date"), c.Query("start_date"), c.Query("end_date"), c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationsResponse(items)})
}

func (h *Handler) Availability(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	item, err := h.svc.Availability(c.Request.Context(), ac.BusinessID, c.Query("date"), c.Query("start_time"), c.Query("duration_minutes"), c.Query("party_size"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": availabilityResponse(item)})
}

func (h *Handler) CreateReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createReservationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Create(c.Request.Context(), ac.BusinessID, CreateReservationInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) GetReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) UpdateReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		CustomerID *string          `json:"customer_id"`
		StartAt    *string          `json:"start_at"`
		EndAt      *string          `json:"end_at"`
		PartySize  *int             `json:"party_size"`
		TableID    *json.RawMessage `json:"table_id"`
		Notes      *string          `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateReservationInput{CustomerID: req.CustomerID, StartAt: req.StartAt, EndAt: req.EndAt, PartySize: req.PartySize, Notes: req.Notes}
	if req.TableID != nil {
		if string(*req.TableID) == "null" {
			in.ClearTable = true
		} else {
			var value string
			if err := json.Unmarshal(*req.TableID, &value); err != nil {
				writeError(c, http.StatusBadRequest, "BAD_REQUEST", "table_id is invalid")
				return
			}
			in.TableID = &value
		}
	}
	item, err := h.svc.Update(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) RescheduleReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req struct {
		StartAt   string  `json:"start_at"`
		EndAt     string  `json:"end_at"`
		PartySize *int    `json:"party_size"`
		TableID   *string `json:"table_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Reschedule(c.Request.Context(), ac.BusinessID, id, RescheduleInput{
		StartAt: req.StartAt, EndAt: req.EndAt, PartySize: req.PartySize, TableID: req.TableID,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) ConfirmReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Confirm(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) CancelReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req cancelRequest
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.Cancel(c.Request.Context(), ac.BusinessID, id, req.Reason)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) CompleteReservation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Complete(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reservationResponse(item)})
}

func (h *Handler) GetHours(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListHours(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": hoursResponse(items)})
}

func (h *Handler) ReplaceHours(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req replaceHoursRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := make([]HourInput, 0, len(req.Hours))
	for _, hitem := range req.Hours {
		in = append(in, HourInput{DayOfWeek: hitem.DayOfWeek, SortOrder: hitem.SortOrder, OpenTime: hitem.OpenTime, CloseTime: hitem.CloseTime})
	}
	items, err := h.svc.ReplaceHours(c.Request.Context(), ac.BusinessID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": hoursResponse(items)})
}

func (h *Handler) ListExceptions(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListExceptions(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": exceptionsResponse(items)})
}

func (h *Handler) CreateException(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req exceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.IsClosed == nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "is_closed is required")
		return
	}
	item, err := h.svc.CreateException(c.Request.Context(), ac.BusinessID, ExceptionInput{
		ExceptionDate: req.ExceptionDate, IsClosed: *req.IsClosed, OpenTime: req.OpenTime, CloseTime: req.CloseTime, Note: req.Note,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": exceptionResponse(item)})
}

func (h *Handler) UpdateException(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req exceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateExceptionInput{OpenTime: req.OpenTime, CloseTime: req.CloseTime, Note: req.Note}
	if strings.TrimSpace(req.ExceptionDate) != "" {
		in.ExceptionDate = &req.ExceptionDate
	}
	if req.IsClosed != nil {
		in.IsClosed = req.IsClosed
	}
	item, err := h.svc.UpdateException(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": exceptionResponse(item)})
}

func (h *Handler) DeleteException(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteException(c.Request.Context(), ac.BusinessID, id); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
		h.log.Error("reservation handler failed", "error", err)
	}
	writeServiceError(c, err)
}
