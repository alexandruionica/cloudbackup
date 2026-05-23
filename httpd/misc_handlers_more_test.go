package httpd

import (
	"cloudbackup/shared"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/julienschmidt/httprouter"
)

// handlerVersion should always succeed with 200 — it does not read config or state,
// just emits the build's version metadata.
func TestVersionHandlerReturns200(t *testing.T) {
	srv := SrvData{
		Mutex:           &sync.RWMutex{},
		backupJobsState: shared.NewJobsState(),
	}
	req := httptest.NewRequest("GET", "http://example.com/api/v1/report/version", nil)
	w := httptest.NewRecorder()
	srv.handlerVersion(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from version handler, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Errorf("expected Content-Type header on version response")
	}
}

// handlerGetRestoreList enumerates running restores from the job state and returns
// JSON. With a freshly-initialised state there are no running restores; the handler
// should still return 200 with a JSON body (an empty slice).
func TestGetRestoreListReturns200OnEmptyState(t *testing.T) {
	srv := SrvData{
		Mutex:           &sync.RWMutex{},
		backupJobsState: shared.NewJobsState(),
	}
	req := httptest.NewRequest("GET", "http://example.com/api/v1/restore/list", nil)
	w := httptest.NewRecorder()
	srv.handlerGetRestoreList(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from restore list handler on empty state, got %d", resp.StatusCode)
	}
}

func postRestoreWatch(t *testing.T, body string) *http.Response {
	t.Helper()
	srv := SrvData{
		Mutex:           &sync.RWMutex{},
		backupJobsState: shared.NewJobsState(),
	}
	req := httptest.NewRequest("POST", "http://example.com/api/v1/restore/watch",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPostRestoreWatch(w, req, []httprouter.Param{})
	return w.Result()
}

func TestRestoreWatchRejectsInvalidJson(t *testing.T) {
	resp := postRestoreWatch(t, `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestRestoreWatchRejectsMissingName(t *testing.T) {
	resp := postRestoreWatch(t, `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", resp.StatusCode)
	}
}

func TestRestoreWatchRejectsNotRunning(t *testing.T) {
	resp := postRestoreWatch(t, `{"name":"not_running"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when restore not running, got %d", resp.StatusCode)
	}
}
