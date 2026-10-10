package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func retentionRouter(rc *RetentionController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewSettingsHandler(rc)
	r.GET("/settings/retention", h.GetRetention)
	r.PUT("/settings/retention", h.UpdateRetention)
	return r
}

func doJSON(r http.Handler, method, path string, body any) (*httptest.ResponseRecorder, RetentionSettings) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var s RetentionSettings
	json.Unmarshal(w.Body.Bytes(), &s)
	return w, s
}

func TestRetentionSettingsDefaultOffAnd30Days(t *testing.T) {
	setupExecDB(t)
	s, err := LoadRetentionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Enabled || s.Days != 30 {
		t.Errorf("defaults = %+v, want disabled with 30 days", s)
	}
}

func TestRetentionSettingsValidation(t *testing.T) {
	setupExecDB(t)
	r := retentionRouter(nil)
	for _, days := range []int{0, -1, 3651} {
		w, _ := doJSON(r, http.MethodPut, "/settings/retention", map[string]any{"enabled": true, "days": days})
		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%d: status %d, want 400", days, w.Code)
		}
	}
	// Nothing was persisted by the rejected requests.
	if s, _ := LoadRetentionSettings(); s.Enabled {
		t.Error("a rejected update was persisted")
	}
}

func TestRetentionSettingsRoundTrip(t *testing.T) {
	setupExecDB(t)
	r := retentionRouter(nil)
	w, got := doJSON(r, http.MethodPut, "/settings/retention", RetentionSettings{Enabled: true, Days: 7})
	if w.Code != http.StatusOK || !got.Enabled || got.Days != 7 {
		t.Fatalf("PUT: %d %+v", w.Code, got)
	}
	w, got = doJSON(r, http.MethodGet, "/settings/retention", nil)
	if w.Code != http.StatusOK || !got.Enabled || got.Days != 7 {
		t.Fatalf("GET after PUT: %d %+v", w.Code, got)
	}
}

func TestSavingRetentionAppliesImmediately(t *testing.T) {
	setupExecDB(t)
	rc := NewRetentionController(newFakeClock())
	defer rc.Stop()
	r := retentionRouter(rc)

	old := finishedExecution(t, time.Now().AddDate(0, 0, -40))
	// Disabled by default: applying the stored settings deletes nothing.
	if err := rc.ApplyStored(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", old) != 1 {
		t.Fatal("cleanup ran while disabled")
	}

	// Enabling sweeps right away, without waiting for a daily tick.
	doJSON(r, http.MethodPut, "/settings/retention", RetentionSettings{Enabled: true, Days: 30})
	waitForRowCount(t, "SELECT COUNT(*) FROM executions WHERE id = ?", 0, old)

	// Disabling stops it: a new old row survives.
	doJSON(r, http.MethodPut, "/settings/retention", RetentionSettings{Enabled: false, Days: 30})
	another := finishedExecution(t, time.Now().AddDate(0, 0, -40))
	time.Sleep(100 * time.Millisecond)
	if count(t, "SELECT COUNT(*) FROM executions WHERE id = ?", another) != 1 {
		t.Error("cleanup still ran after being disabled")
	}
}

// A body that omits a field must be rejected, not read as its zero value:
// binding straight into RetentionSettings took {"days": 7} as enabled=false
// and quietly turned cleanup off behind the caller's back.
func TestRetentionUpdateRequiresBothFields(t *testing.T) {
	setupExecDB(t)
	r := retentionRouter(nil)
	if w, _ := doJSON(r, http.MethodPut, "/settings/retention", RetentionSettings{Enabled: true, Days: 7}); w.Code != http.StatusOK {
		t.Fatalf("setup PUT: status %d", w.Code)
	}
	for _, body := range []map[string]any{{"days": 7}, {"enabled": true}, {}} {
		if w, _ := doJSON(r, http.MethodPut, "/settings/retention", body); w.Code != http.StatusBadRequest {
			t.Errorf("body %v: status %d, want 400", body, w.Code)
		}
	}
	s, err := LoadRetentionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || s.Days != 7 {
		t.Errorf("a rejected update changed the stored policy: %+v", s)
	}
}
