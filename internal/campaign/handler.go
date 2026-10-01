package campaign

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

func (h *Handler) ListCampaigns(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.List(c.Request.Context(), ac.BusinessID, c.Query("status"), c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": campaignsResponse(items)})
}

func (h *Handler) CreateCampaign(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Create(c.Request.Context(), ac.BusinessID, ac.UserID, CreateCampaignInput{
		Name: req.Name, Description: req.Description, Channel: req.Channel, MessageKind: req.MessageKind,
		TemplateName: req.TemplateName, TemplateLanguage: req.TemplateLanguage, TemplateParameters: req.TemplateParameters,
		Audience: req.Audience, MessageBody: req.MessageBody, ScheduledAt: req.ScheduledAt,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": campaignResponse(item)})
}

func (h *Handler) GetCampaign(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": campaignResponse(item)})
}

func (h *Handler) UpdateCampaign(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Update(c.Request.Context(), ac.BusinessID, id, ac.UserID, UpdateCampaignInput{
		Name: req.Name, Description: req.Description, MessageKind: req.MessageKind,
		TemplateName: req.TemplateName, TemplateLanguage: req.TemplateLanguage, TemplateParameters: req.TemplateParameters,
		Audience: req.Audience, MessageBody: req.MessageBody,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": campaignResponse(item)})
}

func (h *Handler) DeleteCampaign(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Delete(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": campaignResponse(item)})
}

func (h *Handler) ScheduleCampaign(c *gin.Context) {
	h.act(c, func(ac auth.Context, id uuid.UUID) (*Campaign, error) {
		var req scheduleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return nil, invalid("invalid request body")
		}
		return h.svc.Schedule(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.ScheduledAt)
	})
}

func (h *Handler) StartCampaign(c *gin.Context) {
	h.act(c, func(ac auth.Context, id uuid.UUID) (*Campaign, error) {
		return h.svc.Start(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	})
}

func (h *Handler) PauseCampaign(c *gin.Context) {
	h.act(c, func(ac auth.Context, id uuid.UUID) (*Campaign, error) {
		return h.svc.Pause(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	})
}

func (h *Handler) ResumeCampaign(c *gin.Context) {
	h.act(c, func(ac auth.Context, id uuid.UUID) (*Campaign, error) {
		return h.svc.Resume(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	})
}

func (h *Handler) CancelCampaign(c *gin.Context) {
	h.act(c, func(ac auth.Context, id uuid.UUID) (*Campaign, error) {
		return h.svc.Cancel(c.Request.Context(), ac.BusinessID, id, ac.UserID)
	})
}

func (h *Handler) ListRuns(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListRuns(c.Request.Context(), ac.BusinessID, id, c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runsResponse(items)})
}

func (h *Handler) GetRun(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	runID, ok := pathID(c, "run_id")
	if !ok {
		return
	}
	item, err := h.svc.GetRun(c.Request.Context(), ac.BusinessID, id, runID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runResponse(item)})
}

func (h *Handler) ListRecipients(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	runID, ok := pathID(c, "run_id")
	if !ok {
		return
	}
	items, err := h.svc.ListRecipients(c.Request.Context(), ac.BusinessID, id, runID, c.Query("status"), c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": recipientsResponse(items)})
}

func (h *Handler) PreviewAudience(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	out, err := h.svc.Preview(c.Request.Context(), ac.BusinessID, req.Channel, req.Audience)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": previewResponse(out)})
}

func (h *Handler) act(c *gin.Context, fn func(auth.Context, uuid.UUID) (*Campaign, error)) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	item, err := fn(ac, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": campaignResponse(item)})
}

func (h *Handler) fail(c *gin.Context, err error) {
	writeServiceError(c, err)
}

func tenantContext(c *gin.Context) (auth.Context, bool) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Context{}, false
	}
	return ac, true
}

func pathID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := parsePathUUID(c.Param(name))
	if err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "id is invalid")
		return uuid.Nil, false
	}
	return id, true
}
