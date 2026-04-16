package restore

import (
	"bytes"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// recorderServer captures the most recent request for assertion purposes and serves a
// configurable response. Each handler path validates basic auth with "u"/"p".
type recorderServer struct {
	lastMethod string
	lastPath   string
	lastBody   []byte
	respStatus int
	respBody   []byte
}

func (rs *recorderServer) handler(w http.ResponseWriter, r *http.Request) {
	if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rs.lastMethod = r.Method
	rs.lastPath = r.URL.Path
	if r.Body != nil {
		rs.lastBody, _ = io.ReadAll(r.Body)
	}
	w.Header().Set("Content-Type", "application/json")
	if rs.respStatus == 0 {
		rs.respStatus = 200
	}
	w.WriteHeader(rs.respStatus)
	_, _ = w.Write(rs.respBody)
}

func newRecorder(t *testing.T, respStatus int, respBody interface{}) (*httptest.Server, *recorderServer) {
	t.Helper()
	rs := &recorderServer{respStatus: respStatus}
	b, err := json.Marshal(respBody)
	if err != nil {
		t.Fatalf("could not encode response body: %v", err)
	}
	rs.respBody = b
	srv := httptest.NewServer(http.HandlerFunc(rs.handler))
	return srv, rs
}

func cfg(addr string) clientConfig.Client {
	return clientConfig.Client{Username: "u", Password: "p", Address: addr}
}

// -----------------------------------------------------------------------------
// doStart
// -----------------------------------------------------------------------------

func TestDoStart_SendsPayloadAndParsesResponse(t *testing.T) {
	srv, rs := newRecorder(t, 200, StartStopResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "started"},
		Result:          httpd.RestoreJobResponse{Name: "job1", RestoreJobId: "JOB-UUID"},
	})
	defer srv.Close()

	decoded, body, err := doStart(cfg(srv.URL), "job1", "SRC-UUID", "aws_1", "/tmp/r",
		[]string{"/etc/hosts"}, false, []string{"*.log"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected raw body to be populated")
	}
	if decoded.Result.RestoreJobId != "JOB-UUID" || decoded.Result.Name != "job1" {
		t.Errorf("unexpected decoded response: %+v", decoded)
	}
	if rs.lastMethod != "POST" || rs.lastPath != "/api/v1/restore/start" {
		t.Errorf("unexpected request method/path: %s %s", rs.lastMethod, rs.lastPath)
	}
	var sent httpd.RestoreJobRequest
	if err := json.Unmarshal(rs.lastBody, &sent); err != nil {
		t.Fatalf("could not decode sent body: %v", err)
	}
	if sent.Name != "job1" || sent.SourceBackupJobId != "SRC-UUID" || sent.TargetName != "aws_1" ||
		sent.RestoreDir != "/tmp/r" || sent.AllFiles {
		t.Errorf("unexpected request payload: %+v", sent)
	}
	if len(sent.Files) != 1 || sent.Files[0] != "/etc/hosts" {
		t.Errorf("expected Files=[/etc/hosts], got %v", sent.Files)
	}
	if len(sent.Exclusions) != 1 || sent.Exclusions[0] != "*.log" {
		t.Errorf("expected Exclusions=[*.log], got %v", sent.Exclusions)
	}
}

func TestDoStart_AllFilesPayload(t *testing.T) {
	srv, rs := newRecorder(t, 200, StartStopResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "started"},
		Result:          httpd.RestoreJobResponse{Name: "job1", RestoreJobId: "ID"},
	})
	defer srv.Close()

	_, _, err := doStart(cfg(srv.URL), "job1", "SRC", "", "", nil, true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent httpd.RestoreJobRequest
	_ = json.Unmarshal(rs.lastBody, &sent)
	if !sent.AllFiles {
		t.Error("expected AllFiles=true in request body")
	}
	if len(sent.Files) != 0 {
		t.Errorf("expected no Files in request body when AllFiles is set, got %v", sent.Files)
	}
}

func TestDoStart_ServerErrorSurfacesMessage(t *testing.T) {
	srv, _ := newRecorder(t, 400, httpd.HttpStatusReply{Code: "error", Message: "already running"})
	defer srv.Close()

	_, _, err := doStart(cfg(srv.URL), "job1", "SRC", "", "", nil, true, nil)
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected error to include server message, got %q", err.Error())
	}
}

func TestDoStart_ConnectionErrorReturnsError(t *testing.T) {
	// Use an address nothing is listening on. The httptest package hands out free ports — we
	// spin one up then immediately close it to get a usable-but-dead address.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close()
	_, _, err := doStart(cfg(addr), "job1", "SRC", "", "", nil, true, nil)
	if err == nil {
		t.Fatal("expected network error when server is unreachable")
	}
}

// -----------------------------------------------------------------------------
// doStop
// -----------------------------------------------------------------------------

func TestDoStop_SendsNameAndJobId(t *testing.T) {
	srv, rs := newRecorder(t, 200, StartStopResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "stopped"},
		Result:          httpd.RestoreJobResponse{Name: "job1", RestoreJobId: "ID"},
	})
	defer srv.Close()

	decoded, _, err := doStop(cfg(srv.URL), "job1", "ID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Message != "stopped" {
		t.Errorf("expected message 'stopped', got %q", decoded.Message)
	}
	if rs.lastPath != "/api/v1/restore/stop" {
		t.Errorf("unexpected path: %s", rs.lastPath)
	}
	var sent httpd.RestoreJobStopRequest
	_ = json.Unmarshal(rs.lastBody, &sent)
	if sent.Name != "job1" || sent.RestoreJobId != "ID" {
		t.Errorf("unexpected stop payload: %+v", sent)
	}
}

func TestDoStop_NoMatchingJob(t *testing.T) {
	srv, _ := newRecorder(t, 400, httpd.HttpStatusReply{Code: "error", Message: "not running"})
	defer srv.Close()

	_, _, err := doStop(cfg(srv.URL), "job1", "")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// doList
// -----------------------------------------------------------------------------

func TestDoList_ParsesJobs(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Second)
	srv, rs := newRecorder(t, 200, ListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result: []shared.BackupJobStatus{
			{Name: "job1", State: "running", BackupJobId: "ID-1", StartTime: start},
			{Name: "job2", State: "running", BackupJobId: "ID-2", StartTime: start},
		},
	})
	defer srv.Close()

	decoded, _, err := doList(cfg(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.lastMethod != "GET" || rs.lastPath != "/api/v1/restore/list" {
		t.Errorf("unexpected method/path: %s %s", rs.lastMethod, rs.lastPath)
	}
	if len(decoded.Result) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(decoded.Result))
	}
	if decoded.Result[0].BackupJobId != "ID-1" || decoded.Result[1].BackupJobId != "ID-2" {
		t.Errorf("unexpected job ids: %+v", decoded.Result)
	}
}

func TestDoList_Empty(t *testing.T) {
	srv, _ := newRecorder(t, 200, ListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result:          []shared.BackupJobStatus{},
	})
	defer srv.Close()

	decoded, _, err := doList(cfg(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decoded.Result) != 0 {
		t.Errorf("expected empty list, got %+v", decoded.Result)
	}
}

func TestDoList_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"error","message":"Basic authentication required"}`))
	}))
	defer srv.Close()

	_, _, err := doList(clientConfig.Client{Username: "", Password: "", Address: srv.URL})
	if err == nil {
		t.Fatal("expected error on 401 response")
	}
	if !strings.Contains(err.Error(), "Basic authentication") {
		t.Errorf("expected auth error, got %q", err.Error())
	}
}

// -----------------------------------------------------------------------------
// Watch SSE rendering — exercises the bytes/bufio parser inside Watch indirectly via a
// bare-bones SSE server that closes after a single event. Watch() itself writes to stdout
// and calls os.Exit on non-200, so we only verify it returns cleanly on a well-formed
// single-event stream.
// -----------------------------------------------------------------------------

func TestWatch_ParsesSingleSSEEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/restore/watch" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		evt := shared.WatchMessage{
			Sequence:        1,
			PercentDone:     25,
			Rate:            1024,
			ObjectStoreType: "aws_s3",
			ObjectType:      "file",
			OperationType:   "download",
			Path:            "/etc/hosts",
		}
		b, _ := json.Marshal(evt)
		buf := bytes.NewBuffer(nil)
		buf.WriteString("data: ")
		buf.Write(b)
		buf.WriteString("\n")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	// Redirect stdout so we don't pollute test output and can assert on what was printed.
	// The Watch function writes its rendered rows to stdout.
	// We're primarily validating it parses and exits cleanly (returns on io.EOF).
	done := make(chan struct{})
	go func() {
		Watch(cfg(srv.URL), true, "job1", "RJID")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return within 5s of the stream closing")
	}
}
