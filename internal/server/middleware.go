package server

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
	"github.com/alghifrz/diuk-backend/internal/observability"
)

// requestLogger logs one structured line per request once it completes.
func requestLogger(log *slog.Logger, metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		duration := time.Since(start)
		if metrics != nil {
			metrics.ObserveHTTP(c.Request.Method, route, status, duration)
		}

		attrs := []any{
			"request_id", observability.RequestIDFromGin(c),
			"method", c.Request.Method,
			"path", route,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"error_category", observability.HTTPStatusCategory(status),
		}
		if ac, ok := auth.GetAuthContext(c); ok {
			if ac.BusinessID.String() != "" && ac.HasTenant() {
				attrs = append(attrs, "business_id", ac.BusinessID)
			}
			if ac.UserID != uuid.Nil {
				attrs = append(attrs, "user_id", ac.UserID)
			}
		}

		switch {
		case status >= http.StatusInternalServerError:
			log.Error("request failed", attrs...)
		case status >= http.StatusBadRequest:
			log.Warn("request rejected", attrs...)
		default:
			log.Info("request handled", attrs...)
		}
	}
}

// recovery turns a panic into a 500 response instead of killing the process.
func recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		log.Error("panic recovered",
			"request_id", observability.RequestIDFromGin(c),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"panic", recovered,
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
	})
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Next()
	}
}

func maxJSONBody(limit int64) gin.HandlerFunc {
	if limit <= 0 {
		limit = 1 << 20
	}
	return func(c *gin.Context) {
		ct := c.ContentType()
		if strings.HasPrefix(ct, "multipart/") {
			c.Next()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

// cors allows the configured frontend origins to call the API, including
// preflight requests. Wildcard origins never enable credentials.
func cors(allowedOrigins []string) gin.HandlerFunc {
	allowAll := slices.Contains(allowedOrigins, "*")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		switch {
		case allowAll:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && slices.Contains(allowedOrigins, origin):
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", strings.Join([]string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		}, ", "))
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Business-ID, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Access-Control-Max-Age", "600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
