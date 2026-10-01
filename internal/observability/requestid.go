package observability

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	HeaderRequestID = "X-Request-ID"
	contextKey      = ctxKey("request_id")
)

type ctxKey string

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(contextKey).(string)
	return value
}

func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey, id)
}

func NormalizeRequestID(raw string) string {
	trimmed := trimRequestID(raw)
	if trimmed == "" {
		return uuid.NewString()
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := NormalizeRequestID(c.GetHeader(HeaderRequestID))
		c.Set(string(contextKey), id)
		c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), id))
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

func RequestIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(string(contextKey)); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return RequestIDFromContext(c.Request.Context())
}

func trimRequestID(raw string) string {
	out := make([]rune, 0, len(raw))
	for _, r := range raw {
		if r <= 32 || r == 127 {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
