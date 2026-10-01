package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	codeUnauthorized = "UNAUTHORIZED"
	codeForbidden    = "FORBIDDEN"
	codeBadRequest   = "BAD_REQUEST"
	codeInternal     = "INTERNAL"
)

func abortUnauthorized(c *gin.Context, message string) {
	writeError(c, http.StatusUnauthorized, codeUnauthorized, message)
}

func abortForbidden(c *gin.Context) {
	writeError(c, http.StatusForbidden, codeForbidden, "access denied")
}

func abortBadRequest(c *gin.Context, message string) {
	writeError(c, http.StatusBadRequest, codeBadRequest, message)
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
