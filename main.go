package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/config"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/handlers"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

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
		// Webhook trigger: POST /hooks/:id handled by route above, other methods 404
		if strings.HasPrefix(path, "/hooks/") && c.Request.Method != http.MethodPost {
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

	authHandler := handlers.NewAuthHandler(cfg.AdminPassword)
	hookHandler := handlers.NewHookHandler()
	executor := services.NewExecutor(cfg.AllowedCommands)
	webhookHandler := handlers.NewWebhookHandler(executor)
	executionHandler := handlers.NewExecutionHandler()

	r.POST("/api/auth/login", authHandler.Login)
	r.GET("/api/auth/check", authHandler.Check)

	r.POST("/hooks/:id", webhookHandler.Trigger)

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
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Starting server on %s", addr)
	log.Fatal(r.Run(addr))
}
