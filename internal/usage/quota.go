package usage

import (
	"context"

	"github.com/google/uuid"
)

// Meter is the small interface other domains depend on.
// Check is advisory. Consume is the authoritative atomic increment.
type Meter interface {
	Check(ctx context.Context, businessID uuid.UUID, feature FeatureKey) error
	Consume(ctx context.Context, businessID uuid.UUID, in ConsumeInput) (*ConsumeResult, error)
	CheckResource(ctx context.Context, businessID uuid.UUID, feature FeatureKey) error
}

func (s *Service) GetLimit(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (*int, error) {
	ent, err := s.entitlement(ctx, businessID, feature)
	if err != nil {
		return nil, err
	}
	return ent.LimitValue, nil
}

func (s *Service) GetRemaining(ctx context.Context, businessID uuid.UUID, feature FeatureKey) (*int64, error) {
	snap, err := s.currentUsage(ctx, businessID, feature)
	if err != nil {
		return nil, err
	}
	return snap.Remaining, nil
}

func (s *Service) GetUsage(ctx context.Context, businessID uuid.UUID) (Snapshot, error) {
	return s.snapshot(ctx, businessID)
}
