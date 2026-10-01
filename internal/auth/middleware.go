package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// UserFinder looks up the local profile that belongs to a Supabase Auth user.
type UserFinder interface {
	GetActiveByAuthUserID(ctx context.Context, authUserID uuid.UUID) (*tenant.User, error)
}

// AccessFinder looks up an active membership of a user in a business.
type AccessFinder interface {
	GetActiveAccess(ctx context.Context, userID, businessID uuid.UUID) (*tenant.Access, error)
}

// Middleware holds the dependencies of the authentication and tenant layers.
type Middleware struct {
	log      *slog.Logger
	verifier Verifier
	users    UserFinder
	access   AccessFinder
}

// NewMiddleware wires JWT verification to the local user and membership tables.
func NewMiddleware(log *slog.Logger, verifier Verifier, users UserFinder, access AccessFinder) *Middleware {
	return &Middleware{log: log, verifier: verifier, users: users, access: access}
}

// Authenticate verifies the Bearer token and loads the local user. It does
// not require a business.
func (m *Middleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := BearerToken(c.GetHeader(headerAuthorization))
		if err != nil {
			abortUnauthorized(c, "authentication required")
			return
		}

		authUserID, err := m.verifier.Verify(token)
		if err != nil {
			abortUnauthorized(c, "authentication required")
			return
		}

		user, err := m.users.GetActiveByAuthUserID(c.Request.Context(), authUserID)
		if err != nil {
			if errors.Is(err, tenant.ErrNotFound) {
				abortUnauthorized(c, "authentication required")
				return
			}
			m.log.Error("lookup authenticated user", "error", err)
			abortInternal(c)
			return
		}

		setAuthContext(c, Context{
			AuthUserID: user.AuthUserID,
			UserID:     user.ID,
			FullName:   user.FullName,
			Email:      user.Email,
		})
		c.Next()
	}
}

// RequireTenant reads X-Business-ID and confirms an active membership. The
// header is never trusted on its own.
func (m *Middleware) RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		ac, ok := GetAuthContext(c)
		if !ok {
			abortUnauthorized(c, "authentication required")
			return
		}

		businessID, err := ParseBusinessID(c.GetHeader(headerBusinessID))
		if err != nil {
			if errors.Is(err, ErrMissingBusinessID) {
				abortBadRequest(c, "business id is required")
				return
			}
			abortBadRequest(c, "invalid business id")
			return
		}

		access, err := m.access.GetActiveAccess(c.Request.Context(), ac.UserID, businessID)
		if err != nil {
			if errors.Is(err, tenant.ErrNotFound) {
				abortForbidden(c)
				return
			}
			m.log.Error("lookup business membership", "error", err)
			abortInternal(c)
			return
		}

		ac.BusinessID = access.Business.ID
		ac.BusinessName = access.Business.Name
		ac.BusinessSlug = access.Business.Slug
		ac.RoleID = access.Role.ID
		ac.RoleName = access.Role.Name
		setAuthContext(c, ac)
		c.Next()
	}
}
