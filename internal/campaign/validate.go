package campaign

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func parseAudience(raw json.RawMessage) (AudienceDefinition, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return AudienceDefinition{}, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var out AudienceDefinition
	if err := dec.Decode(&out); err != nil {
		return AudienceDefinition{}, invalid("audience_definition contains unsupported fields")
	}
	if out.CustomerStatus != nil {
		status := strings.ToUpper(strings.TrimSpace(*out.CustomerStatus))
		if status != "ACTIVE" && status != "INACTIVE" {
			return AudienceDefinition{}, invalid("customer_status is invalid")
		}
		out.CustomerStatus = &status
	}
	channels := make([]string, 0, len(out.Channels))
	for _, ch := range out.Channels {
		ch = strings.ToUpper(strings.TrimSpace(ch))
		if ch != ChannelWhatsApp && ch != "INSTAGRAM" && ch != "WEBSITE" {
			return AudienceDefinition{}, invalid("channels contains an invalid value")
		}
		channels = append(channels, ch)
	}
	out.Channels = channels
	tags := make([]string, 0, len(out.Tags))
	for _, tag := range out.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return AudienceDefinition{}, invalid("tags contains an empty value")
		}
		if len([]rune(tag)) > 80 {
			return AudienceDefinition{}, invalid("tag is too long")
		}
		tags = append(tags, tag)
	}
	out.Tags = tags
	for _, field := range []*string{
		out.CreatedAfter, out.CreatedBefore,
		out.LastConversationAfter, out.LastConversationBefore,
		out.LastReservationAfter, out.LastReservationBefore,
	} {
		if field == nil {
			continue
		}
		if _, err := parseAudienceTime(*field); err != nil {
			return AudienceDefinition{}, err
		}
	}
	if out.LastReservationStatus != nil {
		status := strings.ToUpper(strings.TrimSpace(*out.LastReservationStatus))
		switch status {
		case "PENDING", "CONFIRMED", "CANCELLED", "COMPLETED":
			out.LastReservationStatus = &status
		default:
			return AudienceDefinition{}, invalid("last_reservation_status is invalid")
		}
	}
	return out, nil
}

func parseAudienceTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, invalid("audience date must be an ISO date or RFC3339 timestamp")
}

func optionalAudienceTime(raw *string) *time.Time {
	if raw == nil {
		return nil
	}
	t, err := parseAudienceTime(*raw)
	if err != nil {
		return nil
	}
	return &t
}

func parseName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", invalid("name is required")
	}
	if len([]rune(name)) > maxNameLen {
		return "", invalid("name must be at most 150 characters")
	}
	return name, nil
}

func parseOptionalText(raw *string, max int, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if max > 0 && len([]rune(trimmed)) > max {
		return nil, invalid(field + " is too long")
	}
	return &trimmed, nil
}

func parseChannel(raw string) (string, error) {
	ch := strings.ToUpper(strings.TrimSpace(raw))
	if ch == "" {
		return ChannelWhatsApp, nil
	}
	if ch != ChannelWhatsApp {
		return "", invalid("only WHATSAPP campaigns are supported")
	}
	return ch, nil
}

func parseKind(raw string) (string, error) {
	kind := strings.ToUpper(strings.TrimSpace(raw))
	if kind == "" {
		return KindTemplate, nil
	}
	if kind != KindText && kind != KindTemplate {
		return "", invalid("message_kind is invalid")
	}
	return kind, nil
}

func parseScheduledAt(raw *string, now time.Time) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, invalid("scheduled_at must be RFC3339")
	}
	t = t.UTC()
	if t.Before(now.Add(-time.Minute)) {
		return nil, invalid("scheduled_at must not be in the past")
	}
	return &t, nil
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

func parsePathUUID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid("id is invalid")
	}
	return id, nil
}

func canTransition(from, to string) bool {
	switch from {
	case StatusDraft:
		return to == StatusScheduled || to == StatusCancelled || to == StatusRunning
	case StatusScheduled:
		return to == StatusRunning || to == StatusPaused || to == StatusCancelled
	case StatusRunning:
		return to == StatusPaused || to == StatusCompleted || to == StatusCancelled
	case StatusPaused:
		return to == StatusRunning || to == StatusCancelled
	default:
		return false
	}
}

func isTerminalCampaign(status string) bool {
	return status == StatusCompleted || status == StatusCancelled
}

func isTerminalRecipient(status string) bool {
	switch status {
	case RecipientSent, RecipientDelivered, RecipientRead, RecipientFailed, RecipientSkipped:
		return true
	default:
		return false
	}
}

func recipientFromMessage(status string) string {
	switch status {
	case "SENT":
		return RecipientSent
	case "DELIVERED":
		return RecipientDelivered
	case "READ":
		return RecipientRead
	case "FAILED":
		return RecipientFailed
	default:
		return ""
	}
}
