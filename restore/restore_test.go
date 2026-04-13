package restore

import (
	"cloudbackup/shared"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestPickTarget(t *testing.T) {
	cfg := shared.ConfigBackup{
		Name: "demo",
		Target: []shared.ConfigBackupTarget{
			{Name: "primary"},
			{Name: "secondary"},
		},
	}

	if got, err := pickTarget(cfg, ""); err != nil || got.Name != "primary" {
		t.Errorf("empty target name should return first target; got=%q err=%v", got.Name, err)
	}
	if got, err := pickTarget(cfg, "secondary"); err != nil || got.Name != "secondary" {
		t.Errorf("named target lookup failed; got=%q err=%v", got.Name, err)
	}
	if _, err := pickTarget(cfg, "nope"); err == nil {
		t.Error("unknown target name should return error")
	}
	if _, err := pickTarget(shared.ConfigBackup{Name: "demo"}, ""); err == nil {
		t.Error("backup with no targets should return error")
	}
}

func TestResolveRestoreDir(t *testing.T) {
	mu := &sync.RWMutex{}

	// 1. explicit per-request override always wins
	override := filepath.Join("tmp", "override")
	cfg := shared.CfgTemplate{DataDir: "/var/data", RestoreDir: "/var/restores", Mutex: mu}
	got, err := resolveRestoreDir(Request{RestoreJobId: "rj", RestoreDirOverride: override}, cfg, "demo")
	if err != nil || got != filepath.Clean(override) {
		t.Errorf("override path expected %q, got %q (err=%v)", filepath.Clean(override), got, err)
	}

	// 2. configured RestoreDir is used as base
	cfg2 := shared.CfgTemplate{DataDir: "/var/data", RestoreDir: "/srv/restores", Mutex: mu}
	got, err = resolveRestoreDir(Request{RestoreJobId: "rj"}, cfg2, "demo")
	want := filepath.Join("/srv/restores", "demo", "rj")
	if err != nil || got != want {
		t.Errorf("configured base expected %q, got %q (err=%v)", want, got, err)
	}

	// 3. empty RestoreDir falls back to <DataDir>/restores
	cfg3 := shared.CfgTemplate{DataDir: "/var/data", Mutex: mu}
	got, err = resolveRestoreDir(Request{RestoreJobId: "rj"}, cfg3, "demo")
	want = filepath.Join("/var/data", "restores", "demo", "rj")
	if err != nil || got != want {
		t.Errorf("default base expected %q, got %q (err=%v)", want, got, err)
	}
}

func TestMapPathIntoRestoreDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only path mapping check")
	}
	cases := []struct {
		restoreDir string
		source     string
		want       string
	}{
		{"/r", "/etc/hosts", "/r/etc/hosts"},
		{"/r", "/var/log/", "/r/var/log"},
		{"/r", "relative/file", "/r/relative/file"},
	}
	for _, c := range cases {
		got := mapPathIntoRestoreDir(c.restoreDir, c.source)
		if got != c.want {
			t.Errorf("mapPathIntoRestoreDir(%q,%q) = %q, want %q", c.restoreDir, c.source, got, c.want)
		}
	}
}

func TestMapPathIntoRestoreDirNoEscape(t *testing.T) {
	// Sanity: result should always be under restoreDir, never escape via the leading slash.
	got := mapPathIntoRestoreDir("/r", "/etc/hosts")
	if !strings.HasPrefix(got, "/r/") {
		t.Errorf("mapped path %q should remain under /r/", got)
	}
}

// --- fetchItems tests ---

const testJobID = "job-1"

// newTestDB creates an in-memory SQLite database with the tables needed by fetchItems
// and returns it. The caller should defer db.Close().
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	schema := `
		CREATE TABLE jobs (id TEXT NOT NULL PRIMARY KEY, name TEXT, type TEXT, start_time INTEGER,
			end_time INTEGER, state TEXT, report TEXT, src_os TEXT);
		CREATE TABLE targets (name TEXT NOT NULL PRIMARY KEY, backup_name TEXT, type TEXT,
			bucket TEXT, prefix TEXT, date_added INTEGER);
		CREATE TABLE remote_files (uuid TEXT NOT NULL PRIMARY KEY, local_path TEXT, parent TEXT,
			target TEXT, upload_date INTEGER, job_id TEXT, delete_marker INTEGER, version INTEGER,
			remote_version TEXT, type TEXT, link_target TEXT, size INTEGER, mtime INTEGER,
			ctime INTEGER, owner TEXT, permissions TEXT, checksum TEXT, checksum_type TEXT,
			encrypted INTEGER,
			FOREIGN KEY(target) REFERENCES targets(name),
			FOREIGN KEY(job_id) REFERENCES jobs(id));
		CREATE INDEX remote_files_local_path ON remote_files(local_path);
		CREATE TABLE backup_collections (file_uuid TEXT, job_id TEXT, target TEXT,
			FOREIGN KEY(file_uuid) REFERENCES remote_files(uuid),
			FOREIGN KEY(job_id) REFERENCES jobs(id));
		CREATE INDEX backup_collections_jobid ON backup_collections(job_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	// Seed a job and a target so foreign keys are satisfied.
	if _, err := db.Exec(`INSERT INTO jobs VALUES(?,?,'backup',0,0,'finished','','linux')`,
		testJobID, "demo"); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO targets VALUES('t1','demo','s3','bucket','prefix',0)`); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	return db
}

// insertItem is a helper that inserts a remote_files row and its backup_collections link for
// the test job. Only the fields relevant to fetchItems are populated.
func insertItem(t *testing.T, db *sql.DB, uuid, localPath, typ string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO remote_files
		(uuid, local_path, parent, target, upload_date, job_id, delete_marker, version,
		 remote_version, type, link_target, size, mtime, ctime, owner, permissions, checksum,
		 checksum_type, encrypted)
		VALUES (?,?,?,?,?,?,0,1,'v1',?,'',0,0,0,'root','0755','','sha256',0)`,
		uuid, localPath, filepath.Dir(localPath), "t1", 0, testJobID, typ)
	if err != nil {
		t.Fatalf("insert remote_files %s: %v", uuid, err)
	}
	_, err = db.Exec(`INSERT INTO backup_collections VALUES(?,?,'t1')`, uuid, testJobID)
	if err != nil {
		t.Fatalf("insert backup_collections %s: %v", uuid, err)
	}
}

// localPaths extracts sorted local_path values from a slice of remoteItem for easy comparison.
func localPaths(items []remoteItem) []string {
	paths := make([]string, len(items))
	for i, it := range items {
		paths[i] = it.localPath
	}
	sort.Strings(paths)
	return paths
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFetchItemsAllFiles(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data", "dir")
	insertItem(t, db, "u2", "/data/file1.txt", "file")
	insertItem(t, db, "u3", "/data/file2.txt", "file")

	items, err := fetchItems(db, Request{SourceBackupJobId: testJobID, AllFiles: true})
	if err != nil {
		t.Fatalf("fetchItems AllFiles: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data", "/data/file1.txt", "/data/file2.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("AllFiles: got %v, want %v", got, want)
	}
}

func TestFetchItemsExactFileMatch(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data/file1.txt", "file")
	insertItem(t, db, "u2", "/data/file2.txt", "file")
	insertItem(t, db, "u3", "/other/file3.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data/file1.txt", "/other/file3.txt"},
	})
	if err != nil {
		t.Fatalf("fetchItems exact: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data/file1.txt", "/other/file3.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("exact match: got %v, want %v", got, want)
	}
}

func TestFetchItemsDirectoryRecursive(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data", "dir")
	insertItem(t, db, "u2", "/data/file1.txt", "file")
	insertItem(t, db, "u3", "/data/sub", "dir")
	insertItem(t, db, "u4", "/data/sub/file2.txt", "file")
	insertItem(t, db, "u5", "/data/sub/deep", "dir")
	insertItem(t, db, "u6", "/data/sub/deep/file3.txt", "file")
	insertItem(t, db, "u7", "/other/unrelated.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data"},
	})
	if err != nil {
		t.Fatalf("fetchItems dir recursive: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data", "/data/file1.txt", "/data/sub", "/data/sub/deep", "/data/sub/deep/file3.txt", "/data/sub/file2.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("dir recursive: got %v, want %v", got, want)
	}
	// /other/unrelated.txt must not appear.
	for _, p := range got {
		if p == "/other/unrelated.txt" {
			t.Error("unrelated file outside the directory was incorrectly included")
		}
	}
}

func TestFetchItemsMixedFilesAndDirectories(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/etc", "dir")
	insertItem(t, db, "u2", "/etc/hosts", "file")
	insertItem(t, db, "u3", "/etc/conf.d", "dir")
	insertItem(t, db, "u4", "/etc/conf.d/app.conf", "file")
	insertItem(t, db, "u5", "/var/log/app.log", "file")
	insertItem(t, db, "u6", "/other/file.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/etc", "/var/log/app.log"},
	})
	if err != nil {
		t.Fatalf("fetchItems mixed: %v", err)
	}
	got := localPaths(items)
	want := []string{"/etc", "/etc/conf.d", "/etc/conf.d/app.conf", "/etc/hosts", "/var/log/app.log"}
	if !equalStringSlices(got, want) {
		t.Errorf("mixed: got %v, want %v", got, want)
	}
}

func TestFetchItemsDeduplication(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data", "dir")
	insertItem(t, db, "u2", "/data/file1.txt", "file")
	insertItem(t, db, "u3", "/data/sub", "dir")
	insertItem(t, db, "u4", "/data/sub/file2.txt", "file")

	// Request both the directory and a file that is a child of it — the child should appear
	// only once.
	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data", "/data/file1.txt"},
	})
	if err != nil {
		t.Fatalf("fetchItems dedup: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data", "/data/file1.txt", "/data/sub", "/data/sub/file2.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("dedup: got %v, want %v", got, want)
	}

	// Also verify uniqueness directly.
	seen := make(map[string]struct{})
	for _, item := range items {
		if _, exists := seen[item.uuid]; exists {
			t.Errorf("duplicate uuid %s in results", item.uuid)
		}
		seen[item.uuid] = struct{}{}
	}
}

func TestFetchItemsDirectoryWithLikeSpecialChars(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Directory name contains SQL LIKE wildcards: % and _
	insertItem(t, db, "u1", "/data_100%done", "dir")
	insertItem(t, db, "u2", "/data_100%done/report.txt", "file")
	// Another path that would match an unescaped LIKE pattern "/data_100%done/%".
	insertItem(t, db, "u3", "/dataX100Ydone/other.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data_100%done"},
	})
	if err != nil {
		t.Fatalf("fetchItems special chars: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data_100%done", "/data_100%done/report.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("special chars: got %v, want %v", got, want)
	}
}

func TestFetchItemsNonDirectoryFileNotExpanded(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// A file whose path is a prefix of another file — should NOT trigger recursive expansion.
	insertItem(t, db, "u1", "/data/app", "file")
	insertItem(t, db, "u2", "/data/app.log", "file")
	insertItem(t, db, "u3", "/data/app/config.yaml", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data/app"},
	})
	if err != nil {
		t.Fatalf("fetchItems non-dir: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data/app"}
	if !equalStringSlices(got, want) {
		t.Errorf("non-dir expansion: got %v, want %v", got, want)
	}
}

func TestFetchItemsMultipleDirectories(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/etc", "dir")
	insertItem(t, db, "u2", "/etc/hosts", "file")
	insertItem(t, db, "u3", "/var", "dir")
	insertItem(t, db, "u4", "/var/log", "dir")
	insertItem(t, db, "u5", "/var/log/syslog", "file")
	insertItem(t, db, "u6", "/home/user/doc.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/etc", "/var"},
	})
	if err != nil {
		t.Fatalf("fetchItems multi dir: %v", err)
	}
	got := localPaths(items)
	want := []string{"/etc", "/etc/hosts", "/var", "/var/log", "/var/log/syslog"}
	if !equalStringSlices(got, want) {
		t.Errorf("multi dir: got %v, want %v", got, want)
	}
}

func TestFetchItemsEmptyDirectoryNoChildren(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/empty", "dir")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/empty"},
	})
	if err != nil {
		t.Fatalf("fetchItems empty dir: %v", err)
	}
	got := localPaths(items)
	want := []string{"/empty"}
	if !equalStringSlices(got, want) {
		t.Errorf("empty dir: got %v, want %v", got, want)
	}
}

func TestFetchItemsNoMatch(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data/file1.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/nonexistent"},
	})
	if err != nil {
		t.Fatalf("fetchItems no match: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for non-matching path, got %d", len(items))
	}
}

func TestFetchItemsDirNamePrefixCollision(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// /data is a directory, /data-extra is a separate directory that shares the prefix "/data"
	// but must NOT be included when restoring /data.
	insertItem(t, db, "u1", "/data", "dir")
	insertItem(t, db, "u2", "/data/file.txt", "file")
	insertItem(t, db, "u3", "/data-extra", "dir")
	insertItem(t, db, "u4", "/data-extra/other.txt", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data"},
	})
	if err != nil {
		t.Fatalf("fetchItems prefix collision: %v", err)
	}
	got := localPaths(items)
	want := []string{"/data", "/data/file.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("prefix collision: got %v, want %v", got, want)
	}
}

func TestFetchItemsLargeDirectoryTree(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Build a tree with 3 levels of nesting and multiple files per level.
	insertItem(t, db, "root", "/tree", "dir")
	var want []string
	want = append(want, "/tree")
	n := 1
	for i := 0; i < 3; i++ {
		dirPath := fmt.Sprintf("/tree/l1_%d", i)
		insertItem(t, db, fmt.Sprintf("d1_%d", i), dirPath, "dir")
		want = append(want, dirPath)
		n++
		for j := 0; j < 3; j++ {
			subdir := fmt.Sprintf("%s/l2_%d", dirPath, j)
			insertItem(t, db, fmt.Sprintf("d2_%d_%d", i, j), subdir, "dir")
			want = append(want, subdir)
			n++
			for k := 0; k < 2; k++ {
				filePath := fmt.Sprintf("%s/file_%d.txt", subdir, k)
				insertItem(t, db, fmt.Sprintf("f_%d_%d_%d", i, j, k), filePath, "file")
				want = append(want, filePath)
				n++
			}
		}
	}

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/tree"},
	})
	if err != nil {
		t.Fatalf("fetchItems large tree: %v", err)
	}
	got := localPaths(items)
	sort.Strings(want)
	if !equalStringSlices(got, want) {
		t.Errorf("large tree: got %d items, want %d items", len(got), len(want))
	}
}

// --- applyExclusions tests ---

// makeItems is a helper that builds a []remoteItem from a list of (localPath, type) pairs.
func makeItems(entries ...string) []remoteItem {
	if len(entries)%2 != 0 {
		panic("makeItems requires pairs of (path, type)")
	}
	items := make([]remoteItem, 0, len(entries)/2)
	for i := 0; i < len(entries); i += 2 {
		items = append(items, remoteItem{
			uuid:      fmt.Sprintf("u%d", i/2),
			localPath: entries[i],
			typ:       entries[i+1],
		})
	}
	return items
}

func TestApplyExclusionsNoExclusions(t *testing.T) {
	items := makeItems("/data/file1.txt", "file", "/data/file2.txt", "file")
	got, err := applyExclusions(items, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 items, got %d", len(got))
	}
}

func TestApplyExclusionsExactPath(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.txt", "file",
		"/data/file3.log", "file",
	)
	got, err := applyExclusions(items, []string{"/data/file1.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data/file2.txt", "/data/file3.log"}
	if !equalStringSlices(paths, want) {
		t.Errorf("exact exclusion: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsGlobStar(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/sub/file2.txt", "file",
		"/data/sub/deep/file3.txt", "file",
		"/data/keep.log", "file",
	)
	// ** matches across directory boundaries
	got, err := applyExclusions(items, []string{"**/*.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data/keep.log"}
	if !equalStringSlices(paths, want) {
		t.Errorf("globstar exclusion: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsSingleStar(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.log", "file",
		"/data/sub/file3.txt", "file",
	)
	// Single * does NOT cross directory boundaries
	got, err := applyExclusions(items, []string{"/data/*.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data/file2.log", "/data/sub/file3.txt"}
	if !equalStringSlices(paths, want) {
		t.Errorf("single star exclusion: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsQuestionMark(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.txt", "file",
		"/data/file10.txt", "file",
	)
	// ? matches exactly one character
	got, err := applyExclusions(items, []string{"/data/file?.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data/file10.txt"}
	if !equalStringSlices(paths, want) {
		t.Errorf("question mark exclusion: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsDirectory(t *testing.T) {
	items := makeItems(
		"/data", "dir",
		"/data/file1.txt", "file",
		"/data/sub", "dir",
		"/data/sub/file2.txt", "file",
		"/other/file3.txt", "file",
	)
	// Exclude an entire subtree by matching the directory and everything under it
	got, err := applyExclusions(items, []string{"/data/sub", "/data/sub/**"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data", "/data/file1.txt", "/other/file3.txt"}
	if !equalStringSlices(paths, want) {
		t.Errorf("directory exclusion: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsMultiplePatterns(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.log", "file",
		"/data/cache", "dir",
		"/data/cache/tmp.dat", "file",
		"/data/important.doc", "file",
	)
	got, err := applyExclusions(items, []string{"**/*.log", "/data/cache", "/data/cache/**"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := localPaths(got)
	want := []string{"/data/file1.txt", "/data/important.doc"}
	if !equalStringSlices(paths, want) {
		t.Errorf("multiple patterns: got %v, want %v", paths, want)
	}
}

func TestApplyExclusionsNoMatch(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.txt", "file",
	)
	got, err := applyExclusions(items, []string{"**/*.log", "/nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("no-match exclusion: expected 2 items, got %d", len(got))
	}
}

func TestApplyExclusionsAllExcluded(t *testing.T) {
	items := makeItems(
		"/data/file1.txt", "file",
		"/data/file2.txt", "file",
	)
	got, err := applyExclusions(items, []string{"**/*.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("all-excluded: expected 0 items, got %d", len(got))
	}
}

func TestApplyExclusionsWithFetchItems(t *testing.T) {
	// End-to-end test: fetch items with directory expansion, then apply exclusions.
	db := newTestDB(t)
	defer db.Close()

	insertItem(t, db, "u1", "/data", "dir")
	insertItem(t, db, "u2", "/data/file1.txt", "file")
	insertItem(t, db, "u3", "/data/file2.log", "file")
	insertItem(t, db, "u4", "/data/sub", "dir")
	insertItem(t, db, "u5", "/data/sub/file3.txt", "file")
	insertItem(t, db, "u6", "/data/sub/file4.log", "file")

	items, err := fetchItems(db, Request{
		SourceBackupJobId: testJobID,
		Files:             []string{"/data"},
	})
	if err != nil {
		t.Fatalf("fetchItems: %v", err)
	}

	// Exclude all .log files
	filtered, err := applyExclusions(items, []string{"**/*.log"})
	if err != nil {
		t.Fatalf("applyExclusions: %v", err)
	}
	got := localPaths(filtered)
	want := []string{"/data", "/data/file1.txt", "/data/sub", "/data/sub/file3.txt"}
	if !equalStringSlices(got, want) {
		t.Errorf("fetch+exclude: got %v, want %v", got, want)
	}
}
