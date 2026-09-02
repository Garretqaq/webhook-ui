package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
)

// shCommand builds a command the test executor's whitelist accepts: an sh
// invocation of a script that just exits successfully.
func shCommand(t *testing.T) string {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	script := filepath.Join(t.TempDir(), "ok.sh")
	if err := os.WriteFile(script, []byte("exit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return shPath + " " + script
}

// insertTokenHook creates a hook whose only credential is a fixed trigger
// token, so a test can exercise the token check without HMAC getting in the
// way.
func insertTokenHook(t *testing.T, id, token, command string) {
	t.Helper()
	if _, err := database.DB.Exec(
		`INSERT INTO hooks (id, name, command, trigger_token, timeout_seconds)
		 VALUES (?, 'token hook', ?, ?, 60)`,
		id, command, token,
	); err != nil {
		t.Fatal(err)
	}
}

func triggerWithHeaders(t *testing.T, h *WebhookHandler, hookID string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/hooks/:id", h.Trigger)

	req := httptest.NewRequest(http.MethodPost, "/hooks/"+hookID, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTriggerTokenAcceptsXTokenHeader(t *testing.T) {
	setupExecDB(t)
	insertTokenHook(t, "h-token", "sekrit", shCommand(t))
	h := newExecTestHandler(t)

	if w := triggerWithHeaders(t, h, "h-token", map[string]string{"X-Token": "sekrit"}); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	if w := triggerWithHeaders(t, h, "h-token", map[string]string{"X-Token": "wrong"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", w.Code, w.Body.String())
	}
	if w := triggerWithHeaders(t, h, "h-token", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", w.Code, w.Body.String())
	}
}

// GitLab sends the webhook secret as plaintext in X-Gitlab-Token. That header
// used to be read only by the HMAC path, where a plaintext token can never
// match a hex digest — so a GitLab hook with a fixed token could never pass.
func TestTriggerTokenAcceptsGitlabTokenHeader(t *testing.T) {
	setupExecDB(t)
	insertTokenHook(t, "h-gitlab", "gitlab-secret", shCommand(t))
	h := newExecTestHandler(t)

	if w := triggerWithHeaders(t, h, "h-gitlab", map[string]string{"X-Gitlab-Token": "gitlab-secret"}); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	if w := triggerWithHeaders(t, h, "h-gitlab", map[string]string{"X-Gitlab-Token": "wrong"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", w.Code, w.Body.String())
	}
}
