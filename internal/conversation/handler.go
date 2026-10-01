package conversation

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Handler serves tenant-scoped conversation and message endpoints.
type Handler struct {
	log *slog.Logger
	svc *Service
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListConversations(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListConversations(
		c.Request.Context(),
		ac.BusinessID,
		c.Query("status"),
		c.Query("channel"),
		c.Query("assigned_user_id"),
		c.Query("customer_id"),
		c.Query("search"),
		c.Query("unread"),
		c.Query("limit"),
		c.Query("offset"),
	)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationsResponse(items)})
}

func (h *Handler) CreateConversation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, created, err := h.svc.CreateConversation(c.Request.Context(), ac.BusinessID, CreateConversationInput{
		CustomerID:         req.CustomerID,
		CustomerIdentityID: req.CustomerIdentityID,
		Channel:            req.Channel,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) GetConversation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetConversation(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) UpdateConversation(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateConversationInput{}
	if req.Status != nil {
		status, err := parseConversationStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	assignedID, clear, err := parseAssignedUserPatch(req.AssignedUserID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	in.AssignedUserID = assignedID
	in.ClearAssignedUser = clear

	item, err := h.svc.UpdateConversation(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) MarkRead(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.MarkRead(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) ListMessages(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListMessages(c.Request.Context(), ac.BusinessID, id, c.Query("limit"), c.Query("offset"), c.Query("before"), c.Query("after"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": messagesResponse(items)})
}

func (h *Handler) CreateMessage(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req createMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, created, err := h.svc.CreateMessage(c.Request.Context(), ac.BusinessID, id, CreateMessageInput{
		Direction:   req.Direction,
		SenderType:  req.SenderType,
		MessageType: req.MessageType,
		Content:     req.Content,
		ExternalID:  req.ExternalID,
		Status:      req.Status,
		Metadata:    req.Metadata,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": messageResponse(item)})
}

func (h *Handler) ListHumanInbox(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListHumanInbox(c.Request.Context(), ac.BusinessID, c.Query("state"), c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": humanInboxResponse(items)})
}

func (h *Handler) RequestHandover(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req handoverRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		req.Reason = string(ReasonManualHandoff)
	}
	item, err := h.svc.RequestHumanHandover(c.Request.Context(), ac.BusinessID, id, req.Reason)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) Takeover(c *gin.Context) {
	h.mutateConversation(c, h.svc.TakeoverConversation)
}

func (h *Handler) Resolve(c *gin.Context) {
	h.mutateConversation(c, h.svc.ResolveConversation)
}

func (h *Handler) ResumeAI(c *gin.Context) {
	h.mutateConversation(c, h.svc.ResumeAI)
}

func (h *Handler) mutateConversation(c *gin.Context, fn func(context.Context, uuid.UUID, uuid.UUID) (*Conversation, error)) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := fn(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversationResponse(item)})
}

func (h *Handler) Reply(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req replyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.Reply(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.Content)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": messageResponse(item)})
}

func (h *Handler) CreateNote(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateNote(c.Request.Context(), ac.BusinessID, id, ac.UserID, req.Content)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": messageResponse(item)})
}

func parseAssignedUserPatch(raw *json.RawMessage) (*uuid.UUID, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if string(*raw) == "null" {
		return nil, true, nil
	}
	var value string
	if err := json.Unmarshal(*raw, &value); err != nil {
		return nil, false, invalid("assigned_user_id is invalid")
	}
	if strings.TrimSpace(value) == "" {
		return nil, true, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, false, invalid("assigned_user_id is invalid")
	}
	return &id, false, nil
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
		h.log.Error("conversation handler failed", "error", err)
	}
	writeServiceError(c, err)
}
