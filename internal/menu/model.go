// Package menu owns the tenant menu catalog: categories, items, and image paths.
package menu

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Category is a menu grouping such as Coffee or Food.
type Category struct {
	ID              uuid.UUID
	BusinessID      uuid.UUID
	Name            string
	Description     *string
	Status          tenant.Status
	SortOrder       int
	ItemCount       int
	ActiveItemCount int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Item is a single menu product. ImagePath is a storage object key, never a URL.
type Item struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	CategoryID    uuid.UUID
	CategoryName  string
	Name          string
	Description   *string
	Price         string
	ImagePath     *string
	ImageURL      *string
	ImageThumbURL *string
	Status        tenant.Status
	SortOrder     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
