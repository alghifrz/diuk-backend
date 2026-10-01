// Package channel is the messaging integration layer. It maps provider events
// onto existing customer, conversation, and AI services.
package channel

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelWhatsApp  = "WHATSAPP"
	ChannelInstagram = "INSTAGRAM"

	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"

	MessageTypeText        = "TEXT"
	MessageTypeUnsupported = "UNSUPPORTED"
)

type BusinessChannel struct {
	ID                    uuid.UUID
	BusinessID            uuid.UUID
	Channel               string
	Status                string
	DisplayName           *string
	ExternalAccountID     *string
	ExternalPhoneNumberID *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type IncomingMessage struct {
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

type StatusUpdate struct {
	Channel           string
	ExternalAccountID string
	ExternalMessageID string
	Status            string
}

type CreateChannelInput struct {
	Channel               string
	DisplayName           *string
	ExternalAccountID     *string
	ExternalPhoneNumberID *string
	Status                string
}

type UpdateChannelInput struct {
	DisplayName           *string
	ExternalAccountID     *string
	ExternalPhoneNumberID *string
	Status                *string
}

type HandleResult struct {
	BusinessID     uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	Created        bool
	Duplicate      bool
	Unsupported    bool
}

type ProviderCredentials struct {
	AccessToken string
	BaseURL     string
	APIVersion  string
	Timeout     time.Duration
}
