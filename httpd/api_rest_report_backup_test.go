package httpd

import (
	"bytes"
	"cloudbackup/shared"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/julienschmidt/httprouter"
)

// minimalReportSrv mirrors minimalBackupSrv (api_rest_backup_test.go) but is duplicated as a
// separate helper so each test file can be read in isolation.
func minimalReportSrv(backups []shared.ConfigBackup) SrvData {
	return SrvData{
		Mutex: &sync.RWMutex{},
		globalcfg: &shared.RuntimeConfig{
			Mutex: &sync.RWMutex{},
			Config: shared.CfgTemplate{
				Mutex:  &sync.RWMutex{},
				Backup: backups,
			},
		},
		backupJobsState: shared.NewJobsState(),
	}
}

func postReport(t *testing.T, h func(http.ResponseWriter, *http.Request, httprouter.Params), path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "http://example.com"+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req, []httprouter.Param{})
	return w.Result()
}

// -----------------------------------------------------------------------------
// handlerPostReportBackupList
// -----------------------------------------------------------------------------

func TestReportBackupListRejectsInvalidJson(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList, "/api/v1/report/backup/list", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsMissingName(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList, "/api/v1/report/backup/list", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsInvalidFromStartTime(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"j","from_start_time":"not-a-time"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsInvalidUntilStartTime(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"j","until_start_time":"not-a-time"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsUntilBeforeFrom(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"j","from_start_time":"2026-01-02T00:00:00Z","until_start_time":"2026-01-01T00:00:00Z"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsCorruptNextToken(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"j","next":"!!! not base64 !!!"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed next token, got %d", resp.StatusCode)
	}
}

func TestReportBackupListRejectsNextTokenWithWrongPartCount(t *testing.T) {
	// Base64 of "only:two" — decodes cleanly but doesn't have the 4 colon-separated parts that
	// decodeNextTokenOfReportBackupList expects, so it should be rejected as malformed.
	tok := base64.StdEncoding.EncodeToString([]byte("only:two"))
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"j","next":"`+tok+`"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for next token with wrong part count, got %d", resp.StatusCode)
	}
}

func TestReportBackupListReturns404ForUnknownJob(t *testing.T) {
	srv := minimalReportSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postReport(t, srv.handlerPostReportBackupList,
		"/api/v1/report/backup/list",
		`{"name":"missing"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostReportBackupShow
// -----------------------------------------------------------------------------

func TestReportBackupShowRejectsInvalidJson(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupShow, "/api/v1/report/backup/show", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupShowRejectsMissingName(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupShow, "/api/v1/report/backup/show", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupShowRejectsMissingJobId(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupShow, "/api/v1/report/backup/show", `{"name":"j"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupShowReturns404ForUnknownJob(t *testing.T) {
	srv := minimalReportSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postReport(t, srv.handlerPostReportBackupShow,
		"/api/v1/report/backup/show",
		`{"name":"missing","job_id":"abc"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostReportBackupFileList
// -----------------------------------------------------------------------------

func TestReportBackupFileListRejectsInvalidJson(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupFileList, "/api/v1/report/backup/file/list", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupFileListRejectsMissingName(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupFileList, "/api/v1/report/backup/file/list", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestReportBackupFileListRejectsCorruptNextToken(t *testing.T) {
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupFileList,
		"/api/v1/report/backup/file/list",
		`{"name":"j","next":"!!! not base64 !!!"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed next token, got %d", resp.StatusCode)
	}
}

func TestReportBackupFileListRejectsNextTokenWithWrongPartCount(t *testing.T) {
	// Base64 of "1:2:3:4" — four parts; the file-list token expects five.
	tok := base64.StdEncoding.EncodeToString([]byte("1:2:3:4"))
	srv := minimalReportSrv(nil)
	resp := postReport(t, srv.handlerPostReportBackupFileList,
		"/api/v1/report/backup/file/list",
		`{"name":"j","next":"`+tok+`"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for next token with wrong part count, got %d", resp.StatusCode)
	}
}

func TestReportBackupFileListReturns404ForUnknownJob(t *testing.T) {
	srv := minimalReportSrv([]shared.ConfigBackup{{Name: "real_backup"}})
	resp := postReport(t, srv.handlerPostReportBackupFileList,
		"/api/v1/report/backup/file/list",
		`{"name":"missing","job_id":"abc"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// handlerPostNotificationTest
// -----------------------------------------------------------------------------

// With no notification entries configured the handler must short-circuit with 500
// rather than try to send a phantom notification.
func TestNotificationTestRejectsWhenNoNotificationsConfigured(t *testing.T) {
	srv := minimalReportSrv(nil)
	req := httptest.NewRequest("POST", "http://example.com/api/v1/report/notification/test", nil)
	w := httptest.NewRecorder()
	srv.handlerPostNotificationTest(w, req, []httprouter.Param{})
	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 when no notifications configured, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// decodeNextTokenOfReportBackupList — direct unit coverage of the parser since
// the handler path only exercises the 'malformed' branch
// -----------------------------------------------------------------------------

func TestDecodeNextTokenOfReportBackupListRoundtrip(t *testing.T) {
	tok := buildNextTokenOfReportBackupList(10, 5, 100, 200)
	limit, offset, earliest, latest, err := decodeNextTokenOfReportBackupList(tok)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if limit != 10 || offset != 5 || earliest != 100 || latest != 200 {
		t.Errorf("roundtrip mismatch: got limit=%d offset=%d earliest=%d latest=%d",
			limit, offset, earliest, latest)
	}
}

func TestDecodeNextTokenOfReportBackupListRejectsNonInt(t *testing.T) {
	tok := base64.StdEncoding.EncodeToString([]byte("abc:5:100:200"))
	_, _, _, _, err := decodeNextTokenOfReportBackupList(tok)
	if err == nil {
		t.Fatal("expected error for non-integer limit, got nil")
	}
}

// -----------------------------------------------------------------------------
// decodeNextTokenOfReportBackupFileList — direct unit coverage
// -----------------------------------------------------------------------------

func TestDecodeNextTokenOfReportBackupFileListRoundtrip(t *testing.T) {
	tok := buildNextTokenOfReportBackupFileList(20, 40, "JOB-UUID", "/some/path", true)
	limit, offset, jobId, path, descend, err := decodeNextTokenOfReportBackupFileList(tok)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if limit != 20 || offset != 40 || jobId != "JOB-UUID" || path != "/some/path" || !descend {
		t.Errorf("roundtrip mismatch: got limit=%d offset=%d jobId=%q path=%q descend=%v",
			limit, offset, jobId, path, descend)
	}
}

func TestDecodeNextTokenOfReportBackupFileListRejectsBadDescend(t *testing.T) {
	tok := base64.StdEncoding.EncodeToString([]byte("1:2:job:path:not-a-bool"))
	_, _, _, _, _, err := decodeNextTokenOfReportBackupFileList(tok)
	if err == nil {
		t.Fatal("expected error for non-bool descend, got nil")
	}
}

// A Windows absolute path contains a ':' in the drive letter (e.g. "C:\\Users").
// Since ':' is also the token field separator, the path must still round-trip
// intact; a naive Split(":") that requires exactly five parts would mis-parse it.
func TestDecodeNextTokenOfReportBackupFileListRoundtripWindowsPath(t *testing.T) {
	winPath := `C:\Users\vagrant\AppData\Local\Temp\integration_test_bzpo4xob`
	tok := buildNextTokenOfReportBackupFileList(2, 2, "08ecef18-6a11-4932-bc9e-8e8e1a56076b", winPath, true)
	limit, offset, jobId, path, descend, err := decodeNextTokenOfReportBackupFileList(tok)
	if err != nil {
		t.Fatalf("unexpected decode error for Windows path: %v", err)
	}
	if limit != 2 || offset != 2 || jobId != "08ecef18-6a11-4932-bc9e-8e8e1a56076b" ||
		path != winPath || !descend {
		t.Errorf("roundtrip mismatch: got limit=%d offset=%d jobId=%q path=%q descend=%v",
			limit, offset, jobId, path, descend)
	}
}
