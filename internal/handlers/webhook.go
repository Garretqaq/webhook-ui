package handlers

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type WebhookHandler struct {
	executor *services.Executor
}

func NewWebhookHandler(executor *services.Executor) *WebhookHandler {
	return &WebhookHandler{executor: executor}
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	hookID := c.Param("id")

	var hook models.Hook
	err := database.DB.QueryRow(`
		SELECT id, name, command, script_id, working_dir, response_message,
		       hmac_secret, hmac_algorithm, trigger_token, pass_arguments, pass_headers, pass_payload_to
		FROM hooks WHERE id = ?
	`, hookID).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.ScriptID, &hook.WorkingDir,
		&hook.ResponseMessage, &hook.HMACSecret, &hook.HMACAlgorithm, &hook.TriggerToken,
		&hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
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

	execID := h.logExecutionStart(hookID, c.ClientIP())

	result := h.execute(&hook, env, args)

	status := "success"
	if !result.Success {
		status = "failed"
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

// execute runs the hook's bound script, or its free-form command.
func (h *WebhookHandler) execute(hook *models.Hook, env map[string]string, args []string) *services.ExecuteResult {
	if hook.ScriptID == "" {
		return h.executor.Execute(hook, env, args)
	}

	var script models.Script
	err := database.DB.QueryRow(`
		SELECT interpreter, content, ssh_host_id FROM scripts WHERE id = ?
	`, hook.ScriptID).Scan(&script.Interpreter, &script.Content, &script.SSHHostID)
	if err != nil {
		return &services.ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("script not found: %s", hook.ScriptID),
		}
	}
	return runScript(h.executor, script.Interpreter, script.Content, script.SSHHostID, args, env, hook.WorkingDir)
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

func (h *WebhookHandler) logExecutionStart(hookID, source string) int64 {
	result, err := database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, started_at)
		VALUES (?, ?, 'running', ?)
	`, hookID, source, time.Now())
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
