package automation

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/reservation"
)

func EventFromReservation(res *reservation.Reservation, eventType string) Event {
	occurred := time.Now().UTC()
	return Event{
		ID:             uuid.New(),
		BusinessID:     res.BusinessID,
		Type:           eventType,
		SubjectType:    SubjectReservation,
		SubjectID:      res.ID,
		IdempotencyKey: fmt.Sprintf("%s:%s", res.ID.String(), eventType),
		Payload: map[string]any{
			"reservation_id": res.ID.String(),
			"customer_id":    res.CustomerID.String(),
			"status":         string(res.Status),
			"party_size":     res.PartySize,
			"start_at":       res.StartAt.UTC().Format(time.RFC3339),
		},
		OccurredAt: occurred,
	}
}

func EventFromConversation(conv *conversation.Conversation) Event {
	return Event{
		ID:             uuid.New(),
		BusinessID:     conv.BusinessID,
		Type:           EventConversationCreated,
		SubjectType:    SubjectConversation,
		SubjectID:      conv.ID,
		IdempotencyKey: fmt.Sprintf("%s:%s", conv.ID.String(), EventConversationCreated),
		Payload: map[string]any{
			"conversation_id": conv.ID.String(),
			"customer_id":     conv.CustomerID.String(),
			"status":          string(conv.Status),
			"channel":         string(conv.Channel),
		},
		OccurredAt: time.Now().UTC(),
	}
}

func EventFromConversationHandling(conv *conversation.Conversation, eventType string) Event {
	payload := map[string]any{
		"conversation_id": conv.ID.String(),
		"customer_id":     conv.CustomerID.String(),
		"status":          string(conv.Status),
		"channel":         string(conv.Channel),
		"handling_state":  string(conv.HandlingState),
	}
	if conv.HandoverReason != nil {
		payload["handover_reason"] = string(*conv.HandoverReason)
	}
	return Event{
		ID:             uuid.New(),
		BusinessID:     conv.BusinessID,
		Type:           eventType,
		SubjectType:    SubjectConversation,
		SubjectID:      conv.ID,
		IdempotencyKey: fmt.Sprintf("%s:%s:%s", conv.ID.String(), eventType, conv.UpdatedAt.UTC().Format(time.RFC3339Nano)),
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
	}
}

func EventFromCustomer(cust *customer.Customer) Event {
	return Event{
		ID:             uuid.New(),
		BusinessID:     cust.BusinessID,
		Type:           EventCustomerCreated,
		SubjectType:    SubjectCustomer,
		SubjectID:      cust.ID,
		IdempotencyKey: fmt.Sprintf("%s:%s", cust.ID.String(), EventCustomerCreated),
		Payload: map[string]any{
			"customer_id": cust.ID.String(),
			"status":      string(cust.Status),
		},
		OccurredAt: time.Now().UTC(),
	}
}

func ScheduledReservationKey(reservationID uuid.UUID, offsetMinutes int) string {
	return fmt.Sprintf("reservation:%s:offset:%d", reservationID.String(), offsetMinutes)
}

func ScheduledInactiveKey(conversationID uuid.UUID, lastInbound time.Time) string {
	return fmt.Sprintf("conversation:%s:inactive:%d", conversationID.String(), lastInbound.UTC().Unix())
}

func ScheduledAt(reference time.Time, offsetMinutes int) time.Time {
	return reference.Add(time.Duration(offsetMinutes) * time.Minute)
}

func IsInactiveDue(lastInbound time.Time, inactiveMinutes int, now time.Time) bool {
	if inactiveMinutes < 1 || lastInbound.IsZero() {
		return false
	}
	return !lastInbound.After(now.Add(-time.Duration(inactiveMinutes) * time.Minute))
}
