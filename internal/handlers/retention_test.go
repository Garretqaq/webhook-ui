package handlers

import (
	"testing"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/database"
)

// finishedExecution inserts an execution that finished at the given time.
func finishedExecution(t *testing.T, finished time.Time) int64 {
	t.Helper()
	res, err := database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, started_at, finished_at)
		VALUES ('h1', 'test', 'success', ?, ?)
	`, finished, finished)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestCleanupRemovesOldExecutionsAndTheirLogs(t *testing.T) {
	setupExecDB(t)
	old := finishedExecution(t, time.Now().AddDate(0, 0, -40))
	recent := finishedExecution(t, time.Now().AddDate(0, 0, -5))
	sink := newExecutionLogSink(old, 0)
	sink.WriteChunk("stdout", "old output")
	sink2 := newExecutionLogSink(recent, 0)
	sink2.WriteChunk("stdout", "recent output")

	n, err := CleanupOldExecutions(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("removed %d executions, want the 1 old one", n)
	}

	// The old one is gone, logs and all.
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", old) != 0 {
		t.Error("the old execution survived")
	}
	if count(t, "SELECT COUNT(*) FROM execution_logs WHERE execution_id = ?", old) != 0 {
		t.Error("the old execution's logs survived as orphans — the declared FK cascade is inert")
	}
	// The recent one is untouched.
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", recent) != 1 {
		t.Error("a recent execution was swept too")
	}
	if count(t, "SELECT COUNT(*) FROM execution_logs WHERE execution_id = ?", recent) != 1 {
		t.Error("a recent execution's logs were swept too")
	}
}

func TestCleanupLeavesUnfinishedExecutionsAlone(t *testing.T) {
	setupExecDB(t)
	// Old, but still marked running: it may be genuinely in flight, and a
	// retention sweep must not delete something that is not finished.
	stuck := startedExecution(t)
	if _, err := database.DB.Exec(
		"UPDATE executions SET started_at = ? WHERE id = ?",
		time.Now().AddDate(0, 0, -60), stuck,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := CleanupOldExecutions(time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatal(err)
	}
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", stuck) != 1 {
		t.Error("an unfinished execution was deleted by a retention sweep")
	}
}

// fakeClock lets a test advance the sweep a day at a time.
type fakeClock struct {
	now time.Time
	ch  chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now(), ch: make(chan time.Time, 1)}
}
func (c *fakeClock) Now() time.Time                         { return c.now }
func (c *fakeClock) After(d time.Duration) <-chan time.Time { return c.ch }

func TestSweepRunsAtStartupAndOnADailyTick(t *testing.T) {
	setupExecDB(t)
	old := finishedExecution(t, time.Now().AddDate(0, 0, -40))

	clock := newFakeClock()
	stop := StartRetentionSweep(30, clock)
	defer stop()

	// The startup pass fires before any day has elapsed.
	waitForRowCount(t, "SELECT COUNT(*) FROM executions WHERE id = ?", 0, old)

	// A daily tick fires too. Seed a fresh old row, then advance the clock.
	another := finishedExecution(t, time.Now().AddDate(0, 0, -40))
	clock.ch <- time.Now()
	waitForRowCount(t, "SELECT COUNT(*) FROM executions WHERE id = ?", 0, another)
}

func TestSweepDisabledDeletesNothing(t *testing.T) {
	setupExecDB(t)
	old := finishedExecution(t, time.Now().AddDate(0, 0, -40))
	stop := StartRetentionSweep(0, newFakeClock())
	defer stop()
	time.Sleep(100 * time.Millisecond)
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", old) != 1 {
		t.Error("days <= 0 must never delete, yet a row is gone")
	}
}

// waitForRowCount polls until the row reaches want, since the sweep runs on
// its own goroutine.
func waitForRowCount(t *testing.T, query string, want int, args ...interface{}) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if count(t, query, args...) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("row count never became %d (last %d)", want, count(t, query, args...))
}

func count(t *testing.T, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := database.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCleanupHandlesABacklogLargerThanTheVariableLimit(t *testing.T) {
	setupExecDB(t)
	old := time.Now().AddDate(0, 0, -40)

	// Far beyond SQLite's ~32k host-variable limit, which an IN-list of
	// collected ids would trip on — and the very first sweep of a deployment
	// that never cleaned up faces exactly this.
	const backlog = 33000
	stmt, err := database.DB.Prepare(`
		INSERT INTO executions (hook_id, trigger_source, status, started_at, finished_at)
		VALUES ('h1', 'test', 'success', ?, ?)
	`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < backlog; i++ {
		if _, err := stmt.Exec(old, old); err != nil {
			t.Fatal(err)
		}
	}
	stmt.Close()

	n, err := CleanupOldExecutions(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("the sweep failed at backlog scale: %v", err)
	}
	if n != backlog {
		t.Errorf("removed %d of %d", n, backlog)
	}
}
