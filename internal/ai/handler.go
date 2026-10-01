package ai

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

func (h *Handler) GetSettings(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	item, err := h.svc.GetSettings(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": settingsResponse(item)})
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req updateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	temp, clearTemp, err := parseOptionalFloatPatch(req.Temperature)
	if err != nil {
		h.fail(c, err)
		return
	}
	tokens, clearTokens, err := parseOptionalIntPatch(req.MaxOutputTokens)
	if err != nil {
		h.fail(c, err)
		return
	}
	item, err := h.svc.UpdateSettings(c.Request.Context(), ac.BusinessID, UpdateSettingsInput{
		Enabled:         req.Enabled,
		Provider:        req.Provider,
		Model:           req.Model,
		Temperature:     temp,
		ClearTemp:       clearTemp,
		MaxOutputTokens: tokens,
		ClearMaxTokens:  clearTokens,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": settingsResponse(item)})
}

func (h *Handler) ListPrompts(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListPrompts(c.Request.Context(), ac.BusinessID, c.Query("name"), c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": promptsResponse(items)})
}

func (h *Handler) CreatePrompt(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreatePrompt(c.Request.Context(), ac.BusinessID, ac.UserID, CreatePromptInput{
		Name:         req.Name,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": promptResponse(item)})
}

func (h *Handler) GetPrompt(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetPrompt(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": promptResponse(item)})
}

func (h *Handler) UpdatePrompt(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.UpdatePrompt(c.Request.Context(), ac.BusinessID, id, UpdatePromptInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": promptResponse(item)})
}

func (h *Handler) ActivatePrompt(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.ActivatePrompt(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": promptResponse(item)})
}

func (h *Handler) DeletePrompt(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.DeletePrompt(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": promptResponse(item)})
}

func (h *Handler) Generate(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req generateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	conversationID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "conversation_id is invalid")
		return
	}
	persistIncoming := true
	if req.PersistIncoming != nil {
		persistIncoming = *req.PersistIncoming
	}
	item, err := h.svc.GenerateReply(c.Request.Context(), ac.BusinessID, GenerateReplyInput{
		ConversationID:  conversationID,
		UserID:          ac.UserID,
		Message:         req.Message,
		PersistIncoming: persistIncoming,
		UsageSourceID:   uuid.New(),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": generateResponse(item)})
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
		h.log.Error("ai handler failed", "error", err)
	}
	writeServiceError(c, err)
}
