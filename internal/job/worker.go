package job

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type WorkerConfig struct {
	Name        string
	Queue       string
	Concurrency int
	Timeout     time.Duration
	IdleClaim   time.Duration
	Policy      RetryPolicy
}

type JobTelemetry interface {
	ObserveJob(jobType, queue, result string, duration time.Duration)
}

type Worker struct {
	log       *slog.Logger
	queue     Queue
	handler   Handler
	store     OutboxStore
	cfg       WorkerConfig
	telemetry JobTelemetry
}

func (w *Worker) SetTelemetry(t JobTelemetry) {
	w.telemetry = t
}

func NewWorker(log *slog.Logger, queue Queue, handler Handler, store OutboxStore, cfg WorkerConfig) *Worker {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.IdleClaim <= 0 {
		cfg.IdleClaim = 60 * time.Second
	}
	if cfg.Policy.MaxAttempts < 1 {
		cfg.Policy = DefaultRetryPolicy()
	}
	return &Worker{log: log, queue: queue, handler: handler, store: store, cfg: cfg}
}

func (w *Worker) Run(ctx context.Context) error {
	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	reclaim := time.NewTicker(w.cfg.IdleClaim)
	defer reclaim.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-reclaim.C:
			claims, err := w.queue.Reclaim(ctx, w.cfg.Queue, w.cfg.IdleClaim, w.cfg.Name)
			if err != nil && ctx.Err() == nil {
				w.log.Error("reclaim failed", "queue", w.cfg.Queue, "error", err.Error())
			}
			for _, claim := range claims {
				w.start(ctx, &wg, sem, claim)
			}
		default:
			claim, err := w.queue.Claim(ctx, w.cfg.Queue, w.cfg.Name, 2*time.Second)
			if err != nil {
				if ctx.Err() != nil {
					wg.Wait()
					return nil
				}
				w.log.Error("claim failed", "queue", w.cfg.Queue, "error", err.Error())
				time.Sleep(time.Second)
				continue
			}
			if claim.StreamID == "" {
				continue
			}
			w.start(ctx, &wg, sem, claim)
		}
	}
}

func (w *Worker) start(ctx context.Context, wg *sync.WaitGroup, sem chan struct{}, claim Claim) {
	select {
	case <-ctx.Done():
		return
	case sem <- struct{}{}:
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { <-sem }()
		w.process(ctx, claim)
	}()
}

func (w *Worker) process(parent context.Context, claim Claim) {
	// Shutdown cancels the parent, but in-flight jobs still get a bounded
	// timeout so a SIGTERM does not abort a mid-flight database write.
	ctlCtx, ctlCancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer ctlCancel()

	defer func() {
		if rec := recover(); rec != nil {
			w.log.Error("job panic recovered",
				"job_id", claim.Job.ID,
				"job_type", claim.Job.Type,
				"attempt", claim.Job.Attempt,
				"worker_id", w.cfg.Name,
				"error", rec,
			)
			w.fail(ctlCtx, claim, Permanent(ErrPermanent), 0)
		}
	}()

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), w.cfg.Timeout)
	defer cancel()

	err := w.handler.Handle(ctx, claim.Job)
	duration := time.Since(started)
	if err == nil {
		if ackErr := w.queue.Ack(ctlCtx, claim); ackErr != nil {
			w.log.Error("ack failed", "job_id", claim.Job.ID, "error", ackErr.Error())
		}
		w.observe(string(claim.Job.Type), "success", duration)
		w.log.Info("job processed",
			"job_id", claim.Job.ID,
			"job_type", claim.Job.Type,
			"attempt", claim.Job.Attempt,
			"worker_id", w.cfg.Name,
			"duration_ms", duration.Milliseconds(),
			"result", "ok",
		)
		return
	}
	w.fail(ctlCtx, claim, err, duration)
}

func (w *Worker) observe(jobType, result string, duration time.Duration) {
	if w.telemetry != nil {
		w.telemetry.ObserveJob(jobType, w.cfg.Queue, result, duration)
	}
}

func (w *Worker) fail(ctx context.Context, claim Claim, err error, duration time.Duration) {
	class := Classify(err)
	job := claim.Job
	job.Attempt++
	safeErr := err.Error()

	if !class.Retryable || job.Attempt >= job.MaxAttempts {
		_ = w.queue.DeadLetter(ctx, claim, job, safeErr)
		if w.store != nil {
			_ = w.store.MarkFailed(ctx, job.ID, safeErr)
		}
		w.observe(string(job.Type), "dead_lettered", duration)
		w.log.Error("job dead-lettered",
			"job_id", job.ID, "job_type", job.Type, "attempt", job.Attempt,
			"worker_id", w.cfg.Name, "result", "dlq", "error", safeErr,
		)
		return
	}

	delay := class.RetryAfter
	if delay <= 0 {
		delay = Backoff(w.cfg.Policy, job.Attempt)
	}
	job.AvailableAt = time.Now().UTC().Add(delay)
	if w.store != nil {
		_ = w.store.Reschedule(ctx, job.ID, job.AvailableAt, safeErr)
	}
	if retryErr := w.queue.Retry(ctx, claim, job); retryErr != nil {
		w.log.Error("job retry enqueue failed", "job_id", job.ID, "error", retryErr.Error())
	}
	w.observe(string(job.Type), "retried", duration)
	w.log.Error("job retry scheduled",
		"job_id", job.ID, "job_type", job.Type, "attempt", job.Attempt,
		"worker_id", w.cfg.Name, "result", "retry", "error", safeErr,
	)
}
