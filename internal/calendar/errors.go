package calendar

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid      = errors.New("invalid request")
	ErrNotFound     = errors.New("calendar block not found")
	ErrInvalidRange = errors.New("calendar range is invalid")
	ErrInvalidScope = errors.New("calendar block scope is invalid")
	ErrBlocked      = errors.New("time range is blocked")
	ErrConflict     = errors.New("calendar block conflicts with existing reservations")
	ErrAreaMissing  = errors.New("area not found")
	ErrTableMissing = errors.New("table not found")
)

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

type conflictError struct {
	Conflicts []ReservationConflict
}

func (e conflictError) Error() string { return ErrConflict.Error() }
func (e conflictError) Unwrap() error { return ErrConflict }

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrInvalidRange) ||
		errors.Is(err, ErrInvalidScope) ||
		errors.Is(err, ErrBlocked) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrAreaMissing) ||
		errors.Is(err, ErrTableMissing)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrInvalidRange):
		writeError(c, http.StatusBadRequest, "CALENDAR_INVALID_RANGE", err.Error())
	case errors.Is(err, ErrInvalidScope):
		writeError(c, http.StatusBadRequest, "CALENDAR_INVALID_SCOPE", "calendar block scope is invalid")
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "CALENDAR_BLOCK_NOT_FOUND", "calendar block not found")
	case errors.Is(err, ErrAreaMissing):
		writeError(c, http.StatusNotFound, "CALENDAR_AREA_NOT_FOUND", "area not found")
	case errors.Is(err, ErrTableMissing):
		writeError(c, http.StatusNotFound, "CALENDAR_TABLE_NOT_FOUND", "table not found")
	case errors.Is(err, ErrBlocked):
		writeError(c, http.StatusConflict, "CALENDAR_BLOCKED", "time range is blocked")
	case errors.Is(err, ErrConflict):
		var ce conflictError
		if errors.As(err, &ce) && len(ce.Conflicts) > 0 {
			items := make([]gin.H, 0, len(ce.Conflicts))
			for _, item := range ce.Conflicts {
				items = append(items, gin.H{
					"reservation_id": item.ReservationID.String(),
					"start_at":       item.StartAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
					"end_at":         item.EndAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
					"status":         item.Status,
				})
			}
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":      "CALENDAR_BLOCK_CONFLICT",
					"message":   "calendar block conflicts with existing reservations",
					"conflicts": items,
				},
			})
			return
		}
		writeError(c, http.StatusConflict, "CALENDAR_BLOCK_CONFLICT", "calendar block conflicts with an existing block")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
