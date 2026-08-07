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

// TestScriptTestRunsNeedASession runs the REAL route tree. Test runs execute
// arbitrary script content, so they must sit behind the session group — and a
// read-only token must not reach them either.
func TestScriptTestRunsNeedASession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig(t)
	r, err := buildRouter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := setAPITokenForTest("tok-secret"); err != nil {
		t.Fatal(err)
	}

	check := func(method, path string, header bool, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		if header {
			req.Header.Set("X-API-Token", "tok-secret")
		}
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("%s %s (token=%v) = %d, want %d", method, path, header, w.Code, want)
		}
	}

	// The routes exist, and a stranger is turned away by the session guard
	// rather than by a missing route.
	check(http.MethodPost, "/api/script-test-runs", false, http.StatusUnauthorized)
	check(http.MethodGet, "/api/script-test-runs/abc/logs", false, http.StatusUnauthorized)

	// A token can read execution history; it cannot start or watch test runs.
	check(http.MethodPost, "/api/external/script-test-runs", true, http.StatusNotFound)
	check(http.MethodPost, "/api/script-test-runs", true, http.StatusUnauthorized)
}

// setAPITokenForTest writes a token row for tests in this package.
func setAPITokenForTest(token string) error {
	_, err := database.DB.Exec(
		"INSERT INTO settings (key, value) VALUES ('api_token', ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value", token)
	return err
}
