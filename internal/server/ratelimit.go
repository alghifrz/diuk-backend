package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/auth"
)

// memoryLimiter is a process-local token bucket. Redis outage must not block
// the API, so rate limiting is fail-open and never depends on Redis.
type memoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   float64
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newMemoryLimiter(perMinute int) *memoryLimiter {
	if perMinute < 1 {
		return nil
	}
	burst := float64(perMinute)
	if burst < 1 {
		burst = 1
	}
	return &memoryLimiter{
		buckets: map[string]*tokenBucket{},
		rate:    float64(perMinute) / 60.0,
		burst:   burst,
	}
}

func (l *memoryLimiter) allow(key string) bool {
	if l == nil || key == "" {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b == nil {
		b = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}
	if len(l.buckets) > 512 {
		for key, stale := range l.buckets {
			if now.Sub(stale.last) > 10*time.Minute {
				delete(l.buckets, key)
			}
		}
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *memoryLimiter) gc(now time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, b := range l.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(l.buckets, key)
		}
	}
}

type rateLimiters struct {
	webhook  *memoryLimiter
	ai       *memoryLimiter
	upload   *memoryLimiter
	campaign *memoryLimiter
}

func newRateLimiters(enabled bool, webhook, ai, upload, campaign int) *rateLimiters {
	if !enabled {
		return &rateLimiters{}
	}
	return &rateLimiters{
		webhook:  newMemoryLimiter(webhook),
		ai:       newMemoryLimiter(ai),
		upload:   newMemoryLimiter(upload),
		campaign: newMemoryLimiter(campaign),
	}
}

func (r *rateLimiters) middleware(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if r == nil {
			c.Next()
			return
		}
		limiter := (*memoryLimiter)(nil)
		key := c.ClientIP()
		switch kind {
		case "webhook":
			limiter = r.webhook
		case "ai":
			limiter = r.ai
			if ac, ok := auth.GetAuthContext(c); ok && ac.HasTenant() && ac.BusinessID != uuid.Nil {
				key = "biz:" + ac.BusinessID.String()
			}
		case "upload":
			limiter = r.upload
			if ac, ok := auth.GetAuthContext(c); ok && ac.HasTenant() && ac.BusinessID != uuid.Nil {
				key = "biz:" + ac.BusinessID.String()
			}
		case "campaign":
			limiter = r.campaign
			if ac, ok := auth.GetAuthContext(c); ok && ac.HasTenant() && ac.BusinessID != uuid.Nil {
				key = "biz:" + ac.BusinessID.String()
			}
		}
		if !limiter.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": "RATE_LIMITED", "message": "too many requests"},
			})
			return
		}
		c.Next()
	}
}
