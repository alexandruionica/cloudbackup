package dbops

import (
	"cloudbackup/shared"
	"database/sql"
	"errors"
	"fmt"
)

// RestoreFileRow describes one row to be written to the restore database's
// restore_files table. Fields mirror the columns defined in
// database.CreateRestoreDb(). The package-local type avoids a circular import
// between restore/ and database/dbops/ while still letting the manifest be
// persisted from restore/restore.go.
type RestoreFileRow struct {
	FileUuid      string
	LocalPath     string
	Type          string
	LinkTarget    string
	Size          int64
	Mtime         int64
	Ctime         int64
	Owner         string
	Permissions   string
	Checksum      string
	ChecksumType  string
	Encrypted     bool
	Version       int64
	RemoteVersion string
	DeleteMarker  bool
}

// PrepareRestore assembles the SQL statements used against a per-target
// restore database. It mirrors Prepare() for the backup database but returns a
// shared.RestoreDbPreparedStatements. No *sql.Stmt handles are created here;
// the restore code uses Db.Exec / Db.Query with text from these fields because
// (a) prepared statements on a WAL sqlite3 DB are serialised through the same
// single connection we already set via SetMaxOpenConns(1), and (b) restore
// operations are low-frequency compared to backups so the performance delta
// is negligible. Keeping everything as text also means there is nothing to
// close when the DB is disconnected.
func PrepareRestore(db *sql.DB) (shared.RestoreDbPreparedStatements, error) {
	var stmts shared.RestoreDbPreparedStatements

	stmts.InsertJob = "INSERT INTO jobs (id, backup_name, target_name, source_backup_job_id, start_time, " +
		"end_time, state, report, restore_dir, all_files, request_files, exclusions, src_os) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	stmts.UpdateJobFinal = "UPDATE jobs SET end_time=?, state=?, report=? WHERE id=?"

	stmts.UpdateJobState = "UPDATE jobs SET state=? WHERE id=?"

	stmts.JobExists = "SELECT count(*) FROM jobs WHERE id=?"

	stmts.JobFetch = "SELECT id, backup_name, target_name, source_backup_job_id, start_time, end_time, " +
		"state, report, restore_dir, all_files, request_files, exclusions, src_os FROM jobs WHERE id=?"

	stmts.InsertManifestRow = "INSERT OR IGNORE INTO restore_files (restore_job_id, file_uuid, local_path, type, " +
		"link_target, size, mtime, ctime, owner, permissions, checksum, checksum_type, encrypted, version, " +
		"remote_version, delete_marker, state, error_msg, start_time, end_time) VALUES " +
		"(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', 0, 0)"

	stmts.SetFileState = "UPDATE restore_files SET state=?, error_msg=?, start_time=?, end_time=? " +
		"WHERE restore_job_id=? AND file_uuid=?"

	stmts.LoadPendingManifest = "SELECT file_uuid, local_path, type, link_target, size, mtime, ctime, " +
		"owner, permissions, checksum, checksum_type, encrypted, version, remote_version, delete_marker " +
		"FROM restore_files WHERE restore_job_id=? AND state != 'done' ORDER BY type DESC, local_path ASC"

	stmts.BumpCounter = "UPDATE restore_counters SET %s = %s + ? WHERE restore_job_id=?"

	stmts.InitCounterRow = "INSERT OR IGNORE INTO restore_counters (restore_job_id) VALUES (?)"

	stmts.ReadCounters = "SELECT restored_files, restored_directories, restored_symlinks, " +
		"failed_to_restore_files, skipped_delete_markers, bytes_restored FROM restore_counters WHERE restore_job_id=?"

	stmts.ReportJobsListQuery = "SELECT backup_name, target_name, id, start_time, end_time, state FROM jobs " +
		"WHERE backup_name=? AND state != 'started' AND start_time >= ? AND start_time <= ? " +
		"ORDER BY start_time LIMIT ? OFFSET ?"

	stmts.ReportJobShowQuery = "SELECT report, state FROM jobs WHERE backup_name=? AND id=? AND state != 'started'"

	stmts.ReportFilesListQuery = "SELECT local_path, type, size, state, error_msg, start_time, end_time " +
		"FROM restore_files WHERE restore_job_id=? ORDER BY local_path ASC LIMIT ? OFFSET ?"

	stmts.FindCrashedJobsQuery = "SELECT id, start_time, state FROM jobs WHERE state='started' OR state='stopping'"

	stmts.MarkCrashedJobQuery = "UPDATE jobs SET state='crashed' WHERE id=? AND (state='started' OR state='stopping')"

	// Touch db so the signature mirrors Prepare() and future additions (e.g. *sql.Stmt) can be
	// wired without changing call sites.
	_ = db
	return stmts, nil
}

// InsertRestoreJob writes the initial jobs-table row for a newly-started
// restore. requestFiles and exclusions are expected to be pre-serialised
// JSON strings (the caller owns the encoding format).
func InsertRestoreJob(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string, backupName string,
	targetName string, sourceJobId string, startTimeUnixNano int64, restoreDir string, allFiles bool,
	requestFiles string, exclusions string, srcOs string) error {

	allFilesInt := 0
	if allFiles {
		allFilesInt = 1
	}
	_, err := db.Exec(stmts.InsertJob, jobId, backupName, targetName, sourceJobId, startTimeUnixNano, 0,
		"started", "", restoreDir, allFilesInt, requestFiles, exclusions, srcOs)
	if err != nil {
		logger.Errorf("While inserting restore job '%s' for backup '%s' target '%s': %s",
			jobId, backupName, targetName, err)
		return err
	}
	// Ensure the counters row exists so BumpCounter can use a plain UPDATE.
	if _, err := db.Exec(stmts.InitCounterRow, jobId); err != nil {
		logger.Errorf("While initialising restore_counters row for job '%s': %s", jobId, err)
		return err
	}
	return nil
}

// UpdateRestoreJobFinal flips the job's end_time, state, and stored report
// in one shot. Used by the scheduler's cleanupAfterRestore.
func UpdateRestoreJobFinal(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string,
	endTimeUnixNano int64, state string, report string) error {

	result, err := db.Exec(stmts.UpdateJobFinal, endTimeUnixNano, state, report, jobId)
	if err != nil {
		logger.Errorf("While finalising restore job '%s' (state='%s'): %s", jobId, state, err)
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("expected exactly one row to be updated for restore job '%s' but affected %d", jobId, affected)
	}
	return nil
}

// UpdateRestoreJobState flips only the state column. Used by Resume() to take
// a "crashed" row back to "started" without touching end_time or report.
func UpdateRestoreJobState(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string, state string) error {
	_, err := db.Exec(stmts.UpdateJobState, state, jobId)
	if err != nil {
		logger.Errorf("While updating state of restore job '%s' to '%s': %s", jobId, state, err)
		return err
	}
	return nil
}

// RestoreJobExists reports whether a row with the supplied jobId exists in
// the restore DB's jobs table.
func RestoreJobExists(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string) (bool, error) {
	row := db.QueryRow(stmts.JobExists, jobId)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// RestoreJobRecord mirrors the jobs table for in-memory use by Resume().
type RestoreJobRecord struct {
	Id                string
	BackupName        string
	TargetName        string
	SourceBackupJobId string
	StartTime         int64
	EndTime           int64
	State             string
	Report            string
	RestoreDir        string
	AllFiles          bool
	RequestFiles      string
	Exclusions        string
	SrcOs             string
}

// FetchRestoreJob reads a single jobs row by id. Returns sql.ErrNoRows when
// the row is not present so callers can distinguish "missing" from "error".
func FetchRestoreJob(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string) (RestoreJobRecord, error) {
	row := db.QueryRow(stmts.JobFetch, jobId)
	var rec RestoreJobRecord
	var allFilesInt int
	err := row.Scan(&rec.Id, &rec.BackupName, &rec.TargetName, &rec.SourceBackupJobId, &rec.StartTime,
		&rec.EndTime, &rec.State, &rec.Report, &rec.RestoreDir, &allFilesInt, &rec.RequestFiles,
		&rec.Exclusions, &rec.SrcOs)
	if err != nil {
		return RestoreJobRecord{}, err
	}
	rec.AllFiles = allFilesInt != 0
	return rec, nil
}

// InsertManifestBatch writes every row in items into restore_files using a
// single transaction. Uses INSERT OR IGNORE so a resume call that re-enters
// this function is a no-op when the manifest has already been persisted.
func InsertManifestBatch(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string, items []RestoreFileRow) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		logger.Errorf("While starting manifest-insert transaction for restore job '%s': %s", jobId, err)
		return err
	}
	stmt, err := tx.Prepare(stmts.InsertManifestRow)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			logger.Warnf("Rollback after manifest prepare failure for job '%s' failed: %s", jobId, rbErr)
		}
		return err
	}
	defer func() {
		if cErr := stmt.Close(); cErr != nil {
			logger.Warnf("Closing manifest-insert statement for job '%s': %s", jobId, cErr)
		}
	}()
	for _, item := range items {
		encInt := 0
		if item.Encrypted {
			encInt = 1
		}
		dmInt := 0
		if item.DeleteMarker {
			dmInt = 1
		}
		if _, err := stmt.Exec(jobId, item.FileUuid, item.LocalPath, item.Type, item.LinkTarget, item.Size,
			item.Mtime, item.Ctime, item.Owner, item.Permissions, item.Checksum, item.ChecksumType,
			encInt, item.Version, item.RemoteVersion, dmInt); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Warnf("Rollback after manifest exec failure for job '%s' failed: %s", jobId, rbErr)
			}
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		logger.Errorf("While committing manifest insert for restore job '%s': %s", jobId, err)
		return err
	}
	return nil
}

// SetFileState updates the per-file state machine cell (pending →
// in_progress → done | failed | skipped | cancelled). The timestamps are
// written in the same statement so that we can later compute per-file
// duration without a second round-trip.
func SetFileState(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string, fileUuid string,
	state string, errMsg string, startTimeUnixNano int64, endTimeUnixNano int64) error {

	_, err := db.Exec(stmts.SetFileState, state, errMsg, startTimeUnixNano, endTimeUnixNano, jobId, fileUuid)
	if err != nil {
		logger.Warnf("While setting state of file '%s' in restore job '%s' to '%s': %s", fileUuid, jobId, state, err)
		return err
	}
	return nil
}

// LoadPendingManifest returns all rows for jobId that are not in the
// terminal "done" state. On a fresh run this is the full manifest;
// on a resume it is whatever is left to process.
func LoadPendingManifest(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string) ([]RestoreFileRow, error) {
	rows, err := db.Query(stmts.LoadPendingManifest, jobId)
	if err != nil {
		return nil, fmt.Errorf("while loading pending manifest for restore job '%s': %w", jobId, err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			logger.Warnf("Closing LoadPendingManifest rows for job '%s': %s", jobId, cErr)
		}
	}()
	results := make([]RestoreFileRow, 0)
	for rows.Next() {
		var item RestoreFileRow
		var encInt, dmInt int
		if err := rows.Scan(&item.FileUuid, &item.LocalPath, &item.Type, &item.LinkTarget, &item.Size,
			&item.Mtime, &item.Ctime, &item.Owner, &item.Permissions, &item.Checksum, &item.ChecksumType,
			&encInt, &item.Version, &item.RemoteVersion, &dmInt); err != nil {
			return nil, fmt.Errorf("while scanning restore_files row for job '%s': %w", jobId, err)
		}
		item.Encrypted = encInt != 0
		item.DeleteMarker = dmInt != 0
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("while enumerating restore_files rows for job '%s': %w", jobId, err)
	}
	return results, nil
}

// allowedCounters lists the column names that BumpCounter accepts. Using a
// fixed allow-list means we can safely interpolate the column name into the
// prepared-statement template without risking SQL injection from caller bugs.
var allowedCounters = map[string]struct{}{
	"restored_files":          {},
	"restored_directories":    {},
	"restored_symlinks":       {},
	"failed_to_restore_files": {},
	"skipped_delete_markers":  {},
	"bytes_restored":          {},
}

// BumpCounter increments one of the columns in restore_counters for a given
// job. $counterName must match exactly one of the whitelisted column names.
func BumpCounter(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string, counterName string, by int64) error {
	if _, ok := allowedCounters[counterName]; !ok {
		return fmt.Errorf("unknown restore counter '%s'", counterName)
	}
	stmt := fmt.Sprintf(stmts.BumpCounter, counterName, counterName)
	if _, err := db.Exec(stmt, by, jobId); err != nil { // #nosec G201 - counterName is validated against allowedCounters
		logger.Warnf("While incrementing counter '%s' for restore job '%s' by %d: %s", counterName, jobId, by, err)
		return err
	}
	return nil
}

// ReadCounters loads the restore_counters row for the given job. Returns
// zeros (not an error) when the row is missing; callers use these values to
// pre-seed the in-memory StatsCounters on resume.
func ReadCounters(db *sql.DB, stmts shared.RestoreDbPreparedStatements, jobId string) (map[string]uint64, error) {
	row := db.QueryRow(stmts.ReadCounters, jobId)
	var restoredFiles, restoredDirs, restoredSymlinks, failed, skipped, bytesRestored int64
	err := row.Scan(&restoredFiles, &restoredDirs, &restoredSymlinks, &failed, &skipped, &bytesRestored)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]uint64{}, nil
		}
		return nil, err
	}
	return map[string]uint64{
		"restored_files":          uint64(restoredFiles),
		"restored_directories":    uint64(restoredDirs),
		"restored_symlinks":       uint64(restoredSymlinks),
		"failed_to_restore_files": uint64(failed),
		"skipped_delete_markers":  uint64(skipped),
		"bytes_restored":          uint64(bytesRestored),
	}, nil
}
