package objectstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloudbackup/shared"

	"golang.org/x/time/rate"
)

func TestCalculateRemotePath(t *testing.T) {
	cases := []struct {
		name     string
		prefix   string
		path     string
		metadata bool
		want     string
	}{
		{"data keeps the full local path", "backups/srv1", "/var/lib/app/file.db", false, "backups/srv1/" + DataPrepend + "/var/lib/app/file.db"},
		{"metadata keeps only the base name", "backups/srv1", "/tmp/data/first_backup.sqlite.gz", true, "backups/srv1/" + MetaDataPrepend + "/first_backup.sqlite.gz"},
		{"double slashes are squashed", "backups/srv1/", "/etc//hosts", false, "backups/srv1/" + DataPrepend + "/etc/hosts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateRemotePath(tc.prefix, tc.path, tc.metadata); got != tc.want {
				t.Fatalf("calculateRemotePath(%q, %q, %v) = %q, want %q", tc.prefix, tc.path, tc.metadata, got, tc.want)
			}
		})
	}
	if runtime.GOOS == "windows" {
		got := calculateRemotePath("p", `C:\data\f.txt`, false)
		if strings.Contains(got, `\`) {
			t.Fatalf("windows separators must be converted to forward slashes, got %q", got)
		}
	}
}

func TestResolveBoolParameter(t *testing.T) {
	params := []shared.ConfigBackupTargetParams{
		{Name: "Use_SSL", Value: "yes"},
		{Name: "verify", Value: "false"},
		{Name: "broken", Value: "maybe"},
	}
	var got bool
	resolveBoolParameter("use_ssl", &got, params, false)
	if !got {
		t.Fatal("case-insensitive match with 'yes' must yield true")
	}
	resolveBoolParameter("verify", &got, params, true)
	if got {
		t.Fatal("explicit 'false' must win over the default")
	}
	resolveBoolParameter("broken", &got, params, true)
	if !got {
		t.Fatal("an unparsable value must fall back to the default")
	}
	resolveBoolParameter("absent", &got, params, true)
	if !got {
		t.Fatal("an absent parameter must yield the default")
	}
}

func TestCrc32Hash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "check.txt")
	// "123456789" is the standard CRC-32C (Castagnoli) check input; its checksum is 0xE3069283
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := crc32Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0xE3069283 {
		t.Fatalf("crc32c = %#x, want 0xe3069283", got)
	}
	if _, err := crc32Hash(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestFileReaderSeekAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seek.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	fr, err := NewFileReader(path, nil, &noopJobsState{}, "job", "t1", "test_null", 0, 0, 10, context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if off, err := fr.Seek(4, io.SeekStart); err != nil || off != 4 {
		t.Fatalf("Seek(4) = %d, %v", off, err)
	}
	buf := make([]byte, 3)
	n, err := fr.Read(buf)
	if err != nil || string(buf[:n]) != "456" {
		t.Fatalf("read after seek = %q, %v", buf[:n], err)
	}
	if off, err := fr.Seek(-2, io.SeekEnd); err != nil || off != 8 {
		t.Fatalf("Seek(-2, end) = %d, %v", off, err)
	}
	fr.Close()
}

type countingBody struct {
	r      io.Reader
	closed bool
}

func (c *countingBody) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *countingBody) Close() error               { c.closed = true; return nil }

func TestRateLimitedRequestBodyPassesThroughWithoutLimit(t *testing.T) {
	body := &countingBody{r: bytes.NewReader([]byte("payload"))}
	w := &wrapAroundTransportRequestBody{origBody: body, ctx: context.Background(), rateLimit: 0}
	got, err := io.ReadAll(w)
	if err != nil || string(got) != "payload" {
		t.Fatalf("ReadAll = %q, %v", got, err)
	}
	if err := w.Close(); err != nil || !body.closed {
		t.Fatal("Close must reach the wrapped body")
	}
}

func TestRateLimitedRequestBodyThrottlesAndHonoursBurst(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 4096)
	body := &countingBody{r: bytes.NewReader(payload)}
	// 1 KiB/s with a 1 KiB burst: 4 KiB must take on the order of three seconds; reads larger than
	// the burst are served in burst-sized pieces rather than refused
	w := &wrapAroundTransportRequestBody{origBody: body, ctx: context.Background(),
		bucket: rate.NewLimiter(rate.Limit(1024), 1024), rateLimit: 1024, burst: 1024}
	start := time.Now()
	buf := make([]byte, 8192) // larger than burst on purpose
	var total int
	for {
		n, err := w.Read(buf)
		total += n
		if n > 1024 {
			t.Fatalf("a single read returned %d bytes, above the %d burst", n, 1024)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if total != len(payload) {
		t.Fatalf("read %d bytes, want %d", total, len(payload))
	}
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Fatalf("4 KiB at 1 KiB/s finished in %s; the limiter is not applied", elapsed)
	}
}

func TestRateLimitedRequestBodyStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &wrapAroundTransportRequestBody{origBody: &countingBody{r: bytes.NewReader([]byte("abc"))}, ctx: ctx,
		bucket: rate.NewLimiter(rate.Limit(10), 10), rateLimit: 10, burst: 10}
	if _, err := w.Read(make([]byte, 4)); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type recordingTransport struct{ seen *http.Request }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.seen = req
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

func TestWrapAroundTransportWrapsOnlyRealBodies(t *testing.T) {
	inner := &recordingTransport{}
	tr := &wrapAroundTransport{origTransport: inner, ctx: context.Background(),
		bucket: rate.NewLimiter(rate.Inf, 1<<20), rateLimit: 1024, burst: 1 << 20}

	get, _ := http.NewRequest(http.MethodGet, "http://example/x", nil)
	if _, err := tr.RoundTrip(get); err != nil {
		t.Fatal(err)
	}
	if inner.seen.Body != nil {
		t.Fatal("a nil body must stay nil")
	}
	noBody, _ := http.NewRequest(http.MethodPost, "http://example/x", http.NoBody)
	if _, err := tr.RoundTrip(noBody); err != nil {
		t.Fatal(err)
	}
	if inner.seen.Body != http.NoBody {
		t.Fatal("http.NoBody must stay untouched")
	}
	put, _ := http.NewRequest(http.MethodPut, "http://example/x", bytes.NewReader([]byte("upload")))
	if _, err := tr.RoundTrip(put); err != nil {
		t.Fatal(err)
	}
	if _, ok := inner.seen.Body.(*wrapAroundTransportRequestBody); !ok {
		t.Fatalf("a real body must be wrapped for rate limiting, got %T", inner.seen.Body)
	}
	got, _ := io.ReadAll(inner.seen.Body)
	if string(got) != "upload" {
		t.Fatalf("wrapped body delivered %q", got)
	}
}
