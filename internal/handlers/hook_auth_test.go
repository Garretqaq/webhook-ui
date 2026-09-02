package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupHookRouter wires only the hook CRUD endpoints, which is all the
// auth-mutual-exclusion tests touch.
func setupHookRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHookHandler()
	r.POST("/api/hooks", h.Create)
	r.PUT("/api/hooks/:id", h.Update)
	return r
}

func hookRequest(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// bothCredsBody carries both credentials at once — the state the auth
// mutual-exclusion rule exists to reject.
const bothCredsBody = `{
	"id": "h1", "name": "hook", "command": "/bin/sh -c true",
	"async": true, "timeout_seconds": 60,
	"hmac_secret": "sec", "hmac_algorithm": "sha256", "trigger_token": "tok"
}`

func TestCreateHookRejectsBothAuthMethods(t *testing.T) {
	setupExecDB(t)
	r := setupHookRouter()

	w := hookRequest(t, r, http.MethodPost, "/api/hooks", bothCredsBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "" {
		t.Fatal("expected an error message naming the conflict")
	}
}

func TestUpdateHookRejectsBothAuthMethods(t *testing.T) {
	setupExecDB(t)
	r := setupHookRouter()

	w := hookRequest(t, r, http.MethodPut, "/api/hooks/h1", bothCredsBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateHookAcceptsSingleAuthMethod(t *testing.T) {
	setupExecDB(t)
	r := setupHookRouter()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"hmac only", `{"id":"h1","name":"hook","command":"/bin/sh -c true","async":true,"timeout_seconds":60,"hmac_secret":"sec"}`},
		{"token only", `{"id":"h2","name":"hook","command":"/bin/sh -c true","async":true,"timeout_seconds":60,"trigger_token":"tok"}`},
		{"no auth", `{"id":"h3","name":"hook","command":"/bin/sh -c true","async":true,"timeout_seconds":60}`},
	} {
		w := hookRequest(t, r, http.MethodPost, "/api/hooks", tc.body)
		if w.Code != http.StatusCreated {
			t.Errorf("%s: expected 201, got %d: %s", tc.name, w.Code, w.Body.String())
		}
	}
}
