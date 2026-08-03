package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

type HookHandler struct{}

func NewHookHandler() *HookHandler {
	return &HookHandler{}
}

type HookListItem struct {
	models.Hook
	HMACEnabled         bool   `json:"hmac_enabled"`
	TriggerTokenEnabled bool   `json:"trigger_token_enabled"`
	ScriptName          string `json:"script_name"`
	SSHHostName         string `json:"ssh_host_name"`
}

func (h *HookHandler) List(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT h.id, h.name, h.command, h.script_id, h.ssh_host_id, h.working_dir, h.response_message,
		       h.hmac_algorithm, h.pass_arguments, h.pass_headers, h.pass_payload_to,
		       h.created_at, h.updated_at, h.hmac_secret != '' as hmac_enabled,
		       h.trigger_token != '' as trigger_token_enabled,
		       COALESCE(s.name, '') as script_name,
		       COALESCE(sh.name, '') as ssh_host_name
		FROM hooks h
		LEFT JOIN scripts s ON s.id = h.script_id
		LEFT JOIN ssh_hosts sh ON sh.id = h.ssh_host_id
		ORDER BY h.created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	items := []HookListItem{}
	for rows.Next() {
		var item HookListItem
		err := rows.Scan(
			&item.ID, &item.Name, &item.Command, &item.ScriptID, &item.SSHHostID, &item.WorkingDir,
			&item.ResponseMessage, &item.HMACAlgorithm,
			&item.PassArguments, &item.PassHeaders, &item.PassPayloadTo,
			&item.CreatedAt, &item.UpdatedAt, &item.HMACEnabled,
			&item.TriggerTokenEnabled, &item.ScriptName, &item.SSHHostName,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, items)
}

func (h *HookHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var hook models.Hook
	err := database.DB.QueryRow(`
		SELECT id, name, command, script_id, ssh_host_id, working_dir, response_message,
		       hmac_secret, hmac_algorithm, trigger_token, pass_arguments, pass_headers, pass_payload_to,
		       created_at, updated_at
		FROM hooks WHERE id = ?
	`, id).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.ScriptID, &hook.SSHHostID, &hook.WorkingDir,
		&hook.ResponseMessage, &hook.HMACSecret, &hook.HMACAlgorithm, &hook.TriggerToken,
		&hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
		&hook.CreatedAt, &hook.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hook)
}

func (h *HookHandler) Create(c *gin.Context) {
	var hook models.Hook
	if err := c.ShouldBindJSON(&hook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if hook.ID == "" {
		hook.ID = uuid.New().String()[:8]
	}
	if hook.HMACAlgorithm == "" {
		hook.HMACAlgorithm = "sha256"
	}

	if err := hook.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := checkSSHHostExists(hook.SSHHostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := database.DB.Exec(`
		INSERT INTO hooks (id, name, command, script_id, ssh_host_id, working_dir, response_message,
		                  hmac_secret, hmac_algorithm, trigger_token, pass_arguments, pass_headers, pass_payload_to)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, hook.ID, hook.Name, hook.Command, hook.ScriptID, hook.SSHHostID, hook.WorkingDir, hook.ResponseMessage,
		hook.HMACSecret, hook.HMACAlgorithm, hook.TriggerToken,
		hook.PassArguments, hook.PassHeaders, hook.PassPayloadTo)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, hook)
}

func (h *HookHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var hook models.Hook
	if err := c.ShouldBindJSON(&hook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hook.ID = id
	if err := hook.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := checkSSHHostExists(hook.SSHHostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := database.DB.Exec(`
		UPDATE hooks SET name=?, command=?, script_id=?, ssh_host_id=?, working_dir=?, response_message=?,
		                hmac_secret=?, hmac_algorithm=?, trigger_token=?, pass_arguments=?, pass_headers=?,
		                pass_payload_to=?, updated_at=?
		WHERE id=?
	`, hook.Name, hook.Command, hook.ScriptID, hook.SSHHostID, hook.WorkingDir, hook.ResponseMessage,
		hook.HMACSecret, hook.HMACAlgorithm, hook.TriggerToken, hook.PassArguments, hook.PassHeaders,
		hook.PassPayloadTo, time.Now(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}

	c.JSON(http.StatusOK, hook)
}

func (h *HookHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	result, err := database.DB.Exec("DELETE FROM hooks WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// checkSSHHostExists rejects a hook pointing at a host that is not
// configured. Empty means local execution and always passes.
func checkSSHHostExists(sshHostID string) error {
	if sshHostID == "" {
		return nil
	}
	var exists bool
	err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM ssh_hosts WHERE id = ?)", sshHostID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("ssh host not found: %s", sshHostID)
	}
	return nil
}
