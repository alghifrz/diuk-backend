package redisq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/alghifrz/diuk-backend/internal/job"
)

type Queue struct {
	client    *Client
	prefix    string
	retention time.Duration
}

func NewQueue(client *Client, prefix string, dlqRetention time.Duration) *Queue {
	if strings.TrimSpace(prefix) == "" {
		prefix = "diuk"
	}
	if dlqRetention <= 0 {
		dlqRetention = 7 * 24 * time.Hour
	}
	return &Queue{client: client, prefix: prefix, retention: dlqRetention}
}

func (q *Queue) stream(name string) string { return q.prefix + ":stream:" + name }
func (q *Queue) group(name string) string  { return name + "-workers" }
func (q *Queue) dead(name string) string   { return q.prefix + ":dead:" + name }

func (q *Queue) Enqueue(ctx context.Context, j job.Job) error {
	raw, err := job.Encode(j)
	if err != nil {
		return err
	}
	stream := q.stream(j.QueueName())
	if err := q.ensureGroup(ctx, stream, q.group(j.QueueName())); err != nil {
		return err
	}
	return q.client.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"data": raw},
	}).Err()
}

func (q *Queue) Claim(ctx context.Context, queue, workerID string, block time.Duration) (job.Claim, error) {
	stream := q.stream(queue)
	group := q.group(queue)
	if err := q.ensureGroup(ctx, stream, group); err != nil {
		return job.Claim{}, err
	}
	if block <= 0 {
		block = 2 * time.Second
	}
	streams, err := q.client.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: workerID,
		Streams:  []string{stream, ">"},
		Count:    1,
		Block:    block,
	}).Result()
	if err != nil {
		if err == redis.Nil || isCanceled(err) {
			return job.Claim{}, nil
		}
		return job.Claim{}, fmt.Errorf("%w: %s", job.ErrQueueUnavailable, err.Error())
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return job.Claim{}, nil
	}
	return q.toClaim(queue, streams[0].Messages[0])
}

func (q *Queue) Ack(ctx context.Context, claim job.Claim) error {
	return q.client.rdb.XAck(ctx, q.stream(claim.Queue), q.group(claim.Queue), claim.StreamID).Err()
}

func (q *Queue) Retry(ctx context.Context, claim job.Claim, _ job.Job) error {
	return q.Ack(ctx, claim)
}

func (q *Queue) DeadLetter(ctx context.Context, claim job.Claim, j job.Job, lastError string) error {
	body, err := json.Marshal(job.DeadLetter{
		JobID: j.ID, JobType: j.Type, Payload: j.Payload, Attempts: j.Attempt,
		CreatedAt: j.CreatedAt, FailedAt: time.Now().UTC(), LastError: lastError,
	})
	if err != nil {
		return err
	}
	dlq := q.dead(j.QueueName())
	if err := q.client.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: dlq,
		Values: map[string]any{"data": body},
	}).Err(); err != nil {
		return err
	}
	minID := fmt.Sprintf("%d-0", time.Now().UTC().Add(-q.retention).UnixMilli())
	_ = q.client.rdb.XTrimMinID(ctx, dlq, minID).Err()
	_ = q.client.rdb.Expire(ctx, dlq, q.retention).Err()
	return q.Ack(ctx, claim)
}

func (q *Queue) Reclaim(ctx context.Context, queue string, idle time.Duration, workerID string) ([]job.Claim, error) {
	stream := q.stream(queue)
	group := q.group(queue)
	if err := q.ensureGroup(ctx, stream, group); err != nil {
		return nil, err
	}
	if idle <= 0 {
		idle = time.Minute
	}
	messages, _, err := q.client.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: workerID,
		MinIdle:  idle,
		Start:    "0-0",
		Count:    10,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	out := make([]job.Claim, 0, len(messages))
	for _, msg := range messages {
		claim, err := q.toClaim(queue, msg)
		if err != nil {
			continue
		}
		out = append(out, claim)
	}
	return out, nil
}

func (q *Queue) Ping(ctx context.Context) error { return q.client.Ping(ctx) }
func (q *Queue) Close() error                   { return q.client.Close() }

type QueueStat struct {
	Queue   string
	Stream  string
	Group   string
	Pending int64
}

func (q *Queue) Stats(ctx context.Context, queues []string) []QueueStat {
	out := make([]QueueStat, 0, len(queues))
	for _, name := range queues {
		stream := q.stream(name)
		group := q.group(name)
		pending, err := q.client.rdb.XPending(ctx, stream, group).Result()
		count := int64(0)
		if err == nil && pending != nil {
			count = pending.Count
		}
		out = append(out, QueueStat{Queue: name, Stream: stream, Group: group, Pending: count})
	}
	return out
}

func (q *Queue) ensureGroup(ctx context.Context, stream, group string) error {
	err := q.client.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err == nil || isBusyGroup(err) {
		return nil
	}
	return err
}

func (q *Queue) toClaim(queue string, msg redis.XMessage) (job.Claim, error) {
	raw, _ := msg.Values["data"].(string)
	j, err := job.Decode([]byte(raw))
	if err != nil {
		return job.Claim{}, err
	}
	return job.Claim{Job: j, Queue: queue, StreamID: msg.ID}, nil
}

func isBusyGroup(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

func isCanceled(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline"))
}
