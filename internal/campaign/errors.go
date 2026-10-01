package campaign

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid           = errors.New("invalid request")
	ErrNotFound          = errors.New("campaign not found")
	ErrRunNotFound       = errors.New("campaign run not found")
	ErrInvalidTransition = errors.New("campaign status transition is invalid")
	ErrBusinessInactive  = errors.New("business is not active")
	ErrNotClaimed        = errors.New("campaign recipient was not claimed")
	ErrRunNotRunning     = errors.New("campaign run is not running")
	ErrAlreadyTerminal   = errors.New("campaign is already finished")
)

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

type transitionError struct{ msg string }

func (e transitionError) Error() string { return e.msg }
func (e transitionError) Unwrap() error { return ErrInvalidTransition }

func badTransition(msg string) error { return transitionError{msg: msg} }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrInvalidTransition):
		writeError(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, ErrBusinessInactive):
		writeError(c, http.StatusConflict, "BUSINESS_INACTIVE", "business is not active")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrRunNotFound):
		writeError(c, http.StatusNotFound, "NOT_FOUND", "not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
