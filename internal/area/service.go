package area

import (
	"context"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Service applies area and table rules. The business id must come from tenant context.
type Service struct {
	store Store
}

// NewService returns a service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListAreas(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]Area, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListAreas(ctx, businessID, status)
}

func (s *Service) GetArea(ctx context.Context, businessID, areaID uuid.UUID) (*Area, error) {
	return s.store.GetArea(ctx, businessID, areaID)
}

func (s *Service) CreateArea(ctx context.Context, businessID uuid.UUID, in CreateAreaInput) (*Area, error) {
	name, err := normalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	return s.store.CreateArea(ctx, businessID, name, normalizeOptionalDescription(in.Description))
}

func (s *Service) UpdateArea(ctx context.Context, businessID, areaID uuid.UUID, in UpdateAreaInput) (*Area, error) {
	current, err := s.store.GetArea(ctx, businessID, areaID)
	if err != nil {
		return nil, err
	}

	next := *current
	if in.Name != nil {
		name, err := normalizeName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		next.Description = normalizeOptionalDescription(in.Description)
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		if *in.Status == tenant.StatusInactive && current.Status != tenant.StatusInactive {
			if err := s.ensureNoActiveTables(ctx, businessID, areaID); err != nil {
				return nil, err
			}
		}
		next.Status = *in.Status
	}

	next.BusinessID = current.BusinessID
	return s.store.UpdateArea(ctx, businessID, next)
}

func (s *Service) DeleteArea(ctx context.Context, businessID, areaID uuid.UUID) (*Area, error) {
	current, err := s.store.GetArea(ctx, businessID, areaID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveTables(ctx, businessID, areaID); err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}

	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateArea(ctx, businessID, next)
}

func (s *Service) ListTables(ctx context.Context, businessID, areaID uuid.UUID, statusRaw string) ([]Table, error) {
	if _, err := s.store.GetArea(ctx, businessID, areaID); err != nil {
		return nil, err
	}
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListTables(ctx, businessID, areaID, status)
}

func (s *Service) GetTable(ctx context.Context, businessID, areaID, tableID uuid.UUID) (*Table, error) {
	return s.store.GetTable(ctx, businessID, areaID, tableID)
}

func (s *Service) CreateTable(ctx context.Context, businessID, areaID uuid.UUID, in CreateTableInput) (*Table, error) {
	area, err := s.store.GetArea(ctx, businessID, areaID)
	if err != nil {
		return nil, err
	}
	if area.Status != tenant.StatusActive {
		return nil, ErrAreaInactive
	}

	name, err := normalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	if err := validateCapacity(in.Capacity); err != nil {
		return nil, err
	}
	return s.store.CreateTable(ctx, businessID, areaID, name, in.Capacity)
}

func (s *Service) UpdateTable(ctx context.Context, businessID, areaID, tableID uuid.UUID, in UpdateTableInput) (*Table, error) {
	current, err := s.store.GetTable(ctx, businessID, areaID, tableID)
	if err != nil {
		return nil, err
	}

	next := *current
	if in.Name != nil {
		name, err := normalizeName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Capacity != nil {
		if err := validateCapacity(*in.Capacity); err != nil {
			return nil, err
		}
		next.Capacity = *in.Capacity
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		next.Status = *in.Status
	}

	next.AreaID = current.AreaID
	return s.store.UpdateTable(ctx, businessID, areaID, next)
}

func (s *Service) DeleteTable(ctx context.Context, businessID, areaID, tableID uuid.UUID) (*Table, error) {
	current, err := s.store.GetTable(ctx, businessID, areaID, tableID)
	if err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}

	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateTable(ctx, businessID, areaID, next)
}

func (s *Service) ensureNoActiveTables(ctx context.Context, businessID, areaID uuid.UUID) error {
	n, err := s.store.CountActiveTables(ctx, businessID, areaID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrAreaHasActiveTables
	}
	return nil
}
