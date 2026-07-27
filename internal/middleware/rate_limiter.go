package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a per-key fixed-window rate limiter.
// State is in-memory and resets on restart. Stale per-key windows are
// swept lazily (no background goroutine).
type RateLimiter struct {
	limit  int
	window time.Duration

	mu        sync.Mutex
	windows   map[string]*window
	lastSweep time.Time
}

type window struct {
	count int
	start time.Time
}

func NewRateLimiter(limit int, windowDur time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  windowDur,
		windows: make(map[string]*window),
	}
}

// Allow reports whether a request for key at time now is within the limit.
func (r *RateLimiter) Allow(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Sub(r.lastSweep) >= r.window {
		r.lastSweep = now
		for k, w := range r.windows {
			if now.Sub(w.start) >= r.window {
				delete(r.windows, k)
			}
		}
	}
	w, ok := r.windows[key]
	if !ok || now.Sub(w.start) >= r.window {
		w = &window{start: now}
		r.windows[key] = w
	}
	w.count++
	return w.count <= r.limit
}

// LoginRateLimit returns middleware that rate limits requests by client IP.
func LoginRateLimit(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.Allow(c.ClientIP(), time.Now()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后重试"})
			c.Abort()
			return
		}
		c.Next()
	}
}
