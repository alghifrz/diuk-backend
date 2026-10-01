package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type CreateConversationInput struct {
	CustomerID         string
	CustomerIdentityID *string
	Channel            string
	SuppressAutomation bool
}

type UpdateConversationInput struct {
	Status            *Status
	AssignedUserID    *uuid.UUID
	ClearAssignedUser bool
}

type CreateMessageInput struct {
	Direction     string
	SenderType    string
	MessageType   string
	Content       *string
	ExternalID    *string
	Status        string
	Metadata      json.RawMessage
	AllowCampaign bool
}

type createConversationRequest struct {
	CustomerID         string  `json:"customer_id"`
	CustomerIdentityID *string `json:"customer_identity_id"`
	Channel            string  `json:"channel"`
}

type updateConversationRequest struct {
	Status         *string          `json:"status"`
	AssignedUserID *json.RawMessage `json:"assigned_user_id"`
}

type createMessageRequest struct {
	Direction   string          `json:"direction"`
	SenderType  string          `json:"sender_type"`
	MessageType string          `json:"message_type"`
	Content     *string         `json:"content"`
	ExternalID  *string         `json:"external_id"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
}

type conversationJSON struct {
	ID                  string            `json:"id"`
	CustomerID          string            `json:"customer_id"`
	Channel             string            `json:"channel"`
	CustomerIdentityID  *string           `json:"customer_identity_id"`
	Status              string            `json:"status"`
	AssignedUserID      *string           `json:"assigned_user_id"`
	LastMessageAt       *string           `json:"last_message_at"`
	UnreadCount         int               `json:"unread_count"`
	HandlingState       string            `json:"handling_state"`
	HandoverReason      *string           `json:"handover_reason"`
	HandoverRequestedAt *string           `json:"handover_requested_at"`
	HumanStartedAt      *string           `json:"human_started_at"`
	ResolvedAt          *string           `json:"resolved_at"`
	Customer            customerJSON      `json:"customer"`
	Identity            *identityJSON     `json:"identity"`
	AssignedUser        *assignedUserJSON `json:"assigned_user"`
	LastMessage         *inboxMessageJSON `json:"last_message,omitempty"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

type humanInboxJSON struct {
	conversationJSON
}

type inboxMessageJSON struct {
	Content    *string `json:"content"`
	CreatedAt  string  `json:"created_at"`
	SenderType string  `json:"sender_type"`
	Direction  string  `json:"direction"`
	Status     string  `json:"status"`
}

type handoverRequest struct {
	Reason string `json:"reason"`
}

type replyRequest struct {
	Content string `json:"content"`
}

type noteRequest struct {
	Content string `json:"content"`
}

type customerJSON struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Phone *string `json:"phone"`
	Email *string `json:"email,omitempty"`
}

type identityJSON struct {
	ID         string `json:"id"`
	Channel    string `json:"channel"`
	ExternalID string `json:"external_id"`
}

type assignedUserJSON struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
}

type messageJSON struct {
	ID          string          `json:"id"`
	Direction   string          `json:"direction"`
	SenderType  string          `json:"sender_type"`
	MessageType string          `json:"message_type"`
	Content     *string         `json:"content"`
	ExternalID  *string         `json:"external_id"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func conversationResponse(c *Conversation) conversationJSON {
	out := conversationJSON{
		ID:            c.ID.String(),
		CustomerID:    c.CustomerID.String(),
		Channel:       string(c.Channel),
		Status:        string(c.Status),
		UnreadCount:   c.UnreadCount,
		HandlingState: string(normalizeHandling(c.HandlingState)),
		Customer:      customerJSON{ID: c.Customer.ID.String(), Name: c.Customer.Name, Phone: c.Customer.Phone, Email: c.Customer.Email},
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if c.HandoverReason != nil {
		s := string(*c.HandoverReason)
		out.HandoverReason = &s
	}
	out.HandoverRequestedAt = formatTimePtr(c.HandoverRequestedAt)
	out.HumanStartedAt = formatTimePtr(c.HumanStartedAt)
	out.ResolvedAt = formatTimePtr(c.ResolvedAt)
	if c.CustomerIdentityID != nil {
		s := c.CustomerIdentityID.String()
		out.CustomerIdentityID = &s
	}
	if c.AssignedUserID != nil {
		s := c.AssignedUserID.String()
		out.AssignedUserID = &s
	}
	if c.LastMessageAt != nil {
		s := c.LastMessageAt.UTC().Format(time.RFC3339)
		out.LastMessageAt = &s
	}
	if c.Identity != nil {
		out.Identity = &identityJSON{ID: c.Identity.ID.String(), Channel: string(c.Identity.Channel), ExternalID: c.Identity.ExternalID}
	}
	if c.AssignedUser != nil {
		out.AssignedUser = &assignedUserJSON{ID: c.AssignedUser.ID.String(), FullName: c.AssignedUser.FullName}
	}
	if c.LastMessage != nil {
		out.LastMessage = &inboxMessageJSON{
			Content:    c.LastMessage.Content,
			CreatedAt:  c.LastMessage.CreatedAt.UTC().Format(time.RFC3339),
			SenderType: string(c.LastMessage.SenderType),
			Direction:  string(c.LastMessage.Direction),
			Status:     string(c.LastMessage.Status),
		}
	}
	return out
}

func messageResponse(m *Message) messageJSON {
	meta := m.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage("null")
	}
	return messageJSON{
		ID:          m.ID.String(),
		Direction:   string(m.Direction),
		SenderType:  string(m.SenderType),
		MessageType: string(m.MessageType),
		Content:     m.Content,
		ExternalID:  m.ExternalID,
		Status:      string(m.Status),
		Metadata:    meta,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	s := value.UTC().Format(time.RFC3339)
	return &s
}

func humanInboxResponse(items []HumanInboxItem) []humanInboxJSON {
	out := make([]humanInboxJSON, 0, len(items))
	for i := range items {
		out = append(out, humanInboxJSON{conversationJSON: conversationResponse(&items[i].Conversation)})
	}
	return out
}

func conversationsResponse(items []Conversation) []conversationJSON {
	out := make([]conversationJSON, 0, len(items))
	for i := range items {
		out = append(out, conversationResponse(&items[i]))
	}
	return out
}

func messagesResponse(items []Message) []messageJSON {
	out := make([]messageJSON, 0, len(items))
	for i := range items {
		out = append(out, messageResponse(&items[i]))
	}
	return out
}
