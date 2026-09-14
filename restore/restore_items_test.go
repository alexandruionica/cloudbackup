package restore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
)

func TestValidateSourceJobId(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	if err := validateSourceJobId(db, "demo", testJobID); err != nil {
		t.Fatalf("known job id must validate: %v", err)
	}
	if err := validateSourceJobId(db, "demo", ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty id must be rejected as required, got %v", err)
	}
	if err := validateSourceJobId(db, "demo", "nope"); err == nil || !strings.Contains(err.Error(), "no backup job found") {
		t.Fatalf("unknown id must be rejected, got %v", err)
	}
	if err := validateSourceJobId(db, "other-definition", testJobID); err == nil {
		t.Fatal("a job id belonging to a different backup definition must be rejected")
	}
}

// restoreOneEnv wires a persistent test_null store and a running restore job so restoreOne can be
// driven directly, with its counters observable.
type restoreOneEnv struct {
	store *objectstore.StoreTestNull
	// uploads go through an unthrottled instance sharing the same persist_dir, so a rate limit on
	// $store slows down only the downloads under test
	uploader *objectstore.StoreTestNull
	state    *shared.BackupJobsState
	src      string
	dst      string
}

func newRestoreOneEnv(t *testing.T, ratelimit string) *restoreOneEnv {
	t.Helper()
	root := t.TempDir()
	cfg := shared.ConfigBackup{Name: "job", Target: []shared.ConfigBackupTarget{{
		Name: "t1", Type: "test_null", Prefix: "prefix",
		Parameters: []shared.ConfigBackupTargetParams{{Name: "persist_dir", Value: filepath.Join(root, "objects")}},
	}}}
	state := shared.NewJobsState()
	if err := state.MarkRestoreRunning("job", "test", "rid"); err != nil {
		t.Fatal(err)
	}
	store, err := objectstore.InitialiseStoreTestNull(context.Background(), cfg, cfg.Target[0], ratelimit, state)
	if err != nil {
		t.Fatal(err)
	}
	uploader, err := objectstore.InitialiseStoreTestNull(context.Background(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	return &restoreOneEnv{store: store, uploader: uploader, state: state, src: filepath.Join(root, "src"), dst: filepath.Join(root, "restored")}
}

func (e *restoreOneEnv) counter(t *testing.T, name string) uint64 {
	t.Helper()
	e.state.Lock.RLock()
	defer e.state.Lock.RUnlock()
	for _, j := range e.state.Running {
		if j.Name == "job" {
			return j.StatsCounters[name]
		}
	}
	t.Fatal("restore job not found in running state")
	return 0
}

func (e *restoreOneEnv) upload(t *testing.T, name string, content []byte, version int64) dbops.RestoreFileRow {
	t.Helper()
	if err := os.MkdirAll(e.src, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.src, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := shared.BackedUpFileProperties{Path: path, Type: "file", Size: int64(len(content))}
	remoteVersion, _, err := e.uploader.Upload(rec, version, e.state, false)
	if err != nil {
		t.Fatal(err)
	}
	return dbops.RestoreFileRow{FileUuid: name, LocalPath: path, Type: "file", Size: int64(len(content)), Version: version, RemoteVersion: remoteVersion}
}

func TestRestoreOneFileDirSymlinkAndUnknown(t *testing.T) {
	env := newRestoreOneEnv(t, "0")
	ctx := context.Background()

	file := env.upload(t, "a.txt", []byte("hello restore"), 1)
	if out := restoreOne(ctx, env.store, file, env.dst, "job", env.state); out.state != "done" {
		t.Fatalf("file: %+v", out)
	}
	got, err := os.ReadFile(mapPathIntoRestoreDir(env.dst, file.LocalPath))
	if err != nil || string(got) != "hello restore" {
		t.Fatalf("restored file content %q, %v", got, err)
	}
	if env.counter(t, "restored_files") != 1 {
		t.Fatalf("restored_files = %d", env.counter(t, "restored_files"))
	}

	dir := dbops.RestoreFileRow{FileUuid: "d", LocalPath: filepath.Join(env.src, "sub", "dir"), Type: "dir"}
	if out := restoreOne(ctx, env.store, dir, env.dst, "job", env.state); out.state != "done" {
		t.Fatalf("dir: %+v", out)
	}
	if fi, err := os.Stat(mapPathIntoRestoreDir(env.dst, dir.LocalPath)); err != nil || !fi.IsDir() {
		t.Fatalf("directory not restored: %v", err)
	}
	if env.counter(t, "restored_directories") != 1 {
		t.Fatal("restored_directories not incremented")
	}

	if runtime.GOOS != "windows" {
		link := dbops.RestoreFileRow{FileUuid: "l", LocalPath: filepath.Join(env.src, "link"), Type: "symlink", LinkTarget: "/somewhere/else"}
		if out := restoreOne(ctx, env.store, link, env.dst, "job", env.state); out.state != "done" {
			t.Fatalf("symlink: %+v", out)
		}
		if target, err := os.Readlink(mapPathIntoRestoreDir(env.dst, link.LocalPath)); err != nil || target != "/somewhere/else" {
			t.Fatalf("symlink target %q, %v", target, err)
		}
		// restoring the same symlink twice (a resume) must not fail on "file exists"
		if out := restoreOne(ctx, env.store, link, env.dst, "job", env.state); out.state != "done" {
			t.Fatalf("symlink again: %+v", out)
		}
		if env.counter(t, "restored_symlinks") != 2 {
			t.Fatal("restored_symlinks not incremented")
		}
	}

	odd := dbops.RestoreFileRow{FileUuid: "s", LocalPath: filepath.Join(env.src, "socket"), Type: "socket"}
	if out := restoreOne(ctx, env.store, odd, env.dst, "job", env.state); out.state != "failed" || out.errMsg != "unknown type" {
		t.Fatalf("unknown type: %+v", out)
	}
	if env.counter(t, "failed_to_restore_files") != 1 {
		t.Fatal("failed_to_restore_files not incremented for the unknown type")
	}
}

// A file the store never received is a silent no-op for the test backend (Get returns nil); this
// pins that restoreOne reports it as done without creating anything on disk.
func TestRestoreOneMissingObjectLeavesNoFile(t *testing.T) {
	env := newRestoreOneEnv(t, "0")
	missing := dbops.RestoreFileRow{FileUuid: "m", LocalPath: filepath.Join(env.src, "never-uploaded"), Type: "file", Size: 3, Version: 9}
	if out := restoreOne(context.Background(), env.store, missing, env.dst, "job", env.state); out.state != "done" {
		t.Fatalf("missing object: %+v", out)
	}
	if _, err := os.Stat(mapPathIntoRestoreDir(env.dst, missing.LocalPath)); !os.IsNotExist(err) {
		t.Fatalf("no file must be created for a missing object, stat err = %v", err)
	}
}

func TestRestoreOneCancelledMidDownload(t *testing.T) {
	// 100 B/s makes the throttled Get wait long enough for the cancellation to win the race
	env := newRestoreOneEnv(t, "100")
	file := env.upload(t, "slow.bin", make([]byte, 4096), 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan restoreOneOutcome, 1)
	go func() { done <- restoreOne(ctx, env.store, file, env.dst, "job", env.state) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case out := <-done:
		if out.state != "cancelled" {
			t.Fatalf("expected cancelled, got %+v", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("restoreOne did not return after cancellation")
	}
	if env.counter(t, "restored_files") != 0 {
		t.Fatal("a cancelled download must not count as restored")
	}
}

func TestFinalizeJobRecord(t *testing.T) {
	dataDir := t.TempDir()
	cfg := shared.CfgTemplate{DataDir: dataDir, Mutex: &sync.RWMutex{}}
	state := shared.NewJobsState()
	db, err := database.StartRestoreDb(dataDir, "job", "t1", state)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := dbops.PrepareRestore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbops.InsertRestoreJob(db, stmts, "rid", "job", "t1", "src", time.Now().UnixNano(), filepath.Join(dataDir, "out"), true, "null", "null", "linux"); err != nil {
		t.Fatal(err)
	}
	database.DisconnectFromDb(database.GetRestoreDbKey("job", "t1"), state, db)

	if err := FinalizeJobRecord(cfg, "job", "t1", "rid", "finished", `{"ok":true}`, state); err != nil {
		t.Fatal(err)
	}
	db, err = database.StartRestoreDb(dataDir, "job", "t1", state)
	if err != nil {
		t.Fatal(err)
	}
	defer database.DisconnectFromDb(database.GetRestoreDbKey("job", "t1"), state, db)
	rec, err := dbops.FetchRestoreJob(db, stmts, "rid")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != "finished" {
		t.Fatalf("state = %q", rec.State)
	}
	if err := FinalizeJobRecord(cfg, "job", "t1", "no-such-job", "finished", "", state); err == nil {
		t.Fatal("finalising an unknown job id must fail")
	}
}
