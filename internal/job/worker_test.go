package job

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecodeAndUnknownType(t *testing.T) {
	j, err := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Encode(j)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil || got.Type != TypeAIGenerate || got.ID != j.ID {
		t.Fatalf("roundtrip = %+v err=%v", got, err)
	}
	k, err := NewKnowledgeIngest(uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if k.QueueName() != "knowledge" {
		t.Fatalf("queue = %s", k.QueueName())
	}
	autoEval, err := NewAutomationEvaluate(uuid.New(), uuid.New(), 3)
	if err != nil || autoEval.QueueName() != "automation" {
		t.Fatalf("evaluate queue = %s err=%v", autoEval.QueueName(), err)
	}
	autoRun, err := NewAutomationRun(uuid.New(), uuid.New(), 3)
	if err != nil || autoRun.QueueName() != "automation" {
		t.Fatalf("run queue = %s err=%v", autoRun.QueueName(), err)
	}
	campStart, err := NewCampaignStart(uuid.New(), uuid.New(), 3)
	if err != nil || campStart.QueueName() != "campaign" {
		t.Fatalf("campaign start queue = %s err=%v", campStart.QueueName(), err)
	}
	campDispatch, err := NewCampaignDispatch(uuid.New(), uuid.New(), 3)
	if err != nil || campDispatch.QueueName() != "campaign" {
		t.Fatalf("campaign dispatch queue = %s err=%v", campDispatch.QueueName(), err)
	}
	raw, err = Encode(k)
	if err != nil {
		t.Fatal(err)
	}
	got, err = Decode(raw)
	if err != nil || got.Type != TypeKnowledgeIngest {
		t.Fatalf("knowledge roundtrip = %+v err=%v", got, err)
	}
	payload, err := ParseKnowledgeIngest(got.Payload)
	if err != nil || payload.DocumentID == uuid.Nil {
		t.Fatalf("parse knowledge = %+v err=%v", payload, err)
	}
	if _, err := Decode([]byte(`{"job_id":"` + uuid.NewString() + `","type":"NOPE","payload":{}}`)); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("unknown = %v", err)
	}
}

func TestClassifyRetryAndPermanent(t *testing.T) {
	if !Classify(Retryable(errors.New("x"), time.Second)).Retryable {
		t.Fatal("retryable")
	}
	if Classify(Permanent(errors.New("bad"))).Retryable {
		t.Fatal("permanent")
	}
	if d := Backoff(DefaultRetryPolicy(), 1); d <= 0 {
		t.Fatalf("backoff = %s", d)
	}
}

func TestMemoryQueueAckRetryDLQAndReclaim(t *testing.T) {
	q := NewMemoryQueue()
	ctx := context.Background()
	j, _ := NewMessageDelivery(uuid.New(), uuid.New(), 2)
	if err := q.Enqueue(ctx, j); err != nil {
		t.Fatal(err)
	}
	claim, err := q.Claim(ctx, "delivery", "w1", time.Second)
	if err != nil || claim.StreamID == "" {
		t.Fatalf("claim = %+v err=%v", claim, err)
	}
	if q.PendingCount("delivery") != 1 {
		t.Fatalf("pending = %d", q.PendingCount("delivery"))
	}
	if err := q.Ack(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if q.PendingCount("delivery") != 0 {
		t.Fatal("ack did not clear pending")
	}

	j2, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 2)
	_ = q.Enqueue(ctx, j2)
	c2, _ := q.Claim(ctx, "ai", "w1", time.Second)
	j2.Attempt = 1
	j2.AvailableAt = time.Now().UTC().Add(-time.Second)
	if err := q.Retry(ctx, c2, j2); err != nil {
		t.Fatal(err)
	}
	c3, _ := q.Claim(ctx, "ai", "w2", time.Second)
	if c3.Job.ID != j2.ID {
		t.Fatalf("retry claim = %+v", c3)
	}
	_ = q.DeadLetter(ctx, c3, c3.Job, "boom")
	if len(q.Dead("ai")) != 1 {
		t.Fatal("expected dlq entry")
	}

	j3, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(ctx, j3)
	c4, _ := q.Claim(ctx, "ai", "w1", time.Second)
	reclaimed, err := q.Reclaim(ctx, "ai", 0, "w2")
	if err != nil || len(reclaimed) != 1 || reclaimed[0].Job.ID != c4.Job.ID {
		t.Fatalf("reclaim = %+v err=%v", reclaimed, err)
	}
}

func TestWorkerSuccessRetryAndDLQ(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := NewMemoryQueue()
	store := NewMemoryStore()
	var calls atomic.Int32
	h := handlerFunc(func(ctx context.Context, j Job) error {
		n := calls.Add(1)
		if n == 1 {
			return Retryable(errors.New("temp"), 0)
		}
		return nil
	})
	w := NewWorker(log, q, h, store, WorkerConfig{
		Name: "t", Queue: "ai", Concurrency: 1, Timeout: time.Second,
		IdleClaim: time.Hour, Policy: RetryPolicy{MaxAttempts: 3, Base: time.Millisecond, Cap: 5 * time.Millisecond},
	})
	j, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	j.AvailableAt = time.Now().UTC()
	_ = q.Enqueue(context.Background(), j)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
	cancel()
	<-done
}

func TestWorkerDeadLettersPermanent(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := NewMemoryQueue()
	store := NewMemoryStore()
	w := NewWorker(log, q, handlerFunc(func(context.Context, Job) error {
		return Permanent(errors.New("nope"))
	}), store, WorkerConfig{Name: "t", Queue: "delivery", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour})
	j, _ := NewMessageDelivery(uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), j)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(q.Dead("delivery")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(q.Dead("delivery")) != 1 {
		t.Fatal("expected dead letter")
	}
}

func TestWorkerPanicDoesNotDie(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := NewMemoryQueue()
	w := NewWorker(log, q, handlerFunc(func(context.Context, Job) error {
		panic("boom")
	}), NewMemoryStore(), WorkerConfig{Name: "t", Queue: "ai", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour})
	j, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 1)
	_ = q.Enqueue(context.Background(), j)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(q.Dead("ai")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(q.Dead("ai")) != 1 {
		t.Fatal("panic should dead-letter")
	}
}

func TestOutboxSurvivesRedisFailure(t *testing.T) {
	store := NewMemoryStore()
	outbox := NewOutbox(slog.New(slog.NewTextHandler(io.Discard, nil)), store, failingQueue{}, 3)
	if err := outbox.EnqueueAIGenerate(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	due, err := store.ListDue(context.Background(), 10)
	if err != nil || len(due) != 1 || due[0].Status != OutboxPending {
		t.Fatalf("outbox due = %+v err=%v", due, err)
	}
}

func TestWorkerShutdownFinishesClaim(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := NewMemoryQueue()
	started := make(chan struct{})
	h := handlerFunc(func(ctx context.Context, _ Job) error {
		close(started)
		time.Sleep(80 * time.Millisecond)
		return nil
	})
	w := NewWorker(log, q, h, NewMemoryStore(), WorkerConfig{Name: "t", Queue: "ai", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour})
	j, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), j)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not exit")
	}
	if q.PendingCount("ai") != 0 {
		t.Fatal("in-flight job was not acked")
	}
}

func TestWorkerTelemetrySuccessRetryAndDLQ(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := &jobTelemetryRecorder{}

	q := NewMemoryQueue()
	success := NewWorker(log, q, handlerFunc(func(context.Context, Job) error { return nil }), NewMemoryStore(), WorkerConfig{
		Name: "t", Queue: "ai", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour,
	})
	success.SetTelemetry(rec)
	j, _ := NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), j)
	runWorkerUntil(t, success, func() bool { return rec.success.Load() >= 1 })

	q = NewMemoryQueue()
	retried := NewWorker(log, q, handlerFunc(func(context.Context, Job) error {
		return Retryable(errors.New("later"), time.Millisecond)
	}), NewMemoryStore(), WorkerConfig{
		Name: "t", Queue: "ai", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour,
		Policy: RetryPolicy{MaxAttempts: 3, Base: time.Millisecond, Cap: 5 * time.Millisecond},
	})
	retried.SetTelemetry(rec)
	j, _ = NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), j)
	runWorkerUntil(t, retried, func() bool { return rec.retry.Load() >= 1 })

	q = NewMemoryQueue()
	dlq := NewWorker(log, q, handlerFunc(func(context.Context, Job) error {
		return Permanent(errors.New("nope"))
	}), NewMemoryStore(), WorkerConfig{Name: "t", Queue: "delivery", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour})
	dlq.SetTelemetry(rec)
	d, _ := NewMessageDelivery(uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), d)
	runWorkerUntil(t, dlq, func() bool { return rec.dead.Load() >= 1 })

	if rec.success.Load() < 1 || rec.retry.Load() < 1 || rec.dead.Load() < 1 {
		t.Fatalf("telemetry success=%d retry=%d dead=%d", rec.success.Load(), rec.retry.Load(), rec.dead.Load())
	}
}

func runWorkerUntil(t *testing.T, w *Worker, done func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker telemetry did not update")
}

type jobTelemetryRecorder struct {
	success atomic.Int64
	retry   atomic.Int64
	dead    atomic.Int64
}

func (r *jobTelemetryRecorder) ObserveJob(_, _, result string, _ time.Duration) {
	switch result {
	case "success":
		r.success.Add(1)
	case "retried":
		r.retry.Add(1)
	case "dead_lettered":
		r.dead.Add(1)
	}
}

type handlerFunc func(context.Context, Job) error

func (f handlerFunc) Handle(ctx context.Context, j Job) error { return f(ctx, j) }

type failingQueue struct{}

func (failingQueue) Enqueue(context.Context, Job) error { return ErrQueueUnavailable }
func (failingQueue) Ping(context.Context) error         { return nil }
func (failingQueue) Close() error                       { return nil }
func (failingQueue) Claim(context.Context, string, string, time.Duration) (Claim, error) {
	return Claim{}, nil
}
func (failingQueue) Ack(context.Context, Claim) error                     { return nil }
func (failingQueue) Retry(context.Context, Claim, Job) error              { return nil }
func (failingQueue) DeadLetter(context.Context, Claim, Job, string) error { return nil }
func (failingQueue) Reclaim(context.Context, string, time.Duration, string) ([]Claim, error) {
	return nil, nil
}
