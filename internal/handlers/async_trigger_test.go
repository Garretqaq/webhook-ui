package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// asyncHook inserts a hook whose script sleeps, so a test can inspect the
// world while the execution is still in flight.
func asyncHook(t *testing.T, id, script string, timeoutSeconds int) {
	t.Helper()
	if _, err := database.DB.Exec(
		`INSERT INTO scripts (id, name, interpreter, content) VALUES (?, 'slow', 'sh', ?)`,
		"s-"+id, script,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO hooks (id, name, command, script_id, async, timeout_seconds)
		 VALUES (?, 'async hook', '', ?, 1, ?)`,
		id, "s-"+id, timeoutSeconds,
	); err != nil {
		t.Fatal(err)
	}
}

func triggerHook(t *testing.T, h *WebhookHandler, hookID string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/hooks/:id", h.Trigger)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hooks/"+hookID, nil))

	body := map[string]any{}
	json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func asyncTestHandler(t *testing.T, runner *Runner) *WebhookHandler {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	// Executions outlive the request that started them, and every test swaps
	// the package-level database handle, so a test that returns while its own
	// executions are still writing would corrupt the next one.
	t.Cleanup(runner.WaitIdle)
	return NewWebhookHandler(services.NewExecutor([]string{shPath}, t.TempDir()), 0, runner, NewCancelRegistry())
}

func executionStatus(t *testing.T, execID int64) string {
	t.Helper()
	var status string
	if err := database.DB.QueryRow(
		"SELECT status FROM executions WHERE id = ?", execID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func waitForStatus(t *testing.T, execID int64, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if executionStatus(t, execID) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("execution %d never reached %q (last seen %q)", execID, want, executionStatus(t, execID))
}

func TestAsyncTriggerReturns202WithoutWaiting(t *testing.T) {
	setupExecDB(t)
	asyncHook(t, "h-async", "sleep 1; echo done", 0)
	h := asyncTestHandler(t, NewRunner(4, 16))

	start := time.Now()
	w, body := triggerHook(t, h, "h-async")
	elapsed := time.Since(start)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", w.Code, w.Body.String())
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("the response waited %s; an async trigger must not hold the request open", elapsed)
	}
	execID := int64(body["execution_id"].(float64))
	if execID == 0 {
		t.Fatal("the response must carry an execution_id to poll")
	}
	if body["status"] != models.StatusQueued {
		t.Errorf("status = %v, want %q", body["status"], models.StatusQueued)
	}

	waitForStatus(t, execID, models.StatusSuccess)
}

func TestAsyncHookRefusesAConcurrentTriggerWith409(t *testing.T) {
	setupExecDB(t)
	asyncHook(t, "h-busy", "sleep 1", 0)
	h := asyncTestHandler(t, NewRunner(4, 16))

	_, first := triggerHook(t, h, "h-busy")
	firstID := int64(first["execution_id"].(float64))

	w, body := triggerHook(t, h, "h-busy")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", w.Code, w.Body.String())
	}
	if got := int64(body["running_execution_id"].(float64)); got != firstID {
		t.Errorf("the refusal named execution %d, want the running one %d", got, firstID)
	}

	// And the refusal itself is not recorded as an execution.
	var rows int
	database.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE hook_id = 'h-busy'").Scan(&rows)
	if rows != 1 {
		t.Errorf("%d execution rows for the hook; only the one that actually ran should exist", rows)
	}
}

func TestAsyncTriggerRefusedByAFullQueueDoesNotStrandTheRow(t *testing.T) {
	setupExecDB(t)
	asyncHook(t, "h-a", "sleep 1", 0)
	asyncHook(t, "h-b", "sleep 1", 0)
	h := asyncTestHandler(t, NewRunner(1, 0))

	triggerHook(t, h, "h-a")

	w, body := triggerHook(t, h, "h-b")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body %s", w.Code, w.Body.String())
	}
	// Admission is decided before anything is recorded, so a refused trigger
	// leaves no row at all — a caller retrying against a busy service would
	// otherwise fill the execution log with runs that never happened.
	var recorded int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM executions WHERE hook_id = 'h-b'",
	).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Errorf("a refused trigger recorded %d execution(s); it should record none", recorded)
	}
	if body["error"] == nil {
		t.Error("the refusal should say why")
	}
}

func TestSyncHookIsUnaffectedByTheSingleInstanceRule(t *testing.T) {
	setupExecDB(t)
	if _, err := database.DB.Exec(
		`INSERT INTO scripts (id, name, interpreter, content) VALUES ('s-sync', 'x', 'sh', 'echo hi')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO hooks (id, name, command, script_id, async, timeout_seconds)
		 VALUES ('h-sync', 'sync', '', 's-sync', 0, 300)`,
	); err != nil {
		t.Fatal(err)
	}
	h := asyncTestHandler(t, NewRunner(4, 16))

	for i := 0; i < 3; i++ {
		w, _ := triggerHook(t, h, "h-sync")
		if w.Code != http.StatusOK {
			t.Fatalf("sync trigger %d got %d, want 200 — the 409 rule must not reach sync hooks", i, w.Code)
		}
	}
}

func TestAsyncHookWithNoTimeoutOutlivesTheOldFiveMinuteBound(t *testing.T) {
	// The whole point of the feature: timeout_seconds 0 must not be turned into
	// a default. Verified through the options the handler builds rather than by
	// actually waiting, which no test can afford.
	hook := &models.Hook{ID: "h", Async: true, TimeoutSeconds: 0}
	h := &WebhookHandler{}
	if got := h.execOptions(hook, 1, nil).Timeout; got != 0 {
		t.Errorf("Timeout = %s, want 0 (no limit)", got)
	}

	bounded := &models.Hook{ID: "h", Async: true, TimeoutSeconds: 7200}
	if got := h.execOptions(bounded, 1, nil).Timeout; got != 2*time.Hour {
		t.Errorf("Timeout = %s, want 2h", got)
	}
}

func TestSweepRetiresExecutionsLeftBehindByARestart(t *testing.T) {
	setupExecDB(t)
	for _, status := range []string{models.StatusRunning, models.StatusQueued, models.StatusSuccess} {
		if _, err := database.DB.Exec(
			`INSERT INTO executions (hook_id, trigger_source, status, started_at)
			 VALUES ('h1', 'test', ?, CURRENT_TIMESTAMP)`, status,
		); err != nil {
			t.Fatal(err)
		}
	}

	n, err := SweepInterruptedExecutions()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("swept %d rows, want the 2 unfinished ones", n)
	}

	var interrupted, unfinished int
	database.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE status = ?", models.StatusInterrupted).Scan(&interrupted)
	database.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE finished_at IS NULL").Scan(&unfinished)
	if interrupted != 2 {
		t.Errorf("%d executions marked interrupted, want 2", interrupted)
	}
	if unfinished != 1 {
		t.Errorf("%d executions still lack finished_at; only the pre-existing success row should", unfinished)
	}
}

func TestSyncHookDefaultsItsTimeoutInsteadOfBeingRejected(t *testing.T) {
	// A client written before the field exists sends nothing, which arrives as
	// 0 — and 0 means no limit, which a sync hook may not have. Rejecting it
	// would break every existing integration.
	hook := models.Hook{ID: "h1", Name: "legacy", Command: "echo hi"}
	applyTimeoutDefault(&hook)

	if err := hook.Validate(); err != nil {
		t.Fatalf("a sync hook sending no timeout must still validate, got %v", err)
	}
	if hook.TimeoutSeconds != 300 {
		t.Errorf("TimeoutSeconds = %d, want the 5 minute default it always had", hook.TimeoutSeconds)
	}

	// An async hook keeps its unlimited setting.
	async := models.Hook{ID: "h2", Name: "long", Command: "echo hi", Async: true}
	applyTimeoutDefault(&async)
	if async.TimeoutSeconds != 0 {
		t.Errorf("an async hook's 0 must survive as 'no limit', got %d", async.TimeoutSeconds)
	}
}

func TestScriptTestRunStaysBounded(t *testing.T) {
	// The script tester answers a request that is waiting on it, so it must
	// never inherit the unlimited run async hooks are allowed. A zero Timeout
	// reaches the executor as "no limit" and hangs the request forever.
	opts := (&ScriptHandler{logTailBytes: 1024}).execOptions()
	if opts.Timeout <= 0 {
		t.Errorf("Timeout = %s; a synchronous endpoint must be bounded", opts.Timeout)
	}
	if opts.TailBytes != 1024 {
		t.Errorf("TailBytes = %d, want the configured cap", opts.TailBytes)
	}
}

func TestAsyncExecutionPanicDoesNotTakeTheProcessDown(t *testing.T) {
	setupExecDB(t)
	runner := NewRunner(2, 8)
	t.Cleanup(runner.WaitIdle)

	execID := startedExecution(t)
	slot, err := runner.Admit("h-panic")
	if err != nil {
		t.Fatal(err)
	}
	slot.SetExecution(execID)

	h := &WebhookHandler{runner: runner}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer slot.Release()
		defer func() {
			if r := recover(); r != nil {
				h.logExecutionEnd(execID, models.StatusFailed, "", "execution panicked")
			}
		}()
		runner.Start()
		defer runner.Finish()
		panic("boom")
	}()

	<-done
	// Surviving to here is the assertion: an unrecovered panic on this stack
	// would have killed the test binary, not failed the test.
	if got := executionStatus(t, execID); got != models.StatusFailed {
		t.Errorf("status = %q, want the execution retired as failed", got)
	}
	// The slot must be back, or one panic would wedge the hook forever.
	readmitted, err := runner.Admit("h-panic")
	if err != nil {
		t.Errorf("the slot was not released after the panic: %v", err)
	} else {
		readmitted.Release()
	}
}

func cancelExecution(t *testing.T, h *ExecutionHandler, execID int64) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/executions/:id/cancel", h.Cancel)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		"/executions/"+strconv.FormatInt(execID, 10)+"/cancel", nil))

	body := map[string]any{}
	json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestCancelStopsARunningAsyncExecution(t *testing.T) {
	setupExecDB(t)
	asyncHook(t, "h-cancel", "sleep 30", 0)
	cancels := NewCancelRegistry()
	runner := NewRunner(4, 16)
	t.Cleanup(runner.WaitIdle)

	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	h := NewWebhookHandler(services.NewExecutor([]string{shPath}, t.TempDir()), 1024, runner, cancels)

	_, body := triggerHook(t, h, "h-cancel")
	execID := int64(body["execution_id"].(float64))
	waitForStatus(t, execID, models.StatusRunning)

	w, _ := cancelExecution(t, NewExecutionHandler(cancels), execID)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", w.Code, w.Body.String())
	}

	waitForStatus(t, execID, models.StatusCanceled)

	// The log has to say it was stopped, or after the fact a cancellation is
	// indistinguishable from the script dying on its own.
	var logged string
	rows, err := database.DB.Query(
		"SELECT chunk FROM execution_logs WHERE execution_id = ? ORDER BY seq", execID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var chunk string
		rows.Scan(&chunk)
		logged += chunk
	}
	if !strings.Contains(logged, "中断") {
		t.Errorf("the log carries no cancellation marker: %q", logged)
	}
}

func TestCancelIsRefusedForAnExecutionThatIsNotRunning(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	if _, err := database.DB.Exec(
		"UPDATE executions SET status='success', finished_at=CURRENT_TIMESTAMP WHERE id=?", execID,
	); err != nil {
		t.Fatal(err)
	}

	w, body := cancelExecution(t, NewExecutionHandler(NewCancelRegistry()), execID)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if body["status"] != models.StatusSuccess {
		t.Errorf("the refusal should report the state the client can see, got %v", body["status"])
	}
}

func TestCancelUnknownExecutionIs404(t *testing.T) {
	setupExecDB(t)
	w, _ := cancelExecution(t, NewExecutionHandler(NewCancelRegistry()), 4242)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSyncExecutionIsNotCancellable(t *testing.T) {
	// Sync executions never register, so the endpoint has nothing to signal —
	// which is the intended design, not an oversight.
	cancels := NewCancelRegistry()
	if cancels.Cancel(1) {
		t.Error("an unregistered execution must not report as cancelled")
	}

	// And a second cancel of the same execution is refused rather than closing
	// an already-closed channel.
	cancels.Register(2)
	if !cancels.Cancel(2) {
		t.Fatal("the first cancel should succeed")
	}
	if cancels.Cancel(2) {
		t.Error("the second cancel must be refused, not panic on a closed channel")
	}
}
