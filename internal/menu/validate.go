package menu

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	maxCategoryNameLen = 100
	maxItemNameLen     = 150
)

func normalizeCategoryName(name string) (string, error) {
	return normalizeName(name, maxCategoryNameLen)
}

func normalizeItemName(name string) (string, error) {
	return normalizeName(name, maxItemNameLen)
}

func normalizeName(name string, max int) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("name is required")
	}
	if len([]rune(name)) > max {
		return "", invalid("name must be at most " + strconv.Itoa(max) + " characters")
	}
	return name, nil
}

func normalizeOptionalDescription(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseStatus(value string) (tenant.Status, error) {
	switch tenant.Status(strings.TrimSpace(value)) {
	case tenant.StatusActive:
		return tenant.StatusActive, nil
	case tenant.StatusInactive:
		return tenant.StatusInactive, nil
	default:
		return "", invalid("status is invalid")
	}
}

func parseStatusFilter(raw string) (tenant.Status, error) {
	if strings.TrimSpace(raw) == "" {
		return tenant.StatusActive, nil
	}
	return parseStatus(raw)
}

func parseSortOrder(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, ErrInvalidSortOrder
	}
	return n, nil
}

func validateSortOrder(n int) error {
	if n < 0 {
		return ErrInvalidSortOrder
	}
	return nil
}

func parsePrice(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidPrice
	}
	if strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return "", ErrInvalidPrice
	}

	whole, frac, hasDot := strings.Cut(raw, ".")
	if whole == "" || !digitsOnly(whole) || len(whole) > 13 {
		return "", ErrInvalidPrice
	}
	if hasDot {
		if frac == "" || !digitsOnly(frac) || len(frac) > 2 {
			return "", ErrInvalidPrice
		}
		if len(frac) == 1 {
			frac += "0"
		}
		return whole + "." + frac, nil
	}
	return whole + ".00", nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func priceJSON(canonical string) string {
	if strings.HasSuffix(canonical, ".00") {
		return strings.TrimSuffix(canonical, ".00")
	}
	return canonical
}
