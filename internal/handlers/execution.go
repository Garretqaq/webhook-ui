package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

type ExecutionHandler struct {
	cancels *CancelRegistry
}

func NewExecutionHandler(cancels *CancelRegistry) *ExecutionHandler {
	return &ExecutionHandler{cancels: cancels}
}

func (h *ExecutionHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	hookID := c.Query("hook_id")

	// output and error are deliberately not selected: both can be megabytes
	// per row, the list never renders them, and the detail endpoint serves
	// them on demand.
	query := `
		SELECT id, hook_id, trigger_source, exec_target, status, started_at, finished_at, parent_execution_id, success_hook_id, success_status, success_error, success_execution_id, success_attempt
		FROM executions
	`
	args := []interface{}{}

	if hookID != "" {
		query += " WHERE hook_id = ?"
		args = append(args, hookID)
	}

	query += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	executions := []models.Execution{}
	for rows.Next() {
		var exec models.Execution
		var finishedAt sql.NullTime
		err := rows.Scan(
			&exec.ID, &exec.HookID, &exec.TriggerSource, &exec.ExecTarget, &exec.Status,
			&exec.StartedAt, &finishedAt, &exec.ParentExecutionID, &exec.SuccessHookID, &exec.SuccessStatus, &exec.SuccessError, &exec.SuccessExecutionID, &exec.SuccessAttempt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if finishedAt.Valid {
			exec.FinishedAt = &finishedAt.Time
		}
		executions = append(executions, exec)
	}

	c.JSON(http.StatusOK, executions)
}

// maxLogChunksPerResponse bounds one poll so a client that has fallen far
// behind cannot pull an entire multi-megabyte log into memory at once.
const maxLogChunksPerResponse = 500

// Logs serves an execution's output incrementally. Clients poll with the
// next_seq from their previous response.
//
// oldest_seq matters because the log is capped and the head is rolled off: a
// client whose cursor is below oldest_seq has lost a stretch it will never
// see, and only it can tell, since the gap is invisible in the chunks alone.
func (h *ExecutionHandler) Logs(c *gin.Context) {
	id := c.Param("id")
	afterSeq, _ := strconv.ParseInt(c.DefaultQuery("after_seq", "0"), 10, 64)

	var status string
	var finishedAt sql.NullTime
	err := database.DB.QueryRow(
		"SELECT status, finished_at FROM executions WHERE id = ?", id,
	).Scan(&status, &finishedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, err := database.DB.Query(`
		SELECT seq, stream, chunk FROM execution_logs
		WHERE execution_id = ? AND seq > ?
		ORDER BY seq LIMIT ?
	`, id, afterSeq, maxLogChunksPerResponse)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	chunks := []models.ExecutionLogChunk{}
	nextSeq := afterSeq
	for rows.Next() {
		var chunk models.ExecutionLogChunk
		if err := rows.Scan(&chunk.Seq, &chunk.Stream, &chunk.Text); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		chunks = append(chunks, chunk)
		nextSeq = chunk.Seq
	}

	var oldestSeq sql.NullInt64
	database.DB.QueryRow(
		"SELECT MIN(seq) FROM execution_logs WHERE execution_id = ?", id,
	).Scan(&oldestSeq)

	// A client that stops polling on finished alone would never see the rest of
	// a backlog longer than one page, so say outright whether more is waiting.
	var hasMore bool
	database.DB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM execution_logs WHERE execution_id = ? AND seq > ?)", id, nextSeq,
	).Scan(&hasMore)

	c.JSON(http.StatusOK, gin.H{
		"chunks":     chunks,
		"next_seq":   nextSeq,
		"oldest_seq": oldestSeq.Int64,
		"has_more":   hasMore,
		"status":     status,
		"finished":   finishedAt.Valid,
	})
}

func (h *ExecutionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var exec models.Execution
	var finishedAt sql.NullTime
	var output, errMsg string

	includeOutput := c.DefaultQuery("include_output", "true") != "false"
	outputColumns, bodyColumn := "output, error", "success_body"
	if !includeOutput {
		outputColumns, bodyColumn = "'', ''", "''"
	}
	// Projection is selected here, not filtered after loading MB-sized log tails.
	err := database.DB.QueryRow(`
 SELECT id, hook_id, trigger_source, exec_target, status, `+outputColumns+`, started_at, finished_at, parent_execution_id, success_hook_id, success_status, success_error, success_execution_id, success_attempt, `+bodyColumn+`
 FROM executions WHERE id = ?`, id).Scan(
		&exec.ID, &exec.HookID, &exec.TriggerSource, &exec.ExecTarget, &exec.Status,
		&output, &errMsg, &exec.StartedAt, &finishedAt, &exec.ParentExecutionID, &exec.SuccessHookID, &exec.SuccessStatus, &exec.SuccessError, &exec.SuccessExecutionID, &exec.SuccessAttempt, &exec.SuccessBody,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if includeOutput {
		exec.Output = &output
		exec.Error = &errMsg
	}

	if finishedAt.Valid {
		exec.FinishedAt = &finishedAt.Time
	}

	rows, err := database.DB.Query("SELECT id, hook_id, status FROM executions WHERE parent_execution_id=? ORDER BY id", exec.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var child models.ExecutionLink
		if err := rows.Scan(&child.ID, &child.HookID, &child.Status); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		exec.SuccessExecutions = append(exec.SuccessExecutions, child)
	}
	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, exec)
}

// SweepInterruptedExecutions retires executions the previous process was still
// tracking. Nothing survives a restart — the goroutines are gone and the child
// processes are unreachable — so leaving them as running would hang a spinner
// in the UI forever. The status says only that this service stopped following
// them; a detached remote process may well still be alive.
func SweepInterruptedExecutions() (int64, error) {
	tx, err := database.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE executions SET success_status='interrupted',success_error='服务重启，交接已中断；未自动补发，请确认目标状态后手动重跑' WHERE success_status='pending'`); err != nil {
		return 0, err
	}
	result, err := tx.Exec(`
		UPDATE executions SET status = ?, finished_at = ?
		WHERE status IN (?, ?)
	`, models.StatusInterrupted, time.Now(), models.StatusRunning, models.StatusQueued)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// Cancel stops an execution that is still in flight.
//
// Only asynchronous executions can be reached: a synchronous one is bounded by
// its timeout and has a request waiting on it. What "stopped" means depends on
// where the hook runs — locally the whole process group is signalled, but a
// remote process that already detached from its SSH session survives, and the
// execution merely stops being tracked.
func (h *ExecutionHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
		return
	}

	var status string
	err = database.DB.QueryRow("SELECT status FROM executions WHERE id = ?", id).Scan(&status)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !h.cancels.Cancel(id) {
		// Either it has already finished, or it belongs to a synchronous hook,
		// or a restart left the row behind — the registry cannot tell which, so
		// report the state the client can actually see.
		c.JSON(http.StatusConflict, gin.H{
			"error":  "execution is not running and cannot be canceled",
			"status": status,
		})
		return
	}

	// The executing goroutine writes the final status; answering before it does
	// keeps the request from waiting on a process that may take a moment to die.
	c.JSON(http.StatusAccepted, gin.H{"message": "cancellation requested"})
}

// CleanupOldExecutions deletes finished executions older than cutoff, and
// their logs. The logs have to go explicitly: the connection never enables the
// foreign_keys pragma, so the ON DELETE CASCADE declared on execution_logs is
// inert — the same reason the hooks FK above executions has always been inert.
// (Enabling it now would turn that on for real and start deleting execution
// history when a hook is deleted, which is a separate behaviour change.)
//
// Only finished rows are touched; an old execution still marked running is
// either genuinely in flight or left behind by a crash, and neither should be
// silently deleted by a retention sweep. Both DELETEs are written as
// subqueries rather than an IN list of collected ids: the first sweep of a
// deployment that never cleaned up faces the whole backlog at once, and a
// parameter list long enough to trip SQLite's variable limit would fail
// loudly every day while deleting nothing.
func CleanupOldExecutions(cutoff time.Time) (int64, error) {
	tx, err := database.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Keep the ancestors of any retained execution so a recent retry still has
	// its source and original payload. UNION also bounds malformed cyclic data.
	const retained = `WITH RECURSIVE retained(id) AS (
 SELECT id FROM executions WHERE finished_at IS NULL OR started_at >= ? OR success_status='pending'
 UNION
 SELECT e.parent_execution_id FROM executions e JOIN retained r ON e.id=r.id WHERE e.parent_execution_id!=0
 ) `
	if _, err := tx.Exec(retained+`DELETE FROM execution_logs WHERE execution_id IN (
 SELECT id FROM executions WHERE id NOT IN (SELECT id FROM retained))`, cutoff); err != nil {
		return 0, err
	}
	result, err := tx.Exec(retained+`DELETE FROM executions WHERE id NOT IN (SELECT id FROM retained)`, cutoff)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
