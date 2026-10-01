package whatsapp

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	channelWhatsApp        = "WHATSAPP"
	messageTypeText        = "TEXT"
	messageTypeUnsupported = "UNSUPPORTED"
)

// Incoming is a provider-local inbound event. The channel package maps this
// onto its own IncomingMessage so WhatsApp never imports channel core.
type Incoming struct {
	Channel            string
	ExternalAccountID  string
	ExternalIdentityID string
	ExternalMessageID  string
	CustomerName       string
	CustomerPhone      string
	MessageType        string
	Text               string
	ReceivedAt         time.Time
}

// DeliveryStatus is a provider-local status event.
type DeliveryStatus struct {
	Channel           string
	ExternalAccountID string
	ExternalMessageID string
	Status            string
}

func Normalize(raw []byte) ([]Incoming, []DeliveryStatus, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, ErrInvalidPayload
	}
	if strings.TrimSpace(payload.Object) != "" && payload.Object != "whatsapp_business_account" {
		return nil, nil, ErrInvalidPayload
	}

	incoming := make([]Incoming, 0)
	statuses := make([]DeliveryStatus, 0)
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			phoneID := strings.TrimSpace(change.Value.Metadata.PhoneNumberID)
			if phoneID == "" {
				return nil, nil, ErrInvalidPayload
			}
			names := map[string]string{}
			for _, contact := range change.Value.Contacts {
				names[strings.TrimSpace(contact.WaID)] = strings.TrimSpace(contact.Profile.Name)
			}
			for _, msg := range change.Value.Messages {
				item, err := normalizeMessage(phoneID, names, msg)
				if err != nil {
					return nil, nil, err
				}
				incoming = append(incoming, item)
			}
			for _, status := range change.Value.Statuses {
				if strings.TrimSpace(status.ID) == "" {
					continue
				}
				statuses = append(statuses, DeliveryStatus{
					Channel:           channelWhatsApp,
					ExternalAccountID: phoneID,
					ExternalMessageID: strings.TrimSpace(status.ID),
					Status:            mapStatus(status.Status),
				})
			}
		}
	}
	return incoming, statuses, nil
}

func normalizeMessage(phoneID string, names map[string]string, msg Message) (Incoming, error) {
	from := strings.TrimSpace(msg.From)
	id := strings.TrimSpace(msg.ID)
	if from == "" || id == "" {
		return Incoming{}, ErrInvalidPayload
	}
	item := Incoming{
		Channel:            channelWhatsApp,
		ExternalAccountID:  phoneID,
		ExternalIdentityID: from,
		ExternalMessageID:  id,
		CustomerName:       names[from],
		CustomerPhone:      from,
		ReceivedAt:         parseUnix(msg.Timestamp),
	}
	if strings.EqualFold(strings.TrimSpace(msg.Type), "text") && msg.Text != nil {
		item.MessageType = messageTypeText
		item.Text = strings.TrimSpace(msg.Text.Body)
		return item, nil
	}
	item.MessageType = messageTypeUnsupported
	return item, nil
}

func parseUnix(raw string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(n, 0).UTC()
}

func mapStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sent":
		return "SENT"
	case "delivered":
		return "DELIVERED"
	case "read":
		return "READ"
	case "failed":
		return "FAILED"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}
