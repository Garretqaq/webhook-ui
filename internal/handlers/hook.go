package handlers

import (
	"database/sql"
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

func (h *HookHandler) List(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT id, name, command, working_dir, response_message,
		       hmac_algorithm, pass_arguments, pass_headers, pass_payload_to,
		       created_at, updated_at
		FROM hooks ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var hooks []models.Hook
	for rows.Next() {
		var hook models.Hook
		err := rows.Scan(
			&hook.ID, &hook.Name, &hook.Command, &hook.WorkingDir,
			&hook.ResponseMessage, &hook.HMACAlgorithm,
			&hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
			&hook.CreatedAt, &hook.UpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		hooks = append(hooks, hook)
	}

	c.JSON(http.StatusOK, hooks)
}

func (h *HookHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var hook models.Hook
	err := database.DB.QueryRow(`
		SELECT id, name, command, working_dir, response_message,
		       hmac_secret, hmac_algorithm, pass_arguments, pass_headers, pass_payload_to,
		       created_at, updated_at
		FROM hooks WHERE id = ?
	`, id).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.WorkingDir,
		&hook.ResponseMessage, &hook.HMACSecret, &hook.HMACAlgorithm,
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

	_, err := database.DB.Exec(`
		INSERT INTO hooks (id, name, command, working_dir, response_message,
		                  hmac_secret, hmac_algorithm, pass_arguments, pass_headers, pass_payload_to)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, hook.ID, hook.Name, hook.Command, hook.WorkingDir, hook.ResponseMessage,
		hook.HMACSecret, hook.HMACAlgorithm, hook.PassArguments, hook.PassHeaders, hook.PassPayloadTo)

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

	_, err := database.DB.Exec(`
		UPDATE hooks SET name=?, command=?, working_dir=?, response_message=?,
		                hmac_secret=?, hmac_algorithm=?, pass_arguments=?, pass_headers=?,
		                pass_payload_to=?, updated_at=?
		WHERE id=?
	`, hook.Name, hook.Command, hook.WorkingDir, hook.ResponseMessage,
		hook.HMACSecret, hook.HMACAlgorithm, hook.PassArguments, hook.PassHeaders,
		hook.PassPayloadTo, time.Now(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hook)
}

func (h *HookHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	_, err := database.DB.Exec("DELETE FROM hooks WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
