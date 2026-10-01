package automation

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

func (h *Handler) ListAutomations(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.List(c.Request.Context(), ac.BusinessID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listResponse(items)})
}

func (h *Handler) CreateAutomation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Create(c.Request.Context(), ac.BusinessID, ac.UserID, CreateAutomationInput{
		Name:          req.Name,
		Description:   req.Description,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		Conditions:    req.Conditions,
		ActionType:    req.ActionType,
		ActionConfig:  req.ActionConfig,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": detailResponse(item)})
}

func (h *Handler) GetAutomation(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{"data": detailResponse(item)})
}

func (h *Handler) UpdateAutomation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Update(c.Request.Context(), ac.BusinessID, id, ac.UserID, UpdateAutomationInput{
		Name:          req.Name,
		Description:   req.Description,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		Conditions:    req.Conditions,
		ActionType:    req.ActionType,
		ActionConfig:  req.ActionConfig,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detailResponse(item)})
}

func (h *Handler) DeleteAutomation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Archive(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detailResponse(item)})
}

func (h *Handler) ActivateAutomation(c *gin.Context) {
	h.setStatus(c, true)
}

func (h *Handler) DeactivateAutomation(c *gin.Context) {
	h.setStatus(c, false)
}

func (h *Handler) setStatus(c *gin.Context, active bool) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var (
		item *Automation
		err  error
	)
	if active {
		item, err = h.svc.Activate(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	} else {
		item, err = h.svc.Deactivate(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detailResponse(item)})
}

func (h *Handler) ListRuns(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListRuns(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runsResponse(items)})
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
		h.log.Error("automation handler failed", "error", err)
	}
	writeServiceError(c, err)
}
