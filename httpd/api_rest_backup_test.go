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

// minimalBackupSrv returns a SrvData populated just enough that handlers can call
// GetCopyWithLock and globalcfg.GetCopyWithLock without panicking. The Backup slice is
// caller-supplied so tests can drive both the "name not found → 404" path and the
// happy path.
func minimalBackupSrv(backups []shared.ConfigBackup) SrvData {
	cfg := &shared.RuntimeConfig{
		Mutex: &sync.RWMutex{},
		Config: shared.CfgTemplate{
			Mutex:  &sync.RWMutex{},
			Backup: backups,
		},
	}
	return SrvData{
		Mutex:           &sync.RWMutex{},
		globalcfg:       cfg,
		backupJobsState: shared.NewJobsState(),
	}
}

func postBackup(t *testing.T, h func(http.ResponseWriter, *http.Request, httprouter.Params), path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "http://example.com"+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req, []httprouter.Param{})
	return w.Result()
}

// -----------------------------------------------------------------------------
// handlerPostBackupStart
// -----------------------------------------------------------------------------

func TestBackupStartRejectsInvalidJson(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupStart, "/api/v1/backup/start", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupStartRejectsMissingName(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupStart, "/api/v1/backup/start", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupStartReturns404ForUnknownJob(t *testing.T) {
	srv := minimalBackupSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postBackup(t, srv.handlerPostBackupStart, "/api/v1/backup/start", `{"name":"missing"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostBackupStop
// -----------------------------------------------------------------------------

func TestBackupStopRejectsInvalidJson(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupStop, "/api/v1/backup/stop", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupStopRejectsMissingName(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupStop, "/api/v1/backup/stop", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupStopRejectsNotRunning(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupStop, "/api/v1/backup/stop", `{"name":"some_backup"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (not running), got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostBackupDryRun
// -----------------------------------------------------------------------------

func TestBackupDryRunRejectsInvalidJson(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupDryRun, "/api/v1/backup/dryrun", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupDryRunRejectsMissingName(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupDryRun, "/api/v1/backup/dryrun", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupDryRunReturns404ForUnknownJob(t *testing.T) {
	srv := minimalBackupSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postBackup(t, srv.handlerPostBackupDryRun, "/api/v1/backup/dryrun", `{"name":"missing"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostBackupWatch
// -----------------------------------------------------------------------------

func TestBackupWatchRejectsInvalidJson(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupWatch, "/api/v1/backup/watch", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupWatchRejectsMissingName(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupWatch, "/api/v1/backup/watch", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostBackupTargetTest
// -----------------------------------------------------------------------------

func TestBackupTargetTestRejectsInvalidJson(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupTargetTest, "/api/v1/backup/target/test", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupTargetTestRejectsMissingName(t *testing.T) {
	srv := minimalBackupSrv(nil)
	resp := postBackup(t, srv.handlerPostBackupTargetTest, "/api/v1/backup/target/test", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupTargetTestReturns404ForUnknownJob(t *testing.T) {
	srv := minimalBackupSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postBackup(t, srv.handlerPostBackupTargetTest, "/api/v1/backup/target/test", `{"name":"missing"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerGetBackupList
// -----------------------------------------------------------------------------

// The list handler doesn't take a body — confirm it produces a JSON response from an
// empty job state. This protects against panics when the state is freshly initialised.
func TestBackupListReturnsJsonOnEmptyState(t *testing.T) {
	srv := minimalBackupSrv(nil)
	req := httptest.NewRequest("GET", "http://example.com/api/v1/backup/list", nil)
	w := httptest.NewRecorder()
	srv.handlerGetBackupList(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Errorf("expected a Content-Type header on list response")
	}
}
