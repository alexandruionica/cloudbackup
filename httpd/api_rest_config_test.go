package httpd

import (
	"bytes"
	"cloudbackup/shared"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/julienschmidt/httprouter"
)

func minimalConfigSrv() SrvData {
	return SrvData{
		Mutex: &sync.RWMutex{},
		globalcfg: &shared.RuntimeConfig{
			Mutex:  &sync.RWMutex{},
			Config: shared.CfgTemplate{Mutex: &sync.RWMutex{}},
		},
		backupJobsState: shared.NewJobsState(),
	}
}

func postConfig(t *testing.T, h func(http.ResponseWriter, *http.Request, httprouter.Params), path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "http://example.com"+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req, []httprouter.Param{})
	return w.Result()
}

// -----------------------------------------------------------------------------
// handlerGetConfig
// -----------------------------------------------------------------------------

// GET /api/v1/config should return 200 with a JSON body even when the loaded config
// is essentially empty. This guards against a regression in SanitizeCfgTemplate or
// the JSONSuccessWithResult writer when the input has no users/backups/notifications.
func TestGetConfigReturnsJsonOnEmptyConfig(t *testing.T) {
	srv := minimalConfigSrv()
	req := httptest.NewRequest("GET", "http://example.com/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.handlerGetConfig(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPutConfig
// -----------------------------------------------------------------------------

func TestPutConfigRejectsEmptyBody(t *testing.T) {
	srv := minimalConfigSrv()
	req := httptest.NewRequest("POST", "http://example.com/api/v1/config", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPutConfig(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestPutConfigRejectsInvalidJson(t *testing.T) {
	srv := minimalConfigSrv()
	resp := postConfig(t, srv.handlerPutConfig, "/api/v1/config", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// A syntactically valid JSON body that fails advanced config validation should be
// rejected with 400 — not silently accepted. An entirely empty config has no users
// or data_dir, which config.Validate refuses.
func TestPutConfigRejectsConfigFailingValidation(t *testing.T) {
	srv := minimalConfigSrv()
	resp := postConfig(t, srv.handlerPutConfig, "/api/v1/config", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for config that fails validation, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPutConfigBackup
// -----------------------------------------------------------------------------

func TestPutConfigBackupRejectsEmptyBody(t *testing.T) {
	srv := minimalConfigSrv()
	req := httptest.NewRequest("POST", "http://example.com/api/v1/config/backup", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handlerPutConfigBackup(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestPutConfigBackupRejectsInvalidJson(t *testing.T) {
	srv := minimalConfigSrv()
	resp := postConfig(t, srv.handlerPutConfigBackup, "/api/v1/config/backup", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPutConfigBackupRejectsBackupFailingValidation(t *testing.T) {
	// Missing name, target etc. — config.ValidateBackup must refuse this.
	srv := minimalConfigSrv()
	resp := postConfig(t, srv.handlerPutConfigBackup, "/api/v1/config/backup", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for backup config that fails validation, got %d", resp.StatusCode)
	}
}
