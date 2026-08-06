package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/config"
	"github.com/songguangzhi/webhook-ui/internal/database"
)

// testConfig builds a minimal config that lets buildRouter run in-process.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	database.DB = db
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		Port:            "0",
		TrustedProxies:  []string{"127.0.0.1"},
		SessionSecret:   "test-secret",
		AdminUsername:   "admin",
		AdminPassword:   "pw",
		AllowedCommands: []string{"/bin/sh"},
		LogTailBytes:    1024,
	}
}

// TestTokenCannotReachPlaintextCredentials runs the REAL route tree. The
// boundary this guards is the whole point of the token: the ssh-hosts detail
// endpoint returns plaintext private keys, so it must stay unreachable by
// token. A hand-built copy of the routes in a test would keep passing while
// this file widened the token's reach — this one fails instead.
func TestTokenCannotReachPlaintextCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig(t)
	r, err := buildRouter(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a real token through the real settings endpoint (session auth
	// bypassed for setup only).
	router := httptest.NewRecorder()
	_ = router

	// Seed a token directly — going through the endpoint needs a session.
	if err := setAPITokenForTest("tok-secret"); err != nil {
		t.Fatal(err)
	}

	check := func(method, path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-API-Token", "tok-secret")
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("%s %s with a token = %d, want %d", method, path, w.Code, want)
		}
	}

	// Allowed: read-only executions.
	check(http.MethodGet, "/api/external/executions", http.StatusOK)

	// Forbidden: the credential-bearing endpoints, through the token group.
	// These paths do not exist there, so a token gets the same 404 as a
	// stranger — the group is the boundary, not each handler.
	check(http.MethodGet, "/api/external/ssh-hosts/x", http.StatusNotFound)
	check(http.MethodGet, "/api/external/hooks", http.StatusNotFound)
	check(http.MethodPost, "/api/external/executions/1/cancel", http.StatusNotFound)
}

// setAPITokenForTest writes a token row for tests in this package.
func setAPITokenForTest(token string) error {
	_, err := database.DB.Exec(
		"INSERT INTO settings (key, value) VALUES ('api_token', ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value", token)
	return err
}
