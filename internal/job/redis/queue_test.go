package redisq

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/job"
)

func TestRedisQueueIntegration(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("TEST_REDIS_URL"))
	if url == "" {
		t.Skip("TEST_REDIS_URL is not set; skipping Redis integration tests")
	}
	ctx := context.Background()
	client, err := Connect(ctx, Config{URL: url, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	prefix := "diuk-test-" + uuid.NewString()[:8]
	q := NewQueue(client, prefix, time.Hour)

	j, err := job.NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(ctx, j); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claim, err := q.Claim(ctx, "ai", "c1", 2*time.Second)
	if err != nil || claim.Job.ID != j.ID {
		t.Fatalf("claim = %+v err=%v", claim, err)
	}
	if err := q.Ack(ctx, claim); err != nil {
		t.Fatalf("ack: %v", err)
	}

	j2, _ := job.NewMessageDelivery(uuid.New(), uuid.New(), 2)
	_ = q.Enqueue(ctx, j2)
	c2, err := q.Claim(ctx, "delivery", "c1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	j2.Attempt = 2
	if err := q.DeadLetter(ctx, c2, j2, "failed"); err != nil {
		t.Fatalf("dlq: %v", err)
	}

	j3, _ := job.NewAIGenerate(uuid.New(), uuid.New(), uuid.New(), 3)
	_ = q.Enqueue(ctx, j3)
	c3, err := q.Claim(ctx, "ai", "crash", 2*time.Second)
	if err != nil || c3.StreamID == "" {
		t.Fatalf("claim crash = %+v err=%v", c3, err)
	}
	reclaimed, err := q.Reclaim(ctx, "ai", 0, "survivor")
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	found := false
	for _, item := range reclaimed {
		if item.Job.ID == j3.ID {
			found = true
			_ = q.Ack(ctx, item)
		}
	}
	if !found {
		t.Fatal("abandoned job was not reclaimed")
	}
}
