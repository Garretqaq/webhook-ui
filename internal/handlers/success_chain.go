package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

// Both a connection and a transaction can load the current target configuration.
type hookStore interface {
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}

func loadChainHook(db hookStore, id string) (*models.Hook, error) {
	var hook models.Hook
	err := db.QueryRow(`SELECT id, name, command, script_id, ssh_host_id, working_dir, response_message,
 hmac_secret, hmac_algorithm, trigger_token, pass_arguments, pass_headers, pass_payload_to,
 async, timeout_seconds, success_hook_id, success_body FROM hooks WHERE id=?`, id).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.ScriptID, &hook.SSHHostID, &hook.WorkingDir, &hook.ResponseMessage,
		&hook.HMACSecret, &hook.HMACAlgorithm, &hook.TriggerToken, &hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
		&hook.Async, &hook.TimeoutSeconds, &hook.SuccessHookID, &hook.SuccessBody)
	return &hook, err
}

func startHookExecution(db hookStore, hook *models.Hook, source, status string, parent int64, path []string) (int64, error) {
	pathJSON, err := json.Marshal(path)
	if err != nil {
		return 0, err
	}
	target := "local"
	if hook.SSHHostID != "" {
		var user, host string
		var port int
		err := db.QueryRow("SELECT user,host,port FROM ssh_hosts WHERE id=?", hook.SSHHostID).Scan(&user, &host, &port)
		if err != nil {
			return 0, fmt.Errorf("执行主机不可用: %w", err)
		}
		target = fmt.Sprintf("%s@%s:%d", user, host, port)
	}
	result, err := db.Exec(`INSERT INTO executions (hook_id, trigger_source, exec_target, status, started_at,
 parent_execution_id, chain_path, success_hook_id, success_body) VALUES (?,?,?,?,?,?,?,?,?)`,
		hook.ID, source, target, status, time.Now(), parent, string(pathJSON), hook.SuccessHookID, hook.SuccessBody)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Persist the successful result and pending handoff in one statement. Dispatch
// may happen only after the caller has closed SSH and released its runner slot.
func (h *WebhookHandler) finishHookExecution(id int64, status, output, errMsg string) {
	_, err := database.DB.Exec(`UPDATE executions SET status=?, output=?, error=?, finished_at=?,
 success_status=CASE WHEN ?='success' AND success_hook_id!='' THEN 'pending' ELSE '' END,
 success_attempt=CASE WHEN ?='success' AND success_hook_id!='' THEN 1 ELSE 0 END
 WHERE id=? AND status IN ('running','queued')`, status, output, errMsg, time.Now(), status, status, id)
	if err != nil {
		log.Printf("finish execution %d: %v", id, err)
	}
}

func (h *WebhookHandler) dispatchSuccess(parent int64) {
	if err := h.dispatchPendingSuccess(parent); err != nil {
		_, saveErr := database.DB.Exec(`UPDATE executions SET success_status='failed', success_error=? WHERE id=? AND success_status='pending'`, err.Error(), parent)
		if saveErr != nil {
			log.Printf("record handoff failure for %d: %v (dispatch: %v)", parent, saveErr, err)
		}
	}
}

func (h *WebhookHandler) dispatchPendingSuccess(parent int64) error {
	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var target, body, pathJSON, state string
	err = tx.QueryRow(`SELECT success_hook_id, success_body, chain_path, success_status FROM executions WHERE id=? AND status='success'`, parent).Scan(&target, &body, &pathJSON, &state)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	hook, err := loadChainHook(tx, target)
	if err != nil {
		return fmt.Errorf("后续 Hook 不可用: %w", err)
	}
	var path []string
	if err = json.Unmarshal([]byte(pathJSON), &path); err != nil {
		return err
	}
	for _, visited := range path {
		if visited == target {
			return fmt.Errorf("成功链已执行过 Hook %s，停止循环触发", target)
		}
	}
	slot, err := h.runner.Admit(target)
	if err != nil {
		return err
	}
	launched := false
	defer func() {
		if !launched {
			slot.Release()
		}
	}()
	child, err := startHookExecution(tx, hook, "success-hook", models.StatusQueued, parent, append(path, target))
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE executions SET success_status='dispatched',success_error='',success_execution_id=? WHERE id=?`, child, parent)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Build only the configured payload: no caller query, headers or secrets leak.
	request, err := http.NewRequest("POST", "/", strings.NewReader(body))
	if err != nil {
		return err
	}
	c := &gin.Context{Request: request}
	env, args := h.buildCommandInput(hook, c, []byte(body))
	h.launchHook(hook, child, env, args, slot)
	launched = true
	return nil
}

// Validate and save under the same transaction; serialized SQLite access keeps
// concurrent A->B and B->A updates from both passing against an old graph.
func validateSuccessTarget(tx *sql.Tx, hook *models.Hook) error {
	seen := map[string]bool{hook.ID: true}
	for id := hook.SuccessHookID; id != ""; {
		if seen[id] {
			return fmt.Errorf("成功后触发不能形成循环引用（%s）", id)
		}
		seen[id] = true
		var next string
		if err := tx.QueryRow("SELECT success_hook_id FROM hooks WHERE id=?", id).Scan(&next); err != nil {
			return fmt.Errorf("后续 Hook 不存在或不可用（%s）: %w", id, err)
		}
		id = next
	}
	return nil
}

// RetrySuccessor uses a generation supplied by the detail view. Repeating the
// same HTTP request cannot create another attempt, even if the child finishes
// before the duplicate request arrives.
func (h *WebhookHandler) RetrySuccessor(c *gin.Context) {
	var request struct {
		Attempt *int `json:"attempt"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Attempt == nil {
		c.JSON(400, gin.H{"error": "需要当前交接 attempt"})
		return
	}
	parent, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || parent <= 0 {
		c.JSON(400, gin.H{"error": "invalid execution id"})
		return
	}
	code, err := claimSuccessRetry(parent, *request.Attempt)
	if err != nil {
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	h.dispatchSuccess(parent)
	var state, message string
	var child int64
	if err := database.DB.QueryRow("SELECT success_status,success_error,success_execution_id FROM executions WHERE id=?", parent).Scan(&state, &message, &child); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if state != "dispatched" {
		c.JSON(409, gin.H{"error": message})
		return
	}
	c.JSON(202, gin.H{"execution_id": child, "status": "queued"})
}

func claimSuccessRetry(parent int64, attempt int) (int, error) {
	tx, err := database.DB.Begin()
	if err != nil {
		return 500, err
	}
	defer tx.Rollback()
	var state, status, target string
	var generation int
	var child int64
	err = tx.QueryRow(`SELECT status,success_status,success_hook_id,success_attempt,success_execution_id FROM executions WHERE id=?`, parent).Scan(&status, &state, &target, &generation, &child)
	if err == sql.ErrNoRows {
		return 404, fmt.Errorf("execution not found")
	}
	if err != nil {
		return 500, err
	}
	if status != "success" || target == "" || generation != attempt {
		return 409, fmt.Errorf("交接状态已变化，请刷新后重试")
	}
	eligible := state == "failed" || state == "interrupted"
	if state == "dispatched" {
		var childStatus string
		if err := tx.QueryRow("SELECT status FROM executions WHERE id=?", child).Scan(&childStatus); err != nil {
			return 409, fmt.Errorf("后续执行记录不可用，无法确认重跑资格")
		}
		eligible = childStatus == models.StatusFailed || childStatus == models.StatusCanceled || childStatus == models.StatusInterrupted || childStatus == models.StatusTimeout
	}
	if !eligible {
		return 409, fmt.Errorf("后续任务未失败或仍在执行，不能重复启动")
	}
	_, err = tx.Exec(`UPDATE executions SET success_status='pending',success_error='',success_attempt=success_attempt+1 WHERE id=?`, parent)
	if err != nil {
		return 500, err
	}
	if err = tx.Commit(); err != nil {
		return 500, err
	}
	return 202, nil
}
