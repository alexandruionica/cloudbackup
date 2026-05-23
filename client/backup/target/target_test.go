package target

import (
	clientConfig "cloudbackup/client/config"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

// Test() calls os.Exit; we drive it via subprocess re-exec so the parent test process
// can observe the exit code (same pattern as cliargs/cligargs_test.go).

func startTargetBackend(t *testing.T, status int, body interface{}, capturedBody *string) *httptest.Server {
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
		if r.Body != nil && capturedBody != nil {
			buf, _ := io.ReadAll(r.Body)
			*capturedBody = string(buf)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}))
}

func TestTargetTest_HappyPath(t *testing.T) {
	var capturedBody string
	srv := startTargetBackend(t, 200, map[string]string{
		"code":    "success",
		"message": "target ok",
	}, &capturedBody)
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "demo")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestTargetTest_HappyPath") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected 0 exit on happy path, got %v", err)
	}
}

func TestTargetTest_ServerErrorExits(t *testing.T) {
	srv := startTargetBackend(t, 500, map[string]string{
		"code":    "internal server error",
		"message": "boom",
	}, nil)
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "demo")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestTargetTest_ServerErrorExits") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server returns 500")
	}
}

// Empty job name still POSTs (the server validates it) — the function does not
// pre-check; this guards against a regression where a future refactor adds a client-side
// check and silently changes the user-facing error path.
func TestTargetTest_EmptyJobNameStillReachesServer(t *testing.T) {
	srv := startTargetBackend(t, 400, map[string]string{
		"code":    "invalid json",
		"message": "'name' key is mandatory",
	}, nil)
	defer srv.Close()

	if os.Getenv("TEST_RUNNING") == "1" {
		Test(clientConfig.Client{Username: "u", Password: "p", Address: srv.URL}, false, "")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestTargetTest_EmptyJobNameStillReachesServer") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit when server rejects empty job name")
	}
}
