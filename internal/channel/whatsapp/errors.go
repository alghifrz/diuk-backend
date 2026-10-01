package whatsapp

import (
	"errors"
	"time"
)

var (
	ErrInvalidSignature = errors.New("webhook signature is invalid")
	ErrInvalidPayload   = errors.New("webhook payload is invalid")
	ErrUnsupported      = errors.New("message type is unsupported")
	ErrRejected         = errors.New("whatsapp rejected the request")
	ErrUnavailable      = errors.New("whatsapp is unavailable")
	ErrRateLimited      = errors.New("whatsapp rate limited")
)

type RateLimitedError struct {
	After time.Duration
}

func (e RateLimitedError) Error() string             { return ErrRateLimited.Error() }
func (e RateLimitedError) Unwrap() error             { return ErrRateLimited }
func (e RateLimitedError) RetryAfter() time.Duration { return e.After }
