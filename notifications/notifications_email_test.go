package notifications

import (
	"cloudbackup/shared"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// GetNumNotificators
// -----------------------------------------------------------------------------

func TestGetNumNotificators_CountsBothKinds(t *testing.T) {
	cases := []struct {
		name string
		cfg  shared.ConfigNotification
		want int
	}{
		{"empty", shared.ConfigNotification{}, 0},
		{"only email", shared.ConfigNotification{Email: []shared.ConfigNotificationEmail{{}, {}}}, 2},
		{"only script", shared.ConfigNotification{Script: []shared.ConfigNotificationScript{{}}}, 1},
		{"mixed", shared.ConfigNotification{
			Email:  []shared.ConfigNotificationEmail{{}},
			Script: []shared.ConfigNotificationScript{{}, {}, {}},
		}, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GetNumNotificators(c.cfg); got != c.want {
				t.Errorf("GetNumNotificators = %d, want %d", got, c.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// prepareFromAddress
// -----------------------------------------------------------------------------

func TestPrepareFromAddress_ContainsCloudbackupAtHost(t *testing.T) {
	got := prepareFromAddress()
	if !strings.HasPrefix(got, "cloudbackup@") {
		t.Errorf("prepareFromAddress() = %q, expected to start with 'cloudbackup@'", got)
	}
	if !strings.Contains(got, "@") {
		t.Errorf("prepareFromAddress() = %q, expected to contain '@'", got)
	}
}

// -----------------------------------------------------------------------------
// prepareHtmlEmail
// -----------------------------------------------------------------------------

// Verify the rendered HTML body wires in each of the encryption-related counters
// added by the recent CSE work. Spot-checking the exact label strings catches
// regressions in the email template that wouldn't show up in higher-level tests.
func TestPrepareHtmlEmail_RendersEncryptionCounters(t *testing.T) {
	status := shared.BackupJobStatus{
		StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC),
		StatsCounters: map[string]uint64{
			"examined_files":               42,
			"skipped_reserved_path":        2,
			"skipped_too_large_for_target": 1,
			"keystore_inconsistent":        3,
			"decrypt_keystore_mismatch":    7,
		},
		StatsText: map[string]string{},
	}
	html := prepareHtmlEmail([]string{"backup job 'demo' has finished"}, status)
	for _, expected := range []string{
		"Files skipped (path collides with .cbcrypt/)",
		"Files skipped (encrypted size > target limit)",
		"Keystore inconsistency events",
		"Decrypt failures (keystore UUID mismatch)",
		"backup job &#39;demo&#39; has finished", // strings.Join escapes inside? actually no, raw passthrough
	} {
		// The Join uses <br>; the actual rendered text body should appear verbatim
		// except where Go's HTML-escape escapes single quotes — we accept either form
		// to keep this test loose around incidental escaping changes.
		if !strings.Contains(html, expected) && !strings.Contains(html, strings.ReplaceAll(expected, "&#39;", "'")) {
			t.Errorf("rendered HTML missing %q", expected)
		}
	}
	// Non-zero counters get the orange highlight via "bgcolor='orange'".
	if !strings.Contains(html, "bgcolor='orange'") {
		t.Error("rendered HTML did not highlight any non-zero counters in orange")
	}
}

// Duration is rounded to whole seconds — verify a 1-second gap renders as "1s".
// (time.Duration's String() uses bare units like "1s", "1m30s", etc.)
func TestPrepareHtmlEmail_DurationIsSecondRounded(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	status := shared.BackupJobStatus{
		StartTime:     start,
		EndTime:       start.Add(time.Second + 200*time.Millisecond),
		StatsCounters: map[string]uint64{},
		StatsText:     map[string]string{},
	}
	html := prepareHtmlEmail([]string{"x"}, status)
	if !strings.Contains(html, ">1s") {
		t.Errorf("expected duration of 1s for 1.2s gap (after Round); html prefix: %s", html[:min(len(html), 600)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// -----------------------------------------------------------------------------
// Execute — wiring through to sendEmail/runScript, with a stub SMTP listener
// -----------------------------------------------------------------------------

// startStubSMTPServer accepts a single TCP connection on 127.0.0.1, reads SMTP commands
// until QUIT or the client disconnects, and respond just enough for jordan-wright/email
// to consider the send a success. It is intentionally minimal — the goal is to verify
// the notifications.Execute path connects and emits the EHLO/MAIL/RCPT/DATA dance, not
// to validate SMTP itself. Returns the listener address and a stop function.
func startStubSMTPServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not start stub SMTP listener: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		write := func(s string) { _, _ = conn.Write([]byte(s)) }
		buf := make([]byte, 4096)
		write("220 stub ESMTP ready\r\n")
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			line := strings.ToUpper(string(buf[:n]))
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				write("250-stub\r\n250 OK\r\n")
			case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
				write("250 OK\r\n")
			case strings.HasPrefix(line, "DATA"):
				write("354 OK\r\n")
			case strings.Contains(line, "\r\n.\r\n"):
				write("250 OK\r\n")
			case strings.HasPrefix(line, "QUIT"):
				write("221 bye\r\n")
				return
			default:
				write("250 OK\r\n")
			}
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		wg.Wait()
	}
}

func TestExecute_ReturnsErrorWhenEmailServerUnavailable(t *testing.T) {
	// Use a port that is definitely closed — the connection will be refused and we
	// expect Execute to bubble the error back.
	cfg := shared.CfgTemplate{
		Notifications: shared.ConfigNotification{
			Email: []shared.ConfigNotificationEmail{{
				Server: "127.0.0.1",
				Port:   "1", // privileged port that nothing is listening on
				To:     "ops@example.com",
			}},
		},
	}
	_, err := Execute(cfg, "job-id", "backup", "test", "demo", "", "")
	if err == nil {
		t.Fatal("expected Execute to surface the SMTP connection error, got nil")
	}
	if !strings.Contains(err.Error(), "1 notification definitions were run") {
		t.Errorf("expected error to summarize one-of-one failure, got: %v", err)
	}
}

func TestExecute_SuccessAgainstStubSMTPServer(t *testing.T) {
	addr, stop := startStubSMTPServer(t)
	defer stop()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	cfg := shared.CfgTemplate{
		Notifications: shared.ConfigNotification{
			Email: []shared.ConfigNotificationEmail{{
				Server: host,
				Port:   port,
				From:   "from@example.com",
				To:     "to@example.com",
			}},
		},
	}
	_, err = Execute(cfg, "job-id", "backup", "test", "demo", "", "")
	if err != nil {
		t.Errorf("Execute returned error against stub SMTP server: %v", err)
	}
}

func TestExecute_ReturnsErrorWithMixedScriptFailure(t *testing.T) {
	cfg := shared.CfgTemplate{
		Notifications: shared.ConfigNotification{
			Script: []shared.ConfigNotificationScript{{
				Path: "/this/script/does/not/exist",
			}},
		},
	}
	failedScripts, err := Execute(cfg, "job-id", "backup", "failed", "demo", "", "boom")
	if err == nil {
		t.Fatal("expected Execute to report script failure")
	}
	if failedScripts != 1 {
		t.Errorf("expected failedScripts=1, got %d", failedScripts)
	}
}

// ensure the package-level smtp import is exercised so future refactors that drop the
// import are caught quickly; otherwise some Go test caches will compile-ignore unused
// imports in test files.
var _ = smtp.PlainAuth
