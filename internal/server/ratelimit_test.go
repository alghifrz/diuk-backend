package server

import "testing"

func TestMemoryLimiterAllowsThenRejects(t *testing.T) {
	l := newMemoryLimiter(1)
	if !l.allow("ip") {
		t.Fatal("first request should pass")
	}
	if l.allow("ip") {
		t.Fatal("burst of 1 should reject the second request")
	}
	if !l.allow("other") {
		t.Fatal("a different key should have its own bucket")
	}
}

func TestMemoryLimiterNilIsFailOpen(t *testing.T) {
	var l *memoryLimiter
	if !l.allow("ip") {
		t.Fatal("nil limiter must not block")
	}
}
