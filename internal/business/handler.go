package business

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Handler serves tenant-scoped business profile and F&B settings endpoints.
type Handler struct {
	log *slog.Logger
	svc *Service
}

// NewHandler returns HTTP handlers for the business domain.
func NewHandler(log *slog.Logger, svc *Service) *Handler {
	return &Handler{log: log, svc: svc}
}

func (h *Handler) GetProfile(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	profile, err := h.svc.Profile(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profileResponse(profile)})
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	profile, err := h.svc.UpdateProfile(c.Request.Context(), ac.BusinessID, UpdateProfileInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profileResponse(profile)})
}

func (h *Handler) GetFNBSettings(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	settings, err := h.svc.FNBSettings(c.Request.Context(), ac.BusinessID)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": fnbSettingsResponse(settings)})
}

func (h *Handler) UpdateFNBSettings(c *gin.Context) {
	ac, ok := tenantContext(c)
	if !ok {
		return
	}

	var req updateFNBSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	settings, err := h.svc.UpdateFNBSettings(c.Request.Context(), ac.BusinessID, UpdateFNBSettingsInput{
		Currency:                       req.Currency,
		ReservationEnabled:             req.ReservationEnabled,
		ReservationSlotDurationMinutes: req.ReservationSlotDurationMinutes,
		ReservationMinPartySize:        req.ReservationMinPartySize,
		ReservationMaxPartySize:        req.ReservationMaxPartySize,
		ReservationMinAdvanceMinutes:   req.ReservationMinAdvanceMinutes,
		ReservationMaxAdvanceDays:      req.ReservationMaxAdvanceDays,
		DepositEnabled:                 req.DepositEnabled,
		DepositType:                    req.DepositType,
		DepositValue:                   req.DepositValue,
		BankName:                       req.BankName,
		BankAccountNumber:              req.BankAccountNumber,
		BankAccountHolder:              req.BankAccountHolder,
	})
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": fnbSettingsResponse(settings)})
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
	if !isClientError(err) {
		h.log.Error("business handler failed", "error", err)
	}
	writeServiceError(c, err)
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound)
}
