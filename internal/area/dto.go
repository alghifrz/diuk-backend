package area

import (
	"time"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type CreateAreaInput struct {
	Name        string
	Description *string
}

type UpdateAreaInput struct {
	Name        *string
	Description *string
	Status      *tenant.Status
}

type CreateTableInput struct {
	Name     string
	Capacity int
}

type UpdateTableInput struct {
	Name     *string
	Capacity *int
	Status   *tenant.Status
}

type areaJSON struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	Status           string  `json:"status"`
	TableCount       int     `json:"table_count"`
	ActiveTableCount int     `json:"active_table_count"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type tableJSON struct {
	ID        string `json:"id"`
	AreaID    string `json:"area_id"`
	Name      string `json:"name"`
	Capacity  int    `json:"capacity"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type createAreaRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type updateAreaRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type createTableRequest struct {
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

type updateTableRequest struct {
	Name     *string `json:"name"`
	Capacity *int    `json:"capacity"`
	Status   *string `json:"status"`
}

func areaResponse(a *Area) areaJSON {
	return areaJSON{
		ID:               a.ID.String(),
		Name:             a.Name,
		Description:      a.Description,
		Status:           string(a.Status),
		TableCount:       a.TableCount,
		ActiveTableCount: a.ActiveTableCount,
		CreatedAt:        a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func tableResponse(t *Table) tableJSON {
	return tableJSON{
		ID:        t.ID.String(),
		AreaID:    t.AreaID.String(),
		Name:      t.Name,
		Capacity:  t.Capacity,
		Status:    string(t.Status),
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func areasResponse(areas []Area) []areaJSON {
	out := make([]areaJSON, 0, len(areas))
	for i := range areas {
		out = append(out, areaResponse(&areas[i]))
	}
	return out
}

func tablesResponse(tables []Table) []tableJSON {
	out := make([]tableJSON, 0, len(tables))
	for i := range tables {
		out = append(out, tableResponse(&tables[i]))
	}
	return out
}
