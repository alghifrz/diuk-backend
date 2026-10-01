package authz

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// Checker is the authorization surface used by middleware and handlers.
type Checker interface {
	HasPermission(ctx context.Context, roleID uuid.UUID, permission Permission) (bool, error)
	List(ctx context.Context, roleID uuid.UUID) ([]string, error)
}

// Service evaluates permissions for a role already resolved from tenant context.
type Service struct {
	repo *Repository
}

// NewService returns an authorization service backed by repo.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// HasPermission reports whether roleID has permission. A database error is
// returned to the caller and must not be treated as a denial.
func (s *Service) HasPermission(ctx context.Context, roleID uuid.UUID, permission Permission) (bool, error) {
	if roleID == uuid.Nil {
		return false, nil
	}
	if permission == "" {
		return false, fmt.Errorf("permission is required")
	}

	allowed, err := s.repo.HasPermission(ctx, roleID, permission)
	if err != nil {
		return false, err
	}

	return allowed, nil
}

// List returns the permission codes for roleID, sorted alphabetically.
func (s *Service) List(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	if roleID == uuid.Nil {
		return []string{}, nil
	}

	codes, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	sort.Strings(codes)
	return codes, nil
}
