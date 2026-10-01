package job

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"time"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/automation"
	"github.com/alghifrz/diuk-backend/internal/channel"
	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
)

type Classification struct {
	Retryable  bool
	RetryAfter time.Duration
}

func Classify(err error) Classification {
	if err == nil {
		return Classification{}
	}
	if IsPermanentDomain(err) {
		return Classification{Retryable: false}
	}
	var ra interface{ RetryAfter() time.Duration }
	if errors.As(err, &ra) {
		return Classification{Retryable: true, RetryAfter: ra.RetryAfter()}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return Classification{Retryable: true}
	}
	if errors.Is(err, ErrRetryable) ||
		errors.Is(err, ErrQueueUnavailable) ||
		errors.Is(err, channel.ErrProviderUnavailable) ||
		errors.Is(err, channel.ErrRateLimited) ||
		errors.Is(err, whatsapp.ErrUnavailable) ||
		errors.Is(err, whatsapp.ErrRateLimited) ||
		errors.Is(err, ai.ErrProvider) ||
		errors.Is(err, ai.ErrInvalidResponse) ||
		errors.Is(err, knowledge.ErrEmbeddingFailed) ||
		errors.Is(err, knowledge.ErrStorageFailed) ||
		errors.Is(err, automation.ErrDisabled) {
		return Classification{Retryable: true}
	}
	return Classification{Retryable: false}
}

func IsPermanentDomain(err error) bool {
	return errors.Is(err, ErrPermanent) ||
		errors.Is(err, ErrInvalidPayload) ||
		errors.Is(err, ErrUnknownJob) ||
		errors.Is(err, ErrInvalidType) ||
		errors.Is(err, channel.ErrInvalid) ||
		errors.Is(err, channel.ErrChannelNotFound) ||
		errors.Is(err, channel.ErrChannelDisabled) ||
		errors.Is(err, channel.ErrUnsupportedMessage) ||
		errors.Is(err, channel.ErrProviderRejected) ||
		errors.Is(err, channel.ErrMessageNotFound) ||
		errors.Is(err, channel.ErrConversationNotFound) ||
		errors.Is(err, conversation.ErrNotFound) ||
		errors.Is(err, conversation.ErrMessageNotFound) ||
		errors.Is(err, conversation.ErrInvalid) ||
		errors.Is(err, ai.ErrDisabled) ||
		errors.Is(err, ai.ErrProviderNotConfigured) ||
		errors.Is(err, ai.ErrConversationNotFound) ||
		errors.Is(err, ai.ErrHumanHandling) ||
		errors.Is(err, ai.ErrQuotaExceeded) ||
		errors.Is(err, ai.ErrInvalid) ||
		errors.Is(err, ai.ErrToolArguments) ||
		errors.Is(err, whatsapp.ErrRejected) ||
		errors.Is(err, whatsapp.ErrInvalidPayload) ||
		errors.Is(err, knowledge.ErrDocumentNotFound) ||
		errors.Is(err, knowledge.ErrUnsupportedDocumentType) ||
		errors.Is(err, knowledge.ErrDocumentEmpty) ||
		errors.Is(err, knowledge.ErrDocumentTooLarge) ||
		errors.Is(err, knowledge.ErrEmbeddingNotConfigured) ||
		errors.Is(err, knowledge.ErrInvalid) ||
		errors.Is(err, knowledge.ErrDocumentProcessingFailed) ||
		errors.Is(err, automation.ErrNotFound) ||
		errors.Is(err, automation.ErrEventNotFound) ||
		errors.Is(err, automation.ErrRunNotFound) ||
		errors.Is(err, automation.ErrRecipientMissing) ||
		errors.Is(err, automation.ErrSubjectMissing) ||
		errors.Is(err, automation.ErrSubjectInvalid) ||
		errors.Is(err, automation.ErrInvalidTemplate) ||
		errors.Is(err, automation.ErrUnsupportedAction) ||
		errors.Is(err, automation.ErrInactiveAutomation) ||
		errors.Is(err, automation.ErrBusinessInactive) ||
		errors.Is(err, automation.ErrInvalid)
}

func Backoff(policy RetryPolicy, attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	base := policy.Base
	if base <= 0 {
		base = 30 * time.Second
	}
	capAt := policy.Cap
	if capAt <= 0 {
		capAt = 2 * time.Minute
	}
	exp := float64(base) * math.Pow(2, float64(attempt-1))
	if exp > float64(capAt) {
		exp = float64(capAt)
	}
	jitter := jitterFraction()
	d := time.Duration(exp * (0.7 + 0.6*jitter))
	if d > capAt {
		return capAt
	}
	if d < 0 {
		return 0
	}
	return d
}

func jitterFraction() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0.5
	}
	n := binary.LittleEndian.Uint64(b[:])
	return float64(n%1000) / 1000
}
