package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type SSHHostHandler struct{}

func NewSSHHostHandler() *SSHHostHandler {
	return &SSHHostHandler{}
}

func (h *SSHHostHandler) List(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT id, name, host, port, user, auth_type, host_key, created_at, updated_at
		FROM ssh_hosts ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	items := []models.SSHHost{}
	for rows.Next() {
		var item models.SSHHost
		if err := rows.Scan(&item.ID, &item.Name, &item.Host, &item.Port, &item.User,
			&item.AuthType, &item.HostKey, &item.CreatedAt, &item.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, items)
}

func (h *SSHHostHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var host models.SSHHost
	err := database.DB.QueryRow(`
		SELECT id, name, host, port, user, auth_type, credential, host_key, created_at, updated_at
		FROM ssh_hosts WHERE id = ?
	`, id).Scan(&host.ID, &host.Name, &host.Host, &host.Port, &host.User,
		&host.AuthType, &host.Credential, &host.HostKey, &host.CreatedAt, &host.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "ssh host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, host)
}

func (h *SSHHostHandler) Create(c *gin.Context) {
	var host models.SSHHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host.ID = uuid.New().String()[:8]
	if host.Port == 0 {
		host.Port = 22
	}
	if err := host.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := database.DB.Exec(`
		INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential, host_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, host.ID, host.Name, host.Host, host.Port, host.User, host.AuthType, host.Credential, host.HostKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, host)
}

func (h *SSHHostHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var host models.SSHHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host.ID = id
	if err := host.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := database.DB.Exec(`
		UPDATE ssh_hosts SET name=?, host=?, port=?, user=?, auth_type=?, credential=?, host_key=?, updated_at=?
		WHERE id=?
	`, host.Name, host.Host, host.Port, host.User, host.AuthType, host.Credential, host.HostKey, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "ssh host not found"})
		return
	}

	c.JSON(http.StatusOK, host)
}

func (h *SSHHostHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	result, err := database.DB.Exec("DELETE FROM ssh_hosts WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "ssh host not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// Test dials the host from the request body (id set when testing a saved host).
func (h *SSHHostHandler) Test(c *gin.Context) {
	var host models.SSHHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if host.Port == 0 {
		host.Port = 22
	}
	if err := host.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := services.DialSSH(&host)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer result.Client.Close()

	if _, err := services.RunCommand(result.Client, "echo ok"); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	// TOFU: persist the learned key when testing a saved host.
	if result.LearnedHostKey != "" && host.ID != "" {
		if _, err := database.DB.Exec("UPDATE ssh_hosts SET host_key=? WHERE id=?", result.LearnedHostKey, host.ID); err != nil {
			log.Printf("persist learned host key for %s: %v", host.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"learned_host_key": result.LearnedHostKey,
	})
}
