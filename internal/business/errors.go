package business

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid request")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error {
	return validationError{msg: msg}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
