package automation

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/business"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/reservation"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

type Store interface {
	CreateAutomation(ctx context.Context, item Automation) (*Automation, error)
	GetAutomation(ctx context.Context, businessID, automationID uuid.UUID) (*Automation, error)
	ListAutomations(ctx context.Context, businessID uuid.UUID, status string) ([]Automation, error)
	ListActiveByTrigger(ctx context.Context, businessID uuid.UUID, triggerType string) ([]Automation, error)
	ListActiveScheduled(ctx context.Context) ([]Automation, error)
	UpdateAutomation(ctx context.Context, item Automation) (*Automation, error)
	InsertEvent(ctx context.Context, event Event) (*Event, bool, error)
	GetEvent(ctx context.Context, businessID, eventID uuid.UUID) (*Event, error)
	InsertRun(ctx context.Context, run Run) (*Run, bool, error)
	GetRun(ctx context.Context, businessID, runID uuid.UUID) (*Run, error)
	ListRuns(ctx context.Context, businessID, automationID uuid.UUID, limit int) ([]Run, error)
	ClaimRun(ctx context.Context, businessID, runID uuid.UUID) (*Run, error)
	MarkRunStatus(ctx context.Context, businessID, runID uuid.UUID, status string, errorCode, errorMessage *string) (*Run, error)
	CancelPendingRuns(ctx context.Context, businessID, automationID uuid.UUID) error
	HasRunForKey(ctx context.Context, automationID uuid.UUID, executionKey string) (bool, error)
	ListDueReservations(ctx context.Context, businessID uuid.UUID, dueBefore time.Time, lookback time.Time, limit int) ([]ReservationCandidate, error)
	ListInactiveConversations(ctx context.Context, businessID uuid.UUID, inactiveSince time.Time, limit int) ([]ConversationCandidate, error)
	LastInboundCustomerAt(ctx context.Context, businessID, conversationID uuid.UUID) (*time.Time, error)
	TrySchedulerLock(ctx context.Context) (bool, func(), error)
}

type EventPublisher interface {
	EnqueueEvaluate(ctx context.Context, businessID, eventID uuid.UUID) error
	EnqueueRun(ctx context.Context, businessID, runID uuid.UUID) error
}

type QueuePublisher struct {
	Evaluate func(ctx context.Context, businessID, eventID uuid.UUID) error
	Run      func(ctx context.Context, businessID, runID uuid.UUID) error
}

func (p QueuePublisher) EnqueueEvaluate(ctx context.Context, businessID, eventID uuid.UUID) error {
	if p.Evaluate == nil {
		return nil
	}
	return p.Evaluate(ctx, businessID, eventID)
}

func (p QueuePublisher) EnqueueRun(ctx context.Context, businessID, runID uuid.UUID) error {
	if p.Run == nil {
		return nil
	}
	return p.Run(ctx, businessID, runID)
}

type DeliveryEnqueuer interface {
	EnqueueDelivery(ctx context.Context, businessID, messageID uuid.UUID) error
}

type Conversations interface {
	GetConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*conversation.Conversation, error)
	LatestConversation(ctx context.Context, businessID, customerID uuid.UUID, channelRaw string, identityID *uuid.UUID) (*conversation.Conversation, error)
	CreateConversation(ctx context.Context, businessID uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error)
	CreateMessage(ctx context.Context, businessID, conversationID uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error)
}

type Customers interface {
	GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*customer.Customer, error)
}

type Reservations interface {
	Get(ctx context.Context, businessID, reservationID uuid.UUID) (*reservation.Reservation, error)
}

type Businesses interface {
	Profile(ctx context.Context, businessID uuid.UUID) (*business.Profile, error)
}

type AIGenerator interface {
	GenerateReply(ctx context.Context, businessID uuid.UUID, in ai.GenerateReplyInput) (*ai.GenerateReplyResult, error)
}

func businessIsActive(profile *business.Profile) bool {
	return profile != nil && profile.Status == tenant.StatusActive
}
