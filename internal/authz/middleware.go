package authz

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// Middleware enforces database-backed permissions on top of Job 03 context.
type Middleware struct {
	log     *slog.Logger
	checker Checker
}

// NewMiddleware returns authorization middleware that consumes the existing
// authenticated tenant context. It does not read Authorization or X-Business-ID.
func NewMiddleware(log *slog.Logger, checker Checker) *Middleware {
	return &Middleware{log: log, checker: checker}
}

// RequirePermission allows the request only when the current tenant role has
// the given permission.
func (m *Middleware) RequirePermission(permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac, ok := auth.GetAuthContext(c)
		if !ok {
			abortUnauthorized(c)
			return
		}
		if !ac.HasTenant() || ac.RoleID == uuid.Nil {
			abortForbidden(c)
			return
		}

		allowed, err := m.checker.HasPermission(c.Request.Context(), ac.RoleID, permission)
		if err != nil {
			m.log.Error("authorization check failed", "error", err)
			abortInternal(c)
			return
		}
		if !allowed {
			abortForbidden(c)
			return
		}

		c.Next()
	}
}

// List returns the permission codes for the given tenant role.
func (m *Middleware) List(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	return m.checker.List(ctx, roleID)
}
