package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type ScriptHandler struct {
	executor     *services.Executor
	logTailBytes int
	testRuns     *TestRunRegistry
}

func NewScriptHandler(executor *services.Executor, logTailBytes int, testRuns *TestRunRegistry) *ScriptHandler {
	return &ScriptHandler{executor: executor, logTailBytes: logTailBytes, testRuns: testRuns}
}

func (h *ScriptHandler) List(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT id, name, interpreter, description, created_at, updated_at
		FROM scripts ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	items := []models.Script{}
	for rows.Next() {
		var item models.Script
		if err := rows.Scan(&item.ID, &item.Name, &item.Interpreter, &item.Description,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, items)
}

func (h *ScriptHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var s models.Script
	err := database.DB.QueryRow(`
		SELECT id, name, interpreter, content, description, created_at, updated_at
		FROM scripts WHERE id = ?
	`, id).Scan(&s.ID, &s.Name, &s.Interpreter, &s.Content, &s.Description, &s.CreatedAt, &s.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, s)
}

func (h *ScriptHandler) Create(c *gin.Context) {
	var s models.Script
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.ID == "" {
		s.ID = uuid.New().String()[:8]
	}
	if err := s.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := database.DB.Exec(`
		INSERT INTO scripts (id, name, interpreter, content, description)
		VALUES (?, ?, ?, ?, ?)
	`, s.ID, s.Name, s.Interpreter, s.Content, s.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, s)
}

func (h *ScriptHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var s models.Script
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.ID = id
	if err := s.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := database.DB.Exec(`
		UPDATE scripts SET name=?, interpreter=?, content=?, description=?, updated_at=?
		WHERE id=?
	`, s.Name, s.Interpreter, s.Content, s.Description, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
		return
	}

	c.JSON(http.StatusOK, s)
}

func (h *ScriptHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	rows, err := database.DB.Query("SELECT name FROM hooks WHERE script_id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var hookNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		hookNames = append(hookNames, name)
	}
	rows.Close()
	if len(hookNames) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("脚本被以下 Hook 引用，无法删除: %s", strings.Join(hookNames, ", ")),
		})
		return
	}

	result, err := database.DB.Exec("DELETE FROM scripts WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

type TestScriptRequest struct {
	Interpreter string   `json:"interpreter"`
	Content     string   `json:"content"`
	Args        []string `json:"args"`
	SSHHostID   string   `json:"ssh_host_id"`
}

// StartTestRun begins a test run and answers with something to poll, rather
// than holding the request open for however long the script takes. The output
// is readable from the log endpoint while it is still being produced.
func (h *ScriptHandler) StartTestRun(c *gin.Context) {
	var req TestScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.IsValidInterpreter(req.Interpreter) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid interpreter"})
		return
	}
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	run, err := h.testRuns.start()
	if err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	}

	go func() {
		// The request has already been answered, so nothing downstream can turn
		// a panic into a response, and gin's recovery middleware is not on this
		// stack. Without this the whole process would go down with it.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("script test run %s panicked: %v", run.id, r)
				run.WriteChunk(services.StreamStderr, fmt.Sprintf("test run panicked: %v", r))
				run.finish(models.StatusFailed)
			}
		}()

		result := runScript(h.executor, req.Interpreter, req.Content, req.SSHHostID, req.Args, nil, "",
			h.testRunOptions(run))
		finishTestRun(run, result)
	}()

	c.JSON(http.StatusAccepted, gin.H{"run_id": run.id, "status": models.StatusRunning})
}

// testRunOptions renders the settings one test run executes under. The run is
// its own log sink, so its output is readable while it is still being
// produced, and its own cancel switch. The time budget is fixed: a test run
// validates a script that is being edited, and anything that legitimately runs
// longer belongs to an asynchronous hook.
func (h *ScriptHandler) testRunOptions(run *testRun) services.ExecOptions {
	return services.ExecOptions{
		Sink:      run,
		TailBytes: h.logTailBytes,
		Timeout:   services.DefaultTimeout,
		Cancel:    run.cancel,
	}
}

// finishTestRun records the outcome, after making sure the log explains it.
// A run that never reached the interpreter — a rejected binary, an unreachable
// host — produced no chunks at all, so its error is written into the log
// itself; otherwise the box would be empty and the status alone would have to
// account for the failure.
func finishTestRun(run *testRun, result *services.ExecuteResult) {
	if result.TimedOut {
		run.WriteChunk(services.StreamStderr, "\n--- 执行超时 ---\n")
	} else if !result.Success && result.Error != "" && run.empty() {
		run.WriteChunk(services.StreamStderr, result.Error)
	}
	run.finish(testRunStatus(result))
}

// TestRunLogs serves a test run's output incrementally, in the same shape as
// an execution's log so both can be read by the same client code.
func (h *ScriptHandler) TestRunLogs(c *gin.Context) {
	run, ok := h.testRuns.get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "test run not found"})
		return
	}

	afterSeq, _ := strconv.ParseInt(c.DefaultQuery("after_seq", "0"), 10, 64)
	c.JSON(http.StatusOK, run.page(afterSeq))
}
