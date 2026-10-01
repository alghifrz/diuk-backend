// Package conversation owns provider-agnostic support threads and messages.
package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelWhatsApp  Channel = "WHATSAPP"
	ChannelInstagram Channel = "INSTAGRAM"
	ChannelWebsite   Channel = "WEBSITE"
)

type Status string

type HandlingState string

type HandoverReason string

const (
	StatusOpen   Status = "OPEN"
	StatusClosed Status = "CLOSED"

	HandlingAIActive       HandlingState = "AI_ACTIVE"
	HandlingHumanRequested HandlingState = "HUMAN_REQUESTED"
	HandlingHumanActive    HandlingState = "HUMAN_ACTIVE"
	HandlingResolved       HandlingState = "RESOLVED"

	ReasonCustomerRequested HandoverReason = "CUSTOMER_REQUESTED_HUMAN"
	ReasonAIUnableToAnswer  HandoverReason = "AI_UNABLE_TO_ANSWER"
	ReasonManualHandoff     HandoverReason = "MANUAL_HANDOFF"
)

type Direction string

const (
	DirectionInbound  Direction = "INBOUND"
	DirectionOutbound Direction = "OUTBOUND"
)

type SenderType string

const (
	SenderCustomer SenderType = "CUSTOMER"
	SenderUser     SenderType = "USER"
	SenderSystem   SenderType = "SYSTEM"
	SenderAI       SenderType = "AI"
	SenderCampaign SenderType = "CAMPAIGN"
)

type MessageType string

const (
	MessageTypeText         MessageType = "TEXT"
	MessageTypeImage        MessageType = "IMAGE"
	MessageTypeVideo        MessageType = "VIDEO"
	MessageTypeAudio        MessageType = "AUDIO"
	MessageTypeFile         MessageType = "FILE"
	MessageTypeLocation     MessageType = "LOCATION"
	MessageTypeSticker      MessageType = "STICKER"
	MessageTypeSystem       MessageType = "SYSTEM"
	MessageTypeInternalNote MessageType = "INTERNAL_NOTE"
)

type MessageStatus string

const (
	MessageReceived  MessageStatus = "RECEIVED"
	MessageQueued    MessageStatus = "QUEUED"
	MessageSending   MessageStatus = "SENDING"
	MessageSent      MessageStatus = "SENT"
	MessageDelivered MessageStatus = "DELIVERED"
	MessageRead      MessageStatus = "READ"
	MessageFailed    MessageStatus = "FAILED"
	MessageInternal  MessageStatus = "INTERNAL"
)

// Conversation is one customer thread on a single channel.
type Conversation struct {
	ID                  uuid.UUID
	BusinessID          uuid.UUID
	CustomerID          uuid.UUID
	Channel             Channel
	CustomerIdentityID  *uuid.UUID
	Status              Status
	AssignedUserID      *uuid.UUID
	LastMessageAt       *time.Time
	UnreadCount         int
	HandlingState       HandlingState
	HandoverReason      *HandoverReason
	HandoverRequestedAt *time.Time
	HumanStartedAt      *time.Time
	ResolvedAt          *time.Time
	Customer            CustomerSummary
	Identity            *IdentitySummary
	AssignedUser        *UserSummary
	LastMessage         *MessagePreview
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type HumanInboxItem struct {
	Conversation
}

type MessagePreview struct {
	Content    *string
	CreatedAt  time.Time
	SenderType SenderType
	Direction  Direction
	Status     MessageStatus
}

// Message is one inbound or outbound item in a conversation.
type Message struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	ConversationID uuid.UUID
	Direction      Direction
	SenderType     SenderType
	MessageType    MessageType
	Content        *string
	ExternalID     *string
	Status         MessageStatus
	Metadata       json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CustomerSummary struct {
	ID    uuid.UUID
	Name  string
	Phone *string
	Email *string
}

type IdentitySummary struct {
	ID         uuid.UUID
	Channel    Channel
	ExternalID string
}

type UserSummary struct {
	ID       uuid.UUID
	FullName string
}

type ListParams struct {
	Status         Status
	Channel        Channel
	AssignedUserID *uuid.UUID
	CustomerID     *uuid.UUID
	Search         string
	UnreadOnly     bool
	Limit          int
	Offset         int
}

type MessageListParams struct {
	Limit  int
	Offset int
	Before *time.Time
	After  *time.Time
}

type CustomerRecord struct {
	ID     uuid.UUID
	Status string
	Name   string
}

type IdentityRecord struct {
	ID         uuid.UUID
	CustomerID uuid.UUID
	Channel    Channel
	Status     string
}
