package knowledge

import (
	"errors"
	"io"
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

func (h *Handler) ListDocuments(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListDocuments(c.Request.Context(), ac.BusinessID, c.Query("status"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": documentsResponse(items)})
}

func (h *Handler) GetDocument(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	doc, err := h.svc.GetDocument(c.Request.Context(), ac.BusinessID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": documentResponse(doc)})
}

func (h *Handler) CreateDocument(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	in, err := parseCreate(c, h.svc.settings.MaxFileBytes)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	userID := ac.UserID.String()
	in.CreatedBy = &userID
	doc, err := h.svc.CreateDocument(c.Request.Context(), ac.BusinessID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": documentResponse(doc)})
}

func (h *Handler) UpdateDocument(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req updateDocumentJSON
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	userID := ac.UserID.String()
	doc, err := h.svc.UpdateDocument(c.Request.Context(), ac.BusinessID, id, UpdateDocumentInput{
		Title: req.Title, Description: req.Description, UpdatedBy: &userID,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": documentResponse(doc)})
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteDocument(c.Request.Context(), ac.BusinessID, id); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id.String()}})
}

func (h *Handler) ReprocessDocument(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	userID := ac.UserID.String()
	doc, err := h.svc.ReprocessDocument(c.Request.Context(), ac.BusinessID, id, &userID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": documentResponse(doc)})
}

// Status reports whether retrieval is configured so the UI can explain why
// uploads would fail instead of letting documents sit in FAILED.
func (h *Handler) Status(c *gin.Context) {
	if _, ok := tenantContext(c); !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": statusJSON{
		Enabled:       h.svc.Enabled(),
		MaxFileBytes:  h.svc.settings.MaxFileBytes,
		MinSimilarity: h.svc.settings.MinSimilarity,
	}})
}

// Search runs the same retrieval the AI uses so owners can check what it would find.
func (h *Handler) Search(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req searchRequestJSON
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	out, err := h.svc.SearchKnowledge(c.Request.Context(), ac.BusinessID, req.Query, req.Limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": searchResponse(out)})
}

// Ask answers a question from the knowledge base (retrieve, then generate).
func (h *Handler) Ask(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	var req askRequestJSON
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if _, err := normalizeQuery(req.Question); err != nil {
		h.fail(c, err)
		return
	}
	out, err := h.svc.Ask(c.Request.Context(), ac.BusinessID, req.Question)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": askResponse(out)})
}

func parseCreate(c *gin.Context, maxBytes int64) (CreateDocumentInput, error) {
	ctype := strings.ToLower(c.ContentType())
	if strings.HasPrefix(ctype, "multipart/form-data") {
		return parseCreateMultipart(c, maxBytes)
	}
	var req createDocumentJSON
	if err := c.ShouldBindJSON(&req); err != nil {
		return CreateDocumentInput{}, invalid("invalid request body")
	}
	return CreateDocumentInput{
		Title:       req.Title,
		Description: req.Description,
		SourceType:  req.SourceType,
		Content:     []byte(req.Content),
		Filename:    "source.txt",
		MimeType:    "text/plain",
	}, nil
}

func parseCreateMultipart(c *gin.Context, maxBytes int64) (CreateDocumentInput, error) {
	limit := maxBytes + (1 << 20)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	if err := c.Request.ParseMultipartForm(limit); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(strings.ToLower(err.Error()), "too large") {
			return CreateDocumentInput{}, ErrDocumentTooLarge
		}
		return CreateDocumentInput{}, invalid("invalid request body")
	}
	in := CreateDocumentInput{
		Title:       c.PostForm("title"),
		SourceType:  c.PostForm("source_type"),
		Description: formOptional(c, "description"),
	}
	if file, header, err := c.Request.FormFile("file"); err == nil {
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			return CreateDocumentInput{}, invalid("invalid file")
		}
		in.Content = data
		in.Filename = header.Filename
		in.MimeType = header.Header.Get("Content-Type")
	} else if errors.Is(err, http.ErrMissingFile) {
		in.Content = []byte(c.PostForm("content"))
		in.Filename = "source.txt"
		in.MimeType = "text/plain"
	} else {
		return CreateDocumentInput{}, invalid("invalid file")
	}
	return in, nil
}

func formOptional(c *gin.Context, key string) *string {
	if v, ok := c.GetPostForm(key); ok {
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
		h.log.Error("knowledge handler failed", "error", err)
	}
	writeServiceError(c, err)
}
