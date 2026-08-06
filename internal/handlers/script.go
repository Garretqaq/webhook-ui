package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type ScriptHandler struct {
	executor *services.Executor
}

func NewScriptHandler(executor *services.Executor) *ScriptHandler {
	return &ScriptHandler{executor: executor}
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

func (h *ScriptHandler) Test(c *gin.Context) {
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

	// No sink: a test run has no execution row, so its output is only
	// aggregated onto the response.
	result := runScript(h.executor, req.Interpreter, req.Content, req.SSHHostID, req.Args, nil, "", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"output":  result.Output,
		"error":   result.Error,
	})
}
