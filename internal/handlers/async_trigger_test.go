package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	return NewWebhookHandler(services.NewExecutor([]string{shPath}, t.TempDir()), 0, runner)
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
	// The row is inserted before admission is decided, so a refusal has to
	// retire it — otherwise it sits queued forever with nothing to run it.
	var stranded int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM executions WHERE hook_id = 'h-b' AND status = ?", models.StatusQueued,
	).Scan(&stranded); err != nil {
		t.Fatal(err)
	}
	if stranded != 0 {
		t.Errorf("%d refused execution(s) left queued forever", stranded)
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
	if got := h.execOptions(hook, 1).Timeout; got != 0 {
		t.Errorf("Timeout = %s, want 0 (no limit)", got)
	}

	bounded := &models.Hook{ID: "h", Async: true, TimeoutSeconds: 7200}
	if got := h.execOptions(bounded, 1).Timeout; got != 2*time.Hour {
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
