package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const contextKey = "auth.context"

// Context is the authenticated (and optionally tenant-scoped) request identity.
// It never contains tokens or other secrets.
type Context struct {
	AuthUserID   uuid.UUID
	UserID       uuid.UUID
	FullName     string
	Email        string
	BusinessID   uuid.UUID
	BusinessName string
	BusinessSlug string
	RoleID       uuid.UUID
	RoleName     tenant.RoleName
}

// HasTenant reports whether the tenant middleware has resolved a business.
func (c Context) HasTenant() bool {
	return c.BusinessID != uuid.Nil
}

// GetAuthContext returns the request identity stored by the auth middleware.
func GetAuthContext(c *gin.Context) (Context, bool) {
	value, ok := c.Get(contextKey)
	if !ok {
		return Context{}, false
	}

	ac, ok := value.(Context)
	return ac, ok
}

func setAuthContext(c *gin.Context, ac Context) {
	c.Set(contextKey, ac)
}
