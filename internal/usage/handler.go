package usage

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

type Handler struct {
	log *slog.Logger
	svc *Service
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) GetUsage(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	out, err := h.svc.GetUsage(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": snapshotResponse(out)})
}

func (h *Handler) GetHistory(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}
	out, err := h.svc.History(c.Request.Context(), ac.BusinessID, c.Query("limit"), c.Query("offset"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": historyResponse(out)})
}

func tenantContext(c *gin.Context) (auth.Context, bool) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Context{}, false
	}
	return ac, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrQuotaExceeded) && !errors.Is(err, ErrNotEntitled) && !errors.Is(err, ErrFeatureDisabled) && !errors.Is(err, ErrPlanInactive) {
		h.log.Error("usage handler failed", "error", err)
	}
	writeServiceError(c, err)
}
