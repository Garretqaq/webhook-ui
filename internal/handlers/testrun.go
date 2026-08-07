package handlers

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// A script test run is deliberately not an execution. It validates a script
// while it is being edited, so it never reaches the executions table, never
// appears in the execution history, and does not survive a restart —
// everything it produces lives in this process and nowhere else.
const (
	// maxConcurrentTestRuns refuses rather than queues. Queueing is right for a
	// hook whose caller has already gone away; someone watching an editor wants
	// either output or a refusal, not a place in line.
	maxConcurrentTestRuns = 3
	// testRunRetention keeps a finished run readable long enough to reload the
	// page or ride out a dropped connection.
	testRunRetention = 5 * time.Minute
	// testRunSweepInterval is how often expired runs are dropped.
	testRunSweepInterval = time.Minute
)

// ErrTooManyTestRuns is returned when the concurrency cap is already taken.
var ErrTooManyTestRuns = errors.New("too many script test runs are already running")

// testRun is one test run's live state: its log, its status, and the switch
// that stops it. It is also the LogSink the executor writes into, which is
// what lets the log be read while the script is still running.
type testRun struct {
	id string

	mu         sync.Mutex
	chunks     []models.ExecutionLogChunk
	seq        int64
	retained   int
	limitBytes int
	status     string
	finishedAt time.Time
	canceled   bool

	cancel     chan struct{}
	cancelOnce sync.Once
}

func newTestRun(limitBytes int) *testRun {
	return &testRun{
		id:         uuid.New().String()[:8],
		limitBytes: limitBytes,
		status:     models.StatusRunning,
		cancel:     make(chan struct{}),
	}
}

// WriteChunk records output as the script produces it.
func (r *testRun) WriteChunk(stream, chunk string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	r.chunks = append(r.chunks, models.ExecutionLogChunk{Seq: r.seq, Stream: stream, Text: chunk})
	r.retained += len(chunk)

	// Drop the oldest chunks until the retained size is back under the cap. The
	// newest is never dropped: one oversized chunk would otherwise leave the
	// client with nothing at all. Sequence numbers are never reused, so a
	// client's cursor stays valid across the roll-off and the gap stays
	// visible as a jump in the oldest sequence it is told about.
	for r.limitBytes > 0 && r.retained > r.limitBytes && len(r.chunks) > 1 {
		r.retained -= len(r.chunks[0].Text)
		r.chunks = r.chunks[1:]
	}
}

// testRunPage is one poll's worth of a run's log. The shape matches the
// execution log endpoint so a client can read either with the same code.
type testRunPage struct {
	Chunks    []models.ExecutionLogChunk `json:"chunks"`
	NextSeq   int64                      `json:"next_seq"`
	OldestSeq int64                      `json:"oldest_seq"`
	HasMore   bool                       `json:"has_more"`
	Status    string                     `json:"status"`
	Finished  bool                       `json:"finished"`
}

func (r *testRun) page(afterSeq int64) testRunPage {
	r.mu.Lock()
	defer r.mu.Unlock()

	page := testRunPage{
		Chunks:   []models.ExecutionLogChunk{},
		NextSeq:  afterSeq,
		Status:   r.status,
		Finished: !r.finishedAt.IsZero(),
	}
	if len(r.chunks) > 0 {
		page.OldestSeq = r.chunks[0].Seq
	}

	for _, chunk := range r.chunks {
		if chunk.Seq <= afterSeq {
			continue
		}
		if len(page.Chunks) == maxLogChunksPerResponse {
			page.HasMore = true
			break
		}
		page.Chunks = append(page.Chunks, chunk)
		page.NextSeq = chunk.Seq
	}
	return page
}

// finish records the outcome. Nothing may be appended to the log afterwards:
// a client stops polling on the status it reads here, so a later chunk would
// simply never be seen.
func (r *testRun) finish(status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
	r.finishedAt = time.Now()
}

// empty reports whether the run has produced no output at all — including
// output that was produced and then rolled off, which is why the sequence
// counter answers this rather than the retained chunks.
func (r *testRun) empty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq == 0
}

func (r *testRun) running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finishedAt.IsZero()
}

func (r *testRun) statusNow() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// expired reports whether a finished run has outlived the window in which
// somebody might still come back to read it.
func (r *testRun) expired(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.finishedAt.IsZero() && now.Sub(r.finishedAt) > testRunRetention
}

// requestCancel signals the run and reports whether it was still stoppable. A
// second request for a run already stopping succeeds too: the caller asked for
// a state that is on its way, and answering with a conflict would suggest
// otherwise.
func (r *testRun) requestCancel() bool {
	r.mu.Lock()
	if !r.finishedAt.IsZero() {
		r.mu.Unlock()
		return false
	}
	r.canceled = true
	r.mu.Unlock()

	r.cancelOnce.Do(func() { close(r.cancel) })
	return true
}

// TestRunRegistry holds every test run this process is still willing to talk
// about: the ones in flight, and the recently finished ones still inside their
// retention window.
type TestRunRegistry struct {
	mu         sync.Mutex
	runs       map[string]*testRun
	limitBytes int
}

func NewTestRunRegistry(limitBytes int) *TestRunRegistry {
	return &TestRunRegistry{runs: map[string]*testRun{}, limitBytes: limitBytes}
}

// start admits a run, or refuses when the cap is already taken. Only unfinished
// runs count against it — a finished one is waiting to be read, not working.
func (g *TestRunRegistry) start() (*testRun, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	inFlight := 0
	for _, run := range g.runs {
		if run.running() {
			inFlight++
		}
	}
	if inFlight >= maxConcurrentTestRuns {
		return nil, ErrTooManyTestRuns
	}

	run := newTestRun(g.limitBytes)
	g.runs[run.id] = run
	return run, nil
}

func (g *TestRunRegistry) get(id string) (*testRun, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	run, ok := g.runs[id]
	return run, ok
}

// sweep drops runs that finished long enough ago that nobody is coming back
// for them, and reports how many went. A run still in flight is never swept,
// however long it has been going.
func (g *TestRunRegistry) sweep(now time.Time) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	removed := 0
	for id, run := range g.runs {
		if run.expired(now) {
			delete(g.runs, id)
			removed++
		}
	}
	return removed
}

// StartTestRunSweep drops expired runs on a timer. It returns a stop function
// so the sweeper does not outlive its own tests; main lets the process exit
// take care of it.
func StartTestRunSweep(registry *TestRunRegistry) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(testRunSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				registry.sweep(now)
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

// testRunStatus maps an execution result onto the status a test run reports.
func testRunStatus(result *services.ExecuteResult) string {
	switch {
	case result.Canceled:
		return models.StatusCanceled
	case result.TimedOut:
		return models.StatusTimeout
	case !result.Success:
		return models.StatusFailed
	default:
		return models.StatusSuccess
	}
}
