// Package area owns the seating layout of an F&B tenant: areas and tables.
package area

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Area is a seating zone inside a business, such as Indoor or Outdoor.
type Area struct {
	ID               uuid.UUID
	BusinessID       uuid.UUID
	Name             string
	Description      *string
	Status           tenant.Status
	TableCount       int
	ActiveTableCount int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Table is a seat group that belongs to one area.
type Table struct {
	ID        uuid.UUID
	AreaID    uuid.UUID
	Name      string
	Capacity  int
	Status    tenant.Status
	CreatedAt time.Time
	UpdatedAt time.Time
}
