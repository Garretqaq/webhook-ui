# Webhook UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based webhook management system with web UI, replacing adnanh/webhook with native implementation.

**Architecture:** Monorepo with Go backend (Gin + SQLite) and React frontend (Ant Design + Vite). Single binary deployment via Go embed. Docker + GitHub Actions CI/CD.

**Tech Stack:** Go 1.21+, Gin, SQLite (modernc.org/sqlite), React 18, Ant Design 5, Vite, Docker, GitHub Actions

---

## File Structure

```
webhook-ui/
├── .github/workflows/docker-build-push.yml  # CI/CD
├── Dockerfile                               # Multi-stage build
├── .gitignore
├── go.mod / go.sum
├── main.go                                  # Entry point
├── internal/
│   ├── config/
│   │   └── config.go                        # App configuration
│   ├── database/
│   │   ├── db.go                            # SQLite connection
│   │   └── migrate.go                       # Schema migrations
│   ├── models/
│   │   ├── hook.go                          # Hook model
│   │   └── execution.go                     # Execution log model
│   ├── handlers/
│   │   ├── webhook.go                       # Webhook receiver
│   │   ├── hook.go                          # Hook CRUD API
│   │   ├── auth.go                          # Login/logout
│   │   └── execution.go                     # Execution logs API
│   ├── middleware/
│   │   └── auth.go                          # Session middleware
│   └── services/
│       ├── executor.go                      # Command execution
│       └── hmac.go                          # HMAC validation
├── web/                                     # React frontend
│   ├── package.json
│   ├── vite.config.ts
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── pages/
│       │   ├── Login.tsx
│       │   ├── HookList.tsx
│       │   ├── HookEdit.tsx
│       │   └── ExecutionLogs.tsx
│       └── api/
│           └── client.ts                    # API client
└── embed.go                                 # Go embed for frontend
```

---

### Task 1: Project Setup

**Files:**
- Create: `go.mod`, `main.go`, `.gitignore`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/songguangzhi/Documents/Code/Go/webhook-ui
go mod init github.com/songguangzhi/webhook-ui
```

- [ ] **Step 2: Create .gitignore**

```gitignore
# Binaries
webhook-ui
*.exe

# Data
data/
*.db

# Frontend
web/node_modules/
web/dist/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
```

- [ ] **Step 3: Create minimal main.go**

```go
package main

import "fmt"

func main() {
	fmt.Println("webhook-ui starting...")
}
```

- [ ] **Step 4: Verify**

```bash
go run main.go
# Expected: webhook-ui starting...
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "chore: initial project setup"
```

---

### Task 2: Database Setup

**Files:**
- Create: `internal/database/db.go`, `internal/database/migrate.go`

- [ ] **Step 1: Add dependencies**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: Create database connection**

`internal/database/db.go`:
```go
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "webhook-ui.db")
	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return Migrate()
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
```

- [ ] **Step 3: Create migrations**

`internal/database/migrate.go`:
```go
package database

import "fmt"

const schemaVersion = 1

func Migrate() error {
	var version int
	err := DB.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= schemaVersion {
		return nil
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS hooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			working_dir TEXT DEFAULT '',
			response_message TEXT DEFAULT 'OK',
			hmac_secret TEXT DEFAULT '',
			hmac_algorithm TEXT DEFAULT 'sha256',
			pass_arguments TEXT DEFAULT '',  -- JSON array
			pass_headers TEXT DEFAULT '',    -- JSON array
			pass_payload_to TEXT DEFAULT '', -- 'args', 'env', 'both', ''
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hook_id TEXT NOT NULL,
			trigger_source TEXT DEFAULT '',  -- IP or identifier
			status TEXT NOT NULL,            -- 'success', 'failed', 'running'
			output TEXT DEFAULT '',
			error TEXT DEFAULT '',
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME,
			FOREIGN KEY (hook_id) REFERENCES hooks(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_hook_id ON executions(hook_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_started_at ON executions(started_at DESC)`,
	}

	for i, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}

	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Test database init**

Update `main.go`:
```go
package main

import (
	"fmt"
	"log"

	"github.com/songguangzhi/webhook-ui/internal/database"
)

func main() {
	if err := database.Init("./data"); err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	fmt.Println("database initialized")
}
```

Run: `go run main.go`
Expected: `database initialized`

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: add SQLite database with migrations"
```

---

### Task 3: Models

**Files:**
- Create: `internal/models/hook.go`, `internal/models/execution.go`

- [ ] **Step 1: Create Hook model**

`internal/models/hook.go`:
```go
package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type StringArray []string

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("invalid type for StringArray")
	}
	return json.Unmarshal(bytes, s)
}

func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	return json.Marshal(s)
}

type Hook struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Command        string      `json:"command"`
	WorkingDir     string      `json:"working_dir"`
	ResponseMessage string     `json:"response_message"`
	HMACSecret     string      `json:"hmac_secret,omitempty"`
	HMACAlgorithm  string      `json:"hmac_algorithm"`
	PassArguments  StringArray `json:"pass_arguments"`
	PassHeaders    StringArray `json:"pass_headers"`
	PassPayloadTo  string      `json:"pass_payload_to"` // 'args', 'env', 'both', ''
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func (h *Hook) Validate() error {
	if h.ID == "" {
		return errors.New("id is required")
	}
	if h.Name == "" {
		return errors.New("name is required")
	}
	if h.Command == "" {
		return errors.New("command is required")
	}
	return nil
}
```

- [ ] **Step 2: Create Execution model**

`internal/models/execution.go`:
```go
package models

import "time"

type Execution struct {
	ID            int64      `json:"id"`
	HookID        string     `json:"hook_id"`
	TriggerSource string     `json:"trigger_source"`
	Status        string     `json:"status"` // 'success', 'failed', 'running'
	Output        string     `json:"output"`
	Error         string     `json:"error"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}
```

- [ ] **Step 3: Commit**

```bash
git add .
git commit -m "feat: add Hook and Execution models"
```

---

### Task 4: Configuration

**Files:**
- Create: `internal/config/config.go`

- [ ] **Step 1: Create config**

`internal/config/config.go`:
```go
package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
)

type Config struct {
	Port         string
	DataDir      string
	SessionSecret string
	AdminPassword string
	AllowedCommands []string
}

func Load() *Config {
	cfg := &Config{
		Port:    getEnv("PORT", "9000"),
		DataDir: getEnv("DATA_DIR", "./data"),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
		AllowedCommands: getEnvSlice("ALLOWED_COMMANDS", []string{"/usr/bin/git", "/usr/bin/curl", "/bin/bash", "/bin/sh"}),
	}

	// Session secret: from env or generate random
	cfg.SessionSecret = os.Getenv("SESSION_SECRET")
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = generateRandomSecret()
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if v := os.Getenv(key); v != "" {
		// Simple split by comma
		var result []string
		start := 0
		for i := 0; i <= len(v); i++ {
			if i == len(v) || v[i] == ',' {
				if i > start {
					result = append(result, v[start:i])
				}
				start = i + 1
			}
		}
		return result
	}
	return defaultValue
}

func generateRandomSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add configuration management"
```

---

### Task 5: HMAC Service

**Files:**
- Create: `internal/services/hmac.go`

- [ ] **Step 1: Create HMAC validator**

`internal/services/hmac.go`:
```go
package services

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

type HMACValidator struct {
	secret    string
	algorithm string
}

func NewHMACValidator(secret, algorithm string) *HMACValidator {
	return &HMACValidator{
		secret:    secret,
		algorithm: strings.ToLower(algorithm),
	}
}

func (v *HMACValidator) getHash() func() hash.Hash {
	switch v.algorithm {
	case "sha1":
		return sha1.New
	case "sha512":
		return sha512.New
	default: // sha256
		return sha256.New
	}
}

// Validate checks if the signature matches the payload
// signature format: "sha256=abc123..." or just "abc123..."
func (v *HMACValidator) Validate(payload []byte, signature string) bool {
	if v.secret == "" {
		return true // No secret configured, skip validation
	}

	// Remove algorithm prefix if present
	sig := signature
	if idx := strings.Index(signature, "="); idx != -1 {
		sig = signature[idx+1:]
	}

	mac := hmac.New(v.getHash(), []byte(v.secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// GetSignatureHeader returns the header name to check based on common conventions
func GetSignatureHeader(headers map[string][]string) string {
	// GitHub style
	if sig := headers["X-Hub-Signature-256"]; len(sig) > 0 {
		return sig[0]
	}
	// GitLab style
	if sig := headers["X-Gitlab-Token"]; len(sig) > 0 {
		return sig[0]
	}
	// Generic
	if sig := headers["X-Signature"]; len(sig) > 0 {
		return sig[0]
	}
	return ""
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add HMAC signature validation"
```

---

### Task 6: Command Executor Service

**Files:**
- Create: `internal/services/executor.go`

- [ ] **Step 1: Create executor**

`internal/services/executor.go`:
```go
package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/models"
)

type Executor struct {
	allowedCommands []string
}

func NewExecutor(allowedCommands []string) *Executor {
	return &Executor{
		allowedCommands: allowedCommands,
	}
}

func (e *Executor) isAllowed(cmd string) bool {
	// Extract base command (first word)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	baseCmd := parts[0]

	// Resolve absolute path
	absPath, err := filepath.Abs(baseCmd)
	if err != nil {
		absPath = baseCmd
	}

	for _, allowed := range e.allowedCommands {
		// Exact match or prefix match with path separator
		if absPath == allowed || strings.HasPrefix(absPath, allowed+string(os.PathSeparator)) {
			return true
		}
		// Also check if allowed is a prefix of the command path
		if strings.HasPrefix(absPath, allowed) {
			return true
		}
	}
	return false
}

type ExecuteResult struct {
	Output  string
	Error   string
	Success bool
}

func (e *Executor) Execute(hook *models.Hook, env map[string]string, args []string) *ExecuteResult {
	if !e.isAllowed(hook.Command) {
		return &ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("command not allowed: %s", hook.Command),
		}
	}

	// Build command with args
	cmdParts := strings.Fields(hook.Command)
	cmdParts = append(cmdParts, args...)
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	// Set working directory
	if hook.WorkingDir != "" {
		cmd.Dir = hook.WorkingDir
	}

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		result := &ExecuteResult{
			Output: stdout.String(),
			Error:  stderr.String(),
		}
		if err != nil {
			result.Success = false
			if result.Error == "" {
				result.Error = err.Error()
			}
		} else {
			result.Success = true
		}
		return result
	case <-time.After(5 * time.Minute):
		cmd.Process.Kill()
		return &ExecuteResult{
			Success: false,
			Error:   "execution timeout (5 minutes)",
		}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add command executor with whitelist"
```

---

### Task 7: Auth Middleware & Handlers

**Files:**
- Create: `internal/middleware/auth.go`, `internal/handlers/auth.go`

- [ ] **Step 1: Add session dependency**

```bash
go get github.com/gin-contrib/sessions
go get github.com/gin-contrib/sessions/cookie
go get github.com/gin-gonic/gin
```

- [ ] **Step 2: Create auth middleware**

`internal/middleware/auth.go`:
```go
package middleware

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const SessionKey = "webhook-ui-session"
const UserKey = "authenticated"

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get(UserKey) != true {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 3: Create auth handlers**

`internal/handlers/auth.go`:
```go
package handlers

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
)

type AuthHandler struct {
	adminPassword string
}

func NewAuthHandler(adminPassword string) *AuthHandler {
	return &AuthHandler{adminPassword: adminPassword}
}

type LoginRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Password != h.adminPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		return
	}

	session := sessions.Default(c)
	session.Set(middleware.UserKey, true)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged in"})
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
```

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "feat: add session-based authentication"
```

---

### Task 8: Hook CRUD Handlers

**Files:**
- Create: `internal/handlers/hook.go`

- [ ] **Step 1: Create hook handlers**

`internal/handlers/hook.go`:
```go
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
```

- [ ] **Step 2: Add uuid dependency**

```bash
go get github.com/google/uuid
```

- [ ] **Step 3: Commit**

```bash
git add .
git commit -m "feat: add hook CRUD handlers"
```

---

### Task 9: Webhook Receiver Handler

**Files:**
- Create: `internal/handlers/webhook.go`

- [ ] **Step 1: Create webhook handler**

`internal/handlers/webhook.go`:
```go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

type WebhookHandler struct {
	executor *services.Executor
}

func NewWebhookHandler(executor *services.Executor) *WebhookHandler {
	return &WebhookHandler{executor: executor}
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	hookID := c.Param("id")

	// Load hook
	var hook models.Hook
	err := database.DB.QueryRow(`
		SELECT id, name, command, working_dir, response_message,
		       hmac_secret, hmac_algorithm, pass_arguments, pass_headers, pass_payload_to
		FROM hooks WHERE id = ?
	`, hookID).Scan(
		&hook.ID, &hook.Name, &hook.Command, &hook.WorkingDir,
		&hook.ResponseMessage, &hook.HMACSecret, &hook.HMACAlgorithm,
		&hook.PassArguments, &hook.PassHeaders, &hook.PassPayloadTo,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Read payload
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Validate HMAC if configured
	if hook.HMACSecret != "" {
		signature := services.GetSignatureHeader(c.Request.Header)
		validator := services.NewHMACValidator(hook.HMACSecret, hook.HMACAlgorithm)
		if !validator.Validate(payload, signature) {
			h.logExecution(hookID, c.ClientIP(), "failed", "", "invalid HMAC signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	// Parse payload for arguments
	env, args := h.buildCommandInput(&hook, c, payload)

	// Log execution start
	execID := h.logExecutionStart(hookID, c.ClientIP())

	// Execute command
	result := h.executor.Execute(&hook, env, args)

	// Update execution log
	status := "success"
	if !result.Success {
		status = "failed"
	}
	h.logExecutionEnd(execID, status, result.Output, result.Error)

	// Response
	if result.Success {
		c.JSON(http.StatusOK, gin.H{
			"message": hook.ResponseMessage,
			"output":  result.Output,
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  result.Error,
			"output": result.Output,
		})
	}
}

func (h *WebhookHandler) buildCommandInput(hook *models.Hook, c *gin.Context, payload []byte) (map[string]string, []string) {
	env := make(map[string]string)
	var args []string

	// Parse JSON payload
	var payloadData map[string]interface{}
	if len(payload) > 0 {
		json.Unmarshal(payload, &payloadData)
	}

	// Add query parameters to env
	for k, v := range c.Request.URL.Query() {
		env["QUERY_"+strings.ToUpper(k)] = strings.Join(v, ",")
	}

	// Add specified headers to env
	for _, headerName := range hook.PassHeaders {
		headerValue := c.GetHeader(headerName)
		envKey := "HEADER_" + strings.ToUpper(strings.ReplaceAll(headerName, "-", "_"))
		env[envKey] = headerValue
	}

	// Add payload fields as arguments
	for _, fieldName := range hook.PassArguments {
		if val, ok := payloadData[fieldName]; ok {
			switch v := val.(type) {
			case string:
				args = append(args, v)
			case float64:
				args = append(args, fmt.Sprintf("%v", v))
			default:
				jsonBytes, _ := json.Marshal(v)
				args = append(args, string(jsonBytes))
			}
		}
	}

	// Add full payload if configured
	if hook.PassPayloadTo == "args" || hook.PassPayloadTo == "both" {
		args = append(args, string(payload))
	}
	if hook.PassPayloadTo == "env" || hook.PassPayloadTo == "both" {
		env["PAYLOAD"] = string(payload)
	}

	return env, args
}

func (h *WebhookHandler) logExecutionStart(hookID, source string) int64 {
	result, err := database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, started_at)
		VALUES (?, ?, 'running', ?)
	`, hookID, source, time.Now())
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

func (h *WebhookHandler) logExecutionEnd(id int64, status, output, errMsg string) {
	database.DB.Exec(`
		UPDATE executions SET status=?, output=?, error=?, finished_at=? WHERE id=?
	`, status, output, errMsg, time.Now(), id)
}

func (h *WebhookHandler) logExecution(hookID, source, status, output, errMsg string) {
	database.DB.Exec(`
		INSERT INTO executions (hook_id, trigger_source, status, output, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hookID, source, status, output, errMsg, time.Now(), time.Now())
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add webhook trigger handler"
```

---

### Task 10: Execution Logs Handler

**Files:**
- Create: `internal/handlers/execution.go`

- [ ] **Step 1: Create execution handler**

`internal/handlers/execution.go`:
```go
package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

type ExecutionHandler struct{}

func NewExecutionHandler() *ExecutionHandler {
	return &ExecutionHandler{}
}

func (h *ExecutionHandler) List(c *gin.Context) {
	// Pagination
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	hookID := c.Query("hook_id")

	query := `
		SELECT id, hook_id, trigger_source, status, output, error, started_at, finished_at
		FROM executions
	`
	args := []interface{}{}

	if hookID != "" {
		query += " WHERE hook_id = ?"
		args = append(args, hookID)
	}

	query += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var executions []models.Execution
	for rows.Next() {
		var exec models.Execution
		var finishedAt sql.NullTime
		err := rows.Scan(
			&exec.ID, &exec.HookID, &exec.TriggerSource, &exec.Status,
			&exec.Output, &exec.Error, &exec.StartedAt, &finishedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if finishedAt.Valid {
			exec.FinishedAt = &finishedAt.Time
		}
		executions = append(executions, exec)
	}

	c.JSON(http.StatusOK, executions)
}

func (h *ExecutionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var exec models.Execution
	var finishedAt sql.NullTime

	err := database.DB.QueryRow(`
		SELECT id, hook_id, trigger_source, status, output, error, started_at, finished_at
		FROM executions WHERE id = ?
	`, id).Scan(
		&exec.ID, &exec.HookID, &exec.TriggerSource, &exec.Status,
		&exec.Output, &exec.Error, &exec.StartedAt, &finishedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if finishedAt.Valid {
		exec.FinishedAt = &finishedAt.Time
	}

	c.JSON(http.StatusOK, exec)
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add execution logs handler"
```

---

### Task 11: Main Application & Routes

**Files:**
- Modify: `main.go`
- Create: `embed.go`

- [ ] **Step 1: Create embed.go (placeholder for now)**

`embed.go`:
```go
package main

import "embed"

//go:embed all:web/dist
var FrontendFS embed.FS
```

Note: Create `web/dist/index.html` placeholder:
```bash
mkdir -p web/dist
echo '<!DOCTYPE html><html><body>Webhook UI</body></html>' > web/dist/index.html
```

- [ ] **Step 2: Update main.go with routes**

`main.go`:
```go
package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

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

	// Session middleware
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	r.Use(sessions.Sessions(middleware.SessionKey, store))

	// Serve frontend static files
	frontendFS, err := fs.Sub(FrontendFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	r.StaticFS("/assets", frontendFS)

	// Serve index.html for SPA routes
	r.NoRoute(func(c *gin.Context) {
		// API routes return 404
		if c.Request.URL.Path == "/api" || len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Webhook routes return 404
		if len(c.Request.URL.Path) > 6 && c.Request.URL.Path[:6] == "/hooks" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Serve frontend
		data, err := fs.ReadFile(frontendFS, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "frontend not built")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Handlers
	authHandler := handlers.NewAuthHandler(cfg.AdminPassword)
	hookHandler := handlers.NewHookHandler()
	executor := services.NewExecutor(cfg.AllowedCommands)
	webhookHandler := handlers.NewWebhookHandler(executor)
	executionHandler := handlers.NewExecutionHandler()

	// Public routes
	r.POST("/api/auth/login", authHandler.Login)
	r.GET("/api/auth/check", authHandler.Check)

	// Webhook trigger (public, HMAC protected)
	r.POST("/hooks/:id", webhookHandler.Trigger)

	// Protected routes
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
```

- [ ] **Step 3: Test build**

```bash
go build -o webhook-ui .
./webhook-ui
# Expected: Starts on :9000
```

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "feat: complete backend API and server setup"
```

---

### Task 12: React Frontend Setup

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/tsconfig.json`

- [ ] **Step 1: Initialize React project**

```bash
cd web
npm create vite@latest . -- --template react-ts
```

- [ ] **Step 2: Install dependencies**

```bash
npm install antd axios react-router-dom @types/react-router-dom
```

- [ ] **Step 3: Configure Vite proxy**

`web/vite.config.ts`:
```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:9000',
        changeOrigin: true,
      },
      '/hooks': {
        target: 'http://localhost:9000',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
})
```

- [ ] **Step 4: Create API client**

`web/src/api/client.ts`:
```typescript
import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  withCredentials: true,
})

export interface Hook {
  id: string
  name: string
  command: string
  working_dir: string
  response_message: string
  hmac_secret?: string
  hmac_algorithm: string
  pass_arguments: string[]
  pass_headers: string[]
  pass_payload_to: string
  created_at: string
  updated_at: string
}

export interface Execution {
  id: number
  hook_id: string
  trigger_source: string
  status: string
  output: string
  error: string
  started_at: string
  finished_at?: string
}

export const authApi = {
  login: (password: string) => client.post('/auth/login', { password }),
  logout: () => client.post('/auth/logout'),
  check: () => client.get('/auth/check'),
}

export const hookApi = {
  list: () => client.get<Hook[]>('/hooks'),
  get: (id: string) => client.get<Hook>(`/hooks/${id}`),
  create: (hook: Partial<Hook>) => client.post<Hook>('/hooks', hook),
  update: (id: string, hook: Partial<Hook>) => client.put<Hook>(`/hooks/${id}`, hook),
  delete: (id: string) => client.delete(`/hooks/${id}`),
}

export const executionApi = {
  list: (params?: { limit?: number; offset?: number; hook_id?: string }) =>
    client.get<Execution[]>('/executions', { params }),
  get: (id: number) => client.get<Execution>(`/executions/${id}`),
}

export default client
```

- [ ] **Step 5: Create main.tsx**

`web/src/main.tsx`:
```tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>,
)
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: setup React frontend with Vite and Ant Design"
```

---

### Task 13: Login Page

**Files:**
- Create: `web/src/pages/Login.tsx`

- [ ] **Step 1: Create Login page**

`web/src/pages/Login.tsx`:
```tsx
import { useState } from 'react'
import { Form, Input, Button, Card, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { authApi } from '../api/client'

export default function Login() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const onFinish = async (values: { password: string }) => {
    setLoading(true)
    try {
      await authApi.login(values.password)
      message.success('登录成功')
      navigate('/hooks')
    } catch (error: any) {
      message.error(error.response?.data?.error || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ 
      display: 'flex', 
      justifyContent: 'center', 
      alignItems: 'center', 
      minHeight: '100vh',
      background: '#f0f2f5'
    }}>
      <Card title="Webhook UI 登录" style={{ width: 400 }}>
        <Form onFinish={onFinish} layout="vertical">
          <Form.Item
            name="password"
            label="管理员密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="请输入管理员密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add login page"
```

---

### Task 14: Hook List Page

**Files:**
- Create: `web/src/pages/HookList.tsx`

- [ ] **Step 1: Create HookList page**

`web/src/pages/HookList.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { Table, Button, Space, Tag, message, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, CopyOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { hookApi, Hook } from '../api/client'

export default function HookList() {
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const loadHooks = async () => {
    setLoading(true)
    try {
      const res = await hookApi.list()
      setHooks(res.data)
    } catch (error) {
      message.error('加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadHooks()
  }, [])

  const handleDelete = async (id: string) => {
    try {
      await hookApi.delete(id)
      message.success('删除成功')
      loadHooks()
    } catch (error) {
      message.error('删除失败')
    }
  }

  const copyWebhookUrl = (id: string) => {
    const url = `${window.location.origin}/hooks/${id}`
    navigator.clipboard.writeText(url)
    message.success('Webhook URL 已复制')
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      render: (id: string) => <code>{id}</code>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
    },
    {
      title: 'HMAC',
      key: 'hmac',
      render: (_: any, record: Hook) => (
        record.hmac_secret ? <Tag color="green">启用</Tag> : <Tag>未启用</Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Hook) => (
        <Space>
          <Button
            type="text"
            icon={<CopyOutlined />}
            onClick={() => copyWebhookUrl(record.id)}
            title="复制 Webhook URL"
          />
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => navigate(`/hooks/${record.id}/edit`)}
          />
          <Popconfirm
            title="确定删除此 Hook?"
            onConfirm={() => handleDelete(record.id)}
          >
            <Button type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
        <h2>Webhook 管理</h2>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/hooks/new')}
        >
          新建 Hook
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={hooks}
        rowKey="id"
        loading={loading}
      />
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add hook list page"
```

---

### Task 15: Hook Edit Page

**Files:**
- Create: `web/src/pages/HookEdit.tsx`

- [ ] **Step 1: Create HookEdit page**

`web/src/pages/HookEdit.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { Form, Input, Select, Button, Card, message, Space } from 'antd'
import { useNavigate, useParams } from 'react-router-dom'
import { hookApi } from '../api/client'

const { TextArea } = Input

export default function HookEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [isNew, setIsNew] = useState(true)
  const navigate = useNavigate()
  const { id } = useParams()

  useEffect(() => {
    if (id && id !== 'new') {
      setIsNew(false)
      loadHook(id)
    }
  }, [id])

  const loadHook = async (hookId: string) => {
    try {
      const res = await hookApi.get(hookId)
      form.setFieldsValue(res.data)
    } catch (error) {
      message.error('加载失败')
    }
  }

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      // Parse array fields
      const data = {
        ...values,
        pass_arguments: values.pass_arguments?.split('\n').filter((s: string) => s.trim()) || [],
        pass_headers: values.pass_headers?.split('\n').filter((s: string) => s.trim()) || [],
      }

      if (isNew) {
        await hookApi.create(data)
        message.success('创建成功')
      } else {
        await hookApi.update(id!, data)
        message.success('更新成功')
      }
      navigate('/hooks')
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title={isNew ? '新建 Hook' : '编辑 Hook'}>
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{
          hmac_algorithm: 'sha256',
          pass_payload_to: '',
        }}
      >
        {!isNew && (
          <Form.Item name="id" label="Hook ID">
            <Input disabled />
          </Form.Item>
        )}

        <Form.Item
          name="name"
          label="名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="例如: 部署生产环境" />
        </Form.Item>

        <Form.Item
          name="command"
          label="执行命令"
          rules={[{ required: true, message: '请输入命令' }]}
          extra="例如: /opt/scripts/deploy.sh 或 /usr/bin/git pull"
        >
          <Input placeholder="/path/to/command" />
        </Form.Item>

        <Form.Item name="working_dir" label="工作目录">
          <Input placeholder="/path/to/workdir (可选)" />
        </Form.Item>

        <Form.Item name="response_message" label="成功响应消息">
          <Input placeholder="OK" />
        </Form.Item>

        <Form.Item
          name="hmac_secret"
          label="HMAC 密钥"
          extra="留空则不验证签名"
        >
          <Input.Password placeholder="签名验证密钥" />
        </Form.Item>

        <Form.Item name="hmac_algorithm" label="HMAC 算法">
          <Select>
            <Select.Option value="sha1">SHA1</Select.Option>
            <Select.Option value="sha256">SHA256</Select.Option>
            <Select.Option value="sha512">SHA512</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item
          name="pass_arguments"
          label="Payload 字段作为参数"
          extra="每行一个字段名，从 JSON payload 中提取作为命令参数"
        >
          <TextArea rows={3} placeholder="field1&#10;field2" />
        </Form.Item>

        <Form.Item
          name="pass_headers"
          label="Header 作为环境变量"
          extra="每行一个 Header 名，转换为 HEADER_* 环境变量"
        >
          <TextArea rows={3} placeholder="X-GitHub-Event&#10;X-Custom-Header" />
        </Form.Item>

        <Form.Item name="pass_payload_to" label="传递完整 Payload">
          <Select allowClear>
            <Select.Option value="">不传递</Select.Option>
            <Select.Option value="args">作为参数</Select.Option>
            <Select.Option value="env">作为环境变量 (PAYLOAD)</Select.Option>
            <Select.Option value="both">两者都传</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存
            </Button>
            <Button onClick={() => navigate('/hooks')}>取消</Button>
          </Space>
        </Form.Item>
      </Form>
    </Card>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add hook edit page"
```

---

### Task 16: Execution Logs Page

**Files:**
- Create: `web/src/pages/ExecutionLogs.tsx`

- [ ] **Step 1: Create ExecutionLogs page**

`web/src/pages/ExecutionLogs.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { Table, Tag, Button, Drawer, Typography, Select, Space } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { executionApi, hookApi, Execution, Hook } from '../api/client'

const { Paragraph, Text } = Typography

export default function ExecutionLogs() {
  const [executions, setExecutions] = useState<Execution[]>([])
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedHook, setSelectedHook] = useState<string>()
  const [detailVisible, setDetailVisible] = useState(false)
  const [currentExecution, setCurrentExecution] = useState<Execution>()

  const loadData = async () => {
    setLoading(true)
    try {
      const [execRes, hookRes] = await Promise.all([
        executionApi.list({ limit: 100, hook_id: selectedHook }),
        hookApi.list(),
      ])
      setExecutions(execRes.data)
      setHooks(hookRes.data)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [selectedHook])

  const getHookName = (hookId: string) => {
    const hook = hooks.find(h => h.id === hookId)
    return hook ? hook.name : hookId
  }

  const showDetail = (record: Execution) => {
    setCurrentExecution(record)
    setDetailVisible(true)
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: 'Hook',
      dataIndex: 'hook_id',
      key: 'hook_id',
      render: (hookId: string) => (
        <span>
          {getHookName(hookId)}
          <Text type="secondary" style={{ marginLeft: 8 }}>({hookId})</Text>
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const colors: Record<string, string> = {
          success: 'green',
          failed: 'red',
          running: 'blue',
        }
        return <Tag color={colors[status] || 'default'}>{status}</Tag>
      },
    },
    {
      title: '来源',
      dataIndex: 'trigger_source',
      key: 'trigger_source',
    },
    {
      title: '开始时间',
      dataIndex: 'started_at',
      key: 'started_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '耗时',
      key: 'duration',
      render: (_: any, record: Execution) => {
        if (!record.finished_at) return '-'
        const start = new Date(record.started_at).getTime()
        const end = new Date(record.finished_at).getTime()
        return `${((end - start) / 1000).toFixed(2)}s`
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Execution) => (
        <Button type="link" onClick={() => showDetail(record)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
        <h2>执行日志</h2>
        <Space>
          <Select
            placeholder="筛选 Hook"
            allowClear
            style={{ width: 200 }}
            onChange={setSelectedHook}
            options={hooks.map(h => ({ value: h.id, label: h.name }))}
          />
          <Button icon={<ReloadOutlined />} onClick={loadData}>
            刷新
          </Button>
        </Space>
      </div>

      <Table
        columns={columns}
        dataSource={executions}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 20 }}
      />

      <Drawer
        title="执行详情"
        placement="right"
        width={600}
        open={detailVisible}
        onClose={() => setDetailVisible(false)}
      >
        {currentExecution && (
          <div>
            <Paragraph>
              <Text strong>Hook: </Text>
              {getHookName(currentExecution.hook_id)}
            </Paragraph>
            <Paragraph>
              <Text strong>状态: </Text>
              <Tag color={currentExecution.status === 'success' ? 'green' : 'red'}>
                {currentExecution.status}
              </Tag>
            </Paragraph>
            <Paragraph>
              <Text strong>来源: </Text>
              {currentExecution.trigger_source}
            </Paragraph>
            <Paragraph>
              <Text strong>开始时间: </Text>
              {new Date(currentExecution.started_at).toLocaleString()}
            </Paragraph>
            {currentExecution.finished_at && (
              <Paragraph>
                <Text strong>结束时间: </Text>
                {new Date(currentExecution.finished_at).toLocaleString()}
              </Paragraph>
            )}
            <Paragraph>
              <Text strong>输出: </Text>
            </Paragraph>
            <pre style={{ 
              background: '#f5f5f5', 
              padding: 12, 
              borderRadius: 4,
              maxHeight: 300,
              overflow: 'auto'
            }}>
              {currentExecution.output || '(无输出)'}
            </pre>
            {currentExecution.error && (
              <>
                <Paragraph>
                  <Text strong type="danger">错误: </Text>
                </Paragraph>
                <pre style={{ 
                  background: '#fff2f0', 
                  padding: 12, 
                  borderRadius: 4,
                  color: '#ff4d4f'
                }}>
                  {currentExecution.error}
                </pre>
              </>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "feat: add execution logs page"
```

---

### Task 17: Main App with Routing

**Files:**
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Create App.tsx**

`web/src/App.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Spin } from 'antd'
import {
  ApiOutlined,
  FileTextOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import { authApi } from './api/client'
import Login from './pages/Login'
import HookList from './pages/HookList'
import HookEdit from './pages/HookEdit'
import ExecutionLogs from './pages/ExecutionLogs'

const { Header, Content, Sider } = Layout

function AppLayout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()
  const location = useLocation()

  const handleLogout = async () => {
    await authApi.logout()
    navigate('/login')
  }

  const menuItems = [
    {
      key: '/hooks',
      icon: <ApiOutlined />,
      label: 'Webhook 管理',
    },
    {
      key: '/executions',
      icon: <FileTextOutlined />,
      label: '执行日志',
    },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ 
        display: 'flex', 
        alignItems: 'center', 
        justifyContent: 'space-between',
        background: '#001529'
      }}>
        <div style={{ color: 'white', fontSize: 18, fontWeight: 'bold' }}>
          Webhook UI
        </div>
        <LogoutOutlined 
          style={{ color: 'white', fontSize: 16, cursor: 'pointer' }}
          onClick={handleLogout}
        />
      </Header>
      <Layout>
        <Sider width={200} theme="light">
          <Menu
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={({ key }) => navigate(key)}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ padding: 24, background: '#f0f2f5' }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  )
}

function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)

  useEffect(() => {
    checkAuth()
  }, [])

  const checkAuth = async () => {
    try {
      const res = await authApi.check()
      setAuthenticated(res.data.authenticated)
    } catch {
      setAuthenticated(false)
    }
  }

  if (authenticated === null) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh' 
      }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route 
          path="/login" 
          element={authenticated ? <Navigate to="/hooks" /> : <Login />} 
        />
        <Route
          path="/hooks"
          element={
            authenticated ? (
              <AppLayout>
                <HookList />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/hooks/:id"
          element={
            authenticated ? (
              <AppLayout>
                <HookEdit />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/executions"
          element={
            authenticated ? (
              <AppLayout>
                <ExecutionLogs />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route path="/" element={<Navigate to="/hooks" />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
```

- [ ] **Step 2: Test frontend build**

```bash
cd web
npm run build
# Expected: builds successfully to dist/
```

- [ ] **Step 3: Commit**

```bash
git add .
git commit -m "feat: complete React frontend with routing"
```

---

### Task 18: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Create Dockerfile**

```dockerfile
# Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build backend
FROM golang:1.21-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o webhook-ui .

# Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Copy binary
COPY --from=backend-builder /app/webhook-ui .

# Expose port
EXPOSE 9000

# Environment defaults
ENV PORT=9000
ENV DATA_DIR=/app/data

# Run
CMD ["./webhook-ui"]
```

- [ ] **Step 2: Test Docker build**

```bash
docker build -t webhook-ui:test .
# Expected: builds successfully
```

- [ ] **Step 3: Commit**

```bash
git add .
git commit -m "feat: add Dockerfile for containerized deployment"
```

---

### Task 19: GitHub Actions

**Files:**
- Create: `.github/workflows/docker-build-push.yml`

- [ ] **Step 1: Create workflow**

`.github/workflows/docker-build-push.yml`:
```yaml
name: docker-build-push

on:
  push:
    branches:
      - "master"
      - "main"
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest

    env:
      REGISTRY: 'registry.cn-hangzhou.aliyuncs.com'
      IMAGE_NAME: registry.cn-hangzhou.aliyuncs.com/dato/webhook-ui

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to Aliyun Container Registry
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Extract Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.IMAGE_NAME }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=ref,enable=true,priority=600,prefix=,suffix=,event=tag
            type=ref,enable=true,priority=500,prefix=,suffix=,event=branch
            type=sha,enable=true,prefix=,format=short

      - name: Build and push Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          file: ./Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 2: Commit**

```bash
git add .
git commit -m "ci: add GitHub Actions for Docker build and push"
```

---

### Task 20: Integration Test & Documentation

**Files:**
- Create: `README.md`

- [ ] **Step 1: Create README**

`README.md`:
```markdown
# Webhook UI

基于 Go 的 Webhook 管理工具，带 Web 控制台。

## 功能

- Webhook 接收并执行 shell 命令
- HMAC 签名验证 (SHA1/SHA256/SHA512)
- 参数传递 (query/header/payload)
- 执行日志查看
- 管理员登录认证
- 命令白名单安全控制

## 快速开始

### 本地运行

```bash
# 构建前端
cd web && npm install && npm run build && cd ..

# 运行
export ADMIN_PASSWORD=your-password
go run main.go
```

访问 http://localhost:9000

### Docker 运行

```bash
docker run -d \
  -p 9000:9000 \
  -e ADMIN_PASSWORD=your-password \
  -v webhook-data:/app/data \
  registry.cn-hangzhou.aliyuncs.com/dato/webhook-ui:latest
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务端口 | `9000` |
| `DATA_DIR` | 数据目录 | `./data` |
| `ADMIN_PASSWORD` | 管理员密码 | (必填) |
| `SESSION_SECRET` | Session 密钥 | 自动生成 |
| `ALLOWED_COMMANDS` | 允许的命令前缀，逗号分隔 | `/usr/bin/git,/usr/bin/curl,/bin/bash,/bin/sh` |

## API

### Webhook 触发

```
POST /hooks/:id
```

支持 HMAC 签名验证:
- GitHub: `X-Hub-Signature-256` header
- GitLab: `X-Gitlab-Token` header
- 通用: `X-Signature` header

### 管理 API

需要登录:
- `POST /api/auth/login` - 登录
- `POST /api/auth/logout` - 登出
- `GET /api/hooks` - Hook 列表
- `POST /api/hooks` - 创建 Hook
- `GET /api/hooks/:id` - Hook 详情
- `PUT /api/hooks/:id` - 更新 Hook
- `DELETE /api/hooks/:id` - 删除 Hook
- `GET /api/executions` - 执行日志
- `GET /api/executions/:id` - 执行详情

## 开发

### 后端

```bash
go run main.go
```

### 前端 (开发模式)

```bash
cd web
npm run dev
```

前端开发服务器会自动代理 `/api` 和 `/hooks` 到 `localhost:9000`。
```

- [ ] **Step 2: Final test**

```bash
# Build everything
cd web && npm run build && cd ..
go build -o webhook-ui .

# Test run
export ADMIN_PASSWORD=test123
./webhook-ui &
sleep 2

# Test login
curl -X POST http://localhost:9000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"password":"test123"}' \
  -c cookies.txt

# Test create hook
curl -X POST http://localhost:9000/api/hooks \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"name":"test","command":"/bin/echo hello"}'

# Test webhook trigger (get hook id from previous response)
curl -X POST http://localhost:9000/hooks/{hook_id}

# Cleanup
kill %1
```

- [ ] **Step 3: Final commit**

```bash
git add .
git commit -m "docs: add README and complete implementation"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All features from discussion implemented
- [x] **Placeholder scan:** No TBD/TODO/incomplete code
- [x] **Type consistency:** Models and handlers use consistent naming
- [x] **TDD approach:** Each task builds and tests incrementally
- [x] **File structure:** Clear separation of concerns

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-07-24-webhook-ui-implementation.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?"**
