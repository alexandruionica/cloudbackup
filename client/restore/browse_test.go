package restore

import (
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestServer returns an httptest server that serves a single /report/backup/file/list
// response built from the supplied result + next token. It also records the last request
// body so tests can assert on what the client sent.
func newTestServer(t *testing.T, response interface{}) (*httptest.Server, *[]byte) {
	t.Helper()
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/report/backup/file/list" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	return srv, &lastBody
}

func testClientFor(addr string) clientConfig.Client {
	return clientConfig.Client{Username: "u", Password: "p", Address: addr}
}

func TestFetchFileList_MixedDirsAndFiles(t *testing.T) {
	resp := fileListResult{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Next:            "eyJvZmZzZXQiOjEwMH0=",
		Result: []httpd.ReportBackupFileListDbResults{
			{
				Path:   "/data/sub",
				Parent: "/data",
				Instances: []httpd.ReportBackupFileListInstanceDbResults{
					{Type: "directory", Size: 0},
				},
			},
			{
				Path:   "/data/file.txt",
				Parent: "/data",
				Instances: []httpd.ReportBackupFileListInstanceDbResults{
					{Type: "file", Size: 1024},
				},
			},
			// a "self" row for the directory being listed; should be filtered out
			{
				Path:   "/data",
				Parent: "/",
				Instances: []httpd.ReportBackupFileListInstanceDbResults{
					{Type: "directory"},
				},
			},
		},
	}
	srv, lastBody := newTestServer(t, resp)
	defer srv.Close()

	entries, next, err := fetchFileList(testClientFor(srv.URL), "job1", "abc", "/data", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != resp.Next {
		t.Errorf("expected next %q, got %q", resp.Next, next)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (self row filtered), got %d", len(entries))
	}
	// Dirs should sort before files.
	if !entries[0].IsDir || entries[0].Path != "/data/sub" {
		t.Errorf("expected first entry to be the directory /data/sub, got %+v", entries[0])
	}
	if entries[1].IsDir || entries[1].Path != "/data/file.txt" {
		t.Errorf("expected second entry to be the file /data/file.txt, got %+v", entries[1])
	}
	// Assert request body.
	var sent httpd.ReportBackupFileList
	if err := json.Unmarshal(*lastBody, &sent); err != nil {
		t.Fatalf("could not unmarshal sent body: %v", err)
	}
	if sent.Name != "job1" || sent.JobId != "abc" || sent.Path != "/data" || sent.Descend {
		t.Errorf("unexpected request body: %+v", sent)
	}
}

func TestFetchFileList_EmptyPathRequestsDescendAndReturnsRoots(t *testing.T) {
	// Simulate a typical backup: the DB contains every backed-up file under
	// configured roots /etc and /var/lib/backups. With descend=true at the
	// virtual root the server returns all rows. fetchFileList should filter
	// them down to just the two roots (their parent is not itself among the
	// returned paths).
	resp := fileListResult{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result: []httpd.ReportBackupFileListDbResults{
			{Path: "/etc", Parent: "/", Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "directory"}}},
			{Path: "/etc/hosts", Parent: "/etc", Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "file"}}},
			{Path: "/etc/passwd", Parent: "/etc", Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "file"}}},
			{Path: "/var/lib/backups", Parent: "/var/lib", Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "directory"}}},
			{Path: "/var/lib/backups/a.tar", Parent: "/var/lib/backups", Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "file"}}},
		},
	}
	srv, lastBody := newTestServer(t, resp)
	defer srv.Close()

	entries, _, err := fetchFileList(testClientFor(srv.URL), "job1", "abc", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The request must have asked for descend=true.
	var sent httpd.ReportBackupFileList
	if err := json.Unmarshal(*lastBody, &sent); err != nil {
		t.Fatalf("could not decode sent body: %v", err)
	}
	if !sent.Descend {
		t.Errorf("expected descend=true when listing at the virtual root, got %+v", sent)
	}
	// Only the two configured roots should be returned.
	if len(entries) != 2 {
		t.Fatalf("expected 2 root entries, got %d: %+v", len(entries), entries)
	}
	paths := map[string]bool{}
	for _, e := range entries {
		paths[e.Path] = true
		if !e.IsDir {
			t.Errorf("expected root %s to be a directory", e.Path)
		}
	}
	if !paths["/etc"] || !paths["/var/lib/backups"] {
		t.Errorf("expected /etc and /var/lib/backups as roots, got %v", paths)
	}
}

func TestFetchFileList_NextTokenPassedThrough(t *testing.T) {
	resp := fileListResult{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
	}
	srv, lastBody := newTestServer(t, resp)
	defer srv.Close()

	_, _, err := fetchFileList(testClientFor(srv.URL), "job1", "abc", "/data", "TOKEN123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent httpd.ReportBackupFileList
	_ = json.Unmarshal(*lastBody, &sent)
	if sent.Next != "TOKEN123" {
		t.Errorf("expected Next=TOKEN123 on wire, got %q", sent.Next)
	}
}

func TestFetchFileList_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"error","message":"boom"}`))
	}))
	defer srv.Close()
	_, _, err := fetchFileList(testClientFor(srv.URL), "job1", "abc", "/data", "")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to include server message, got %q", err.Error())
	}
}

func TestFetchFileList_DirVariantsRecognised(t *testing.T) {
	resp := fileListResult{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result: []httpd.ReportBackupFileListDbResults{
			{
				Path:      "/a",
				Parent:    "/",
				Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "dir"}},
			},
			{
				Path:      "/b",
				Parent:    "/",
				Instances: []httpd.ReportBackupFileListInstanceDbResults{{Type: "Directory"}},
			},
		},
	}
	srv, _ := newTestServer(t, resp)
	defer srv.Close()

	entries, _, err := fetchFileList(testClientFor(srv.URL), "job1", "abc", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if !e.IsDir {
			t.Errorf("expected %s to be classified as a directory", e.Path)
		}
	}
}

// -----------------------------------------------------------------------------
// Model (Update) tests — no network, no TTY.
// -----------------------------------------------------------------------------

func keyMsg(s string) tea.KeyMsg {
	// tea.KeyMsg's String() dispatch is what the model switches on, so constructing
	// a KeyMsg from Runes works for ordinary character keys (space, 'j', 'k', '?').
	// For named keys we use KeyType.
	switch s {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sampleModel() *model {
	m := initialModel(clientConfig.Client{}, "job1", "abc")
	m.height = 20
	m.entries = []entry{
		{Path: "/a", IsDir: true},
		{Path: "/b", IsDir: false},
		{Path: "/c", IsDir: false},
	}
	m.loading = false
	return m
}

func TestModel_CursorMovement(t *testing.T) {
	m := sampleModel()
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.cursor)
	}
	next, _ := m.Update(keyMsg("down"))
	m = next.(*model)
	if m.cursor != 1 {
		t.Errorf("expected cursor to move to 1, got %d", m.cursor)
	}
	next, _ = m.Update(keyMsg("down"))
	next, _ = next.(*model).Update(keyMsg("down"))
	m = next.(*model)
	if m.cursor != 2 {
		t.Errorf("cursor should clamp at last index (2), got %d", m.cursor)
	}
	next, _ = m.Update(keyMsg("up"))
	m = next.(*model)
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after up, got %d", m.cursor)
	}
}

func TestModel_ToggleSelection(t *testing.T) {
	m := sampleModel()
	m.cursor = 1 // /b
	next, _ := m.Update(keyMsg(" "))
	m = next.(*model)
	if !m.selected["/b"] {
		t.Error("expected /b to be selected after space")
	}
	next, _ = m.Update(keyMsg(" "))
	m = next.(*model)
	if m.selected["/b"] {
		t.Error("expected /b to be deselected after second space")
	}
}

func TestModel_DescendRequiresDirectory(t *testing.T) {
	m := sampleModel()
	m.cursor = 1 // /b is a file — enter should be a no-op
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(*model)
	if cmd != nil {
		t.Error("pressing enter on a file should not produce a command")
	}
	if m.cwd != "" {
		t.Errorf("cwd should not change when descending into a file, got %q", m.cwd)
	}
	if len(m.stack) != 0 {
		t.Errorf("stack should not grow when descending into a file, got %v", m.stack)
	}
}

func TestModel_DescendIntoDirectoryPushesStack(t *testing.T) {
	m := sampleModel()
	m.cursor = 0 // /a is a directory
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected a fetch command when descending into a directory")
	}
	if m.cwd != "/a" {
		t.Errorf("expected cwd=/a after descending, got %q", m.cwd)
	}
	if len(m.stack) != 1 || m.stack[0] != "" {
		t.Errorf("expected stack to contain the previous cwd (\"\"), got %v", m.stack)
	}
	if !m.loading {
		t.Error("expected loading=true after descent")
	}
	// A non-empty cwd should seed a synthetic ".." row so the user can navigate back
	// from within the listing (Midnight Commander style).
	if len(m.entries) != 1 || !m.entries[0].IsParent || m.entries[0].Path != ".." {
		t.Errorf("expected entries to be pre-seeded with a single \"..\" row, got %+v", m.entries)
	}
	if m.cursor != 1 {
		t.Errorf("expected cursor to start on the first real (yet-to-be-loaded) entry (index 1), got %d", m.cursor)
	}
}

func TestModel_EnterOnParentRowGoesUp(t *testing.T) {
	// Simulate being inside /a having descended from the virtual root.
	m := sampleModel()
	m.cwd = "/a"
	m.stack = []string{""}
	m.entries = []entry{
		{Path: "..", IsDir: true, IsParent: true},
		{Path: "/a/file.txt", IsDir: false},
	}
	m.cursor = 0 // on ".."
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected a fetch command when pressing enter on \"..\"")
	}
	if m.cwd != "" {
		t.Errorf("expected cwd to return to root after pressing enter on \"..\", got %q", m.cwd)
	}
	if len(m.stack) != 0 {
		t.Errorf("expected stack to be empty after going up, got %v", m.stack)
	}
}

func TestModel_SpaceOnParentRowIsNoOp(t *testing.T) {
	m := sampleModel()
	m.entries = []entry{
		{Path: "..", IsDir: true, IsParent: true},
		{Path: "/a/file.txt"},
	}
	m.cursor = 0
	next, _ := m.Update(keyMsg(" "))
	m = next.(*model)
	if m.selected[".."] || len(m.selected) != 0 {
		t.Errorf("\"..\" must never be selectable, got selected=%v", m.selected)
	}
}

func TestModel_BackspacePopsStack(t *testing.T) {
	m := sampleModel()
	m.cwd = "/a"
	m.stack = []string{""}
	next, cmd := m.Update(keyMsg("backspace"))
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected a fetch command when going up")
	}
	if m.cwd != "" {
		t.Errorf("expected cwd=\"\" after going up, got %q", m.cwd)
	}
	if len(m.stack) != 0 {
		t.Errorf("expected empty stack after popping, got %v", m.stack)
	}
}

func TestModel_BackspaceAtRootIsNoOp(t *testing.T) {
	m := sampleModel()
	_, cmd := m.Update(keyMsg("backspace"))
	if cmd != nil {
		t.Error("backspace at root should not produce a command")
	}
}

func TestModel_QuitCancels(t *testing.T) {
	m := sampleModel()
	next, cmd := m.Update(keyMsg("q"))
	m = next.(*model)
	if !m.cancelled {
		t.Error("expected cancelled=true after pressing q")
	}
	if cmd == nil {
		t.Error("expected a quit command")
	}
}

func TestModel_DoneMarksFinished(t *testing.T) {
	m := sampleModel()
	m.selected["/b"] = true
	next, cmd := m.Update(keyMsg("d"))
	m = next.(*model)
	if !m.done {
		t.Error("expected done=true after pressing d")
	}
	if m.cancelled {
		t.Error("done should not set cancelled")
	}
	if cmd == nil {
		t.Error("expected a quit command")
	}
}

func TestModel_NextPageOnlyFiresWhenTokenPresent(t *testing.T) {
	m := sampleModel()
	// No nextTok -> 'n' should be a no-op
	_, cmd := m.Update(keyMsg("n"))
	if cmd != nil {
		t.Error("'n' should not schedule a fetch when no next token is present")
	}
	m.nextTok = "TOKEN"
	_, cmd = m.Update(keyMsg("n"))
	if cmd == nil {
		t.Error("'n' should schedule a fetch when a next token is present")
	}
}

func TestModel_PageMsgAppendsEntriesAndKeepsCursor(t *testing.T) {
	m := sampleModel()
	m.cursor = 2
	next, _ := m.Update(pageMsg{entries: []entry{{Path: "/d"}}, next: ""})
	m = next.(*model)
	if len(m.entries) != 4 {
		t.Errorf("expected 4 entries after page append, got %d", len(m.entries))
	}
	if m.cursor != 2 {
		t.Errorf("cursor should stay at 2 when it is still in range, got %d", m.cursor)
	}
}

func TestModel_PageMsgErrorQuits(t *testing.T) {
	m := sampleModel()
	_, cmd := m.Update(pageMsg{err: io.EOF})
	if cmd == nil {
		t.Error("expected a quit command on page error")
	}
	if m.err == nil {
		t.Error("expected err to be recorded on model")
	}
}

func TestModel_ViewRendersWithoutPanic(t *testing.T) {
	m := sampleModel()
	m.selected["/b"] = true
	out := m.View()
	if !strings.Contains(out, "job1") || !strings.Contains(out, "abc") {
		t.Errorf("expected view to include job name and job id, got:\n%s", out)
	}
	if !strings.Contains(out, "Selected: 1") {
		t.Errorf("expected view to include 'Selected: 1', got:\n%s", out)
	}
}
