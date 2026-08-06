package handlers

import (
	"log"
	"time"
)

// StartRetentionSweep runs CleanupOldExecutions once now and then once a day.
// days <= 0 disables it: no goroutine is started and nothing is ever deleted.
// It returns a stop function; main ignores it today since the process simply
// exits, but the sweep must be stoppable for its own tests not to outlive them.
// A nil clock means wall time; tests inject a fake one to advance the sweep
// by hand.
func StartRetentionSweep(days int, clock retentionClock) func() {
	if days <= 0 {
		return func() {}
	}
	if clock == nil {
		clock = realClock{}
	}

	stop := make(chan struct{})
	go func() {
		sweep := func() {
			cutoff := clock.Now().AddDate(0, 0, -days)
			if n, err := CleanupOldExecutions(cutoff); err != nil {
				log.Printf("retention sweep: %v", err)
			} else if n > 0 {
				log.Printf("retention sweep removed %d execution(s) older than %d days", n, days)
			}
		}

		sweep() // at startup, before the first day has elapsed
		for {
			select {
			case <-clock.After(24 * time.Hour):
				sweep()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

// retentionClock is the seam that lets a test drive the sweep by hand instead
// of waiting a day.
type retentionClock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realClock is the production clock.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
