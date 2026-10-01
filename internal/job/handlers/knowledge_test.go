package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/job"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
)

type stubKnowledgeProc struct {
	calls int
	err   error
	lastB uuid.UUID
	lastD uuid.UUID
}

func (s *stubKnowledgeProc) ProcessDocument(_ context.Context, businessID, documentID uuid.UUID) error {
	s.calls++
	s.lastB = businessID
	s.lastD = documentID
	return s.err
}

func TestKnowledgeHandlerSuccessAndDuplicate(t *testing.T) {
	proc := &stubKnowledgeProc{}
	h := NewKnowledgeHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), proc)
	biz, doc := uuid.New(), uuid.New()
	j, err := job.NewKnowledgeIngest(biz, doc, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), j); err != nil || proc.calls != 1 {
		t.Fatalf("first = calls=%d err=%v", proc.calls, err)
	}
	if err := h.Handle(context.Background(), j); err != nil || proc.calls != 2 {
		t.Fatalf("duplicate delivery still delegates idempotent process, calls=%d err=%v", proc.calls, err)
	}
	if proc.lastB != biz || proc.lastD != doc {
		t.Fatal("payload ids")
	}
}

func TestKnowledgeHandlerRetryableAndPermanent(t *testing.T) {
	h := NewKnowledgeHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubKnowledgeProc{err: knowledge.ErrEmbeddingFailed})
	j, _ := job.NewKnowledgeIngest(uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), j); !errors.Is(err, knowledge.ErrEmbeddingFailed) || !job.Classify(err).Retryable {
		t.Fatalf("retryable = %v", err)
	}

	h = NewKnowledgeHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubKnowledgeProc{err: knowledge.ErrDocumentEmpty})
	if err := h.Handle(context.Background(), j); !errors.Is(err, knowledge.ErrDocumentEmpty) || job.Classify(err).Retryable {
		t.Fatalf("permanent = %v", err)
	}

	h = NewKnowledgeHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubKnowledgeProc{err: knowledge.ErrEmbeddingNotConfigured})
	if err := h.Handle(context.Background(), j); !errors.Is(err, knowledge.ErrEmbeddingNotConfigured) || job.Classify(err).Retryable {
		t.Fatalf("disabled = %v", err)
	}
}

func TestKnowledgeHandlerInvalidPayload(t *testing.T) {
	h := NewKnowledgeHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &stubKnowledgeProc{})
	j, _ := job.NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	if err := h.Handle(context.Background(), j); !errors.Is(err, job.ErrUnknownJob) {
		t.Fatalf("wrong type = %v", err)
	}
}

func TestKnowledgeIngestWorkerDLQAndPanic(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := job.NewMemoryQueue()
	store := job.NewMemoryStore()
	w := job.NewWorker(log, q, NewKnowledgeHandler(log, &stubKnowledgeProc{err: knowledge.ErrDocumentEmpty}), store, job.WorkerConfig{
		Name: "k", Queue: "knowledge", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour,
	})
	j, _ := job.NewKnowledgeIngest(uuid.New(), uuid.New(), 2)
	_ = q.Enqueue(context.Background(), j)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(q.Dead("knowledge")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(q.Dead("knowledge")) != 1 {
		t.Fatal("expected knowledge dead letter")
	}

	q2 := job.NewMemoryQueue()
	w2 := job.NewWorker(log, q2, handlerFunc(func(context.Context, job.Job) error {
		panic("ingest boom")
	}), job.NewMemoryStore(), job.WorkerConfig{Name: "k2", Queue: "knowledge", Concurrency: 1, Timeout: time.Second, IdleClaim: time.Hour})
	j2, _ := job.NewKnowledgeIngest(uuid.New(), uuid.New(), 1)
	_ = q2.Enqueue(context.Background(), j2)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	go func() { _ = w2.Run(ctx2) }()
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(q2.Dead("knowledge")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(q2.Dead("knowledge")) != 1 {
		t.Fatal("panic should dead-letter knowledge job")
	}
}

func TestKnowledgeIngestRetryThenSuccess(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := job.NewMemoryQueue()
	proc := &countingProc{}
	w := job.NewWorker(log, q, NewKnowledgeHandler(log, proc), job.NewMemoryStore(), job.WorkerConfig{
		Name: "k", Queue: "knowledge", Concurrency: 1, Timeout: time.Second,
		IdleClaim: time.Hour, Policy: job.RetryPolicy{MaxAttempts: 3, Base: time.Millisecond, Cap: 5 * time.Millisecond},
	})
	j, _ := job.NewKnowledgeIngest(uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(context.Background(), j)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if proc.calls >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if proc.calls < 2 {
		t.Fatalf("calls = %d", proc.calls)
	}
	cancel()
	<-done
}

type countingProc struct{ calls int }

func (c *countingProc) ProcessDocument(context.Context, uuid.UUID, uuid.UUID) error {
	c.calls++
	if c.calls == 1 {
		return knowledge.ErrEmbeddingFailed
	}
	return nil
}

type handlerFunc func(context.Context, job.Job) error

func (f handlerFunc) Handle(ctx context.Context, j job.Job) error { return f(ctx, j) }
