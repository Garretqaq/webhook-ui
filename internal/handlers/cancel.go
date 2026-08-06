package handlers

import "sync"

// CancelRegistry holds the abort switch for every execution that can still be
// stopped. Only asynchronous executions register: a synchronous one is bounded
// by its timeout and has a request waiting on it, so there is nothing sensible
// for an operator to interrupt.
//
// Membership is therefore what makes cancellation possible — an execution that
// is not here either finished, belongs to a synchronous hook, or was started
// by a process that no longer exists.
type CancelRegistry struct {
	mu      sync.Mutex
	pending map[int64]chan struct{}
}

func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{pending: map[int64]chan struct{}{}}
}

// Register returns the channel the executor watches for execID.
func (r *CancelRegistry) Register(execID int64) <-chan struct{} {
	ch := make(chan struct{})
	r.mu.Lock()
	r.pending[execID] = ch
	r.mu.Unlock()
	return ch
}

// Unregister drops execID once it can no longer be cancelled.
func (r *CancelRegistry) Unregister(execID int64) {
	r.mu.Lock()
	delete(r.pending, execID)
	r.mu.Unlock()
}

// Cancel signals execID and reports whether there was anything to signal.
// Removing the entry under the same lock keeps a second request from closing
// an already-closed channel.
func (r *CancelRegistry) Cancel(execID int64) bool {
	r.mu.Lock()
	ch, ok := r.pending[execID]
	if ok {
		delete(r.pending, execID)
	}
	r.mu.Unlock()

	if !ok {
		return false
	}
	close(ch)
	return true
}
