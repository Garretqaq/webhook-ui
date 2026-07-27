package middleware

import (
	"sync"
	"time"
)

// LoginGuard tracks failed login attempts per username+IP and locks out
// combinations that exceed the failure threshold. State is in-memory and
// resets on restart. Records expire lazily: a failure record older than the
// lockout duration is discarded on next access.
type LoginGuard struct {
	maxFailures int
	lockout     time.Duration

	mu       sync.Mutex
	failures map[string]*failureRecord
}

type failureRecord struct {
	count       int
	lastFailure time.Time
}

func NewLoginGuard(maxFailures int, lockout time.Duration) *LoginGuard {
	return &LoginGuard{
		maxFailures: maxFailures,
		lockout:     lockout,
		failures:    make(map[string]*failureRecord),
	}
}

func guardKey(username, ip string) string {
	return username + "|" + ip
}

// LockedRemaining returns the remaining lockout time for the combination,
// or 0 if not locked.
func (g *LoginGuard) LockedRemaining(username, ip string, now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	rec := g.get(username, ip, now)
	if rec == nil || rec.count < g.maxFailures {
		return 0
	}
	remaining := g.lockout - now.Sub(rec.lastFailure)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RecordFailure registers a failed attempt and returns the lockout duration
// if this failure triggered a lock, 0 otherwise.
func (g *LoginGuard) RecordFailure(username, ip string, now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := guardKey(username, ip)
	rec := g.get(username, ip, now)
	if rec == nil {
		rec = &failureRecord{}
		g.failures[key] = rec
	}
	rec.count++
	rec.lastFailure = now
	if rec.count >= g.maxFailures {
		return g.lockout
	}
	return 0
}

// Reset clears the failure count for the combination (on successful login).
func (g *LoginGuard) Reset(username, ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, guardKey(username, ip))
}

// get returns the live record, discarding it if stale. Caller holds mu.
func (g *LoginGuard) get(username, ip string, now time.Time) *failureRecord {
	key := guardKey(username, ip)
	rec, ok := g.failures[key]
	if !ok {
		return nil
	}
	if now.Sub(rec.lastFailure) >= g.lockout {
		delete(g.failures, key)
		return nil
	}
	return rec
}
