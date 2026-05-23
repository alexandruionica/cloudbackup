package notification

import (
	clientConfig "cloudbackup/client/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

// Test() in this package calls os.Exit on every path. We use the subprocess re-exec
// pattern (same as cliargs/cligargs_test.go) so the parent test process survives the
// child's os.Exit and we can assert exit codes.

func startStubBackend(t *testing.T, status int, body interface{}) (*httptest.Server, error) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}))
	return srv, nil
}

func TestNotificationTest_HappyPath(t *testing.T) {
	srv, err := startStubBackend(t, 200, map[string]string{
		"code":    "success",
		"message": "notification test passed",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNotificationTest_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1", "CLD_TEST_BACKEND="+srv.URL)
	// the subprocess re-runs THIS test from scratch which means it will get its own srv URL —
	// but that doesn't matter for the assertion that the call ends with os.Exit(0)
	// (Test() emits os.Exit(0) on jsonOutput=true success). Since we use jsonOutput=false the
	// function falls through to the println and returns from the function — but never os.Exit(0).
	// Therefore we expect a 0 exit code (normal test completion).
	if err := cmd.Run(); err != nil {
		t.Fatalf("process ran with err %v, want 0 exit", err)
	}
}

func TestNotificationTest_ServerErrorExits(t *testing.T) {
	srv, err := startStubBackend(t, 500, map[string]string{
		"code":    "internal server error",
		"message": "boom",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNotificationTest_ServerErrorExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server returns 500, got 0")
	}
}

func TestNotificationTest_JsonOutputExitsZeroOnSuccess(t *testing.T) {
	srv, err := startStubBackend(t, 200, map[string]string{
		"code":    "success",
		"message": "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, true)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNotificationTest_JsonOutputExitsZeroOnSuccess") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0 when jsonOutput=true succeeds, got %v", err)
	}
}
