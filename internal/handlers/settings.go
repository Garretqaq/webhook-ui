package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
)

// SettingsHandler manages the single-instance settings: the external API
// token and the execution-log retention policy. The settings table is generic
// so a later setting does not need another migration.
type SettingsHandler struct {
	retention *RetentionController
	// retentionMu serializes saving the policy with applying it, so two
	// concurrent updates cannot leave the running sweep on an older policy
	// than the one stored.
	retentionMu sync.Mutex
}

// NewSettingsHandler takes the controller that a saved retention policy is
// applied to; nil only persists it (for tests that do not need the sweep).
func NewSettingsHandler(retention *RetentionController) *SettingsHandler {
	return &SettingsHandler{retention: retention}
}

// GetRetention returns the execution-log cleanup policy.
func (h *SettingsHandler) GetRetention(c *gin.Context) {
	s, err := LoadRetentionSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// retentionRequest is the PUT body. The fields are pointers so a body that
// omits one is rejected instead of read as a zero value — an omitted
// "enabled" would otherwise quietly disable cleanup.
type retentionRequest struct {
	Enabled *bool `json:"enabled"`
	Days    *int  `json:"days"`
}

// UpdateRetention validates, persists and immediately applies the policy.
func (h *SettingsHandler) UpdateRetention(c *gin.Context) {
	var req retentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled == nil || req.Days == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled and days are both required"})
		return
	}
	s := RetentionSettings{Enabled: *req.Enabled, Days: *req.Days}
	if s.Days < minRetentionDays || s.Days > maxRetentionDays {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("days must be between %d and %d", minRetentionDays, maxRetentionDays)})
		return
	}
	h.retentionMu.Lock()
	defer h.retentionMu.Unlock()
	if err := SaveRetentionSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.retention != nil {
		h.retention.Apply(s)
	}
	c.JSON(http.StatusOK, s)
}

// GetAPIToken reports whether a token exists and, if so, what it is. It is
// returned in plaintext on purpose: SSH credentials sit in the same database
// unencrypted, so hiding this one string buys no real safety, and an operator
// who loses it would otherwise have to break every external caller at once.
func (h *SettingsHandler) GetAPIToken(c *gin.Context) {
	token, err := getSetting(settingKeyAPIToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured": token != "",
		"token":      token,
	})
}

// RegenerateAPIToken replaces the token, revoking the old one for every
// external caller at once — the deliberate trade-off of a single token.
func (h *SettingsHandler) RegenerateAPIToken(c *gin.Context) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	token := hex.EncodeToString(buf)

	if err := setSetting(settingKeyAPIToken, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configured": true, "token": token})
}

// settingKeyAPIToken is the settings row that holds the external access token.
const settingKeyAPIToken = "api_token"

// getSetting returns the value for key, or empty when unset.
func getSetting(key string) (string, error) {
	var value string
	err := database.DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func setSetting(key, value string) error {
	_, err := database.DB.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// APIToken resolves the configured token, for the token middleware to compare
// against. It is read per request so regenerating takes effect immediately.
func APIToken() (string, error) {
	return getSetting(settingKeyAPIToken)
}
