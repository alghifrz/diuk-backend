package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// handleMe is a protected probe used to verify JWT + tenant resolution. It is
// not a full profile API.
func (s *Server) handleMe(c *gin.Context) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":           ac.UserID,
			"auth_user_id": ac.AuthUserID,
			"full_name":    ac.FullName,
			"email":        ac.Email,
		},
		"business": gin.H{
			"id":   ac.BusinessID,
			"name": ac.BusinessName,
			"slug": ac.BusinessSlug,
		},
		"role": gin.H{
			"id":   ac.RoleID,
			"name": ac.RoleName,
		},
	})
}

// handleMePermissions returns the permission codes of the current tenant role.
func (s *Server) handleMePermissions(c *gin.Context) {
	ac, ok := auth.GetAuthContext(c)
	if !ok || !ac.HasTenant() {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			},
		})
		return
	}

	permissions, err := s.authz.List(c.Request.Context(), ac.RoleID)
	if err != nil {
		s.log.Error("list permissions", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": "internal server error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"business_id": ac.BusinessID,
			"role":        ac.RoleName,
			"permissions": permissions,
		},
	})
}
