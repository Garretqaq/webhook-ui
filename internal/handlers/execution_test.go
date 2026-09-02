package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songguangzhi/webhook-ui/internal/database"
)

// The list is the hot path a browser hits on every page view, and output can be
// megabytes per row (LOG_TAIL_BYTES caps it at 5MB per stream), so the list
// must not carry it; the detail endpoint is where output belongs.
func TestListOmitsOutputWhileDetailServesIt(t *testing.T) {
	setupExecDB(t)
	execID := startedExecution(t)
	if _, err := database.DB.Exec(
		`UPDATE executions SET output = 'hello output' WHERE id = ?`, execID,
	); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewExecutionHandler(NewCancelRegistry())
	r.GET("/api/executions", h.List)
	r.GET("/api/executions/:id", h.Get)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200", w.Code)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list has %d rows, want 1", len(list))
	}
	if _, ok := list[0]["output"]; ok {
		t.Error("list entry has output field, want it omitted")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/executions/"+strconv.FormatInt(execID, 10), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get = %d, want 200", w.Code)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("get body: %v", err)
	}
	if detail["output"] != "hello output" {
		t.Errorf("get output = %v, want %q", detail["output"], "hello output")
	}
}
