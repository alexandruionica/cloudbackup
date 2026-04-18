package database

import (
	"cloudbackup/shared"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// RestoreDbPrefix is the leading component of the on-disk filename for every
// per-target restore database. It is exported so callers outside this package
// (e.g. operators or tests looking at the data directory) can filter these
// files apart from backup databases.
const RestoreDbPrefix = "restore__"

// RestoreDbSeparator is the token used between the backup definition name and
// the target name inside a restore database filename. Config validation
// (see config.Validate) rejects backup/target names that contain this token.
const RestoreDbSeparator = "__"

// GetRestoreDbKey returns the identifier used as the map key inside
// BackupJobsState.DbOpenAllowed and as the on-disk filename (without the
// ".sqlite" suffix). The format is:
//
//	restore__<backupName>__<targetName>
//
// Callers MUST use this helper instead of hand-assembling strings so that any
// future naming change happens in exactly one place.
func GetRestoreDbKey(backupName string, targetName string) string {
	return RestoreDbPrefix + backupName + RestoreDbSeparator + targetName
}

// GetRestoreDbFilePath returns the absolute path to the restore database
// associated with the supplied (backupName, targetName) pair.
func GetRestoreDbFilePath(datadir string, backupName string, targetName string) (string, error) {
	fname := GetRestoreDbKey(backupName, targetName) + ".sqlite"
	dbfilepath, err := filepath.Abs(datadir + string(filepath.Separator) + fname)
	if err != nil {
		logger.Errorf("While trying to establish the absolute path to the restore database for backup '%s' and target '%s', encountered error: %s", backupName, targetName, err)
		return "", errors.New("could not establish the absolute path to the restore database file")
	}
	return dbfilepath, nil
}

// CreateRestoreDb initialises the schema of a freshly-created restore database.
// It is the counterpart to CreateDb() for backup databases.
func CreateRestoreDb(db *sql.DB, dbfilepath string) error {
	sqlStmt := `
	CREATE TABLE jobs (
		id TEXT NOT NULL PRIMARY KEY,
		backup_name TEXT,
		target_name TEXT,
		source_backup_job_id TEXT,
		start_time INTEGER,
		end_time INTEGER,
		state TEXT,
		report TEXT,
		restore_dir TEXT,
		all_files INTEGER,
		request_files TEXT,
		exclusions TEXT,
		src_os TEXT
	);

	CREATE INDEX jobs_state ON jobs(state);
	CREATE INDEX jobs_start_time ON jobs(start_time);

	CREATE TABLE restore_files (
		restore_job_id TEXT NOT NULL,
		file_uuid TEXT NOT NULL,
		local_path TEXT,
		type TEXT,
		link_target TEXT,
		size INTEGER,
		mtime INTEGER,
		ctime INTEGER,
		owner TEXT,
		permissions TEXT,
		checksum TEXT,
		checksum_type TEXT,
		encrypted INTEGER,
		version INTEGER,
		remote_version TEXT,
		delete_marker INTEGER,
		state TEXT,
		error_msg TEXT,
		start_time INTEGER,
		end_time INTEGER,
		PRIMARY KEY (restore_job_id, file_uuid),
		FOREIGN KEY (restore_job_id) REFERENCES jobs(id)
	);

	CREATE INDEX restore_files_state ON restore_files(restore_job_id, state);
	CREATE INDEX restore_files_local_path ON restore_files(restore_job_id, local_path);

	CREATE TABLE restore_counters (
		restore_job_id TEXT NOT NULL PRIMARY KEY,
		restored_files INTEGER DEFAULT 0,
		restored_directories INTEGER DEFAULT 0,
		restored_symlinks INTEGER DEFAULT 0,
		failed_to_restore_files INTEGER DEFAULT 0,
		skipped_delete_markers INTEGER DEFAULT 0,
		bytes_restored INTEGER DEFAULT 0,
		FOREIGN KEY (restore_job_id) REFERENCES jobs(id)
	);
	`
	logger.Debugf("Creating tables in restore database '%s'", dbfilepath)
	_, err := db.Exec(sqlStmt)
	if err != nil {
		logger.Errorf("Encountered error while attempting to create restore database '%s'. The error is: %s",
			dbfilepath, err)
		fExists, _ := DbFileExists(dbfilepath) // #nosec
		if fExists {
			rmErr := os.Remove(dbfilepath)
			if rmErr != nil {
				logger.Errorf("An additional error was encountered when trying to remove the incorrectly "+
					"initialised restore db file '%s'. The error was: %s", dbfilepath, rmErr)
			}
		}
		return err
	}
	return nil
}

// ValidateAndCreateRestoreDb ensures the restore database file for
// (backupName, targetName) exists; creating and initialising it if missing.
// Safe to call repeatedly.
func ValidateAndCreateRestoreDb(datadir string, backupName string, targetName string, backupJobsState *shared.BackupJobsState) error {
	dbfilepath, err := GetRestoreDbFilePath(datadir, backupName, targetName)
	if err != nil {
		return err
	}
	fExists, err := DbFileExists(dbfilepath)
	if err != nil {
		return err
	}
	if fExists {
		return nil
	}
	logger.Infof("Creating restore database '%s' for backup '%s' and target '%s'", dbfilepath, backupName, targetName)
	dbName := GetRestoreDbKey(backupName, targetName)
	db, err := openAndRegisterRestoreDb(datadir, backupName, targetName, false, backupJobsState)
	if err != nil {
		return ErrCouldNotCreateDB
	}
	if err := CreateRestoreDb(db, dbfilepath); err != nil {
		DisconnectFromDb(dbName, backupJobsState, db)
		logger.Errorf("Could not initialise restore database for backup '%s' and target '%s': %s", backupName, targetName, err)
		return err
	}
	DisconnectFromDb(dbName, backupJobsState, db)
	return nil
}

// openAndRegisterRestoreDb registers the database name in BackupJobsState.DbOpenAllowed
// (so that CloseDb/DisconnectFromDb work exactly like they do for backup DBs)
// and returns an opened *sql.DB. The generic OpenDb keys off an arbitrary
// dbName and resolves the filesystem path via GetDbFilePath(datadir, dbName);
// because our dbName already includes the "restore__" prefix the resulting
// path matches what GetRestoreDbFilePath produces.
func openAndRegisterRestoreDb(datadir string, backupName string, targetName string, fileExists bool, backupJobsState *shared.BackupJobsState) (*sql.DB, error) {
	dbName := GetRestoreDbKey(backupName, targetName)
	return OpenDb(datadir, dbName, fileExists, backupJobsState, 0)
}

// StartRestoreDb is the restore-database equivalent of database.Start().
// If the on-disk file is missing it is created and initialised, then opened.
func StartRestoreDb(datadir string, backupName string, targetName string, backupJobsState *shared.BackupJobsState) (*sql.DB, error) {
	if err := ValidateAndCreateRestoreDb(datadir, backupName, targetName, backupJobsState); err != nil {
		return &sql.DB{}, err
	}
	return openAndRegisterRestoreDb(datadir, backupName, targetName, true, backupJobsState)
}

// CheckForCrashedRestoreJobs walks every (backup, target) pair in the config,
// opens the restore DB if the file exists, and marks any job whose state is
// "started" or "stopping" as "crashed". Safe to call exactly once during
// daemon startup, before any new restores are launched.
func CheckForCrashedRestoreJobs(config shared.CfgTemplate, backupJobsState *shared.BackupJobsState) error {
	config.Mutex.RLock()
	dataDir := config.DataDir
	type pair struct {
		backupName string
		targetName string
	}
	pairs := make([]pair, 0)
	for _, b := range config.Backup {
		for _, t := range b.Target {
			pairs = append(pairs, pair{backupName: b.Name, targetName: t.Name})
		}
	}
	config.Mutex.RUnlock()

	for _, p := range pairs {
		err := findAndMarkCrashedRestoreJobs(dataDir, p.backupName, p.targetName, backupJobsState)
		if err != nil {
			logger.Errorf("While checking crashed restore jobs for backup '%s' target '%s': %s",
				p.backupName, p.targetName, err)
			return err
		}
	}
	return nil
}

// findAndMarkCrashedRestoreJobs opens the restore database for a specific
// (backup, target) pair if the file already exists and flips any "started"
// or "stopping" rows to "crashed". It is a no-op when the database file does
// not yet exist (no restore has ever been started for that pair).
func findAndMarkCrashedRestoreJobs(datadir string, backupName string, targetName string, backupJobsState *shared.BackupJobsState) error {
	dbfilepath, err := GetRestoreDbFilePath(datadir, backupName, targetName)
	if err != nil {
		return err
	}
	fExists, err := DbFileExists(dbfilepath)
	if err != nil {
		return err
	}
	if !fExists {
		return nil
	}
	dbName := GetRestoreDbKey(backupName, targetName)
	db, err := OpenDb(datadir, dbName, true, backupJobsState, 0)
	if err != nil {
		return err
	}
	defer DisconnectFromDb(dbName, backupJobsState, db)

	type toMark struct {
		id        string
		startTime int64
		state     string
	}
	results := make([]toMark, 0)
	rows, err := db.Query("SELECT id, start_time, state FROM jobs WHERE state='started' OR state='stopping'")
	if err != nil {
		logger.Warnf("While trying to find crashed restore jobs in database '%s': %s", dbfilepath, err)
		return nil
	}
	for rows.Next() {
		item := toMark{}
		if err := rows.Scan(&item.id, &item.startTime, &item.state); err != nil {
			logger.Warnf("While scanning crashed restore jobs row in '%s': %s", dbfilepath, err)
			continue
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		logger.Warnf("While enumerating crashed restore job rows in '%s': %s", dbfilepath, err)
	}
	if cErr := rows.Close(); cErr != nil {
		logger.Warnf("While closing crashed restore job query in '%s': %s", dbfilepath, cErr)
	}

	for _, entry := range results {
		logger.Warnf("Marking restore job id '%s' (started '%s', state '%s') for backup '%s' target '%s' as crashed",
			entry.id, time.Unix(0, entry.startTime).Format(time.RFC3339Nano), entry.state, backupName, targetName)
		_, err := db.Exec("UPDATE jobs SET state='crashed' WHERE id=? AND (state='started' OR state='stopping')", entry.id)
		if err != nil {
			logger.Warnf("While marking restore job '%s' as crashed: %s", entry.id, err)
		}
	}
	return nil
}
