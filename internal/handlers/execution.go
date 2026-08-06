package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
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

	query := `
		SELECT id, hook_id, trigger_source, exec_target, status, output, error, started_at, finished_at
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
			&exec.Output, &exec.Error, &exec.StartedAt, &finishedAt,
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

	err := database.DB.QueryRow(`
		SELECT id, hook_id, trigger_source, exec_target, status, output, error, started_at, finished_at
		FROM executions WHERE id = ?
	`, id).Scan(
		&exec.ID, &exec.HookID, &exec.TriggerSource, &exec.ExecTarget, &exec.Status,
		&exec.Output, &exec.Error, &exec.StartedAt, &finishedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if finishedAt.Valid {
		exec.FinishedAt = &finishedAt.Time
	}

	c.JSON(http.StatusOK, exec)
}

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
// silently deleted by a retention sweep.
func CleanupOldExecutions(cutoff time.Time) (int64, error) {
	ids, err := database.DB.Query(`
		SELECT id FROM executions
		WHERE finished_at IS NOT NULL AND started_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	var toDelete []int64
	for ids.Next() {
		var id int64
		if err := ids.Scan(&id); err != nil {
			ids.Close()
			return 0, err
		}
		toDelete = append(toDelete, id)
	}
	ids.Close()
	if len(toDelete) == 0 {
		return 0, nil
	}

	marks := strings.Repeat("?,", len(toDelete))
	marks = marks[:len(marks)-1]
	args := make([]interface{}, len(toDelete))
	for i, id := range toDelete {
		args[i] = id
	}

	if _, err := database.DB.Exec(
		"DELETE FROM execution_logs WHERE execution_id IN ("+marks+")", args...,
	); err != nil {
		return 0, err
	}
	result, err := database.DB.Exec(
		"DELETE FROM executions WHERE id IN ("+marks+")", args...,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
