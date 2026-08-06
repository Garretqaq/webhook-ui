package handlers

import (
	"errors"
	"sync"
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
	inFlight map[string]*Slot // hook id -> the admission holding it
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
		inFlight: map[string]*Slot{},
		capacity: maxConcurrent + maxQueue,
	}
}

// Slot is one admitted execution's claim on the runner.
type Slot struct {
	runner *Runner
	hookID string
	execID int64 // guarded by runner.mu
}

// Admit reserves a place for hookID before anything is recorded, so a refused
// trigger leaves no trace: a caller retrying against a busy hook would
// otherwise pile up execution rows for runs that never happened.
func (r *Runner) Admit(hookID string) (*Slot, error) {
	r.mu.Lock()
	if held, ok := r.inFlight[hookID]; ok {
		busy := &HookBusyError{ExecutionID: held.execID}
		r.mu.Unlock()
		return nil, busy
	}
	// Counted against everything admitted, running or waiting, rather than
	// against a separate queue tally: a tally that only drops when a goroutine
	// reaches Start makes admission depend on scheduling, so the same burst
	// would be accepted or refused from one run to the next.
	if len(r.inFlight) >= r.capacity {
		r.mu.Unlock()
		return nil, ErrQueueFull
	}
	slot := &Slot{runner: r, hookID: hookID}
	r.inFlight[hookID] = slot
	r.wg.Add(1)
	r.mu.Unlock()

	return slot, nil
}

// SetExecution records which execution took the slot, so a trigger refused
// while it is held can be told what to poll instead.
func (s *Slot) SetExecution(execID int64) {
	s.runner.mu.Lock()
	s.execID = execID
	s.runner.mu.Unlock()
}

// Release hands the admission back.
func (s *Slot) Release() {
	s.runner.mu.Lock()
	delete(s.runner.inFlight, s.hookID)
	s.runner.mu.Unlock()
	s.runner.wg.Done()
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
// point the client at something it can poll instead of a bare refusal. The id
// is 0 in the narrow window between admission and the row being inserted.
type HookBusyError struct {
	ExecutionID int64
}

func (e *HookBusyError) Error() string { return ErrHookAlreadyRunning.Error() }

func (e *HookBusyError) Is(target error) bool { return target == ErrHookAlreadyRunning }
