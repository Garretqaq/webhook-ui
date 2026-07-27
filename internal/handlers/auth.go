package handlers

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
)

type AuthHandler struct {
	adminUsername string
	adminPassword string
	guard         *middleware.LoginGuard
}

func NewAuthHandler(adminUsername, adminPassword string, guard *middleware.LoginGuard) *AuthHandler {
	return &AuthHandler{
		adminUsername: adminUsername,
		adminPassword: adminPassword,
		guard:         guard,
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ip := c.ClientIP()
	now := time.Now()

	if remaining := h.guard.LockedRemaining(req.Username, ip, now); remaining > 0 {
		respondLocked(c, remaining)
		return
	}

	usernameOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.adminUsername))
	passwordOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.adminPassword))
	if usernameOK&passwordOK != 1 {
		log.Printf("WARN login failed: username=%q ip=%s", req.Username, ip)
		if remaining := h.guard.RecordFailure(req.Username, ip, now); remaining > 0 {
			log.Printf("WARN login locked: username=%q ip=%s duration=%s", req.Username, ip, remaining)
			respondLocked(c, remaining)
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	h.guard.Reset(req.Username, ip)

	session := sessions.Default(c)
	session.Set(middleware.UserKey, true)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged in"})
}

func respondLocked(c *gin.Context, remaining time.Duration) {
	minutes := int((remaining + time.Minute - 1) / time.Minute)
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":       fmt.Sprintf("已锁定，请 %d 分钟后重试", minutes),
		"retry_after": int(remaining.Seconds()),
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) Check(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get(middleware.UserKey) == true {
		c.JSON(http.StatusOK, gin.H{"authenticated": true})
	} else {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
	}
}
