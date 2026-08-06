package handlers

import (
	"errors"
	"sync"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

// ErrHookAlreadyRunning is returned when an async hook is triggered while one
// of its own executions is still queued or running.
var ErrHookAlreadyRunning = errors.New("hook is already running")

// ErrQueueFull is returned when both the running slots and the queue behind
// them are full.
var ErrQueueFull = errors.New("execution queue is full")

// Runner bounds how much asynchronous work is in flight.
//
// Executions live in this process only, so admission is decided here rather
// than by querying the executions table: two triggers arriving together would
// both read "nothing running" before either had inserted its row. The startup
// sweep is what reconciles the table with a process that no longer exists.
type Runner struct {
	slots chan struct{}

	mu       sync.Mutex
	inFlight map[string]int64 // hook id -> the execution holding the slot
	capacity int              // running slots plus the queue behind them

	// wg tracks admitted work so a caller can wait for the fleet to drain.
	wg sync.WaitGroup
}

func NewRunner(maxConcurrent, maxQueue int) *Runner {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if maxQueue < 0 {
		maxQueue = 0
	}
	return &Runner{
		slots:    make(chan struct{}, maxConcurrent),
		inFlight: map[string]int64{},
		capacity: maxConcurrent + maxQueue,
	}
}

// Admit reserves a place for hookID. It fails when that hook already occupies
// one, or when the queue behind the running slots is full. The returned
// release must be called once the execution has finished.
func (r *Runner) Admit(hookID string, execID int64) (release func(), err error) {
	r.mu.Lock()
	if running, ok := r.inFlight[hookID]; ok {
		r.mu.Unlock()
		return nil, &HookBusyError{ExecutionID: running}
	}
	// Counted against everything admitted, running or waiting, rather than
	// against a separate queue tally: a tally that only drops when a goroutine
	// reaches Start makes admission depend on scheduling, so the same burst
	// would be accepted or refused from one run to the next.
	if len(r.inFlight) >= r.capacity {
		r.mu.Unlock()
		return nil, ErrQueueFull
	}
	r.inFlight[hookID] = execID
	r.wg.Add(1)
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		delete(r.inFlight, hookID)
		r.mu.Unlock()
		r.wg.Done()
	}, nil
}

// Start blocks until a running slot frees up. Callers run it on the goroutine
// that does the work.
func (r *Runner) Start() {
	r.slots <- struct{}{}
}

// Finish gives the running slot back.
func (r *Runner) Finish() {
	<-r.slots
}

// WaitIdle blocks until every admitted execution has been released.
func (r *Runner) WaitIdle() {
	r.wg.Wait()
}

// HookBusyError reports which execution is holding the hook, so the caller can
// point the client at something it can poll instead of a bare refusal.
type HookBusyError struct {
	ExecutionID int64
}

func (e *HookBusyError) Error() string { return ErrHookAlreadyRunning.Error() }

func (e *HookBusyError) Is(target error) bool { return target == ErrHookAlreadyRunning }

// SweepInterruptedExecutions retires executions the previous process was still
// tracking. Nothing survives a restart — the goroutines are gone and the child
// processes are unreachable — so leaving them as running would hang a spinner
// in the UI forever. The status says only that this service stopped following
// them; a detached remote process may well still be alive.
func SweepInterruptedExecutions() (int64, error) {
	result, err := database.DB.Exec(`
		UPDATE executions SET status = ?, finished_at = ?
		WHERE status IN (?, ?)
	`, models.StatusInterrupted, time.Now(), models.StatusRunning, models.StatusQueued)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
