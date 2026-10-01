package area

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Handler serves tenant-scoped area and table endpoints.
type Handler struct {
	log *slog.Logger
	svc *Service
}

// NewHandler returns HTTP handlers for the area domain.
func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListAreas(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	areas, err := h.svc.ListAreas(c.Request.Context(), ac.BusinessID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": areasResponse(areas)})
}

func (h *Handler) GetArea(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}

	area, err := h.svc.GetArea(c.Request.Context(), ac.BusinessID, areaID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": areaResponse(area)})
}

func (h *Handler) CreateArea(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	var req createAreaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	area, err := h.svc.CreateArea(c.Request.Context(), ac.BusinessID, CreateAreaInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": areaResponse(area)})
}

func (h *Handler) UpdateArea(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}

	var req updateAreaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	in := UpdateAreaInput{Name: req.Name, Description: req.Description}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}

	area, err := h.svc.UpdateArea(c.Request.Context(), ac.BusinessID, areaID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": areaResponse(area)})
}

func (h *Handler) DeleteArea(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}

	area, err := h.svc.DeleteArea(c.Request.Context(), ac.BusinessID, areaID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": areaResponse(area)})
}

func (h *Handler) ListTables(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}

	tables, err := h.svc.ListTables(c.Request.Context(), ac.BusinessID, areaID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tablesResponse(tables)})
}

func (h *Handler) GetTable(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	tableID, ok := pathUUID(c, "table_id")
	if !ok {
		return
	}

	table, err := h.svc.GetTable(c.Request.Context(), ac.BusinessID, areaID, tableID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tableResponse(table)})
}

func (h *Handler) CreateTable(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}

	var req createTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	table, err := h.svc.CreateTable(c.Request.Context(), ac.BusinessID, areaID, CreateTableInput{
		Name:     req.Name,
		Capacity: req.Capacity,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": tableResponse(table)})
}

func (h *Handler) UpdateTable(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	tableID, ok := pathUUID(c, "table_id")
	if !ok {
		return
	}

	var req updateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	in := UpdateTableInput{Name: req.Name, Capacity: req.Capacity}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}

	table, err := h.svc.UpdateTable(c.Request.Context(), ac.BusinessID, areaID, tableID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tableResponse(table)})
}

func (h *Handler) DeleteTable(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	areaID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	tableID, ok := pathUUID(c, "table_id")
	if !ok {
		return
	}

	table, err := h.svc.DeleteTable(c.Request.Context(), ac.BusinessID, areaID, tableID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tableResponse(table)})
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
		h.log.Error("area handler failed", "error", err)
	}
	writeServiceError(c, err)
}
