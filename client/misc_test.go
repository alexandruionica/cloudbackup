package client

import (
	clientConfig "cloudbackup/client/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"cloudbackup/httpd"
	"cloudbackup/misc"
)

// RetrieveServerVersion calls os.Exit on success and failure; use subprocess re-exec
// to assert exit codes.

func startVersionBackend(t *testing.T, status int, body interface{}) *httptest.Server {
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

func TestRetrieveServerVersion_HappyPath(t *testing.T) {
	srv := startVersionBackend(t, 200, VersionResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "ok"},
		Result:          misc.Version{CloudBackup: "1.2.3", BuildDate: "2026-01-01"},
	})
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		RetrieveServerVersion(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRetrieveServerVersion_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0 on happy path, got %v", err)
	}
}

func TestRetrieveServerVersion_ServerErrorExits(t *testing.T) {
	srv := startVersionBackend(t, 500, map[string]string{
		"code":    "internal server error",
		"message": "boom",
	})
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		RetrieveServerVersion(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRetrieveServerVersion_ServerErrorExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server returns 500")
	}
}

func TestRetrieveServerVersion_BadAuthExits(t *testing.T) {
	srv := startVersionBackend(t, 401, map[string]string{
		"code":    "unauthorized",
		"message": "bad creds",
	})
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		RetrieveServerVersion(clientConfig.Client{Username: "wrong", Password: "creds", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRetrieveServerVersion_BadAuthExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when basic auth fails")
	}
}
