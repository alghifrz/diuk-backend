package conversation

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultListLimit = 50
	maxListLimit     = 100
	maxExternalIDLen = 255
)

func parseChannelFilter(value string) (Channel, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return parseChannel(value)
}

func parseUnread(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "false", "0":
		return false, nil
	case "true", "1":
		return true, nil
	default:
		return false, invalid("unread is invalid")
	}
}

func parseChannel(value string) (Channel, error) {
	switch Channel(strings.ToUpper(strings.TrimSpace(value))) {
	case ChannelWhatsApp, ChannelInstagram, ChannelWebsite:
		return Channel(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidChannel
	}
}

func parseConversationStatus(value string) (Status, error) {
	switch Status(strings.ToUpper(strings.TrimSpace(value))) {
	case StatusOpen, StatusClosed:
		return Status(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidConversationStatus
	}
}

func parseStatusFilter(raw string) (Status, error) {
	if strings.TrimSpace(raw) == "" {
		return StatusOpen, nil
	}
	return parseConversationStatus(raw)
}

func parseDirection(value string) (Direction, error) {
	switch Direction(strings.ToUpper(strings.TrimSpace(value))) {
	case DirectionInbound, DirectionOutbound:
		return Direction(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidDirection
	}
}

func parseSenderType(value string) (SenderType, error) {
	switch SenderType(strings.ToUpper(strings.TrimSpace(value))) {
	case SenderCustomer, SenderUser, SenderSystem, SenderAI, SenderCampaign:
		return SenderType(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidSenderType
	}
}

func parseMessageType(value string) (MessageType, error) {
	if strings.TrimSpace(value) == "" {
		return MessageTypeText, nil
	}
	switch MessageType(strings.ToUpper(strings.TrimSpace(value))) {
	case MessageTypeText, MessageTypeImage, MessageTypeVideo, MessageTypeAudio, MessageTypeFile, MessageTypeLocation, MessageTypeSticker, MessageTypeSystem, MessageTypeInternalNote:
		return MessageType(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidMessageType
	}
}

func parseMessageStatus(value string) (MessageStatus, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	switch MessageStatus(strings.ToUpper(strings.TrimSpace(value))) {
	case MessageReceived, MessageQueued, MessageSending, MessageSent, MessageDelivered, MessageRead, MessageFailed, MessageInternal:
		return MessageStatus(strings.ToUpper(strings.TrimSpace(value))), nil
	default:
		return "", ErrInvalidMessageStatus
	}
}

func validateDirectionSender(dir Direction, sender SenderType) error {
	switch sender {
	case SenderCustomer:
		if dir != DirectionInbound {
			return ErrInvalidSenderType
		}
	case SenderUser, SenderAI, SenderCampaign:
		if dir != DirectionOutbound {
			return ErrInvalidSenderType
		}
	}
	return nil
}

func normalizeContent(value *string, messageType MessageType) (*string, error) {
	if value == nil {
		if messageType == MessageTypeText || messageType == MessageTypeInternalNote {
			return nil, invalid("content is required")
		}
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		if messageType == MessageTypeText || messageType == MessageTypeInternalNote {
			return nil, invalid("content is required")
		}
		return nil, nil
	}
	return &trimmed, nil
}

func normalizeExternalID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > maxExternalIDLen {
		return nil, invalid("external_id must be at most 255 characters")
	}
	return &trimmed, nil
}

func normalizeMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, invalid("metadata is invalid")
	}
	return json.RawMessage(raw), nil
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

func parseOptionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, invalid("timestamp is invalid")
	}
	return &t, nil
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, invalid("id is invalid")
	}
	return &id, nil
}

func likePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(raw) + "%"
}

func defaultMessageStatus(dir Direction) MessageStatus {
	if dir == DirectionInbound {
		return MessageReceived
	}
	return MessageQueued
}
