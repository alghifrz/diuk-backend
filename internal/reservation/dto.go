package reservation

import (
	"time"
)

type createReservationRequest struct {
	CustomerID  string                 `json:"customer_id"`
	StartAt     string                 `json:"start_at"`
	EndAt       string                 `json:"end_at"`
	PartySize   int                    `json:"party_size"`
	TableID     *string                `json:"table_id"`
	Notes       *string                `json:"notes"`
	Items       []ReservationItemInput `json:"items"`
	PaymentType string                 `json:"payment_type"`
}

type updateReservationRequest struct {
	CustomerID *string `json:"customer_id"`
	StartAt    *string `json:"start_at"`
	EndAt      *string `json:"end_at"`
	PartySize  *int    `json:"party_size"`
	TableID    *string `json:"table_id"`
	Notes      *string `json:"notes"`
}

type cancelRequest struct {
	Reason *string `json:"reason"`
}

type replaceHoursRequest struct {
	Hours []hourJSON `json:"hours"`
}

type hourJSON struct {
	ID        string `json:"id,omitempty"`
	DayOfWeek int    `json:"day_of_week"`
	SortOrder int    `json:"sort_order"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

type exceptionRequest struct {
	ExceptionDate string  `json:"exception_date"`
	IsClosed      *bool   `json:"is_closed"`
	OpenTime      *string `json:"open_time"`
	CloseTime     *string `json:"close_time"`
	Note          *string `json:"note"`
}

type reservationJSON struct {
	ID                 string                `json:"id"`
	CustomerID         string                `json:"customer_id"`
	TableID            *string               `json:"table_id"`
	PartySize          int                   `json:"party_size"`
	StartAt            string                `json:"start_at"`
	EndAt              string                `json:"end_at"`
	Status             string                `json:"status"`
	Notes              *string               `json:"notes"`
	CancellationReason *string               `json:"cancellation_reason"`
	ConfirmedAt        *string               `json:"confirmed_at"`
	CompletedAt        *string               `json:"completed_at"`
	Customer           customerJSON          `json:"customer"`
	Table              *tableJSON            `json:"table"`
	Items              []reservationItemJSON `json:"items"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
}

type reservationItemJSON struct {
	ID           string  `json:"id"`
	MenuItemID   string  `json:"menu_item_id"`
	MenuItemName string  `json:"menu_item_name"`
	Quantity     int     `json:"quantity"`
	UnitPrice    string  `json:"unit_price"`
	Notes        *string `json:"notes"`
}

type customerJSON struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Phone *string `json:"phone"`
}

type tableJSON struct {
	TableID   string `json:"table_id"`
	TableName string `json:"table_name"`
	AreaID    string `json:"area_id"`
	AreaName  string `json:"area_name"`
	Capacity  int    `json:"capacity"`
}

type exceptionJSON struct {
	ID            string  `json:"id"`
	ExceptionDate string  `json:"exception_date"`
	IsClosed      bool    `json:"is_closed"`
	OpenTime      *string `json:"open_time"`
	CloseTime     *string `json:"close_time"`
	Note          *string `json:"note"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type availabilityJSON struct {
	Date      string      `json:"date"`
	StartAt   string      `json:"start_at"`
	EndAt     string      `json:"end_at"`
	PartySize int         `json:"party_size"`
	Available bool        `json:"available"`
	Tables    []tableJSON `json:"tables"`
}

func reservationResponse(r *Reservation) reservationJSON {
	out := reservationJSON{
		ID:                 r.ID.String(),
		CustomerID:         r.CustomerID.String(),
		PartySize:          r.PartySize,
		StartAt:            r.StartAt.UTC().Format(time.RFC3339),
		EndAt:              r.EndAt.UTC().Format(time.RFC3339),
		Status:             string(r.Status),
		Notes:              r.Notes,
		CancellationReason: r.CancellationReason,
		Customer: customerJSON{
			ID:    r.Customer.ID.String(),
			Name:  r.Customer.Name,
			Phone: r.Customer.Phone,
		},
		Items:     reservationItemsResponse(r.Items),
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.TableID != nil {
		s := r.TableID.String()
		out.TableID = &s
	}
	if r.ConfirmedAt != nil {
		s := r.ConfirmedAt.UTC().Format(time.RFC3339)
		out.ConfirmedAt = &s
	}
	if r.CompletedAt != nil {
		s := r.CompletedAt.UTC().Format(time.RFC3339)
		out.CompletedAt = &s
	}
	if r.Table != nil {
		out.Table = tableResponse(*r.Table)
	}
	return out
}

func reservationItemsResponse(items []ReservationItem) []reservationItemJSON {
	out := make([]reservationItemJSON, 0, len(items))
	for _, item := range items {
		out = append(out, reservationItemJSON{
			ID:           item.ID.String(),
			MenuItemID:   item.MenuItemID.String(),
			MenuItemName: item.MenuItemName,
			Quantity:     item.Quantity,
			UnitPrice:    item.UnitPrice,
			Notes:        item.Notes,
		})
	}
	return out
}

func tableResponse(t TableSummary) *tableJSON {
	return &tableJSON{
		TableID:   t.ID.String(),
		TableName: t.Name,
		AreaID:    t.AreaID.String(),
		AreaName:  t.AreaName,
		Capacity:  t.Capacity,
	}
}

func reservationsResponse(items []Reservation) []reservationJSON {
	out := make([]reservationJSON, 0, len(items))
	for i := range items {
		out = append(out, reservationResponse(&items[i]))
	}
	return out
}

func hoursResponse(items []Hour) []hourJSON {
	out := make([]hourJSON, 0, len(items))
	for _, h := range items {
		out = append(out, hourJSON{
			ID:        h.ID.String(),
			DayOfWeek: h.DayOfWeek,
			SortOrder: h.SortOrder,
			OpenTime:  h.OpenTime,
			CloseTime: h.CloseTime,
		})
	}
	return out
}

func exceptionResponse(e *ScheduleException) exceptionJSON {
	return exceptionJSON{
		ID:            e.ID.String(),
		ExceptionDate: e.ExceptionDate.Format("2006-01-02"),
		IsClosed:      e.IsClosed,
		OpenTime:      e.OpenTime,
		CloseTime:     e.CloseTime,
		Note:          e.Note,
		CreatedAt:     e.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func exceptionsResponse(items []ScheduleException) []exceptionJSON {
	out := make([]exceptionJSON, 0, len(items))
	for i := range items {
		out = append(out, exceptionResponse(&items[i]))
	}
	return out
}

func availabilityResponse(a *AvailabilityResult) availabilityJSON {
	tables := make([]tableJSON, 0, len(a.Tables))
	for _, t := range a.Tables {
		tables = append(tables, *tableResponse(t))
	}
	return availabilityJSON{
		Date:      a.Date,
		StartAt:   a.StartAt.Format(time.RFC3339),
		EndAt:     a.EndAt.Format(time.RFC3339),
		PartySize: a.PartySize,
		Available: a.Available,
		Tables:    tables,
	}
}
