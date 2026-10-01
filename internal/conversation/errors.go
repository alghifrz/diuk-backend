package conversation

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid                   = errors.New("invalid request")
	ErrNotFound                  = errors.New("conversation not found")
	ErrClosed                    = errors.New("conversation is closed")
	ErrInvalidChannel            = errors.New("channel is invalid")
	ErrCustomerNotFound          = errors.New("customer not found")
	ErrCustomerInactive          = errors.New("customer is not active")
	ErrIdentityNotFound          = errors.New("customer identity not found")
	ErrIdentityMismatch          = errors.New("identity does not belong to customer")
	ErrChannelMismatch           = errors.New("identity channel does not match conversation channel")
	ErrAssignedUserNotFound      = errors.New("assigned user not found")
	ErrAssignedUserInactive      = errors.New("assigned user is not active")
	ErrAssignedUserNotMember     = errors.New("assigned user is not a member of the business")
	ErrMessageNotFound           = errors.New("message not found")
	ErrDuplicateExternalID       = errors.New("message already exists")
	ErrOpenConversationExists    = errors.New("open conversation already exists")
	ErrInvalidDirection          = errors.New("message direction is invalid")
	ErrInvalidSenderType         = errors.New("sender type is invalid")
	ErrInvalidMessageType        = errors.New("message type is invalid")
	ErrInvalidMessageStatus      = errors.New("message status is invalid")
	ErrInvalidConversationStatus = errors.New("conversation status is invalid")
	ErrInvalidHandlingTransition = errors.New("handling state transition is invalid")
	ErrHumanHandlingActive       = errors.New("conversation is handled by a human")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidChannel):
		writeError(c, http.StatusBadRequest, "INVALID_CHANNEL", "channel is invalid")
	case errors.Is(err, ErrInvalidDirection):
		writeError(c, http.StatusBadRequest, "INVALID_MESSAGE_DIRECTION", "message direction is invalid")
	case errors.Is(err, ErrInvalidSenderType):
		writeError(c, http.StatusBadRequest, "INVALID_SENDER_TYPE", "sender type is invalid")
	case errors.Is(err, ErrInvalidMessageType):
		writeError(c, http.StatusBadRequest, "INVALID_MESSAGE_TYPE", "message type is invalid")
	case errors.Is(err, ErrInvalidMessageStatus):
		writeError(c, http.StatusBadRequest, "INVALID_MESSAGE_STATUS", "message status is invalid")
	case errors.Is(err, ErrInvalidConversationStatus):
		writeError(c, http.StatusBadRequest, "INVALID_CONVERSATION_STATUS", "conversation status is invalid")
	case errors.Is(err, ErrIdentityMismatch):
		writeError(c, http.StatusBadRequest, "IDENTITY_CUSTOMER_MISMATCH", "identity does not belong to customer")
	case errors.Is(err, ErrChannelMismatch):
		writeError(c, http.StatusBadRequest, "IDENTITY_CHANNEL_MISMATCH", "identity channel does not match conversation channel")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
	case errors.Is(err, ErrCustomerNotFound):
		writeError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "customer not found")
	case errors.Is(err, ErrIdentityNotFound):
		writeError(c, http.StatusNotFound, "IDENTITY_NOT_FOUND", "customer identity not found")
	case errors.Is(err, ErrAssignedUserNotFound):
		writeError(c, http.StatusNotFound, "ASSIGNED_USER_NOT_FOUND", "assigned user not found")
	case errors.Is(err, ErrMessageNotFound):
		writeError(c, http.StatusNotFound, "MESSAGE_NOT_FOUND", "message not found")
	case errors.Is(err, ErrCustomerInactive):
		writeError(c, http.StatusConflict, "CUSTOMER_INACTIVE", "customer is not active")
	case errors.Is(err, ErrClosed):
		writeError(c, http.StatusConflict, "CONVERSATION_CLOSED", "conversation is closed")
	case errors.Is(err, ErrInvalidHandlingTransition):
		writeError(c, http.StatusConflict, "INVALID_HANDLING_TRANSITION", "handling state transition is invalid")
	case errors.Is(err, ErrHumanHandlingActive):
		writeError(c, http.StatusConflict, "HUMAN_HANDLING_ACTIVE", "conversation is handled by a human")
	case errors.Is(err, ErrAssignedUserInactive):
		writeError(c, http.StatusConflict, "ASSIGNED_USER_INACTIVE", "assigned user is not active")
	case errors.Is(err, ErrAssignedUserNotMember):
		writeError(c, http.StatusConflict, "ASSIGNED_USER_NOT_MEMBER", "assigned user is not a member of the business")
	case errors.Is(err, ErrDuplicateExternalID):
		writeError(c, http.StatusConflict, "MESSAGE_ALREADY_EXISTS", "message already exists")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrClosed) ||
		errors.Is(err, ErrInvalidChannel) ||
		errors.Is(err, ErrCustomerNotFound) ||
		errors.Is(err, ErrCustomerInactive) ||
		errors.Is(err, ErrIdentityNotFound) ||
		errors.Is(err, ErrIdentityMismatch) ||
		errors.Is(err, ErrChannelMismatch) ||
		errors.Is(err, ErrAssignedUserNotFound) ||
		errors.Is(err, ErrAssignedUserInactive) ||
		errors.Is(err, ErrAssignedUserNotMember) ||
		errors.Is(err, ErrMessageNotFound) ||
		errors.Is(err, ErrDuplicateExternalID) ||
		errors.Is(err, ErrInvalidDirection) ||
		errors.Is(err, ErrInvalidSenderType) ||
		errors.Is(err, ErrInvalidMessageType) ||
		errors.Is(err, ErrInvalidMessageStatus) ||
		errors.Is(err, ErrInvalidConversationStatus) ||
		errors.Is(err, ErrInvalidHandlingTransition) ||
		errors.Is(err, ErrHumanHandlingActive)
}
