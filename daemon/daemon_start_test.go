//go:build !windows

package daemon

import (
	"bytes"
	"cloudbackup/testutils"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestStartServesTheApi boots the daemon exactly as "server start" does, on a temp config and a
// free port, and checks the API answers with the configured credentials. Start never returns (it
// waits for signals), so the daemon is left running inside the test binary; the SIGUSR1 and
// unknown-signal branches of ProcessSignal are driven directly because the exiting branches call
// os.Exit.
func TestStartServesTheApi(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_daemon_start_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("\nhttp:")) {
		t.Fatalf("the mock config now carries an http section; adjust this test:\n%s", raw)
	}
	patched := append(append([]byte{}, raw...), []byte(fmt.Sprintf("\nhttp:\n  bind_address: %q\n", addr))...)
	if err := os.WriteFile(path, patched, 0o600); err != nil { // #nosec G703 -- temp path from the test helper
		t.Fatal(err)
	}

	go Start(path, false)

	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/api/v1/backup/list", nil)
		req.SetBasicAuth("testuser1", "HV}H/y?<9$]Z5N4N")
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("daemon did not come up on %s: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/backup/list = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/api/v1/backup/list", nil)
	req.SetBasicAuth("testuser1", "wrong")
	bad, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password answered %d, want 401", bad.StatusCode)
	}

	// non-exiting signal branches
	ProcessSignal(syscall.SIGUSR1, nil, nil)
	ProcessSignal(syscall.SIGHUP, nil, nil)
}
