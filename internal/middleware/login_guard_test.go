package middleware

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestLockoutAfterMaxFailures(t *testing.T) {
	g := NewLoginGuard(5, 15*time.Minute)
	for i := 0; i < 4; i++ {
		if rem := g.RecordFailure("admin", "1.2.3.4", t0); rem != 0 {
			t.Fatalf("failure %d: expected no lockout, got %v", i+1, rem)
		}
	}
	if rem := g.RecordFailure("admin", "1.2.3.4", t0); rem != 15*time.Minute {
		t.Fatalf("5th failure: expected 15m lockout, got %v", rem)
	}
	if rem := g.LockedRemaining("admin", "1.2.3.4", t0.Add(5*time.Minute)); rem != 10*time.Minute {
		t.Fatalf("expected 10m remaining, got %v", rem)
	}
	if rem := g.LockedRemaining("admin", "1.2.3.4", t0.Add(15*time.Minute)); rem != 0 {
		t.Fatalf("expected lockout expired, got %v", rem)
	}
}

func TestLockoutKeyedByUsernameAndIP(t *testing.T) {
	g := NewLoginGuard(5, 15*time.Minute)
	for i := 0; i < 5; i++ {
		g.RecordFailure("admin", "1.2.3.4", t0)
	}
	if rem := g.LockedRemaining("admin", "5.6.7.8", t0); rem != 0 {
		t.Fatalf("different IP should not be locked, got %v", rem)
	}
	if rem := g.LockedRemaining("root", "1.2.3.4", t0); rem != 0 {
		t.Fatalf("different username should not be locked, got %v", rem)
	}
}

func TestResetOnSuccess(t *testing.T) {
	g := NewLoginGuard(5, 15*time.Minute)
	for i := 0; i < 4; i++ {
		g.RecordFailure("admin", "1.2.3.4", t0)
	}
	g.Reset("admin", "1.2.3.4")
	if rem := g.RecordFailure("admin", "1.2.3.4", t0); rem != 0 {
		t.Fatalf("expected counter reset, got lockout %v", rem)
	}
}

func TestLazyExpiry(t *testing.T) {
	g := NewLoginGuard(5, 15*time.Minute)
	for i := 0; i < 4; i++ {
		g.RecordFailure("admin", "1.2.3.4", t0)
	}
	// 16 minutes later: previous failures stale, counter restarts
	if rem := g.RecordFailure("admin", "1.2.3.4", t0.Add(16*time.Minute)); rem != 0 {
		t.Fatalf("expected stale failures expired, got lockout %v", rem)
	}
}
