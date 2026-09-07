package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

func chainRouter(t *testing.T) (*gin.Engine, *WebhookHandler) {
	t.Helper()
	setupExecDB(t)
	h := asyncTestHandler(t, NewRunner(1, 0))
	r := setupHookRouter()
	hooks := NewHookHandler()
	r.GET("/api/hooks", hooks.List)
	r.GET("/api/hooks/:id", hooks.Get)
	r.DELETE("/api/hooks/:id", hooks.Delete)
	r.POST("/hooks/:id", h.Trigger)
	r.POST("/api/executions/:id/retry-successor", h.RetrySuccessor)
	executions := NewExecutionHandler(h.cancels)
	r.GET("/api/executions", executions.List)
	r.GET("/api/executions/:id", executions.Get)
	return r, h
}

func chainConfigure(t *testing.T, r *gin.Engine, id, target, payload string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "name": id, "script_id": "s-" + id, "async": true, "timeout_seconds": 5, "pass_payload_to": "env", "success_hook_id": target, "success_body": payload})
	w := hookRequest(t, r, http.MethodPut, "/api/hooks/"+id, string(body))
	if w.Code != 200 {
		t.Fatalf("configure: %d %s", w.Code, w.Body)
	}
}

func chainDetail(t *testing.T, r *gin.Engine, id int64) map[string]any {
	t.Helper()
	w := hookRequest(t, r, http.MethodGet, fmt.Sprintf("/api/executions/%d", id), "")
	if w.Code != 200 {
		t.Fatalf("detail: %s", w.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func chainWait(t *testing.T, r *gin.Engine, id int64, predicate func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body := chainDetail(t, r, id)
		if predicate(body) {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execution did not settle: %+v", chainDetail(t, r, id))
	return nil
}

func TestSuccessHookDispatchesAfterReleasingOnlySlot(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "echo parent", 5)
	asyncHook(t, "child", `printf '%s' "$PAYLOAD"; exit 7`, 5)
	chainConfigure(t, r, "child", "", "")
	chainConfigure(t, r, "parent", "child", `{"command":"hibernate"}`)
	w, body := triggerHook(t, h, "parent")
	if w.Code != 202 {
		t.Fatal(w.Body)
	}
	id := int64(body["execution_id"].(float64))
	parent := chainWait(t, r, id, func(b map[string]any) bool { return b["success_status"] == "dispatched" })
	childID := int64(parent["success_execution_id"].(float64))
	child := chainWait(t, r, childID, func(b map[string]any) bool { return b["status"] == models.StatusFailed })
	if child["output"] != `{"command":"hibernate"}` || child["parent_execution_id"] != float64(id) {
		t.Fatalf("child: %+v", child)
	}
	if chainDetail(t, r, id)["status"] != "success" {
		t.Fatal("child failure changed parent")
	}
}

func TestSuccessHookRejectsCyclesAndReferencedDeletion(t *testing.T) {
	r, _ := chainRouter(t)
	for _, id := range []string{"a", "b", "c"} {
		asyncHook(t, id, "true", 5)
	}
	chainConfigure(t, r, "b", "c", `{}`)
	chainConfigure(t, r, "a", "b", `{}`)
	for _, target := range []string{"c", "a", "missing"} {
		body := fmt.Sprintf(`{"name":"c","script_id":"s-c","success_hook_id":%q,"success_body":"{}"}`, target)
		w := hookRequest(t, r, http.MethodPut, "/api/hooks/c", body)
		if w.Code != 400 {
			t.Fatalf("target %s accepted: %d %s", target, w.Code, w.Body)
		}
	}
	w := hookRequest(t, r, http.MethodDelete, "/api/hooks/b", "")
	if w.Code != 409 {
		t.Fatalf("referenced deletion: %d %s", w.Code, w.Body)
	}
}

func TestSuccessHookStopsRuntimeCycleAfterConfigurationChanges(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "a", "sleep 0.2; echo a", 5)
	asyncHook(t, "b", "echo b", 5)
	chainConfigure(t, r, "a", "b", `{}`)
	_, body := triggerHook(t, h, "a")
	id := int64(body["execution_id"].(float64))
	chainConfigure(t, r, "a", "", "")
	chainConfigure(t, r, "b", "a", `{}`)
	parent := chainWait(t, r, id, func(b map[string]any) bool { return b["success_status"] == "dispatched" })
	child := chainWait(t, r, int64(parent["success_execution_id"].(float64)), func(b map[string]any) bool { return b["success_status"] == "failed" })
	if child["status"] != "success" || child["success_error"] == "" {
		t.Fatalf("cycle outcome: %+v", child)
	}
}

func TestSuccessHookRestartRetiresPendingWithoutRedispatch(t *testing.T) {
	r, _ := chainRouter(t)
	// Fixture represents a crash after recording success but before dispatch.
	_, err := database.DB.Exec(`INSERT INTO executions (hook_id,status,finished_at,success_hook_id,success_body,success_status) VALUES ('parent','success',CURRENT_TIMESTAMP,'child','{}','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SweepInterruptedExecutions(); err != nil {
		t.Fatal(err)
	}
	body := chainDetail(t, r, 1)
	if body["success_status"] != "interrupted" || body["status"] != "success" || body["success_execution_id"] != float64(0) {
		t.Fatalf("restart: %+v", body)
	}
}

func TestSuccessHookRetryUsesSnapshotAndDoesNotRepeatParent(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "echo parent", 5)
	asyncHook(t, "child", `printf '%s' "$PAYLOAD"; exit 7`, 5)
	asyncHook(t, "last", "echo last", 5)
	chainConfigure(t, r, "child", "last", `{}`)
	chainConfigure(t, r, "parent", "child", `{"original":true}`)
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	first := chainDetail(t, r, id)
	chainConfigure(t, r, "parent", "last", `{"changed":true}`)
	if _, err := database.DB.Exec(`UPDATE scripts SET content='printf ''%s'' "$PAYLOAD"' WHERE id='s-child'`); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/executions/%d/retry-successor", id)
	request := fmt.Sprintf(`{"attempt":%v}`, first["success_attempt"])
	w := hookRequest(t, r, http.MethodPost, path, request)
	if w.Code != 202 {
		t.Fatalf("retry: %d %s", w.Code, w.Body)
	}
	h.runner.WaitIdle()
	parent := chainDetail(t, r, id)
	if parent["status"] != "success" || len(parent["success_executions"].([]any)) != 2 {
		t.Fatalf("parent: %+v", parent)
	}
	child := chainDetail(t, r, int64(parent["success_execution_id"].(float64)))
	if child["output"] != `{"original":true}` || child["hook_id"] != "child" || child["success_status"] != "dispatched" {
		t.Fatalf("retry did not use original handoff/current target: %+v", child)
	}
	w = hookRequest(t, r, http.MethodPost, path, request)
	if w.Code != 409 {
		t.Fatalf("stale retry accepted: %d %s", w.Code, w.Body)
	}
}

func TestSuccessHookCanDispatchToSSHHost(t *testing.T) {
	r, h := chainRouter(t)
	addr := startStagedSSHServer(t, 0)
	host, port, _ := net.SplitHostPort(addr)
	if _, err := database.DB.Exec(`INSERT INTO ssh_hosts (id,name,host,port,user,auth_type,credential) VALUES ('remote','remote',?,?,'tester','password','x')`, host, port); err != nil {
		t.Fatal(err)
	}
	asyncHook(t, "parent", "true", 5)
	asyncHook(t, "child", "true", 5)
	w := hookRequest(t, r, http.MethodPut, "/api/hooks/child", `{"name":"child","script_id":"s-child","ssh_host_id":"remote","async":true,"timeout_seconds":5}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	chainConfigure(t, r, "parent", "child", `{}`)
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	parent := chainWait(t, r, id, func(b map[string]any) bool { return b["success_status"] == "dispatched" })
	child := chainWait(t, r, int64(parent["success_execution_id"].(float64)), func(b map[string]any) bool { return b["status"] == "success" })
	if !strings.Contains(child["output"].(string), "stage-two") {
		t.Fatal(child)
	}
}

func TestSuccessHookConfigurationRoundTripAndConcurrentCycles(t *testing.T) {
	r, _ := chainRouter(t)
	asyncHook(t, "a", "true", 5)
	asyncHook(t, "b", "true", 5)
	chainConfigure(t, r, "a", "b", `{"value":42}`)
	w := hookRequest(t, r, http.MethodGet, "/api/hooks/a", "")
	var hook models.Hook
	if err := json.Unmarshal(w.Body.Bytes(), &hook); err != nil {
		t.Fatal(err)
	}
	if hook.SuccessHookID != "b" || hook.SuccessBody != `{"value":42}` {
		t.Fatalf("roundtrip: %+v", hook)
	}
	w = hookRequest(t, r, http.MethodGet, "/api/hooks", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"success_hook_id":"b"`) {
		t.Fatal(w.Body)
	}
	chainConfigure(t, r, "a", "", "")
	results := make(chan int, 2)
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		go func(id, target string) {
			req := httptest.NewRequest("PUT", "/api/hooks/"+id, strings.NewReader(fmt.Sprintf(`{"name":%q,"script_id":"s-%s","success_hook_id":%q,"success_body":"{}"}`, id, id, target)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			results <- w.Code
		}(pair[0], pair[1])
	}
	first, second := <-results, <-results
	if !((first == 200 && second == 400) || (first == 400 && second == 200)) {
		t.Fatalf("concurrent cycle: %d %d", first, second)
	}
	w = hookRequest(t, r, "POST", "/api/hooks", `{"id":"invalid","name":"invalid","command":"true","success_hook_id":"a","success_body":"not json"}`)
	if w.Code != 400 {
		t.Fatalf("invalid JSON accepted: %s", w.Body)
	}
}

func TestSuccessHookSyncParentRecordsQueueFailureAndRetry(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "busy", "sleep 0.3", 5)
	asyncHook(t, "parent", "echo sync", 5)
	asyncHook(t, "child", "echo child", 5)
	w := hookRequest(t, r, "PUT", "/api/hooks/parent", `{"name":"parent","script_id":"s-parent","success_hook_id":"child","success_body":"{}"}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	triggerHook(t, h, "busy")
	w, _ = triggerHook(t, h, "parent")
	if w.Code != 200 {
		t.Fatalf("sync parent should succeed: %s", w.Body)
	}
	parent := chainDetail(t, r, 2)
	if parent["success_status"] != "failed" || parent["success_error"] != "execution queue is full" || parent["status"] != "success" {
		t.Fatalf("queue failure: %+v", parent)
	}
	h.runner.WaitIdle()
	w = hookRequest(t, r, "POST", "/api/executions/2/retry-successor", `{"attempt":1}`)
	if w.Code != 202 {
		t.Fatalf("queue retry: %d %s", w.Code, w.Body)
	}
	h.runner.WaitIdle()
	parent = chainDetail(t, r, 2)
	if parent["success_status"] != "dispatched" || parent["success_attempt"] != float64(2) {
		t.Fatal(parent)
	}
}

func TestSuccessHookDoesNotRunAfterFailureTimeoutOrCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, script, status string
		timeout              int
		cancel               bool
	}{
		{"failed", "exit 9", "failed", 5, false},
		{"timeout", "sleep 3", "failed", 1, false},
		{"canceled", "sleep 3", "canceled", 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, h := chainRouter(t)
			r.POST("/api/executions/:id/cancel", NewExecutionHandler(h.cancels).Cancel)
			asyncHook(t, "parent", tc.script, tc.timeout)
			asyncHook(t, "child", "echo unexpected", 5)
			chainConfigure(t, r, "parent", "child", `{}`)
			if _, err := database.DB.Exec("UPDATE hooks SET timeout_seconds=? WHERE id='parent'", tc.timeout); err != nil {
				t.Fatal(err)
			}
			_, body := triggerHook(t, h, "parent")
			id := int64(body["execution_id"].(float64))
			if tc.cancel {
				w := hookRequest(t, r, "POST", fmt.Sprintf("/api/executions/%d/cancel", id), "")
				if w.Code != 202 {
					t.Fatal(w.Body)
				}
			}
			h.runner.WaitIdle()
			parent := chainDetail(t, r, id)
			if parent["status"] != tc.status || parent["success_status"] != "" || parent["success_execution_id"] != float64(0) {
				t.Fatalf("non-success dispatch: %+v", parent)
			}
		})
	}
}

func TestSuccessHookRetryRejectsDuplicateAndMissingTarget(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "true", 5)
	asyncHook(t, "child", "exit 9", 5)
	chainConfigure(t, r, "parent", "child", `{}`)
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	// Remove the configuration reference, then delete the target. The snapshot
	// remains in history, but retry must not silently pick the new target.
	chainConfigure(t, r, "parent", "", "")
	if w := hookRequest(t, r, "DELETE", "/api/hooks/child", ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	path := fmt.Sprintf("/api/executions/%d/retry-successor", id)
	w := hookRequest(t, r, "POST", path, `{"attempt":1}`)
	if w.Code != 409 {
		t.Fatalf("deleted target accepted: %d %s", w.Code, w.Body)
	}
	parent := chainDetail(t, r, id)
	if parent["success_status"] != "failed" || parent["success_error"] == "" || parent["success_attempt"] != float64(2) {
		t.Fatal(parent)
	}
	w = hookRequest(t, r, "POST", path, `{"attempt":1}`)
	if w.Code != 409 {
		t.Fatalf("stale request accepted: %d", w.Code)
	}
}

func TestSuccessHookPendingHandoffSurvivesRetention(t *testing.T) {
	r, _ := chainRouter(t)
	_, err := database.DB.Exec(`INSERT INTO executions (hook_id,status,started_at,finished_at,success_hook_id,success_body,success_status) VALUES ('parent','success',?,CURRENT_TIMESTAMP,'child','{}','pending')`, time.Now().AddDate(0, 0, -60))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CleanupOldExecutions(time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatal(err)
	}
	body := chainDetail(t, r, 1)
	if body["success_status"] != "pending" {
		t.Fatal(body)
	}
}

func TestSuccessHookClosesParentSSHBeforeChildRuns(t *testing.T) {
	r, h := chainRouter(t)
	closedFile := filepath.Join(t.TempDir(), "ssh-closed")
	addr := startStagedSSHServer(t, 0, func() { _ = os.WriteFile(closedFile, []byte("closed"), 0600) })
	host, port, _ := net.SplitHostPort(addr)
	if _, err := database.DB.Exec(`INSERT INTO ssh_hosts (id,name,host,port,user,auth_type,credential,target_os) VALUES ('remote','remote',?,?,'tester','password','x','windows')`, host, port); err != nil {
		t.Fatal(err)
	}
	asyncHook(t, "parent", "Write-Output done", 5)
	if _, err := database.DB.Exec("UPDATE scripts SET interpreter='powershell' WHERE id='s-parent'"); err != nil {
		t.Fatal(err)
	}
	// Allow transport notification to arrive; without Close this child fails.
	asyncHook(t, "child", fmt.Sprintf(`i=0; while [ ! -f '%s' ] && [ "$i" -lt 30 ]; do sleep 0.01; i=$((i+1)); done; test -f '%s' && echo closed-before-child`, closedFile, closedFile), 5)
	w := hookRequest(t, r, "PUT", "/api/hooks/parent", `{"name":"parent","script_id":"s-parent","ssh_host_id":"remote","async":true,"timeout_seconds":5,"success_hook_id":"child","success_body":"{}"}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	parent := chainDetail(t, r, id)
	child := chainDetail(t, r, int64(parent["success_execution_id"].(float64)))
	if child["status"] != "success" || child["output"] != "closed-before-child\n" {
		t.Fatalf("SSH not closed before child: %+v", child)
	}
}

func TestSuccessHookConcurrentRetryCreatesOnlyOneAttempt(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "true", 5)
	asyncHook(t, "child", "exit 9", 5)
	chainConfigure(t, r, "parent", "child", `{}`)
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/executions/%d/retry-successor", id), strings.NewReader(`{"attempt":1}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			results <- w.Code
		}()
	}
	first, second := <-results, <-results
	if !((first == 202 && second == 409) || (first == 409 && second == 202)) {
		t.Fatalf("retry responses: %d %d", first, second)
	}
	h.runner.WaitIdle()
	parent := chainDetail(t, r, id)
	if len(parent["success_executions"].([]any)) != 2 {
		t.Fatalf("duplicate retries: %+v", parent)
	}
}

func TestSuccessHookChildInsertFailureIsVisibleAndRetryable(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "true", 5)
	asyncHook(t, "child", "true", 5)
	chainConfigure(t, r, "parent", "child", `{}`)
	if _, err := database.DB.Exec(`CREATE TRIGGER reject_child BEFORE INSERT ON executions WHEN NEW.parent_execution_id != 0 BEGIN SELECT RAISE(ABORT,'child storage unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	parent := chainDetail(t, r, id)
	if parent["status"] != "success" || parent["success_status"] != "failed" || !strings.Contains(parent["success_error"].(string), "child storage unavailable") || parent["success_execution_id"] != float64(0) {
		t.Fatalf("insert failure: %+v", parent)
	}
	if _, err := database.DB.Exec("DROP TRIGGER reject_child"); err != nil {
		t.Fatal(err)
	}
	w := hookRequest(t, r, "POST", fmt.Sprintf("/api/executions/%d/retry-successor", id), `{"attempt":1}`)
	if w.Code != 202 {
		t.Fatal(w.Body)
	}
	h.runner.WaitIdle()
}

func TestSuccessHookRestartKeepsExistingChildAssociation(t *testing.T) {
	r, _ := chainRouter(t)
	_, err := database.DB.Exec(`INSERT INTO executions (id,hook_id,status,finished_at,success_hook_id,success_body,success_status,success_execution_id,success_attempt) VALUES (1,'parent','success',CURRENT_TIMESTAMP,'child','{}','dispatched',2,1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.DB.Exec(`INSERT INTO executions (id,hook_id,status,parent_execution_id) VALUES (2,'child','queued',1)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SweepInterruptedExecutions(); err != nil {
		t.Fatal(err)
	}
	parent := chainDetail(t, r, 1)
	child := chainDetail(t, r, 2)
	if parent["success_status"] != "dispatched" || parent["success_execution_id"] != float64(2) || child["status"] != "interrupted" {
		t.Fatalf("restart association: %+v %+v", parent, child)
	}
}

func TestSuccessHookStatePollingOmitsLogsAndPayload(t *testing.T) {
	r, h := chainRouter(t)
	asyncHook(t, "parent", "echo large-log", 5)
	asyncHook(t, "child", "true", 5)
	chainConfigure(t, r, "parent", "child", `{"large":"payload"}`)
	_, body := triggerHook(t, h, "parent")
	id := int64(body["execution_id"].(float64))
	h.runner.WaitIdle()
	w := hookRequest(t, r, "GET", fmt.Sprintf("/api/executions/%d?include_output=false", id), "")
	var state map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"output", "error", "success_body"} {
		if _, ok := state[key]; ok {
			t.Fatalf("lightweight state includes %s", key)
		}
	}
	if state["success_status"] != "dispatched" {
		t.Fatal(state)
	}
}

func TestSuccessHookRetentionKeepsAncestorsOfRecentAttempts(t *testing.T) {
	r, _ := chainRouter(t)
	old := time.Now().AddDate(0, 0, -60)
	for _, row := range []struct {
		id, parent int
		status     string
		started    time.Time
	}{
		{1, 0, "success", old}, {2, 1, "success", old}, {3, 2, "running", time.Now()},
	} {
		_, err := database.DB.Exec(`INSERT INTO executions (id,hook_id,status,started_at,finished_at,parent_execution_id,success_status,success_body) VALUES (?,'chain',?,?,CASE WHEN ?='running' THEN NULL ELSE CURRENT_TIMESTAMP END,?,'dispatched','{"keep":true}')`, row.id, row.status, row.started, row.status, row.parent)
		if err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Now().AddDate(0, 0, -30)
	if _, err := CleanupOldExecutions(cutoff); err != nil {
		t.Fatal(err)
	}
	if chainDetail(t, r, 1)["success_body"] != `{"keep":true}` {
		t.Fatal("lost original payload")
	}
	if _, err := database.DB.Exec("UPDATE executions SET status='failed',finished_at=CURRENT_TIMESTAMP WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	if _, err := CleanupOldExecutions(cutoff); err != nil {
		t.Fatal(err)
	}
	chainDetail(t, r, 2) // A recent failed retry must retain its source and payload too.
	if _, err := database.DB.Exec("UPDATE executions SET started_at=?", old); err != nil {
		t.Fatal(err)
	}
	if n, err := CleanupOldExecutions(cutoff); err != nil || n != 3 {
		t.Fatalf("expired chain was not cleaned: %d %v", n, err)
	}
}
