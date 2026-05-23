package backup

import (
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// The Start/Stop/Status/List/Watch/DryRun entry points all call os.Exit. We use
// subprocess re-exec to assert exit codes for representative paths. Detailed
// behaviour belongs in a future refactor that extracts non-exiting do* helpers
// (mirroring client/restore/restore.go's pattern).

func startBackupBackend(t *testing.T, status int, body interface{}) *httptest.Server {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}))
}

func TestList_HappyPath(t *testing.T) {
	srv := startBackupBackend(t, 200, ListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "ok"},
		Result: []shared.BackupJobStatus{
			{Name: "demo", State: "stopped"},
		},
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		List(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestList_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v", err)
	}
}

func TestList_ServerErrorExits(t *testing.T) {
	srv := startBackupBackend(t, 500, map[string]string{
		"code":    "internal server error",
		"message": "boom",
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		List(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestList_ServerErrorExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server returns 500")
	}
}

func TestStatus_HappyPath(t *testing.T) {
	srv := startBackupBackend(t, 200, ListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "ok"},
		Result: []shared.BackupJobStatus{
			{Name: "demo", State: "running", BackupJobId: "uuid-1", StartTime: time.Now()},
		},
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		Status(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "demo", "")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestStatus_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v", err)
	}
}

func TestStart_HappyPath(t *testing.T) {
	srv := startBackupBackend(t, 200, StartStopResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "started"},
		Result:          httpd.BackupJob{Name: "demo", JobId: "JOB-UUID"},
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		Start(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "demo", false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestStart_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v", err)
	}
}

func TestStart_ServerErrorExits(t *testing.T) {
	srv := startBackupBackend(t, 404, map[string]string{
		"code":    "not found",
		"message": "no such job",
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		Start(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "no_such_job", false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestStart_ServerErrorExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server returns 404")
	}
}

func TestStop_HappyPath(t *testing.T) {
	srv := startBackupBackend(t, 200, StartStopResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "stopped"},
		Result:          httpd.BackupJob{Name: "demo", JobId: "JOB-UUID"},
	})
	defer srv.Close()
	if os.Getenv("TEST_RUNNING") == "1" {
		Stop(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "demo", "")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestStop_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got %v", err)
	}
}
