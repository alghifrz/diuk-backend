package menu

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Handler serves tenant-scoped menu category and item endpoints.
type Handler struct {
	log *slog.Logger
	svc *Service
}

// NewHandler returns HTTP handlers for the menu domain.
func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) ListCategories(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	categories, err := h.svc.ListCategories(c.Request.Context(), ac.BusinessID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categoriesResponse(categories)})
}

func (h *Handler) GetCategory(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	category, err := h.svc.GetCategory(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categoryResponse(category)})
}

func (h *Handler) CreateCategory(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req createCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	category, err := h.svc.CreateCategory(c.Request.Context(), ac.BusinessID, CreateCategoryInput{
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   sortOrder,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": categoryResponse(category)})
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	in := UpdateCategoryInput{Name: req.Name, Description: req.Description, SortOrder: req.SortOrder}
	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	category, err := h.svc.UpdateCategory(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categoryResponse(category)})
}

func (h *Handler) DeleteCategory(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	category, err := h.svc.DeleteCategory(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categoryResponse(category)})
}

func (h *Handler) ListItems(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var categoryID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("category_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid category_id")
			return
		}
		categoryID = &id
	}
	items, err := h.svc.ListItems(c.Request.Context(), ac.BusinessID, c.Query("status"), categoryID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": itemsResponse(items)})
}

func (h *Handler) ListCategoryItems(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	categoryID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.ListItems(c.Request.Context(), ac.BusinessID, c.Query("status"), &categoryID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": itemsResponse(items)})
}

func (h *Handler) GetItem(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetItem(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": itemResponse(item)})
}

func (h *Handler) CreateItem(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	if !h.parseUpload(c) {
		return
	}
	img, ok := optionalImage(c)
	if !ok {
		return
	}
	desc := formOptional(c, "description")
	sortOrder, err := parseSortOrder(c.PostForm("sort_order"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	item, err := h.svc.CreateItem(c.Request.Context(), ac.BusinessID, CreateItemInput{
		CategoryID:  c.PostForm("category_id"),
		Name:        c.PostForm("name"),
		Description: desc,
		Price:       c.PostForm("price"),
		SortOrder:   sortOrder,
		Image:       img,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": itemResponse(item)})
}

func (h *Handler) UpdateItem(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if !h.parseUpload(c) {
		return
	}
	img, ok := optionalImage(c)
	if !ok {
		return
	}

	in := UpdateItemInput{Image: img}
	if v, exists := formPresent(c, "name"); exists {
		in.Name = &v
	}
	if v, exists := formPresent(c, "description"); exists {
		in.Description = &v
	}
	if v, exists := formPresent(c, "price"); exists {
		in.Price = &v
	}
	if v, exists := formPresent(c, "sort_order"); exists {
		n, err := parseSortOrder(v)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.SortOrder = &n
	}
	if v, exists := formPresent(c, "status"); exists {
		status, err := parseStatus(v)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		in.Status = &status
	}
	if v, exists := formPresent(c, "remove_image"); exists {
		in.RemoveImage = strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	}

	item, err := h.svc.UpdateItem(c.Request.Context(), ac.BusinessID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": itemResponse(item)})
}

func (h *Handler) DeleteItem(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.DeleteItem(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": itemResponse(item)})
}

func (h *Handler) parseUpload(c *gin.Context) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	if err := c.Request.ParseMultipartForm(maxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(strings.ToLower(err.Error()), "too large") {
			writeServiceError(c, ErrImageTooLarge)
			return false
		}
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return false
	}
	return true
}

func optionalImage(c *gin.Context) (*validatedImage, bool) {
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, true
		}
		writeServiceError(c, ErrInvalidImageType)
		return nil, false
	}
	defer file.Close()

	declared := header.Header.Get("Content-Type")
	img, err := validateImage(file, declared)
	if err != nil {
		writeServiceError(c, err)
		return nil, false
	}
	return img, true
}

func formPresent(c *gin.Context, key string) (string, bool) {
	if c.Request.MultipartForm == nil || c.Request.MultipartForm.Value == nil {
		return "", false
	}
	values, ok := c.Request.MultipartForm.Value[key]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func formOptional(c *gin.Context, key string) *string {
	if v, ok := formPresent(c, key); ok {
		return &v
	}
	return nil
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
		h.log.Error("menu handler failed", "error", err)
	}
	writeServiceError(c, err)
}
