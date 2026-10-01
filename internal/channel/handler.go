package channel

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
)

type Handler struct {
	log *slog.Logger
	svc *Service
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListChannels(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListChannels(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": channelsResponse(items)})
}

func (h *Handler) CreateChannel(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateChannel(c.Request.Context(), ac.BusinessID, CreateChannelInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": channelResponse(item)})
}

func (h *Handler) GetChannel(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetChannel(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": channelResponse(item)})
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.UpdateChannel(c.Request.Context(), ac.BusinessID, id, UpdateChannelInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": channelResponse(item)})
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.DeleteChannel(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": channelResponse(item)})
}

func (h *Handler) VerifyWhatsApp(c *gin.Context) {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")
	if mode != "subscribe" || token == "" || token != h.svc.meta.VerifyToken || h.svc.meta.VerifyToken == "" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.String(http.StatusOK, challenge)
}

func (h *Handler) ReceiveWhatsApp(c *gin.Context) {
	limit := h.svc.meta.MaxWebhookBody
	if limit <= 0 {
		limit = 1 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := whatsapp.VerifySignature(h.svc.meta.AppSecret, c.GetHeader("X-Hub-Signature-256"), raw); err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	incoming, statuses, err := whatsapp.Normalize(raw)
	if err != nil {
		h.log.Warn("whatsapp webhook rejected", "reason", "invalid payload")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	h.log.Info("whatsapp webhook received", "incoming", len(incoming), "statuses", len(statuses))
	for _, msg := range incoming {
		if _, err := h.svc.HandleIncoming(c.Request.Context(), incomingFromWhatsApp(msg)); err != nil {
			if !isClientError(err) {
				h.log.Error("whatsapp inbound failed", "error", err)
			} else {
				h.log.Warn("whatsapp inbound skipped", "error", err, "phone_number_id", msg.ExternalAccountID)
			}
			if errors.Is(err, ErrChannelNotFound) || errors.Is(err, ErrChannelDisabled) {
				continue
			}
			c.Status(http.StatusOK)
			return
		}
	}
	for _, status := range statuses {
		if err := h.svc.HandleStatus(c.Request.Context(), statusFromWhatsApp(status)); err != nil && !isClientError(err) {
			h.log.Error("whatsapp status failed", "error", err)
		}
	}
	c.Status(http.StatusOK)
}

func incomingFromWhatsApp(msg whatsapp.Incoming) IncomingMessage {
	return IncomingMessage{
		Channel:            msg.Channel,
		ExternalAccountID:  msg.ExternalAccountID,
		ExternalIdentityID: msg.ExternalIdentityID,
		ExternalMessageID:  msg.ExternalMessageID,
		CustomerName:       msg.CustomerName,
		CustomerPhone:      msg.CustomerPhone,
		MessageType:        msg.MessageType,
		Text:               msg.Text,
		ReceivedAt:         msg.ReceivedAt,
	}
}

func statusFromWhatsApp(status whatsapp.DeliveryStatus) StatusUpdate {
	return StatusUpdate{
		Channel:           status.Channel,
		ExternalAccountID: status.ExternalAccountID,
		ExternalMessageID: status.ExternalMessageID,
		Status:            status.Status,
	}
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
		h.log.Error("channel handler failed", "error", err)
	}
	writeServiceError(c, err)
}
