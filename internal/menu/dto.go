package menu

import (
	"encoding/json"
	"time"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type CreateCategoryInput struct {
	Name        string
	Description *string
	SortOrder   int
}

type UpdateCategoryInput struct {
	Name        *string
	Description *string
	Status      *tenant.Status
	SortOrder   *int
}

type CreateItemInput struct {
	CategoryID  string
	Name        string
	Description *string
	Price       string
	SortOrder   int
	Image       *validatedImage
}

type UpdateItemInput struct {
	Name        *string
	Description *string
	Price       *string
	Status      *tenant.Status
	SortOrder   *int
	Image       *validatedImage
	RemoveImage bool
}

type categoryJSON struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sort_order"`
	ItemCount       int     `json:"item_count"`
	ActiveItemCount int     `json:"active_item_count"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type itemJSON struct {
	ID            string      `json:"id"`
	CategoryID    string      `json:"category_id"`
	CategoryName  string      `json:"category_name"`
	Name          string      `json:"name"`
	Description   *string     `json:"description"`
	Price         json.Number `json:"price"`
	ImagePath     *string     `json:"image_path"`
	ImageURL      *string     `json:"image_url"`
	ImageThumbURL *string     `json:"image_thumb_url"`
	Status        string      `json:"status"`
	SortOrder     int         `json:"sort_order"`
	CreatedAt     string      `json:"created_at"`
	UpdatedAt     string      `json:"updated_at"`
}

type createCategoryRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
}

type updateCategoryRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	SortOrder   *int    `json:"sort_order"`
}

func categoryResponse(c *Category) categoryJSON {
	return categoryJSON{
		ID:              c.ID.String(),
		Name:            c.Name,
		Description:     c.Description,
		Status:          string(c.Status),
		SortOrder:       c.SortOrder,
		ItemCount:       c.ItemCount,
		ActiveItemCount: c.ActiveItemCount,
		CreatedAt:       c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func itemResponse(item *Item) itemJSON {
	return itemJSON{
		ID:            item.ID.String(),
		CategoryID:    item.CategoryID.String(),
		CategoryName:  item.CategoryName,
		Name:          item.Name,
		Description:   item.Description,
		Price:         json.Number(priceJSON(item.Price)),
		ImagePath:     item.ImagePath,
		ImageURL:      item.ImageURL,
		ImageThumbURL: item.ImageThumbURL,
		Status:        string(item.Status),
		SortOrder:     item.SortOrder,
		CreatedAt:     item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func categoriesResponse(categories []Category) []categoryJSON {
	out := make([]categoryJSON, 0, len(categories))
	for i := range categories {
		out = append(out, categoryResponse(&categories[i]))
	}
	return out
}

func itemsResponse(items []Item) []itemJSON {
	out := make([]itemJSON, 0, len(items))
	for i := range items {
		out = append(out, itemResponse(&items[i]))
	}
	return out
}
