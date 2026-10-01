package area

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalid             = errors.New("invalid request")
	ErrAreaNameExists      = errors.New("area name already exists")
	ErrTableNameExists     = errors.New("table name already exists")
	ErrAreaHasActiveTables = errors.New("area has active tables")
	ErrAreaInactive        = errors.New("area is not active")
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
	case errors.Is(err, ErrAreaNameExists):
		writeError(c, http.StatusConflict, "AREA_NAME_ALREADY_EXISTS", "area name already exists")
	case errors.Is(err, ErrTableNameExists):
		writeError(c, http.StatusConflict, "TABLE_NAME_ALREADY_EXISTS", "table name already exists")
	case errors.Is(err, ErrAreaHasActiveTables):
		writeError(c, http.StatusConflict, "AREA_HAS_ACTIVE_TABLES", "area has active tables")
	case errors.Is(err, ErrAreaInactive):
		writeError(c, http.StatusConflict, "AREA_INACTIVE", "area is not active")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrAreaNameExists) ||
		errors.Is(err, ErrTableNameExists) ||
		errors.Is(err, ErrAreaHasActiveTables) ||
		errors.Is(err, ErrAreaInactive)
}
