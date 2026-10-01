package authz

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrDenied is returned when a role does not have the requested permission.
var ErrDenied = errors.New("forbidden")

const (
	codeUnauthorized = "UNAUTHORIZED"
	codeForbidden    = "FORBIDDEN"
	codeInternal     = "INTERNAL"
)

func abortUnauthorized(c *gin.Context) {
	writeError(c, http.StatusUnauthorized, codeUnauthorized, "authentication required")
}

func abortForbidden(c *gin.Context) {
	writeError(c, http.StatusForbidden, codeForbidden, "forbidden")
}

func abortInternal(c *gin.Context) {
	writeError(c, http.StatusInternalServerError, codeInternal, "internal server error")
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
