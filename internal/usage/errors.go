package usage

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid          = errors.New("invalid request")
	ErrQuotaExceeded    = errors.New("quota exceeded")
	ErrFeatureDisabled  = errors.New("feature is not enabled")
	ErrNotEntitled      = errors.New("feature is not entitled")
	ErrPlanInactive     = errors.New("plan is inactive")
	ErrPlanNotFound     = errors.New("plan not found")
	ErrBusinessNotFound = errors.New("business not found")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrQuotaExceeded):
		writeError(c, http.StatusConflict, "QUOTA_EXCEEDED", "account usage limit reached")
	case errors.Is(err, ErrFeatureDisabled), errors.Is(err, ErrNotEntitled):
		writeError(c, http.StatusConflict, "FEATURE_NOT_AVAILABLE", "feature is not available on the current plan")
	case errors.Is(err, ErrPlanInactive):
		writeError(c, http.StatusConflict, "PLAN_INACTIVE", "plan is inactive")
	case errors.Is(err, ErrPlanNotFound), errors.Is(err, ErrBusinessNotFound):
		writeError(c, http.StatusNotFound, "NOT_FOUND", "not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
