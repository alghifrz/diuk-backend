package customer

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Handler serves tenant-scoped customer and CRM endpoints.
type Handler struct {
	log *slog.Logger
	svc *Service
}

// NewHandler returns HTTP handlers for the customer domain.
func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListCustomers(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListCustomersWithOptions(c.Request.Context(), ac.BusinessID, ListOptions{
		Status: c.Query("status"),
		Search: c.Query("search"),
		Tag:    c.Query("tag_id"),
		Sort:   c.Query("sort"),
		Limit:  c.Query("limit"),
		Offset: c.Query("offset"),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": customersResponse(items)})
}

func (h *Handler) CreateCustomer(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateCustomer(c.Request.Context(), ac.BusinessID, CreateCustomerInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": customerResponse(item, true)})
}

func (h *Handler) GetCustomer(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetCustomer(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": customerResponse(item, true)})
}

func (h *Handler) UpdateCustomer(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateCustomerInput{Name: req.Name, Phone: req.Phone, Email: req.Email, Notes: req.Notes, MarketingOptIn: req.MarketingOptIn}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	item, err := h.svc.UpdateCustomer(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": customerResponse(item, true)})
}

func (h *Handler) DeleteCustomer(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.DeleteCustomer(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": customerResponse(item, true)})
}

func (h *Handler) ListIdentities(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListIdentities(c.Request.Context(), ac.BusinessID, id, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": identitiesResponse(items)})
}

func (h *Handler) CreateIdentity(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req createIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateIdentity(c.Request.Context(), ac.BusinessID, id, CreateIdentityInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": identityResponse(item)})
}

func (h *Handler) UpdateIdentity(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	customerID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	identityID, ok := pathUUID(c, "identity_id")
	if !ok {
		return
	}
	var req updateIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateIdentityInput{DisplayName: req.DisplayName}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	item, err := h.svc.UpdateIdentity(c.Request.Context(), ac.BusinessID, customerID, identityID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": identityResponse(item)})
}

func (h *Handler) DeleteIdentity(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	customerID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	identityID, ok := pathUUID(c, "identity_id")
	if !ok {
		return
	}
	item, err := h.svc.DeleteIdentity(c.Request.Context(), ac.BusinessID, customerID, identityID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": identityResponse(item)})
}

func (h *Handler) ListTags(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListTags(c.Request.Context(), ac.BusinessID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tagsResponse(items)})
}

func (h *Handler) CreateTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	item, err := h.svc.CreateTag(c.Request.Context(), ac.BusinessID, CreateTagInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": tagResponse(item)})
}

func (h *Handler) GetTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetTag(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tagResponse(item)})
}

func (h *Handler) UpdateTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateTagInput{Name: req.Name, Description: req.Description}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	item, err := h.svc.UpdateTag(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tagResponse(item)})
}

func (h *Handler) DeleteTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.DeleteTag(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tagResponse(item)})
}

func (h *Handler) ListCustomerTags(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListCustomerTags(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tagsResponse(items)})
}

func (h *Handler) AssignTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	customerID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	tagID, ok := pathUUID(c, "tag_id")
	if !ok {
		return
	}
	item, created, err := h.svc.AssignTag(c.Request.Context(), ac.BusinessID, customerID, tagID)
	if err != nil {
		h.fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": tagResponse(item)})
}

func (h *Handler) UnassignTag(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	customerID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	tagID, ok := pathUUID(c, "tag_id")
	if !ok {
		return
	}
	if err := h.svc.UnassignTag(c.Request.Context(), ac.BusinessID, customerID, tagID); err != nil {
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
		h.log.Error("customer handler failed", "error", err)
	}
	writeServiceError(c, err)
}
