package handlers

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type WebhookHandler struct {
	executor     *services.Executor
	logTailBytes int
	runner       *Runner
	cancels      *CancelRegistry
}

func NewWebhookHandler(executor *services.Executor, logTailBytes int, runner *Runner, cancels *CancelRegistry) *WebhookHandler {
	return &WebhookHandler{
		executor:     executor,
		logTailBytes: logTailBytes,
		runner:       runner,
		cancels:      cancels,
	}
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	hookID := c.Param("id")

	var hook models.Hook
	err := database.DB.QueryRow(`
		SELECT id, name, command, script_id, ssh_host_id, working_dir, response_message,
		       hmac_secret, hmac_algorithm, trigger_token, pass_arguments, pass_headers, pass_payload_to,
		       async, timeout_seconds
		FROM hooks WHERE id = ?
	`, hookID).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.ScriptID, &hook.SSHHostID, &hook.WorkingDir,
		&hook.ResponseMessage, &hook.HMACSecret, &hook.HMACAlgorithm, &hook.TriggerToken,
		&hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
		&hook.Async, &hook.TimeoutSeconds,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	if hook.TriggerToken != "" {
		token := c.GetHeader("X-Token")
		if token == "" {
			token = c.Query("token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(hook.TriggerToken)) != 1 {
			h.logExecution(hookID, c.ClientIP(), "failed", "", "invalid trigger token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
	}

	if hook.HMACSecret != "" {
		signature := services.GetSignatureHeader(c.Request.Header)
		validator := services.NewHMACValidator(hook.HMACSecret, hook.HMACAlgorithm)
		if !validator.Validate(payload, signature) {
			h.logExecution(hookID, c.ClientIP(), "failed", "", "invalid HMAC signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	env, args := h.buildCommandInput(&hook, c, payload)

	if hook.Async {
		h.triggerAsync(c, &hook, env, args)
		return
	}

	execID := h.logExecutionStart(hookID, c.ClientIP(), execTarget(hook.SSHHostID), models.StatusRunning)

	result := h.execute(&hook, env, args, h.execOptions(&hook, execID, nil))

	status := models.StatusSuccess
	if !result.Success {
		status = models.StatusFailed
	}
	h.logExecutionEnd(execID, status, result.Output, result.Error)

	if result.Success {
		c.JSON(http.StatusOK, gin.H{
			"message": hook.ResponseMessage,
			"output":  result.Output,
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  result.Error,
			"output": result.Output,
		})
	}
}

// execute runs the hook's bound script, or its free-form command, at the
// execution location configured on the hook.
func (h *WebhookHandler) execute(hook *models.Hook, env map[string]string, args []string, out services.ExecOptions) *services.ExecuteResult {
	if hook.ScriptID == "" {
		return runCommand(h.executor, hook, args, env, out)
	}

	var script models.Script
	err := database.DB.QueryRow(`
		SELECT interpreter, content FROM scripts WHERE id = ?
	`, hook.ScriptID).Scan(&script.Interpreter, &script.Content)
	if err != nil {
		return &services.ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("script not found: %s", hook.ScriptID),
		}
	}
	return runScript(h.executor, script.Interpreter, script.Content, hook.SSHHostID, args, env, hook.WorkingDir, out)
}

func (h *WebhookHandler) buildCommandInput(hook *models.Hook, c *gin.Context, payload []byte) (map[string]string, []string) {
	env := make(map[string]string)
	var args []string

	var payloadData map[string]interface{}
	if len(payload) > 0 {
		json.Unmarshal(payload, &payloadData)
	}

	for k, v := range c.Request.URL.Query() {
		env["QUERY_"+strings.ToUpper(k)] = strings.Join(v, ",")
	}

	for _, headerName := range hook.PassHeaders {
		headerValue := c.GetHeader(headerName)
		envKey := "HEADER_" + strings.ToUpper(strings.ReplaceAll(headerName, "-", "_"))
		env[envKey] = headerValue
	}

	for _, fieldName := range hook.PassArguments {
		if val, ok := payloadData[fieldName]; ok {
			switch v := val.(type) {
			case string:
				args = append(args, v)
			case float64:
				args = append(args, fmt.Sprintf("%v", v))
			default:
				jsonBytes, _ := json.Marshal(v)
				args = append(args, string(jsonBytes))
			}
		}
	}

	if hook.PassPayloadTo == "args" || hook.PassPayloadTo == "both" {
		args = append(args, string(payload))
	}
	if hook.PassPayloadTo == "env" || hook.PassPayloadTo == "both" {
		env["PAYLOAD"] = string(payload)
	}

	return env, args
}

func (h *WebhookHandler) logExecutionStart(hookID, source, target, status string) int64 {
	result, err := database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, exec_target, status, started_at)
		VALUES (?, ?, ?, ?, ?)
	`, hookID, source, target, status, time.Now())
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

func (h *WebhookHandler) logExecutionEnd(id int64, status, output, errMsg string) {
	database.DB.Exec(`
		UPDATE executions SET status=?, output=?, error=?, finished_at=? WHERE id=?
	`, status, output, errMsg, time.Now(), id)
}

func (h *WebhookHandler) logExecution(hookID, source, status, output, errMsg string) {
	database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, output, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hookID, source, status, output, errMsg, time.Now(), time.Now())
}

// execOptions renders the per-call settings for one execution of hook. cancel
// is nil for a synchronous run, which leaves it uncancellable by design.
func (h *WebhookHandler) execOptions(hook *models.Hook, execID int64, cancel <-chan struct{}) services.ExecOptions {
	return services.ExecOptions{
		Sink:      sinkFor(execID, h.logTailBytes),
		TailBytes: h.logTailBytes,
		Timeout:   hook.Timeout(),
		Cancel:    cancel,
	}
}

// triggerAsync accepts the execution and answers immediately with something
// the caller can poll, instead of holding the request open for the whole run.
func (h *WebhookHandler) triggerAsync(c *gin.Context, hook *models.Hook, env map[string]string, args []string) {
	// Admission comes first so a refused trigger records nothing at all.
	slot, err := h.runner.Admit(hook.ID)
	if err != nil {
		var busy *HookBusyError
		if errors.As(err, &busy) {
			body := gin.H{"error": "hook is already running"}
			if busy.ExecutionID != 0 {
				body["running_execution_id"] = busy.ExecutionID
			}
			c.JSON(http.StatusConflict, body)
			return
		}
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	}

	execID := h.logExecutionStart(hook.ID, c.ClientIP(), execTarget(hook.SSHHostID), models.StatusQueued)
	if execID == 0 {
		slot.Release()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record execution"})
		return
	}
	slot.SetExecution(execID)

	cancel := h.cancels.Register(execID)

	go func() {
		defer slot.Release()
		defer h.cancels.Unregister(execID)
		// The request has already been answered, so nothing is left to turn a
		// panic into a response — and unlike the synchronous path there is no
		// gin recovery middleware on this stack to stop it taking the process
		// down with it.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("execution %d panicked: %v", execID, r)
				h.logExecutionEnd(execID, models.StatusFailed, "", fmt.Sprintf("execution panicked: %v", r))
			}
		}()

		h.runner.Start()
		defer h.runner.Finish()

		h.markRunning(execID)
		opts := h.execOptions(hook, execID, cancel)
		result := h.execute(hook, env, args, opts)

		status := models.StatusSuccess
		switch {
		case result.Canceled:
			status = models.StatusCanceled
			// The log is the only place the two are told apart after the fact:
			// a script that died on its own looks identical to one that was
			// stopped, unless the stop leaves a mark of its own.
			if opts.Sink != nil {
				opts.Sink.WriteChunk(services.StreamStderr, "\n--- 已被手动中断 ---\n")
			}
		case !result.Success:
			status = models.StatusFailed
		}
		h.logExecutionEnd(execID, status, result.Output, result.Error)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message":      hook.ResponseMessage,
		"execution_id": execID,
		"status":       models.StatusQueued,
	})
}

func (h *WebhookHandler) markRunning(execID int64) {
	if _, err := database.DB.Exec(
		"UPDATE executions SET status=?, started_at=? WHERE id=?",
		models.StatusRunning, time.Now(), execID,
	); err != nil {
		log.Printf("mark execution %d running: %v", execID, err)
	}
}
