package tools

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type SearchCustomer struct {
	Customers CustomerLookup
}

func (t SearchCustomer) Name() string { return "search_customer" }

func (t SearchCustomer) Description() string {
	return "Search customers in the current business by name, phone, or email. Results are limited to this business."
}

func (t SearchCustomer) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"query": stringSchema("Name, phone, or email to search", ""),
	}, []string{"query"})
}

func (t SearchCustomer) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	query := strings.TrimSpace(stringArg(input, "query"))
	if query == "" {
		return fail("AI_TOOL_INVALID_ARGUMENTS", "query is required")
	}
	items, err := t.Customers.ListCustomers(ctx, exec.BusinessID, "ACTIVE", query, "10", "0")
	if err != nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "customer search failed")
	}
	type customerView struct {
		CustomerID string   `json:"customer_id"`
		Name       string   `json:"name"`
		Phone      *string  `json:"phone"`
		Email      *string  `json:"email"`
		Tags       []string `json:"tags"`
	}
	out := make([]customerView, 0, len(items))
	for _, item := range items {
		tags := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			tags = append(tags, tag.Name)
		}
		out = append(out, customerView{
			CustomerID: item.ID.String(),
			Name:       item.Name,
			Phone:      item.Phone,
			Email:      item.Email,
			Tags:       tags,
		})
	}
	return success(map[string]any{"customers": out})
}

type CustomerReservations struct {
	Reservations ReservationAPI
}

func (t CustomerReservations) Name() string { return "get_customer_reservations" }

func (t CustomerReservations) Description() string {
	return "Retrieve reservations for the current conversation customer only. Never request another customer's bookings."
}

func (t CustomerReservations) InputSchema() Schema {
	return objectSchema(map[string]Schema{
		"status": stringSchema("Optional reservation status filter (PENDING, CONFIRMED, CANCELLED, COMPLETED)", ""),
	}, nil)
}

func (t CustomerReservations) Execute(ctx context.Context, exec ExecContext, input map[string]any) (Result, error) {
	if exec.CustomerID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "customer context is missing")
	}
	status := strings.ToUpper(strings.TrimSpace(stringArg(input, "status")))
	items, err := t.Reservations.List(ctx, exec.BusinessID, status, exec.CustomerID.String(), "", "", "", "", "20", "0")
	if err != nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "reservations are unavailable")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.CustomerID != exec.CustomerID {
			continue
		}
		out = append(out, reservationView(item))
	}
	return success(map[string]any{"reservations": out})
}
