package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// startedExecution inserts a running execution row and returns its id.
func startedExecution(t *testing.T) int64 {
	t.Helper()
	res, err := database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, started_at)
		VALUES ('h1', 'test', 'running', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func logRowCount(t *testing.T, execID int64) int {
	t.Helper()
	var n int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM execution_logs WHERE execution_id = ?", execID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExecutionLogSinkPersistsChunksInOrder(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 0)

	sink.WriteChunk(services.StreamStdout, "one")
	sink.WriteChunk(services.StreamStderr, "two")
	sink.WriteChunk(services.StreamStdout, "three")

	rows, err := database.DB.Query(
		"SELECT seq, stream, chunk FROM execution_logs WHERE execution_id = ? ORDER BY seq", execID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var seq int64
		var stream, chunk string
		if err := rows.Scan(&seq, &stream, &chunk); err != nil {
			t.Fatal(err)
		}
		got = append(got, stream+":"+chunk)
	}
	want := []string{"stdout:one", "stderr:two", "stdout:three"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExecutionLogSinkRollsOffOldestChunks(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 10)

	for _, s := range []string{"aaaaa", "bbbbb", "ccccc", "ddddd"} {
		sink.WriteChunk(services.StreamStdout, s)
	}

	var remaining string
	rows, err := database.DB.Query(
		"SELECT chunk FROM execution_logs WHERE execution_id = ? ORDER BY seq", execID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var chunk string
		if err := rows.Scan(&chunk); err != nil {
			t.Fatal(err)
		}
		remaining += chunk
	}

	if len(remaining) > 10 {
		t.Errorf("retained %d bytes, over the 10 byte cap: %q", len(remaining), remaining)
	}
	if !strings.Contains(remaining, "ddddd") {
		t.Errorf("the newest chunk must survive, got %q", remaining)
	}
	if strings.Contains(remaining, "aaaaa") {
		t.Errorf("the oldest chunk should have rolled off, got %q", remaining)
	}
}

func TestExecutionLogSinkKeepsNewestChunkOverCap(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 4)

	sink.WriteChunk(services.StreamStdout, "this single chunk is larger than the cap")

	if n := logRowCount(t, execID); n != 1 {
		t.Fatalf("an oversized lone chunk must not be rolled off, %d rows left", n)
	}
}

func TestSinkForReturnsNilWithoutExecutionRow(t *testing.T) {
	if sink := sinkFor(0, 1024); sink != nil {
		t.Error("no execution row means no sink; the executor treats nil as aggregate-only")
	}
}

// logsResponse mirrors the JSON the Logs endpoint returns.
type logsResponse struct {
	Chunks []struct {
		Seq    int64  `json:"seq"`
		Stream string `json:"stream"`
		Text   string `json:"text"`
	} `json:"chunks"`
	NextSeq   int64  `json:"next_seq"`
	OldestSeq int64  `json:"oldest_seq"`
	HasMore   bool   `json:"has_more"`
	Status    string `json:"status"`
	Finished  bool   `json:"finished"`
}

func fetchLogs(t *testing.T, execID int64, query string) (*httptest.ResponseRecorder, logsResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/executions/:id/logs", NewExecutionHandler(NewCancelRegistry()).Logs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/executions/"+strconv.FormatInt(execID, 10)+"/logs"+query, nil)
	r.ServeHTTP(w, req)

	var body logsResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body %q: %v", w.Body.String(), err)
		}
	}
	return w, body
}

func TestLogsReturnsOnlyChunksAfterCursor(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 0)
	sink.WriteChunk(services.StreamStdout, "first")
	sink.WriteChunk(services.StreamStdout, "second")
	sink.WriteChunk(services.StreamStderr, "third")

	w, body := fetchLogs(t, execID, "?after_seq=1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if len(body.Chunks) != 2 {
		t.Fatalf("expected 2 chunks after seq 1, got %d", len(body.Chunks))
	}
	if body.Chunks[0].Text != "second" || body.Chunks[1].Stream != "stderr" {
		t.Errorf("unexpected chunks: %+v", body.Chunks)
	}
	if body.NextSeq != 3 {
		t.Errorf("next_seq = %d, want 3", body.NextSeq)
	}
	if body.Status != "running" || body.Finished {
		t.Errorf("a running execution must report finished=false, got status=%q finished=%v", body.Status, body.Finished)
	}
}

func TestLogsReportsOldestSeqSoClientsCanDetectAGap(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 10)
	for _, s := range []string{"aaaaa", "bbbbb", "ccccc", "ddddd"} {
		sink.WriteChunk(services.StreamStdout, s)
	}

	// A client sitting at seq 1 compares its cursor against oldest_seq and
	// sees that everything between them is gone for good.
	_, body := fetchLogs(t, execID, "?after_seq=0")
	if body.OldestSeq <= 1 {
		t.Errorf("oldest_seq = %d; after roll-off it must be past the dropped chunks, otherwise a client at seq 1 cannot detect the gap", body.OldestSeq)
	}
}

func TestLogsMarksFinishedExecution(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	if _, err := database.DB.Exec(
		"UPDATE executions SET status='success', finished_at=CURRENT_TIMESTAMP WHERE id=?", execID,
	); err != nil {
		t.Fatal(err)
	}

	_, body := fetchLogs(t, execID, "")
	if !body.Finished || body.Status != "success" {
		t.Errorf("finished execution reported as status=%q finished=%v", body.Status, body.Finished)
	}
}

func TestLogsUnknownExecutionIs404(t *testing.T) {
	setupExecDB(t)
	w, _ := fetchLogs(t, 9999, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestLogsSignalsMoreWhenBacklogExceedsOnePage(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	sink := newExecutionLogSink(execID, 0)
	for i := 0; i < maxLogChunksPerResponse+10; i++ {
		sink.WriteChunk(services.StreamStdout, "x")
	}
	// Finished, so a client that stopped on `finished` alone would strand the
	// chunks past the first page.
	if _, err := database.DB.Exec(
		"UPDATE executions SET status='success', finished_at=CURRENT_TIMESTAMP WHERE id=?", execID,
	); err != nil {
		t.Fatal(err)
	}

	_, body := fetchLogs(t, execID, "?after_seq=0")
	if len(body.Chunks) != maxLogChunksPerResponse {
		t.Fatalf("expected a full page of %d chunks, got %d", maxLogChunksPerResponse, len(body.Chunks))
	}
	if !body.HasMore {
		t.Error("has_more must be true while a backlog remains, even on a finished execution")
	}

	_, rest := fetchLogs(t, execID, "?after_seq="+strconv.FormatInt(body.NextSeq, 10))
	if len(rest.Chunks) != 10 {
		t.Errorf("expected the remaining 10 chunks, got %d", len(rest.Chunks))
	}
	if rest.HasMore {
		t.Error("has_more must be false once the backlog is drained")
	}
}

func TestTailLimitReachesTheLocalExecutorToo(t *testing.T) {
	setupExecDB(t)
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	executor := services.NewExecutor([]string{shPath}, t.TempDir())

	// ExecOptions carries the cap for both execution locations; it used to be
	// unwrapped on the local path and silently dropped.
	result := runScript(executor, shPath, "for i in 1 2 3 4 5 6 7 8 9; do echo LINE$i; done",
		"", nil, nil, "", services.ExecOptions{TailBytes: 12})

	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if len(result.Output) > 12 {
		t.Errorf("the local path ignored TailBytes: %d bytes retained", len(result.Output))
	}
}
