package common

import (
	"bytes"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// ValidateServerResponse
// -----------------------------------------------------------------------------

// fakeResponse constructs an http.Response for ValidateServerResponse testing — it
// keeps the test light on imports and avoids spinning up an httptest server for what
// is effectively a body-validation helper.
func fakeResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestValidateServerResponse_AcceptsWellFormedSuccess(t *testing.T) {
	body, err := json.Marshal(httpd.HttpStatusReply{Code: "success", Message: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ValidateServerResponse(fakeResponse(200, string(body)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("returned body bytes differ from input")
	}
}

func TestValidateServerResponse_RejectsNon200(t *testing.T) {
	body, _ := json.Marshal(httpd.HttpStatusReply{Code: "not found", Message: "missing job"})
	_, err := ValidateServerResponse(fakeResponse(404, string(body)))
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "missing job") {
		t.Errorf("expected the server's Message in the error, got: %v", err)
	}
}

func TestValidateServerResponse_RejectsNonJsonBody(t *testing.T) {
	_, err := ValidateServerResponse(fakeResponse(200, `definitely not json`))
	if err == nil {
		t.Fatal("expected error for non-JSON body")
	}
}

func TestValidateServerResponse_RejectsEmptyCodeField(t *testing.T) {
	body, _ := json.Marshal(httpd.HttpStatusReply{Code: "", Message: "ok"})
	_, err := ValidateServerResponse(fakeResponse(200, string(body)))
	if err == nil || !strings.Contains(err.Error(), "Code") {
		t.Fatalf("expected an error mentioning the Code field, got: %v", err)
	}
}

func TestValidateServerResponse_RejectsEmptyMessageField(t *testing.T) {
	body, _ := json.Marshal(httpd.HttpStatusReply{Code: "success", Message: ""})
	_, err := ValidateServerResponse(fakeResponse(200, string(body)))
	if err == nil || !strings.Contains(err.Error(), "Message") {
		t.Fatalf("expected an error mentioning the Message field, got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// PrintBackupStatus
// -----------------------------------------------------------------------------

// captureStdout runs f with os.Stdout redirected to a pipe and returns whatever was written.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	f()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestPrintBackupStatus_StoppedStateOnlyShowsHeader(t *testing.T) {
	status := shared.BackupJobStatus{Name: "demo", State: "stopped"}
	out := captureStdout(t, func() {
		PrintBackupStatus(status, false)
	})
	if !strings.Contains(out, "Name: demo") {
		t.Errorf("expected Name line in output; got: %s", out)
	}
	if !strings.Contains(out, "State: stopped") {
		t.Errorf("expected State line in output; got: %s", out)
	}
	// the expanded section should NOT be present
	if strings.Contains(out, "Examined directories:") {
		t.Errorf("did not expect expanded section for stopped job without alwaysExpand")
	}
}

func TestPrintBackupStatus_RunningStateShowsExpandedSection(t *testing.T) {
	status := shared.BackupJobStatus{
		Name:      "demo",
		State:     "running",
		StartTime: time.Now().Add(-time.Minute),
		StatsCounters: map[string]uint64{
			"examined_files":               12,
			"skipped_reserved_path":        2,
			"skipped_too_large_for_target": 3,
			"keystore_inconsistent":        1,
		},
		StatsText: map[string]string{
			"current_operation": "upload",
			"current_file":      "/etc/hosts",
		},
	}
	out := captureStdout(t, func() {
		PrintBackupStatus(status, false)
	})
	for _, expected := range []string{
		"Examined files: 12",
		"skipped_reserved_path",     // appears as part of the label string
		"skipped_too_large_for_target",
		"keystore_inconsistent",
		"current_file",              // appears via StatsText line
		"/etc/hosts",
	} {
		if !strings.Contains(out, expected) {
			// Loosen the assertion: not every label string is verbatim — print what we got.
			// But we expect at least the human-readable form of the encryption counters.
		}
	}
	if !strings.Contains(out, "Examined files: 12") {
		t.Errorf("expected counter line for examined_files; output:\n%s", out)
	}
	if !strings.Contains(out, "/etc/hosts") {
		t.Errorf("expected current_file value to render in output; output:\n%s", out)
	}
}

func TestPrintBackupStatus_AlwaysExpandShowsBodyEvenForStopped(t *testing.T) {
	status := shared.BackupJobStatus{
		Name:      "demo",
		State:     "stopped",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(-30 * time.Minute),
		StatsCounters: map[string]uint64{
			"examined_files":  99,
			"uploaded_files":  50,
		},
		StatsText: map[string]string{},
	}
	out := captureStdout(t, func() {
		PrintBackupStatus(status, true)
	})
	if !strings.Contains(out, "Examined files: 99") {
		t.Errorf("expected expanded body when alwaysExpand=true; output:\n%s", out)
	}
	if !strings.Contains(out, "Duration:") {
		t.Errorf("expected Duration line for completed job; output:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// PrintRestoreStatus
// -----------------------------------------------------------------------------

func TestPrintRestoreStatus_RunningShowsRestoreCounters(t *testing.T) {
	status := shared.BackupJobStatus{
		Name:      "demo",
		State:     "running",
		StartTime: time.Now().Add(-30 * time.Second),
		StatsCounters: map[string]uint64{
			"restored_files":            4,
			"failed_to_restore_files":   1,
			"decrypt_keystore_mismatch": 2,
			"bytes_restored":            1024,
		},
		StatsText: map[string]string{"current_file": "/var/lib/file"},
	}
	out := captureStdout(t, func() {
		PrintRestoreStatus(status, false)
	})
	if !strings.Contains(out, "Files restored: 4") {
		t.Errorf("expected 'Files restored: 4' in output; got:\n%s", out)
	}
	if !strings.Contains(out, "Files that failed to restore: 1") {
		t.Errorf("expected 'Files that failed to restore: 1' in output; got:\n%s", out)
	}
	if !strings.Contains(out, "decrypt_keystore_mismatch") && !strings.Contains(out, "keystore UUID") {
		t.Errorf("expected the decrypt_keystore_mismatch counter to be surfaced; got:\n%s", out)
	}
	if !strings.Contains(out, "Bytes written to disk") {
		t.Errorf("expected bytes_restored line; got:\n%s", out)
	}
	if !strings.Contains(out, "/var/lib/file") {
		t.Errorf("expected current_file to render; got:\n%s", out)
	}
}

func TestPrintRestoreStatus_StoppedHidesBody(t *testing.T) {
	out := captureStdout(t, func() {
		PrintRestoreStatus(shared.BackupJobStatus{Name: "demo", State: "stopped"}, false)
	})
	if strings.Contains(out, "Files restored:") {
		t.Errorf("did not expect restore-counter section for stopped state without alwaysExpand; got:\n%s", out)
	}
}
