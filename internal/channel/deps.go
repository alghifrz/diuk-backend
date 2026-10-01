package channel

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/customer"
)

type CustomerAPI interface {
	EnsureIdentity(ctx context.Context, businessID uuid.UUID, in customer.EnsureIdentityInput) (*customer.Identity, bool, error)
}

type ConversationAPI interface {
	GetConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*conversation.Conversation, error)
	CreateConversation(ctx context.Context, businessID uuid.UUID, in conversation.CreateConversationInput) (*conversation.Conversation, bool, error)
	LatestConversation(ctx context.Context, businessID, customerID uuid.UUID, channelRaw string, identityID *uuid.UUID) (*conversation.Conversation, error)
	CreateMessage(ctx context.Context, businessID, conversationID uuid.UUID, in conversation.CreateMessageInput) (*conversation.Message, bool, error)
	GetMessage(ctx context.Context, businessID, messageID uuid.UUID) (*conversation.Message, error)
	FindMessageByExternalID(ctx context.Context, businessID uuid.UUID, externalID string) (*conversation.Message, error)
	ApplyDeliveryStatus(ctx context.Context, businessID, messageID uuid.UUID, statusRaw string, externalID *string) (*conversation.Message, bool, error)
	ClaimForDelivery(ctx context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*conversation.Message, error)
}

type AIService interface {
	GenerateReply(ctx context.Context, businessID uuid.UUID, in ai.GenerateReplyInput) (*ai.GenerateReplyResult, error)
}

type ReplyTrigger interface {
	Trigger(ctx context.Context, businessID, conversationID, inboundMessageID uuid.UUID)
}

type JobEnqueuer interface {
	EnqueueAIGenerate(ctx context.Context, businessID, conversationID, messageID uuid.UUID) error
	EnqueueDelivery(ctx context.Context, businessID, messageID uuid.UUID) error
}
