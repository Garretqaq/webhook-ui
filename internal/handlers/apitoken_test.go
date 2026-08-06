package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/middleware"
	"github.com/songguangzhi/webhook-ui/internal/models"
)

// buildExternalRouter mirrors main.go's external group exactly. It is built
// here, not imported, so the test fails if the main.go routes drift out of the
// read-only boundary — which is the thing this ticket exists to prevent.
func buildExternalRouter(t *testing.T, tokenLookup func() (string, error)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewExecutionHandler(NewCancelRegistry())

	ext := r.Group("/api/external")
	ext.Use(middleware.APITokenRequired(tokenLookup))
	ext.GET("/executions", h.List)
	ext.GET("/executions/:id", h.Get)
	ext.GET("/executions/:id/logs", h.Logs)
	return r
}

func get(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set(middleware.APITokenHeader, token)
	}
	r.ServeHTTP(w, req)
	return w
}

func staticToken(t *testing.T, token string) func() (string, error) {
	return func() (string, error) { return token, nil }
}

func TestAPITokenReadsExecutions(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	r := buildExternalRouter(t, staticToken(t, "tok-123"))

	if w := get(r, "/api/external/executions", "tok-123"); w.Code != http.StatusOK {
		t.Errorf("list = %d, want 200", w.Code)
	}
	if w := get(r, "/api/external/executions/"+strconv.FormatInt(execID, 10), "tok-123"); w.Code != http.StatusOK {
		t.Errorf("get = %d, want 200", w.Code)
	}
	if w := get(r, "/api/external/executions/"+strconv.FormatInt(execID, 10)+"/logs", "tok-123"); w.Code != http.StatusOK {
		t.Errorf("logs = %d, want 200", w.Code)
	}
}

func TestAPITokenCannotReachBeyondItsScope(t *testing.T) {
	setupExecDB(t)
	r := buildExternalRouter(t, staticToken(t, "tok-123"))

	// The boundary that justifies the token's existence: a leaked token must
	// not become a leak of the SSH private keys stored in this database.
	for _, path := range []string{
		"/api/external/ssh-hosts", "/api/external/ssh-hosts/abc",
		"/api/external/hooks", "/api/external/scripts",
	} {
		if w := get(r, path, "tok-123"); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 — the token must have no route there at all", path, w.Code)
		}
	}
}

func TestAPITokenCannotCancelAnExecution(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	r := buildExternalRouter(t, staticToken(t, "tok-123"))

	// Read-only means read-only: the cancel endpoint is not mounted for tokens.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/external/executions/"+strconv.FormatInt(execID, 10)+"/cancel", nil)
	req.Header.Set(middleware.APITokenHeader, "tok-123")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("cancel = %d, want 404", w.Code)
	}
	if executionStatus(t, execID) != models.StatusRunning {
		t.Error("the execution was affected despite the refusal")
	}
}

func TestAPITokenRejectsMissingAndWrongTokens(t *testing.T) {
	setupExecDB(t)
	r := buildExternalRouter(t, staticToken(t, "tok-123"))

	if w := get(r, "/api/external/executions", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", w.Code)
	}
	if w := get(r, "/api/external/executions", "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", w.Code)
	}
}

func TestAPITokenIsForbiddenUntilConfigured(t *testing.T) {
	setupExecDB(t)
	// No token has been generated: external access is off, not open.
	r := buildExternalRouter(t, APIToken)
	if w := get(r, "/api/external/executions", "anything"); w.Code != http.StatusForbidden {
		t.Errorf("unconfigured = %d, want 403", w.Code)
	}
}

func TestRegeneratingTheTokenRevokesTheOldOneImmediately(t *testing.T) {
	setupExecDB(t)
	r := buildExternalRouter(t, APIToken)

	h := NewSettingsHandler()
	gin.SetMode(gin.TestMode)
	admin := gin.New()
	admin.POST("/settings/api-token/regenerate", h.RegenerateAPIToken)

	regen := func() map[string]any {
		w := httptest.NewRecorder()
		admin.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/settings/api-token/regenerate", nil))
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return body
	}

	first := regen()["token"].(string)
	if w := get(r, "/api/external/executions", first); w.Code != http.StatusOK {
		t.Fatalf("fresh token = %d, want 200", w.Code)
	}

	regen()
	if w := get(r, "/api/external/executions", first); w.Code != http.StatusUnauthorized {
		t.Errorf("the revoked token still worked: %d", w.Code)
	}
}

func TestSessionAuthIsUnaffectedByTheExternalGroup(t *testing.T) {
	setupExecDB(t)
	// The executions routes also live in the session group; adding the external
	// one must not turn them into a hole. Building the group without the token
	// middleware exercises that the handler itself never grants access.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// AuthRequired consults the session, which only exists when the store
	// middleware is present — exactly as main.go mounts it.
	r.Use(sessions.Sessions(middleware.SessionKey, cookie.NewStore([]byte("test-secret"))))
	h := NewExecutionHandler(NewCancelRegistry())
	r.GET("/api/executions", middleware.AuthRequired(), h.List)

	if w := get(r, "/api/executions", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("session-less = %d, want 401", w.Code)
	}
}
