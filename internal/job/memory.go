package job

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryQueue is an in-process queue for unit tests. It models claim/ack,
// delayed retry, DLQ, and reclaim of unacked jobs.
type MemoryQueue struct {
	mu      sync.Mutex
	ready   map[string][]Job
	pending map[string]map[string]pendingItem
	dead    map[string][]DeadLetter
}

type pendingItem struct {
	claim Claim
	at    time.Time
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		ready:   map[string][]Job{},
		pending: map[string]map[string]pendingItem{},
		dead:    map[string][]DeadLetter{},
	}
}

func (q *MemoryQueue) Enqueue(_ context.Context, job Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	name := job.QueueName()
	q.ready[name] = append(q.ready[name], job)
	return nil
}

func (q *MemoryQueue) Claim(_ context.Context, queue, workerID string, _ time.Duration) (Claim, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := q.ready[queue]
	now := time.Now().UTC()
	for i, job := range list {
		if job.AvailableAt.After(now) {
			continue
		}
		q.ready[queue] = append(list[:i], list[i+1:]...)
		id := now.Format("20060102150405.000000000") + "-" + job.ID.String()
		claim := Claim{Job: job, Queue: queue, StreamID: id}
		if q.pending[queue] == nil {
			q.pending[queue] = map[string]pendingItem{}
		}
		q.pending[queue][id] = pendingItem{claim: claim, at: now}
		return claim, nil
	}
	return Claim{}, nil
}

func (q *MemoryQueue) Ack(_ context.Context, claim Claim) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending[claim.Queue] != nil {
		delete(q.pending[claim.Queue], claim.StreamID)
	}
	return nil
}

func (q *MemoryQueue) Retry(_ context.Context, claim Claim, job Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending[claim.Queue] != nil {
		delete(q.pending[claim.Queue], claim.StreamID)
	}
	q.ready[job.QueueName()] = append(q.ready[job.QueueName()], job)
	return nil
}

func (q *MemoryQueue) DeadLetter(_ context.Context, claim Claim, job Job, lastError string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending[claim.Queue] != nil {
		delete(q.pending[claim.Queue], claim.StreamID)
	}
	q.dead[job.QueueName()] = append(q.dead[job.QueueName()], DeadLetter{
		JobID: job.ID, JobType: job.Type, Payload: job.Payload,
		Attempts: job.Attempt, CreatedAt: job.CreatedAt, FailedAt: time.Now().UTC(), LastError: lastError,
	})
	return nil
}

func (q *MemoryQueue) Reclaim(_ context.Context, queue string, idle time.Duration, _ string) ([]Claim, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Claim, 0)
	now := time.Now().UTC()
	for id, item := range q.pending[queue] {
		if now.Sub(item.at) >= idle {
			out = append(out, item.claim)
			item.at = now
			q.pending[queue][id] = item
		}
	}
	return out, nil
}

func (q *MemoryQueue) Ping(context.Context) error { return nil }
func (q *MemoryQueue) Close() error               { return nil }

func (q *MemoryQueue) Dead(queue string) []DeadLetter {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]DeadLetter, len(q.dead[queue]))
	copy(out, q.dead[queue])
	return out
}

func (q *MemoryQueue) PendingCount(queue string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending[queue])
}

func (q *MemoryQueue) ReadyCount(queue string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ready[queue])
}

type MemoryStore struct {
	mu          sync.Mutex
	outbox      map[uuid.UUID]OutboxRow
	generations map[uuid.UUID]*uuid.UUID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{outbox: map[uuid.UUID]OutboxRow{}, generations: map[uuid.UUID]*uuid.UUID{}}
}

func (s *MemoryStore) Insert(_ context.Context, row OutboxRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row.Status == "" {
		row.Status = OutboxPending
	}
	s.outbox[row.ID] = row
	return nil
}

func (s *MemoryStore) ListDue(_ context.Context, limit int) ([]OutboxRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	out := make([]OutboxRow, 0)
	for _, row := range s.outbox {
		if row.Status == OutboxPending && !row.AvailableAt.After(now) {
			out = append(out, row)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) MarkPublished(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.outbox[id]
	row.Status = OutboxPublished
	s.outbox[id] = row
	return nil
}

func (s *MemoryStore) MarkFailed(_ context.Context, id uuid.UUID, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.outbox[id]
	row.Status = OutboxFailed
	row.LastError = &lastError
	row.Attempts++
	s.outbox[id] = row
	return nil
}

func (s *MemoryStore) Reschedule(_ context.Context, id uuid.UUID, availableAt time.Time, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.outbox[id]
	row.Status = OutboxPending
	row.AvailableAt = availableAt
	row.LastError = &lastError
	row.Attempts++
	s.outbox[id] = row
	return nil
}

func (s *MemoryStore) ClaimAIGeneration(_ context.Context, _, _, inboundID uuid.UUID) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if outbound, ok := s.generations[inboundID]; ok {
		if outbound != nil {
			return false, true, nil
		}
		return true, false, nil
	}
	s.generations[inboundID] = nil
	return true, false, nil
}

func (s *MemoryStore) CompleteAIGeneration(_ context.Context, inboundID, outboundID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := outboundID
	s.generations[inboundID] = &copy
	return nil
}

func (s *MemoryStore) RecordAIGenerationMeta(context.Context, uuid.UUID, string, string) error {
	return nil
}

func (s *MemoryStore) RecordAIGenerationFailure(context.Context, uuid.UUID) error {
	return nil
}
