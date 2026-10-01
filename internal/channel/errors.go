package channel

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid                 = errors.New("invalid request")
	ErrChannelNotFound         = errors.New("channel not found")
	ErrChannelDisabled         = errors.New("channel is disabled")
	ErrInvalidWebhook          = errors.New("webhook is invalid")
	ErrInvalidSignature        = errors.New("webhook signature is invalid")
	ErrUnsupportedMessage      = errors.New("message type is unsupported")
	ErrProviderUnavailable     = errors.New("messaging provider is unavailable")
	ErrProviderRejected        = errors.New("messaging provider rejected the request")
	ErrRateLimited             = errors.New("messaging provider rate limited")
	ErrMessageAlreadyProcessed = errors.New("message already processed")
	ErrConversationNotFound    = errors.New("conversation not found")
	ErrMessageNotFound         = errors.New("message not found")
	ErrQuotaExceeded           = errors.New("channel quota exceeded")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

type RateLimitedError struct {
	After time.Duration
}

func (e RateLimitedError) Error() string             { return ErrRateLimited.Error() }
func (e RateLimitedError) Unwrap() error             { return ErrRateLimited }
func (e RateLimitedError) RetryAfter() time.Duration { return e.After }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidWebhook):
		writeError(c, http.StatusBadRequest, "INVALID_WEBHOOK", "webhook is invalid")
	case errors.Is(err, ErrInvalidSignature):
		writeError(c, http.StatusUnauthorized, "INVALID_SIGNATURE", "webhook signature is invalid")
	case errors.Is(err, ErrUnsupportedMessage):
		writeError(c, http.StatusAccepted, "UNSUPPORTED_MESSAGE", "message type is unsupported")
	case errors.Is(err, ErrChannelDisabled):
		writeError(c, http.StatusConflict, "CHANNEL_DISABLED", "channel is disabled")
	case errors.Is(err, ErrProviderRejected):
		writeError(c, http.StatusBadGateway, "CHANNEL_PROVIDER_REJECTED", "messaging provider rejected the request")
	case errors.Is(err, ErrRateLimited):
		writeError(c, http.StatusTooManyRequests, "CHANNEL_RATE_LIMITED", "messaging provider rate limited")
	case errors.Is(err, ErrProviderUnavailable):
		writeError(c, http.StatusBadGateway, "CHANNEL_PROVIDER_UNAVAILABLE", "messaging provider is unavailable")
	case errors.Is(err, ErrMessageAlreadyProcessed):
		writeError(c, http.StatusOK, "MESSAGE_ALREADY_PROCESSED", "message already processed")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrChannelNotFound):
		writeError(c, http.StatusNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	case errors.Is(err, ErrConversationNotFound):
		writeError(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
	case errors.Is(err, ErrMessageNotFound):
		writeError(c, http.StatusNotFound, "MESSAGE_NOT_FOUND", "message not found")
	case errors.Is(err, ErrQuotaExceeded):
		writeError(c, http.StatusConflict, "CHANNEL_QUOTA_EXCEEDED", "WhatsApp channel limit reached")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrChannelNotFound) ||
		errors.Is(err, ErrChannelDisabled) ||
		errors.Is(err, ErrInvalidWebhook) ||
		errors.Is(err, ErrInvalidSignature) ||
		errors.Is(err, ErrUnsupportedMessage) ||
		errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrProviderRejected) ||
		errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrMessageAlreadyProcessed) ||
		errors.Is(err, ErrConversationNotFound) ||
		errors.Is(err, ErrMessageNotFound) ||
		errors.Is(err, ErrQuotaExceeded)
}
