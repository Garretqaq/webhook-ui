package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/config"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/handlers"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// version is injected at build time via -ldflags "-X main.version=x.y.z"
var version = "dev"

func main() {
	cfg := config.Load()

	if cfg.AdminPassword == "" {
		log.Fatal("ADMIN_PASSWORD environment variable is required")
	}

	if err := database.Init(cfg.DataDir); err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	r := gin.Default()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}

	store := cookie.NewStore([]byte(cfg.SessionSecret))
	r.Use(sessions.Sessions(middleware.SessionKey, store))

	frontendFS, err := fs.Sub(FrontendFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	assetsFS, err := fs.Sub(frontendFS, "assets")
	if err != nil {
		log.Fatal(err)
	}
	r.StaticFS("/assets", http.FS(assetsFS))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// Static assets 404 if not found (don't serve index.html for missing JS/CSS)
		if strings.HasPrefix(path, "/assets/") {
			c.Status(http.StatusNotFound)
			return
		}
		// API routes always 404
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Webhook trigger: GET/POST /hooks/:id handled by routes above, other methods 404
		if strings.HasPrefix(path, "/hooks/") && c.Request.Method != http.MethodPost && c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Serve SPA for all other paths (including /hooks for frontend routing)
		data, err := fs.ReadFile(frontendFS, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "frontend not built")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	loginGuard := middleware.NewLoginGuard(cfg.LoginMaxFailures, time.Duration(cfg.LoginLockoutMinutes)*time.Minute)
	loginLimiter := middleware.NewRateLimiter(cfg.LoginRateLimitPerMin, time.Minute)

	authHandler := handlers.NewAuthHandler(cfg.AdminUsername, cfg.AdminPassword, loginGuard)
	hookHandler := handlers.NewHookHandler()
	executor := services.NewExecutor(cfg.AllowedCommands, cfg.DataDir, cfg.LogTailBytes)
	webhookHandler := handlers.NewWebhookHandler(executor, cfg.LogTailBytes)
	executionHandler := handlers.NewExecutionHandler()
	scriptHandler := handlers.NewScriptHandler(executor)
	sshHostHandler := handlers.NewSSHHostHandler()

	r.POST("/api/auth/login", middleware.LoginRateLimit(loginLimiter), authHandler.Login)
	r.GET("/api/auth/check", authHandler.Check)

	r.POST("/hooks/:id", webhookHandler.Trigger)
	r.GET("/hooks/:id", webhookHandler.Trigger)

	auth := r.Group("/api")
	auth.Use(middleware.AuthRequired())
	{
		auth.POST("/auth/logout", authHandler.Logout)

		auth.GET("/hooks", hookHandler.List)
		auth.POST("/hooks", hookHandler.Create)
		auth.GET("/hooks/:id", hookHandler.Get)
		auth.PUT("/hooks/:id", hookHandler.Update)
		auth.DELETE("/hooks/:id", hookHandler.Delete)

		auth.GET("/executions", executionHandler.List)
		auth.GET("/executions/:id", executionHandler.Get)
		auth.GET("/executions/:id/logs", executionHandler.Logs)

		auth.GET("/scripts", scriptHandler.List)
		auth.POST("/scripts", scriptHandler.Create)
		auth.GET("/scripts/:id", scriptHandler.Get)
		auth.PUT("/scripts/:id", scriptHandler.Update)
		auth.DELETE("/scripts/:id", scriptHandler.Delete)
		auth.POST("/scripts/test", scriptHandler.Test)

		auth.GET("/ssh-hosts", sshHostHandler.List)
		auth.POST("/ssh-hosts", sshHostHandler.Create)
		auth.GET("/ssh-hosts/:id", sshHostHandler.Get)
		auth.PUT("/ssh-hosts/:id", sshHostHandler.Update)
		auth.DELETE("/ssh-hosts/:id", sshHostHandler.Delete)
		auth.POST("/ssh-hosts/test", sshHostHandler.Test)
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Starting webhook-ui %s on %s", version, addr)
	log.Fatal(r.Run(addr))
}
