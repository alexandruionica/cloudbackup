package database

import (
	"cloudbackup/shared"
	"cloudbackup/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGetRestoreDbKey(t *testing.T) {
	got := GetRestoreDbKey("bkp1", "tgtA")
	want := "restore__bkp1__tgtA"
	if got != want {
		t.Fatalf("GetRestoreDbKey() = %q, want %q", got, want)
	}
}

func TestGetRestoreDbFilePath(t *testing.T) {
	datadir := utils.SetupTmpDir("unittest_restore_db_getpath_", t)
	defer func() { _ = os.RemoveAll(datadir) }()

	got, err := GetRestoreDbFilePath(datadir, "bkp1", "tgtA")
	if err != nil {
		t.Fatalf("GetRestoreDbFilePath() err=%s", err)
	}
	want := datadir + string(filepath.Separator) + "restore__bkp1__tgtA.sqlite"
	if got != want {
		t.Fatalf("GetRestoreDbFilePath() = %q, want %q", got, want)
	}
}

func TestStartRestoreDbCreatesFileAndTables(t *testing.T) {
	datadir := utils.SetupTmpDir("unittest_restore_db_start_", t)
	backupName, targetName := "bkp1", "tgtA"
	state := shared.NewJobsState()
	dbName := GetRestoreDbKey(backupName, targetName)

	db, err := StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("StartRestoreDb() err=%s", err)
	}
	defer func() {
		DisconnectFromDb(dbName, state, db)
		CloseDb(dbName, state, true)
		_ = os.RemoveAll(datadir)
	}()

	dbfilepath, _ := GetRestoreDbFilePath(datadir, backupName, targetName)
	if exists, _ := DbFileExists(dbfilepath); !exists {
		t.Fatalf("expected restore DB file to exist at %s", dbfilepath)
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("query sqlite_master err=%s", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan sqlite_master err=%s", err)
		}
		names = append(names, n)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"jobs", "restore_files", "restore_counters"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected table %q to be present, got tables: %s", want, joined)
		}
	}
}

func TestStartRestoreDbIdempotent(t *testing.T) {
	datadir := utils.SetupTmpDir("unittest_restore_db_idem_", t)
	backupName, targetName := "bkp1", "tgtA"
	state := shared.NewJobsState()
	dbName := GetRestoreDbKey(backupName, targetName)

	db1, err := StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("first StartRestoreDb() err=%s", err)
	}
	if _, err := db1.Exec("INSERT INTO jobs (id, backup_name, target_name, state) VALUES ('abc', 'bkp1', 'tgtA', 'started')"); err != nil {
		t.Fatalf("insert sentinel err=%s", err)
	}
	DisconnectFromDb(dbName, state, db1)

	db2, err := StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("second StartRestoreDb() err=%s", err)
	}
	defer func() {
		DisconnectFromDb(dbName, state, db2)
		CloseDb(dbName, state, true)
		_ = os.RemoveAll(datadir)
	}()

	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM jobs WHERE id='abc'").Scan(&count); err != nil {
		t.Fatalf("count sentinel err=%s", err)
	}
	if count != 1 {
		t.Fatalf("expected sentinel row to survive second StartRestoreDb, got count=%d", count)
	}
}

func TestCheckForCrashedRestoreJobsMarksRunningAsCrashed(t *testing.T) {
	datadir := utils.SetupTmpDir("unittest_restore_db_crash_", t)
	backupName, targetName := "bkp1", "tgtA"
	state := shared.NewJobsState()
	dbName := GetRestoreDbKey(backupName, targetName)
	defer func() {
		CloseDb(dbName, state, true)
		_ = os.RemoveAll(datadir)
	}()

	db, err := StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("StartRestoreDb() err=%s", err)
	}
	for _, row := range []struct {
		id, stateVal string
	}{
		{"job-running", "started"},
		{"job-stopping", "stopping"},
		{"job-finished", "finished"},
	} {
		if _, err := db.Exec(
			"INSERT INTO jobs (id, backup_name, target_name, start_time, state) VALUES (?, ?, ?, 1, ?)",
			row.id, backupName, targetName, row.stateVal); err != nil {
			t.Fatalf("seed insert err=%s", err)
		}
	}
	DisconnectFromDb(dbName, state, db)

	cfg := shared.CfgTemplate{
		DataDir: datadir,
		Mutex:   &sync.RWMutex{},
		Backup: []shared.ConfigBackup{{
			Name:   backupName,
			Target: []shared.ConfigBackupTarget{{Name: targetName}},
		}},
	}
	if err := CheckForCrashedRestoreJobs(cfg, state); err != nil {
		t.Fatalf("CheckForCrashedRestoreJobs() err=%s", err)
	}

	db2, err := StartRestoreDb(datadir, backupName, targetName, state)
	if err != nil {
		t.Fatalf("reopen StartRestoreDb() err=%s", err)
	}
	defer DisconnectFromDb(dbName, state, db2)

	got := map[string]string{}
	rows, err := db2.Query("SELECT id, state FROM jobs")
	if err != nil {
		t.Fatalf("query jobs err=%s", err)
	}
	for rows.Next() {
		var id, s string
		if err := rows.Scan(&id, &s); err != nil {
			t.Fatalf("scan err=%s", err)
		}
		got[id] = s
	}
	_ = rows.Close()

	want := map[string]string{
		"job-running":  "crashed",
		"job-stopping": "crashed",
		"job-finished": "finished",
	}
	for id, wantState := range want {
		if got[id] != wantState {
			t.Errorf("job %s: state=%q, want %q", id, got[id], wantState)
		}
	}
}

func TestCheckForCrashedRestoreJobsNoopForMissingFile(t *testing.T) {
	datadir := utils.SetupTmpDir("unittest_restore_db_nofile_", t)
	defer func() { _ = os.RemoveAll(datadir) }()

	state := shared.NewJobsState()
	cfg := shared.CfgTemplate{
		DataDir: datadir,
		Mutex:   &sync.RWMutex{},
		Backup: []shared.ConfigBackup{{
			Name:   "never-restored",
			Target: []shared.ConfigBackupTarget{{Name: "tgtA"}},
		}},
	}
	if err := CheckForCrashedRestoreJobs(cfg, state); err != nil {
		t.Fatalf("CheckForCrashedRestoreJobs() on missing file: err=%s", err)
	}
}
