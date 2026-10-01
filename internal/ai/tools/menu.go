package tools

import (
	"context"
)

type Menu struct {
	Menu MenuLookup
}

func (t Menu) Name() string { return "get_menu" }

func (t Menu) Description() string {
	return "Retrieve the active menu categories and items, including menu_item_id, names, descriptions, prices, and availability. Required during booking: call this right after check_availability, before quote_reservation. Offer real items so the guest can pre-order. Never invent items, prices, or IDs."
}

func (t Menu) InputSchema() Schema {
	return objectSchema(map[string]Schema{}, nil)
}

func (t Menu) Execute(ctx context.Context, exec ExecContext, _ map[string]any) (Result, error) {
	items, err := t.Menu.ListItems(ctx, exec.BusinessID, "ACTIVE", nil)
	if err != nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "menu is unavailable")
	}
	categories, err := t.Menu.ListCategories(ctx, exec.BusinessID, "ACTIVE")
	if err != nil {
		return fail("AI_TOOL_EXECUTION_FAILED", "menu is unavailable")
	}

	type itemView struct {
		MenuItemID  string  `json:"menu_item_id"`
		Category    string  `json:"category"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Price       string  `json:"price"`
		Status      string  `json:"status"`
	}
	out := make([]itemView, 0, len(items))
	for _, item := range items {
		out = append(out, itemView{
			MenuItemID:  item.ID.String(),
			Category:    item.CategoryName,
			Name:        item.Name,
			Description: item.Description,
			Price:       item.Price,
			Status:      string(item.Status),
		})
	}
	categoryNames := make([]string, 0, len(categories))
	for _, category := range categories {
		categoryNames = append(categoryNames, category.Name)
	}
	return success(map[string]any{
		"categories": categoryNames,
		"items":      out,
	})
}
