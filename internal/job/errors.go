package job

import (
	"errors"
	"time"
)

var (
	ErrInvalidType      = errors.New("job type is invalid")
	ErrInvalidPayload   = errors.New("job payload is invalid")
	ErrQueueUnavailable = errors.New("job queue is unavailable")
	ErrUnknownJob       = errors.New("unknown job type")
	ErrPermanent        = errors.New("permanent job failure")
	ErrRetryable        = errors.New("retryable job failure")
)

type retryableError struct {
	err   error
	after time.Duration
}

func (e retryableError) Error() string {
	if e.err == nil {
		return ErrRetryable.Error()
	}
	return e.err.Error()
}
func (e retryableError) Unwrap() error             { return first(e.err, ErrRetryable) }
func (e retryableError) RetryAfter() time.Duration { return e.after }

type permanentError struct{ err error }

func (e permanentError) Error() string {
	if e.err == nil {
		return ErrPermanent.Error()
	}
	return e.err.Error()
}
func (e permanentError) Unwrap() error { return first(e.err, ErrPermanent) }

func Retryable(err error, after time.Duration) error {
	if err == nil {
		return nil
	}
	return retryableError{err: err, after: after}
}

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

func first(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
