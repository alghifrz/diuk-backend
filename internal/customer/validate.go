package customer

import (
	"net/mail"
	"strconv"
	"strings"
	"unicode"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	maxNameLen        = 150
	maxPhoneLen       = 30
	maxEmailLen       = 254
	maxTagNameLen     = 50
	maxTagDescLen     = 255
	maxExternalIDLen  = 255
	maxDisplayNameLen = 150
	defaultListLimit  = 50
	maxListLimit      = 100
)

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

func normalizeOptionalText(value *string, max int, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if max > 0 && len([]rune(trimmed)) > max {
		return nil, invalid(field + " must be at most " + strconv.Itoa(max) + " characters")
	}
	return &trimmed, nil
}

func normalizeOptionalEmail(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.ToLower(strings.TrimSpace(*value))
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > maxEmailLen {
		return nil, invalid("email must be at most 254 characters")
	}
	if _, err := mail.ParseAddress(trimmed); err != nil || !strings.Contains(trimmed, "@") {
		return nil, invalid("email is invalid")
	}
	return &trimmed, nil
}

func normalizeOptionalPhone(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > maxPhoneLen {
		return nil, invalid("phone must be at most 30 characters")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return nil, invalid("phone is invalid")
		}
	}
	return &trimmed, nil
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

func parseChannel(value string) (Channel, error) {
	switch Channel(strings.ToUpper(strings.TrimSpace(value))) {
	case ChannelWhatsApp:
		return ChannelWhatsApp, nil
	case ChannelInstagram:
		return ChannelInstagram, nil
	case ChannelWebsite:
		return ChannelWebsite, nil
	default:
		return "", ErrInvalidChannel
	}
}

func normalizeExternalID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid("external_id is required")
	}
	if len([]rune(value)) > maxExternalIDLen {
		return "", invalid("external_id must be at most 255 characters")
	}
	return value, nil
}

func parseSort(raw string) (Sort, error) {
	switch Sort(strings.ToLower(strings.TrimSpace(raw))) {
	case "", SortNewest:
		return SortNewest, nil
	case SortName:
		return SortName, nil
	case SortSpend:
		return SortSpend, nil
	case SortVisits:
		return SortVisits, nil
	case SortLastVisit:
		return SortLastVisit, nil
	default:
		return "", invalid("sort is invalid")
	}
}

func parseLimitOffset(limitRaw, offsetRaw string) (int, int, error) {
	limit := defaultListLimit
	if strings.TrimSpace(limitRaw) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(limitRaw))
		if err != nil || n < 1 {
			return 0, 0, invalid("limit is invalid")
		}
		if n > maxListLimit {
			n = maxListLimit
		}
		limit = n
	}
	offset := 0
	if strings.TrimSpace(offsetRaw) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(offsetRaw))
		if err != nil || n < 0 {
			return 0, 0, invalid("offset is invalid")
		}
		offset = n
	}
	return limit, offset, nil
}

func likePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(raw) + "%"
}
