package channel

import (
	"strings"
	"unicode"
)

func parseChannel(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case ChannelWhatsApp, ChannelInstagram:
		return strings.ToUpper(strings.TrimSpace(value)), nil
	default:
		return "", invalid("channel is invalid")
	}
}

func parseChannelStatus(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return StatusActive, nil
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case StatusActive, StatusInactive:
		return strings.ToUpper(strings.TrimSpace(value)), nil
	default:
		return "", invalid("status is invalid")
	}
}

func normalizeOptionalName(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	if len([]rune(trimmed)) > 100 {
		trimmed = string([]rune(trimmed)[:100])
	}
	return &trimmed
}

func normalizeExternalRef(value *string, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > 128 {
		return nil, invalid(field + " must be at most 128 characters")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return nil, invalid(field + " is invalid")
		}
	}
	return &trimmed, nil
}

func requirePhoneNumberID(channel string, phone *string) error {
	if channel == ChannelWhatsApp && (phone == nil || strings.TrimSpace(*phone) == "") {
		return invalid("external_phone_number_id is required")
	}
	return nil
}
