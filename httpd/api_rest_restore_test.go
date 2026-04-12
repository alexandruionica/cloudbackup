package httpd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/julienschmidt/httprouter"
)

// postRestoreStart drives handlerPostRestoreStart with the given JSON body and returns the
// recorded response. The handler short-circuits on input validation errors before touching the
// scheduler/config so we can pass an almost-empty SrvData.
func postRestoreStart(t *testing.T, body string) *http.Response {
	t.Helper()
	srv := SrvData{Mutex: &sync.RWMutex{}}
	req := httptest.NewRequest("POST", "http://example.com/api/v1/restore/start",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPostRestoreStart(w, req, []httprouter.Param{})
	return w.Result()
}

func TestRestoreStartRejectsMissingName(t *testing.T) {
	resp := postRestoreStart(t, `{"source_backup_job_id":"x","all_files":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRestoreStartRejectsMissingSourceJobId(t *testing.T) {
	resp := postRestoreStart(t, `{"name":"demo","all_files":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRestoreStartRejectsFilesAndAllFilesMutuallyExclusive(t *testing.T) {
	resp := postRestoreStart(t, `{"name":"demo","source_backup_job_id":"x","all_files":true,"files":["/etc/hosts"]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRestoreStartRejectsNeitherFilesNorAllFiles(t *testing.T) {
	resp := postRestoreStart(t, `{"name":"demo","source_backup_job_id":"x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRestoreStartRejectsInvalidJson(t *testing.T) {
	resp := postRestoreStart(t, `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRestoreStopRejectsMissingName(t *testing.T) {
	srv := SrvData{Mutex: &sync.RWMutex{}}
	req := httptest.NewRequest("POST", "http://example.com/api/v1/restore/stop",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPostRestoreStop(w, req, []httprouter.Param{})
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}
