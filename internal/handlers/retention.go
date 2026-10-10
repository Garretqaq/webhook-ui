package handlers

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/database"
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

// RetentionSettings is the operator-facing cleanup policy, stored in the
// settings table. Cleanup is off by default; Days is kept even while disabled
// so re-enabling restores the previous window.
type RetentionSettings struct {
	Enabled bool `json:"enabled"`
	Days    int  `json:"days"`
}

const (
	settingKeyRetentionEnabled = "retention_enabled"
	settingKeyRetentionDays    = "retention_days"

	defaultRetentionDays = 30
	minRetentionDays     = 1
	maxRetentionDays     = 3650
)

// LoadRetentionSettings reads the stored policy, falling back to the defaults
// (disabled, 30 days) for unset or unparsable values.
func LoadRetentionSettings() (RetentionSettings, error) {
	s := RetentionSettings{Days: defaultRetentionDays}
	enabled, err := getSetting(settingKeyRetentionEnabled)
	if err != nil {
		return s, err
	}
	s.Enabled = enabled == "true"
	days, err := getSetting(settingKeyRetentionDays)
	if err != nil {
		return s, err
	}
	if n, err := strconv.Atoi(days); err == nil && n >= minRetentionDays && n <= maxRetentionDays {
		s.Days = n
	}
	return s, nil
}

// SaveRetentionSettings persists both keys in one transaction.
func SaveRetentionSettings(s RetentionSettings) error {
	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const upsert = `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	if _, err := tx.Exec(upsert, settingKeyRetentionEnabled, strconv.FormatBool(s.Enabled)); err != nil {
		return err
	}
	if _, err := tx.Exec(upsert, settingKeyRetentionDays, strconv.Itoa(s.Days)); err != nil {
		return err
	}
	return tx.Commit()
}

// RetentionController owns the single running sweep and restarts it whenever
// the policy changes, so a saved setting takes effect immediately.
type RetentionController struct {
	mu    sync.Mutex
	stop  func()
	clock retentionClock
}

// NewRetentionController returns an idle controller; a nil clock means wall time.
func NewRetentionController(clock retentionClock) *RetentionController {
	return &RetentionController{clock: clock}
}

// Apply replaces the running sweep with one for s. When enabled, the new
// sweep cleans up at once and then daily.
//
// Stopping the previous sweep does not abort a cleanup already in flight: that
// transaction is bounded and completes under its own cutoff, but no further
// pass of it will start. Blocking here until it finished would stall the
// settings request behind a possibly huge delete for no extra safety.
func (rc *RetentionController) Apply(s RetentionSettings) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.stop != nil {
		rc.stop()
	}
	days := 0
	if s.Enabled {
		days = s.Days
	}
	rc.stop = StartRetentionSweep(days, rc.clock)
}

// ApplyStored loads the persisted policy and applies it; used at startup.
func (rc *RetentionController) ApplyStored() error {
	s, err := LoadRetentionSettings()
	if err != nil {
		return err
	}
	rc.Apply(s)
	return nil
}

// Stop halts the running sweep, if any. An in-flight cleanup still finishes;
// see Apply.
func (rc *RetentionController) Stop() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.stop != nil {
		rc.stop()
		rc.stop = nil
	}
}
