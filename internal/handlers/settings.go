package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SettingsHandler manages the single-instance settings. Today that is the
// external API token; the settings table is generic so a later setting does
// not need another migration.
type SettingsHandler struct{}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
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

// DisableAPIToken clears it, which switches external access off until a new
// one is generated.
func (h *SettingsHandler) DisableAPIToken(c *gin.Context) {
	if err := setSetting(settingKeyAPIToken, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configured": false, "token": ""})
}

// APIToken resolves the configured token, for the token middleware to compare
// against. It is read per request so regenerating takes effect immediately.
func APIToken() (string, error) {
	return getSetting(settingKeyAPIToken)
}
