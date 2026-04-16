package httpd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/julienschmidt/httprouter"
)

// helper to drive handlerPostReportRestoreList with the given JSON body.
func postReportRestoreList(t *testing.T, body string) *http.Response {
	t.Helper()
	srv := SrvData{Mutex: &sync.RWMutex{}}
	req := httptest.NewRequest("POST", "http://example.com/api/v1/report/restore/list",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPostReportRestoreList(w, req, []httprouter.Param{})
	return w.Result()
}

// helper to drive handlerPostReportRestoreShow with the given JSON body.
func postReportRestoreShow(t *testing.T, body string) *http.Response {
	t.Helper()
	srv := SrvData{Mutex: &sync.RWMutex{}}
	req := httptest.NewRequest("POST", "http://example.com/api/v1/report/restore/show",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPostReportRestoreShow(w, req, []httprouter.Param{})
	return w.Result()
}

func decodeBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("could not read response body: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("could not decode response body: %v (body: %s)", err, body)
	}
	return result
}

// --- /report/restore/list input validation ---

func TestReportRestoreListRejectsInvalidJson(t *testing.T) {
	resp := postReportRestoreList(t, `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportRestoreListRejectsMissingName(t *testing.T) {
	resp := postReportRestoreList(t, `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	decoded := decodeBody(t, resp)
	if decoded["code"] != HttpErrInvalidJson {
		t.Errorf("expected code=%q, got %q", HttpErrInvalidJson, decoded["code"])
	}
}

func TestReportRestoreListRejectsInvalidFromStartTime(t *testing.T) {
	resp := postReportRestoreList(t, `{"name":"job1","from_start_time":"not-a-date"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportRestoreListRejectsInvalidUntilStartTime(t *testing.T) {
	resp := postReportRestoreList(t, `{"name":"job1","until_start_time":"not-a-date"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportRestoreListRejectsUntilBeforeFrom(t *testing.T) {
	resp := postReportRestoreList(t, `{"name":"job1","from_start_time":"2026-04-10T00:00:00Z","until_start_time":"2026-04-01T00:00:00Z"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- /report/restore/show input validation ---

func TestReportRestoreShowRejectsInvalidJson(t *testing.T) {
	resp := postReportRestoreShow(t, `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportRestoreShowRejectsMissingName(t *testing.T) {
	resp := postReportRestoreShow(t, `{"job_id":"some-id"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	decoded := decodeBody(t, resp)
	if decoded["code"] != HttpErrInvalidJson {
		t.Errorf("expected code=%q, got %q", HttpErrInvalidJson, decoded["code"])
	}
}

func TestReportRestoreShowRejectsMissingJobId(t *testing.T) {
	resp := postReportRestoreShow(t, `{"name":"job1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	decoded := decodeBody(t, resp)
	if decoded["code"] != HttpErrInvalidJson {
		t.Errorf("expected code=%q, got %q", HttpErrInvalidJson, decoded["code"])
	}
}

func TestReportRestoreShowRejectsEmptyBody(t *testing.T) {
	resp := postReportRestoreShow(t, `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
