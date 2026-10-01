package area

import (
	"strings"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const maxNameLen = 100

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("name is required")
	}
	if len([]rune(name)) > maxNameLen {
		return "", invalid("name must be at most 100 characters")
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

func validateCapacity(capacity int) error {
	if capacity < 1 {
		return invalid("capacity must be at least 1")
	}
	return nil
}
