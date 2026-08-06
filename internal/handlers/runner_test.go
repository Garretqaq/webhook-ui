package handlers

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunnerRejectsASecondExecutionOfTheSameHook(t *testing.T) {
	r := NewRunner(4, 10)

	release, err := r.Admit("h1", 1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Admit("h1", 2)
	if !errors.Is(err, ErrHookAlreadyRunning) {
		t.Fatalf("second trigger of a busy hook must be refused, got %v", err)
	}
	var busy *HookBusyError
	if !errors.As(err, &busy) || busy.ExecutionID != 1 {
		t.Errorf("the refusal must name the execution holding the hook, got %+v", busy)
	}

	// A different hook is unaffected.
	if _, err := r.Admit("h2", 3); err != nil {
		t.Errorf("a different hook must still be admitted, got %v", err)
	}

	release()
	if _, err := r.Admit("h1", 4); err != nil {
		t.Errorf("the hook must be admissible again once released, got %v", err)
	}
}

func TestRunnerRefusesBeyondSlotsPlusQueue(t *testing.T) {
	// One running slot with room for two more waiting behind it.
	r := NewRunner(1, 2)

	for i := 1; i <= 3; i++ {
		if _, err := r.Admit("h"+string(rune('0'+i)), int64(i)); err != nil {
			t.Fatalf("admission %d should fit within slots+queue, got %v", i, err)
		}
	}
	if _, err := r.Admit("h4", 4); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("the fourth must be refused, got %v", err)
	}
}

func TestRunnerAdmissionDoesNotDependOnScheduling(t *testing.T) {
	// With no queue, exactly one execution may be in flight — whether or not
	// its goroutine has reached Start yet.
	r := NewRunner(1, 0)
	if _, err := r.Admit("h1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Admit("h2", 2); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("a second execution must be refused with no queue configured, got %v", err)
	}
}

func TestRunnerCapsConcurrentStarts(t *testing.T) {
	const limit = 2
	r := NewRunner(limit, 10)

	var mu sync.Mutex
	running, peak := 0, 0
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		release, err := r.Admit(string(rune('a'+i)), int64(i))
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer release()
			r.Start()
			defer r.Finish()

			mu.Lock()
			running++
			if running > peak {
				peak = running
			}
			mu.Unlock()

			time.Sleep(30 * time.Millisecond)

			mu.Lock()
			running--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if peak > limit {
		t.Errorf("%d executions ran at once, over the limit of %d", peak, limit)
	}
	if peak < 2 {
		t.Errorf("peak concurrency was %d; the limit should not have serialised everything", peak)
	}
}

func TestRunnerQueueDrainsAsSlotsFree(t *testing.T) {
	// One slot, queue of three: admission must keep succeeding as work
	// completes, rather than the queue counter leaking.
	r := NewRunner(1, 3)
	for i := 0; i < 6; i++ {
		release, err := r.Admit("h1", int64(i))
		if err != nil {
			t.Fatalf("admission %d failed: %v", i, err)
		}
		r.Start()
		r.Finish()
		release()
	}
}
