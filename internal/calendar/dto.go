package calendar

import "time"

type eventJSON struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	ReservationID *string `json:"reservation_id,omitempty"`
	Title         string  `json:"title"`
	StartAt       string  `json:"start_at"`
	EndAt         string  `json:"end_at"`
	Status        string  `json:"status"`
	Scope         string  `json:"scope,omitempty"`
	Description   *string `json:"description,omitempty"`
	CustomerID    *string `json:"customer_id,omitempty"`
	CustomerName  string  `json:"customer,omitempty"`
	PartySize     *int    `json:"party_size,omitempty"`
	TableID       *string `json:"table_id,omitempty"`
	TableName     string  `json:"table,omitempty"`
	AreaID        *string `json:"area_id,omitempty"`
	AreaName      string  `json:"area,omitempty"`
	PaymentStatus *string `json:"payment_status,omitempty"`
}

type blockJSON struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
	Status      string  `json:"status"`
	Scope       string  `json:"scope"`
	AreaID      *string `json:"area_id"`
	Area        string  `json:"area,omitempty"`
	TableID     *string `json:"table_id"`
	Table       string  `json:"table,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type dayJSON struct {
	Date      string         `json:"date"`
	Timezone  string         `json:"timezone"`
	Closed    bool           `json:"closed"`
	Hours     []hourJSON     `json:"hours"`
	Exception *exceptionJSON `json:"exception"`
	Events    []eventJSON    `json:"events"`
}

type hourJSON struct {
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

type exceptionJSON struct {
	ID        string  `json:"id"`
	IsClosed  bool    `json:"is_closed"`
	OpenTime  *string `json:"open_time"`
	CloseTime *string `json:"close_time"`
	Note      *string `json:"note"`
}

func eventResponse(e Event) eventJSON {
	out := eventJSON{
		ID: e.ID.String(), Type: e.Type, Title: e.Title,
		StartAt: e.StartAt.UTC().Format(time.RFC3339), EndAt: e.EndAt.UTC().Format(time.RFC3339),
		Status: e.Status, Scope: e.Scope, Description: e.Description,
		CustomerName: e.CustomerName, PartySize: e.PartySize, TableName: e.TableName,
		AreaName: e.AreaName, PaymentStatus: e.PaymentStatus,
	}
	if e.ReservationID != nil {
		s := e.ReservationID.String()
		out.ReservationID = &s
	}
	if e.CustomerID != nil {
		s := e.CustomerID.String()
		out.CustomerID = &s
	}
	if e.TableID != nil {
		s := e.TableID.String()
		out.TableID = &s
	}
	if e.AreaID != nil {
		s := e.AreaID.String()
		out.AreaID = &s
	}
	return out
}

func eventsResponse(items []Event) []eventJSON {
	out := make([]eventJSON, 0, len(items))
	for _, e := range items {
		out = append(out, eventResponse(e))
	}
	return out
}

func blockResponse(b *Block) blockJSON {
	out := blockJSON{
		ID: b.ID.String(), Type: b.EventType(), Title: b.Title, Description: b.Description,
		StartAt: b.StartAt.UTC().Format(time.RFC3339), EndAt: b.EndAt.UTC().Format(time.RFC3339),
		Status: b.Status, Scope: b.Scope(), Area: b.AreaName, Table: b.TableName,
		CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: b.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if b.AreaID != nil {
		s := b.AreaID.String()
		out.AreaID = &s
	}
	if b.TableID != nil {
		s := b.TableID.String()
		out.TableID = &s
	}
	return out
}

func dayResponse(d *DayView) dayJSON {
	hours := make([]hourJSON, 0, len(d.Hours))
	for _, h := range d.Hours {
		hours = append(hours, hourJSON{OpenTime: h.OpenTime, CloseTime: h.CloseTime})
	}
	out := dayJSON{
		Date: d.Date, Timezone: d.Timezone, Closed: d.Closed, Hours: hours, Events: eventsResponse(d.Events),
	}
	if d.Exception != nil {
		out.Exception = &exceptionJSON{
			ID: d.Exception.ID.String(), IsClosed: d.Exception.IsClosed,
			OpenTime: d.Exception.OpenTime, CloseTime: d.Exception.CloseTime, Note: d.Exception.Note,
		}
	}
	return out
}
