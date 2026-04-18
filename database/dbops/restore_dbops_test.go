package dbops

import (
	"cloudbackup/database"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"database/sql"
	"errors"
	"os"
	"testing"
)

// openTestRestoreDb creates a fresh restore DB in a temp directory and returns
// an opened handle plus a cleanup func. The cleanup disconnects, closes and
// removes the tmp dir so tests don't leave state behind.
func openTestRestoreDb(t *testing.T, prefix string) (*sql.DB, shared.RestoreDbPreparedStatements, func()) {
	t.Helper()
	datadir := utils.SetupTmpDir(prefix, t)
	backupName, targetName := "bkp1", "tgtA"
	state := shared.NewJobsState()
	dbName := database.GetRestoreDbKey(backupName, targetName)

	db, err := database.StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("StartRestoreDb: %s", err)
	}
	stmts, err := PrepareRestore(db)
	if err != nil {
		t.Fatalf("PrepareRestore: %s", err)
	}
	return db, stmts, func() {
		database.DisconnectFromDb(dbName, state, db)
		database.CloseDb(dbName, state, true)
		_ = os.RemoveAll(datadir)
	}
}

func TestPrepareRestoreReturnsAllStatements(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_prepare_")
	defer cleanup()
	_ = db

	// Spot-check that every text field is populated — a regression here usually
	// means someone added a new field to RestoreDbPreparedStatements but forgot
	// to assign it in PrepareRestore, which produces runtime empty-SQL errors.
	cases := map[string]string{
		"InsertJob":            stmts.InsertJob,
		"UpdateJobFinal":       stmts.UpdateJobFinal,
		"UpdateJobState":       stmts.UpdateJobState,
		"JobExists":            stmts.JobExists,
		"JobFetch":             stmts.JobFetch,
		"InsertManifestRow":    stmts.InsertManifestRow,
		"SetFileState":         stmts.SetFileState,
		"LoadPendingManifest":  stmts.LoadPendingManifest,
		"BumpCounter":          stmts.BumpCounter,
		"InitCounterRow":       stmts.InitCounterRow,
		"ReadCounters":         stmts.ReadCounters,
		"ReportJobsListQuery":  stmts.ReportJobsListQuery,
		"ReportJobShowQuery":   stmts.ReportJobShowQuery,
		"ReportFilesListQuery": stmts.ReportFilesListQuery,
	}
	for name, v := range cases {
		if v == "" {
			t.Errorf("statement %q is empty", name)
		}
	}
}

func TestInsertRestoreJobAndFetch(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_insert_")
	defer cleanup()

	err := InsertRestoreJob(db, stmts, "job-1", "bkp1", "tgtA", "src-1", 42, "/tmp/restore", true,
		`["a","b"]`, `[]`, "linux")
	if err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}
	exists, err := RestoreJobExists(db, stmts, "job-1")
	if err != nil || !exists {
		t.Fatalf("RestoreJobExists: exists=%v err=%v", exists, err)
	}
	rec, err := FetchRestoreJob(db, stmts, "job-1")
	if err != nil {
		t.Fatalf("FetchRestoreJob: %s", err)
	}
	if rec.Id != "job-1" || rec.BackupName != "bkp1" || rec.TargetName != "tgtA" ||
		rec.SourceBackupJobId != "src-1" || rec.StartTime != 42 || rec.State != "started" ||
		rec.RestoreDir != "/tmp/restore" || !rec.AllFiles || rec.RequestFiles != `["a","b"]` ||
		rec.SrcOs != "linux" {
		t.Fatalf("FetchRestoreJob returned unexpected record: %+v", rec)
	}

	// Init should have also seeded the counters row so BumpCounter can UPDATE
	// (not INSERT) straight away.
	counters, err := ReadCounters(db, stmts, "job-1")
	if err != nil {
		t.Fatalf("ReadCounters after insert: %s", err)
	}
	if len(counters) == 0 {
		t.Fatalf("expected seeded counters row, got empty map")
	}
	for k, v := range counters {
		if v != 0 {
			t.Errorf("counter %s = %d, want 0", k, v)
		}
	}
}

func TestFetchRestoreJobMissingReturnsNoRows(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_fetchmiss_")
	defer cleanup()

	_, err := FetchRestoreJob(db, stmts, "does-not-exist")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestUpdateRestoreJobStateAndFinal(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_updstate_")
	defer cleanup()

	if err := InsertRestoreJob(db, stmts, "j", "bkp1", "tgtA", "src", 1, "/x", true, "[]", "[]", "linux"); err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}

	if err := UpdateRestoreJobState(db, stmts, "j", "crashed"); err != nil {
		t.Fatalf("UpdateRestoreJobState: %s", err)
	}
	rec, _ := FetchRestoreJob(db, stmts, "j")
	if rec.State != "crashed" {
		t.Fatalf("after UpdateRestoreJobState: state=%q want crashed", rec.State)
	}

	if err := UpdateRestoreJobFinal(db, stmts, "j", 99, "finished", "summary-report"); err != nil {
		t.Fatalf("UpdateRestoreJobFinal: %s", err)
	}
	rec, _ = FetchRestoreJob(db, stmts, "j")
	if rec.State != "finished" || rec.EndTime != 99 || rec.Report != "summary-report" {
		t.Fatalf("after UpdateRestoreJobFinal: %+v", rec)
	}
}

func TestUpdateRestoreJobFinalMissingJob(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_updmiss_")
	defer cleanup()
	if err := UpdateRestoreJobFinal(db, stmts, "nope", 1, "finished", ""); err == nil {
		t.Fatalf("expected error when updating non-existent job")
	}
}

func TestInsertManifestBatchAndLoadPending(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_manifest_")
	defer cleanup()

	if err := InsertRestoreJob(db, stmts, "j", "bkp1", "tgtA", "src", 1, "/x", true, "[]", "[]", "linux"); err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}
	items := []RestoreFileRow{
		{FileUuid: "u1", LocalPath: "/a/b", Type: "file", Size: 100, Owner: "root", Permissions: "0644",
			Checksum: "abcd", ChecksumType: "sha256", Encrypted: true, Version: 1, RemoteVersion: "v1"},
		{FileUuid: "u2", LocalPath: "/a", Type: "dir", Owner: "root", Permissions: "0755"},
		{FileUuid: "u3", LocalPath: "/a/link", Type: "symlink", LinkTarget: "/a/b", Owner: "root"},
	}
	if err := InsertManifestBatch(db, stmts, "j", items); err != nil {
		t.Fatalf("InsertManifestBatch: %s", err)
	}

	got, err := LoadPendingManifest(db, stmts, "j")
	if err != nil {
		t.Fatalf("LoadPendingManifest: %s", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(got))
	}
	// Sort order is "type DESC, local_path ASC" so symlink comes first, then file, then dir — but
	// using TYPE order we want symlink > file > dir alphabetically descending: "symlink", "file", "dir".
	wantOrder := []string{"symlink", "file", "dir"}
	for i, w := range wantOrder {
		if got[i].Type != w {
			t.Errorf("row[%d].Type = %q, want %q", i, got[i].Type, w)
		}
	}

	// Re-insertion of the same manifest must be a no-op (INSERT OR IGNORE).
	if err := InsertManifestBatch(db, stmts, "j", items); err != nil {
		t.Fatalf("InsertManifestBatch (second call): %s", err)
	}
	got2, _ := LoadPendingManifest(db, stmts, "j")
	if len(got2) != 3 {
		t.Fatalf("INSERT OR IGNORE should not duplicate rows, got %d", len(got2))
	}
}

func TestSetFileStateFiltersLoadPendingManifest(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_setstate_")
	defer cleanup()

	if err := InsertRestoreJob(db, stmts, "j", "bkp1", "tgtA", "src", 1, "/x", true, "[]", "[]", "linux"); err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}
	items := []RestoreFileRow{
		{FileUuid: "u1", LocalPath: "/a", Type: "file"},
		{FileUuid: "u2", LocalPath: "/b", Type: "file"},
		{FileUuid: "u3", LocalPath: "/c", Type: "file"},
	}
	if err := InsertManifestBatch(db, stmts, "j", items); err != nil {
		t.Fatalf("InsertManifestBatch: %s", err)
	}

	// Mark u1 done, u2 failed, leave u3 pending — LoadPendingManifest should skip only the "done" one.
	if err := SetFileState(db, stmts, "j", "u1", "done", "", 10, 20); err != nil {
		t.Fatalf("SetFileState done: %s", err)
	}
	if err := SetFileState(db, stmts, "j", "u2", "failed", "boom", 10, 20); err != nil {
		t.Fatalf("SetFileState failed: %s", err)
	}

	got, err := LoadPendingManifest(db, stmts, "j")
	if err != nil {
		t.Fatalf("LoadPendingManifest: %s", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 non-done rows, got %d", len(got))
	}
	for _, r := range got {
		if r.FileUuid == "u1" {
			t.Errorf("done row should be filtered, got %q", r.FileUuid)
		}
	}
}

func TestBumpCounterAndReadCounters(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_counter_")
	defer cleanup()

	if err := InsertRestoreJob(db, stmts, "j", "bkp1", "tgtA", "src", 1, "/x", true, "[]", "[]", "linux"); err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}
	if err := BumpCounter(db, stmts, "j", "restored_files", 3); err != nil {
		t.Fatalf("BumpCounter restored_files: %s", err)
	}
	if err := BumpCounter(db, stmts, "j", "restored_files", 2); err != nil {
		t.Fatalf("BumpCounter restored_files (2): %s", err)
	}
	if err := BumpCounter(db, stmts, "j", "bytes_restored", 1024); err != nil {
		t.Fatalf("BumpCounter bytes_restored: %s", err)
	}

	counters, err := ReadCounters(db, stmts, "j")
	if err != nil {
		t.Fatalf("ReadCounters: %s", err)
	}
	if counters["restored_files"] != 5 {
		t.Errorf("restored_files = %d, want 5", counters["restored_files"])
	}
	if counters["bytes_restored"] != 1024 {
		t.Errorf("bytes_restored = %d, want 1024", counters["bytes_restored"])
	}
	if counters["restored_directories"] != 0 {
		t.Errorf("restored_directories = %d, want 0", counters["restored_directories"])
	}
}

func TestBumpCounterRejectsUnknownColumn(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_badcounter_")
	defer cleanup()

	if err := InsertRestoreJob(db, stmts, "j", "bkp1", "tgtA", "src", 1, "/x", true, "[]", "[]", "linux"); err != nil {
		t.Fatalf("InsertRestoreJob: %s", err)
	}
	// Unknown counter names must be rejected before any SQL is executed to prevent injection
	// via a buggy caller.
	err := BumpCounter(db, stmts, "j", "restored_files; DROP TABLE jobs", 1)
	if err == nil {
		t.Fatalf("expected BumpCounter to reject unknown column name")
	}
}

func TestReadCountersNoRowReturnsEmptyMap(t *testing.T) {
	db, stmts, cleanup := openTestRestoreDb(t, "unittest_rdbops_countermiss_")
	defer cleanup()

	counters, err := ReadCounters(db, stmts, "never-inserted")
	if err != nil {
		t.Fatalf("ReadCounters on missing row err=%s", err)
	}
	if len(counters) != 0 {
		t.Fatalf("expected empty map for missing row, got %+v", counters)
	}
}
