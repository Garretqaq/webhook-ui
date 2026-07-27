package middleware

import (
	"testing"
	"time"
)

func TestRateLimitAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	for i := 0; i < 10; i++ {
		if !rl.Allow("1.2.3.4", t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("1.2.3.4", t0.Add(10*time.Second)) {
		t.Fatal("11th request should be rejected")
	}
}

func TestRateLimitWindowResets(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	for i := 0; i < 10; i++ {
		rl.Allow("1.2.3.4", t0)
	}
	if !rl.Allow("1.2.3.4", t0.Add(time.Minute)) {
		t.Fatal("request after window should be allowed")
	}
}

func TestRateLimitKeyedByIP(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	rl.Allow("1.2.3.4", t0)
	if rl.Allow("1.2.3.4", t0) {
		t.Fatal("second request from same IP should be rejected")
	}
	if !rl.Allow("5.6.7.8", t0) {
		t.Fatal("request from different IP should be allowed")
	}
}
