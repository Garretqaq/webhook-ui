package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

type ExecutionHandler struct{}

func NewExecutionHandler() *ExecutionHandler {
	return &ExecutionHandler{}
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
