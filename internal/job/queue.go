package job

import (
	"context"
	"time"
)

// Queue is the domain-facing job transport. Implementations must not leak
// Redis types into callers.
type Queue interface {
	Enqueue(ctx context.Context, job Job) error
	Claim(ctx context.Context, queue string, workerID string, block time.Duration) (Claim, error)
	Ack(ctx context.Context, claim Claim) error
	Retry(ctx context.Context, claim Claim, job Job) error
	DeadLetter(ctx context.Context, claim Claim, job Job, lastError string) error
	Reclaim(ctx context.Context, queue string, idle time.Duration, workerID string) ([]Claim, error)
	Ping(ctx context.Context) error
	Close() error
}

type Claim struct {
	Job      Job
	Queue    string
	StreamID string
}

type Handler interface {
	Handle(ctx context.Context, job Job) error
}

type RetryPolicy struct {
	MaxAttempts int
	Base        time.Duration
	Cap         time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, Base: 30 * time.Second, Cap: 2 * time.Minute}
}
