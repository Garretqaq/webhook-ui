package handlers

import (
	"bytes"
	"encoding/json"
	"net"
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

func TestTestRunOptionsStayBounded(t *testing.T) {
	// A zero Timeout reaches the executor as "no limit", which is right for an
	// async hook and wrong here: a test run nobody is watching would then hold
	// its slot forever.
	h := &ScriptHandler{logTailBytes: 1024}
	opts := h.testRunOptions(newTestRun(1024))

	if opts.Timeout <= 0 {
		t.Errorf("Timeout = %s; a test run must be bounded", opts.Timeout)
	}
	if opts.TailBytes != 1024 {
		t.Errorf("TailBytes = %d, want the configured cap", opts.TailBytes)
	}
	if opts.Sink == nil {
		t.Error("without a sink the output could not be read until the run ended")
	}
	if opts.Cancel == nil {
		t.Error("without a cancel channel the run could not be stopped")
	}
}

func TestTestRunRollsOffOldestChunksButKeepsTheNewest(t *testing.T) {
	run := newTestRun(10)
	for _, s := range []string{"aaaaa", "bbbbb", "ccccc", "ddddd"} {
		run.WriteChunk(services.StreamStdout, s)
	}

	page := run.page(0)
	var retained string
	for _, chunk := range page.Chunks {
		retained += chunk.Text
	}
	if len(retained) > 10 {
		t.Errorf("retained %d bytes, over the 10 byte cap: %q", len(retained), retained)
	}
	if !strings.Contains(retained, "ddddd") {
		t.Errorf("the newest chunk must survive, got %q", retained)
	}
	if strings.Contains(retained, "aaaaa") {
		t.Errorf("the oldest chunk should have rolled off, got %q", retained)
	}
	// A client sitting at seq 1 compares its cursor against oldest_seq and sees
	// that what lay between them is gone for good.
	if page.OldestSeq <= 1 {
		t.Errorf("oldest_seq = %d; after roll-off it must be past the dropped chunks", page.OldestSeq)
	}
}

func TestTestRunPageReturnsOnlyChunksAfterTheCursor(t *testing.T) {
	run := newTestRun(0)
	run.WriteChunk(services.StreamStdout, "first")
	run.WriteChunk(services.StreamStdout, "second")
	run.WriteChunk(services.StreamStderr, "third")

	page := run.page(1)
	if len(page.Chunks) != 2 {
		t.Fatalf("expected 2 chunks after seq 1, got %d", len(page.Chunks))
	}
	if page.Chunks[0].Text != "second" || page.Chunks[1].Stream != services.StreamStderr {
		t.Errorf("unexpected chunks: %+v", page.Chunks)
	}
	if page.NextSeq != 3 {
		t.Errorf("next_seq = %d, want 3", page.NextSeq)
	}
	if page.Finished || page.Status != models.StatusRunning {
		t.Errorf("a live run must report status=running finished=false, got %q/%v", page.Status, page.Finished)
	}
}

func TestTestRunPageSignalsMoreWhenBacklogExceedsOnePage(t *testing.T) {
	run := newTestRun(0)
	for i := 0; i < maxLogChunksPerResponse+10; i++ {
		run.WriteChunk(services.StreamStdout, "x")
	}
	run.finish(models.StatusSuccess)

	page := run.page(0)
	if len(page.Chunks) != maxLogChunksPerResponse {
		t.Fatalf("expected a full page of %d chunks, got %d", maxLogChunksPerResponse, len(page.Chunks))
	}
	if !page.HasMore {
		t.Error("has_more must be true while a backlog remains, even on a finished run")
	}

	rest := run.page(page.NextSeq)
	if len(rest.Chunks) != 10 {
		t.Errorf("expected the remaining 10 chunks, got %d", len(rest.Chunks))
	}
	if rest.HasMore {
		t.Error("has_more must be false once the backlog is drained")
	}
}

func TestRegistryRefusesPastTheConcurrencyCapAndFinishedRunsFreeASlot(t *testing.T) {
	registry := NewTestRunRegistry(0)

	var runs []*testRun
	for i := 0; i < maxConcurrentTestRuns; i++ {
		run, err := registry.start()
		if err != nil {
			t.Fatalf("run %d refused below the cap: %v", i, err)
		}
		runs = append(runs, run)
	}

	if _, err := registry.start(); err != ErrTooManyTestRuns {
		t.Errorf("past the cap the registry must refuse, got %v", err)
	}

	// A finished run is waiting to be read, not working, so it holds nothing.
	runs[0].finish(models.StatusSuccess)
	if _, err := registry.start(); err != nil {
		t.Errorf("a finished run must free its slot, got %v", err)
	}
}

func TestSweepDropsExpiredRunsButNeverALiveOne(t *testing.T) {
	registry := NewTestRunRegistry(0)
	done, err := registry.start()
	if err != nil {
		t.Fatal(err)
	}
	live, err := registry.start()
	if err != nil {
		t.Fatal(err)
	}
	done.finish(models.StatusSuccess)

	if n := registry.sweep(time.Now()); n != 0 {
		t.Errorf("a run that just finished is still readable, swept %d", n)
	}

	if n := registry.sweep(time.Now().Add(testRunRetention + time.Minute)); n != 1 {
		t.Errorf("swept %d runs, want the one past its retention window", n)
	}
	if _, ok := registry.get(done.id); ok {
		t.Error("the expired run should be gone")
	}
	if _, ok := registry.get(live.id); !ok {
		t.Error("a run still in flight must never be swept, however long it takes")
	}
}

// testRunRouter mounts the test run endpoints on a bare router: session auth
// belongs to the real route tree, not to what these tests are about.
func testRunRouter(t *testing.T, h *ScriptHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/script-test-runs", h.StartTestRun)
	r.GET("/script-test-runs/:id/logs", h.TestRunLogs)
	return r
}

func startTestRun(t *testing.T, r *gin.Engine, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/script-test-runs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var started struct {
		RunID string `json:"run_id"`
	}
	if w.Code == http.StatusAccepted {
		if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
			t.Fatalf("decode body %q: %v", w.Body.String(), err)
		}
	}
	return w, started.RunID
}

func fetchTestRunLogs(t *testing.T, r *gin.Engine, runID string, afterSeq int64) (*httptest.ResponseRecorder, testRunPage) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/script-test-runs/"+runID+"/logs?after_seq="+strconv.FormatInt(afterSeq, 10), nil)
	r.ServeHTTP(w, req)

	var page testRunPage
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode body %q: %v", w.Body.String(), err)
		}
	}
	return w, page
}

// awaitTestRunLog polls the log endpoint until want shows up or time runs out,
// mirroring what the browser does.
func awaitTestRunLog(t *testing.T, r *gin.Engine, runID, want string) testRunPage {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, page := fetchTestRunLogs(t, r, runID, 0)
		var text string
		for _, chunk := range page.Chunks {
			text += chunk.Text
		}
		if strings.Contains(text, want) {
			return page
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q never appeared in the log; last page: %+v", want, page)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func localScriptHandler(t *testing.T) (*ScriptHandler, string) {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	executor := services.NewExecutor([]string{shPath}, t.TempDir())
	return NewScriptHandler(executor, 0, NewTestRunRegistry(0)), shPath
}

func TestStartTestRunAnswersBeforeTheScriptEndsAndStreamsItsOutput(t *testing.T) {
	h, _ := localScriptHandler(t)
	r := testRunRouter(t, h)

	w, runID := startTestRun(t, r,
		`{"interpreter":"sh","content":"echo stage-one; sleep 1.5; echo stage-two"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if runID == "" {
		t.Fatal("no run id to poll")
	}

	// The first stage has to be readable while the script is still sleeping.
	page := awaitTestRunLog(t, r, runID, "stage-one")
	if page.Finished {
		t.Fatal("the run reported finished while the script was still sleeping")
	}

	final := awaitTestRunLog(t, r, runID, "stage-two")
	for !final.Finished {
		time.Sleep(50 * time.Millisecond)
		_, final = fetchTestRunLogs(t, r, runID, 0)
	}
	if final.Status != models.StatusSuccess {
		t.Errorf("status = %q, want success", final.Status)
	}
}

func TestTestRunThatNeverStartedExplainsItselfInTheLog(t *testing.T) {
	// The interpreter is rejected before anything runs, so nothing was ever
	// streamed. With the error left on a discarded result the box would be
	// empty and only the status would hint at what happened.
	executor := services.NewExecutor([]string{"/nothing/allowed"}, t.TempDir())
	h := NewScriptHandler(executor, 0, NewTestRunRegistry(0))
	r := testRunRouter(t, h)

	_, runID := startTestRun(t, r, `{"interpreter":"sh","content":"echo hi"}`)
	page := awaitTestRunLog(t, r, runID, "not allowed")

	if page.Status != models.StatusFailed {
		t.Errorf("status = %q, want failed", page.Status)
	}
	if page.Chunks[len(page.Chunks)-1].Stream != services.StreamStderr {
		t.Error("the reason belongs on stderr, where the view renders it in red")
	}
}

func TestUnknownTestRunIs404(t *testing.T) {
	h, _ := localScriptHandler(t)
	w, _ := fetchTestRunLogs(t, testRunRouter(t, h), "nope", 0)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStartTestRunRefusedPastTheCap(t *testing.T) {
	h, _ := localScriptHandler(t)
	r := testRunRouter(t, h)
	// The occupying runs outlive the assertion, so let them finish before the
	// temp directory they are writing scripts into is torn down.
	t.Cleanup(func() { awaitTestRunsIdle(t, h.testRuns) })

	for i := 0; i < maxConcurrentTestRuns; i++ {
		w, _ := startTestRun(t, r, `{"interpreter":"sh","content":"sleep 1"}`)
		if w.Code != http.StatusAccepted {
			t.Fatalf("run %d refused below the cap: %d %s", i, w.Code, w.Body.String())
		}
	}

	w, _ := startTestRun(t, r, `{"interpreter":"sh","content":"echo hi"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429 once the cap is taken", w.Code)
	}
}

// awaitTestRunsIdle blocks until nothing is still executing in registry.
func awaitTestRunsIdle(t *testing.T, registry *TestRunRegistry) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		registry.mu.Lock()
		busy := 0
		for _, run := range registry.runs {
			if run.running() {
				busy++
			}
		}
		registry.mu.Unlock()

		if busy == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d test run(s) never finished", busy)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRemoteTestRunStreamsBeforeTheSessionEnds covers the wiring the local
// tests cannot: a remote run's chunks have to travel through the SSH branch of
// runScript into the run's own in-memory log, and be readable while the
// session is still open.
func TestRemoteTestRunStreamsBeforeTheSessionEnds(t *testing.T) {
	setupExecDB(t)
	addr := startStagedSSHServer(t, 1500*time.Millisecond)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	if _, err := database.DB.Exec(`
		INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential, target_os)
		VALUES ('h-stage', 'staged', ?, ?, 'tester', 'password', 'x', 'linux')
	`, host, port); err != nil {
		t.Fatal(err)
	}

	executor := services.NewExecutor(nil, t.TempDir())
	h := NewScriptHandler(executor, 0, NewTestRunRegistry(0))
	r := testRunRouter(t, h)

	_, runID := startTestRun(t, r,
		`{"interpreter":"bash","content":"echo hi","ssh_host_id":"h-stage"}`)

	page := awaitTestRunLog(t, r, runID, "stage-one")
	if page.Finished {
		t.Fatal("the run reported finished while the remote session was still open")
	}
	awaitTestRunLog(t, r, runID, "stage-two")
}
