package calendar

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

func (h *Handler) ListEvents(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.Events(c.Request.Context(), ac.BusinessID, c.Query("from"), c.Query("to"), c.Query("area_id"), c.Query("table_id"), c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": eventsResponse(items)})
}

func (h *Handler) Day(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	item, err := h.svc.Day(c.Request.Context(), ac.BusinessID, c.Query("date"), c.Query("area_id"), c.Query("table_id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dayResponse(item)})
}

func (h *Handler) CreateBlock(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req CreateBlockInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateBlock(c.Request.Context(), ac.BusinessID, ac.UserID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": blockResponse(item)})
}

func (h *Handler) UpdateBlock(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c)
	if !ok {
		return
	}
	var req UpdateBlockInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.UpdateBlock(c.Request.Context(), ac.BusinessID, id, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": blockResponse(item)})
}

func (h *Handler) DeleteBlock(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c)
	if !ok {
		return
	}
	item, err := h.svc.CancelBlock(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": blockResponse(item)})
}

func tenantContext(c *gin.Context) (auth.Context, bool) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Context{}, false
	}
	return ac, true
}

func pathUUID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	if !isClientError(err) {
		h.log.Error("calendar handler failed", "error", err)
	}
	writeServiceError(c, err)
}
