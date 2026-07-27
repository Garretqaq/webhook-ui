package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
)

func setupAuthRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))
	r.POST("/api/auth/login", h.Login)
	return r
}

func loginRequest(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func newTestAuthHandler() *AuthHandler {
	return NewAuthHandler("admin", "secret", middleware.NewLoginGuard(5, 15*time.Minute))
}

func TestLoginSuccess(t *testing.T) {
	r := setupAuthRouter(newTestAuthHandler())

	w := loginRequest(t, r, `{"username":"admin","password":"secret"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoginWrongPassword(t *testing.T) {
	r := setupAuthRouter(newTestAuthHandler())

	w := loginRequest(t, r, `{"username":"admin","password":"wrong"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	assertUnifiedError(t, w)
}

func TestLoginWrongUsername(t *testing.T) {
	r := setupAuthRouter(newTestAuthHandler())

	w := loginRequest(t, r, `{"username":"root","password":"secret"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	assertUnifiedError(t, w)
}

func TestLoginLockoutAfterMaxFailures(t *testing.T) {
	r := setupAuthRouter(newTestAuthHandler())

	for i := 0; i < 4; i++ {
		if w := loginRequest(t, r, `{"username":"admin","password":"wrong"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// 5th failure triggers lockout
	w := loginRequest(t, r, `{"username":"admin","password":"wrong"}`)
	assertLocked(t, w)

	// even correct credentials are rejected while locked
	w = loginRequest(t, r, `{"username":"admin","password":"secret"}`)
	assertLocked(t, w)
}

func assertLocked(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected lockout error message")
	}
	if body["retry_after"] == nil {
		t.Fatal("expected retry_after field")
	}
}

func assertUnifiedError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] != "用户名或密码错误" {
		t.Fatalf("expected unified error message, got %q", body["error"])
	}
}
