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
