package restore

import (
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const loggingContext = "restore"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// Request describes everything the orchestration layer needs in order to run a single restore. It
// is the package-internal equivalent of shared.ReceiveRestoreCommand but with fields narrowed to
// what Do() actually consumes.
type Request struct {
	JobName            string
	RestoreJobId       string
	SourceBackupJobId  string
	TargetName         string
	Files              []string
	AllFiles           bool
	RestoreDirOverride string
	Exclusions         []string
}

// Result reports how the restore job ended so the caller (scheduler) can produce the right final
// state entry in the jobs DB table and in BackupJobsState.
type Result struct {
	State             string
	Err               error
	RestoredDirectory string
	// TargetName is filled in once the restore package has resolved which target the job is
	// actually reading from (important when the caller omitted TargetName from the Request and the
	// default first-target fallback kicked in). cleanupAfterRestore needs this to locate the
	// correct per-target restore database when it writes the final jobs-table row.
	TargetName string
}

// Do runs a restore job end to end. It opens the backup's SQLite database, resolves the list of
// files to restore, persists the manifest and per-file progress into a per-target restore
// database, downloads each file into RestoredDirectory, and updates counters both in the restore
// DB (for durability) and in backupJobsState (for live watch streams). It MUST be invoked after
// backupJobsState.MarkRestoreRunning has been called for (jobName, restoreJobId) so that
// ctx/cancel/counters exist in Running[]. Do() is idempotent: calling it again with a
// RestoreJobId that already has a row in the restore DB becomes a resume — only rows with state
// != 'done' are re-attempted.
func Do(jobName string, req Request, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) Result {

	req.Files = sanitizeFilePaths(req.Files)

	ctx, err := backupJobsState.GetContextForJob(jobName, req.RestoreJobId)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("restore job was not found in the running state: %w", err)}
	}

	backupConfig, err := shared.MakeCopyOfBackupJobDefinition(jobName, serverConfigCopy)
	if err != nil {
		return Result{State: "failed", Err: err}
	}

	target, err := pickTarget(backupConfig, req.TargetName)
	if err != nil {
		return Result{State: "failed", Err: err}
	}
	// Make sure the resolved target name is available to the caller so cleanupAfterRestore can
	// find the right per-target restore DB when it writes the final job row.
	req.TargetName = target.Name

	// Only the chosen target is initialised — no reason to spin up connections to stores we are not
	// going to read from for this restore.
	singleTargetConfig := shared.CopyConfigBackupStruct(backupConfig)
	singleTargetConfig.Target = []shared.ConfigBackupTarget{target}
	objectStores, err := objectstore.GetObjectStores(ctx, singleTargetConfig, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not initialise the object store for target '%s': %w", target.Name, err), TargetName: target.Name}
	}
	if len(objectStores) != 1 {
		return Result{State: "failed", Err: fmt.Errorf("expected exactly one object store for target '%s' but got %d", target.Name, len(objectStores)), TargetName: target.Name}
	}
	objStore := objectStores[0]

	restoreDir, err := resolveRestoreDir(req, serverConfigCopy, jobName)
	if err != nil {
		return Result{State: "failed", Err: err, TargetName: target.Name}
	}
	if err := os.MkdirAll(restoreDir, 0o755); err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not create restore directory '%s': %w", restoreDir, err), TargetName: target.Name}
	}

	serverConfigCopy.Mutex.RLock()
	dataDir := serverConfigCopy.DataDir
	serverConfigCopy.Mutex.RUnlock()

	// Backup DB is still needed for source-job validation and manifest resolution.
	backupDb, err := database.Start(dataDir, jobName, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not open database for backup definition '%s': %w", jobName, err), RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	defer database.DisconnectFromDb(jobName, backupJobsState, backupDb)

	if err := validateSourceJobId(backupDb, jobName, req.SourceBackupJobId); err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
	}

	// Bring up encryption for the chosen target. AllowBootstrap=false: restore must NEVER
	// create a sidecar — if it's missing the encrypted data is unrecoverable and we want to
	// surface that loudly rather than silently producing a fresh keystore.
	hasEnc, dbErr := dbops.HasAnyEncryptedFiles(backupDb)
	if dbErr != nil {
		return Result{State: "failed", Err: fmt.Errorf("checking local DB for encrypted files: %w", dbErr), RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	if err := objStore.InitEncryption(objectstore.EncryptionInitOptions{
		HasEncryptedFiles: hasEnc,
		AllowBootstrap:    false,
	}); err != nil {
		return Result{State: "failed", Err: fmt.Errorf("initialising client-side encryption for target '%s': %w", target.Name, err), RestoredDirectory: restoreDir, TargetName: target.Name}
	}

	// Restore DB is per-(backup, target) and holds the manifest, per-file state and counters.
	restoreDb, err := database.StartRestoreDb(dataDir, jobName, target.Name, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not open restore database for backup '%s' target '%s': %w", jobName, target.Name, err), RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	restoreDbKey := database.GetRestoreDbKey(jobName, target.Name)
	defer database.DisconnectFromDb(restoreDbKey, backupJobsState, restoreDb)

	stmts, err := dbops.PrepareRestore(restoreDb)
	if err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
	}

	// A row for this job id in the restore DB means this is a resume of a previously started
	// restore. The manifest is already on disk; we only need to seed the in-memory counters and
	// flip the state back to 'started'.
	isResume, err := dbops.RestoreJobExists(restoreDb, stmts, req.RestoreJobId)
	if err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
	}

	if !isResume {
		if err := initFreshRestoreJob(restoreDb, stmts, backupDb, req, jobName, target.Name, restoreDir, backupJobsState); err != nil {
			return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
		}
	} else {
		if err := resumeRestoreJob(restoreDb, stmts, req.RestoreJobId, jobName, backupJobsState); err != nil {
			return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
		}
	}

	items, err := dbops.LoadPendingManifest(restoreDb, stmts, req.RestoreJobId)
	if err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	if len(items) == 0 {
		// Resume landed on an already-complete job. Treat as a no-op success.
		return Result{State: "finished", RestoredDirectory: restoreDir, TargetName: target.Name}
	}

	backupJobsState.UpdateStatsText(jobName, "current_operation", "Restoring files", "", "")

	var restoredAny bool
	for _, item := range items {
		select {
		case <-ctx.Done():
			backupJobsState.UpdateStatsText(jobName, "current_operation", "", "", "")
			return Result{State: "cancelled", RestoredDirectory: restoreDir, TargetName: target.Name}
		default:
		}

		// Bump the per-job sequence so each item's WatchMessage carries a unique, monotonically
		// increasing Sequence. The CLI watch view uses Sequence to distinguish "redraw current
		// line" from "new line" and to detect dropped messages; without this bump every message
		// would carry Sequence=0 and the CLI would re-print the column header for every event.
		backupJobsState.IncrementSequence(jobName)

		if item.DeleteMarker {
			now := time.Now().UnixNano()
			backupJobsState.IncrementCounter(jobName, "skipped_delete_markers", item.LocalPath, item.Type, "restore", "")
			if err := dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "skipped_delete_markers", 1); err != nil {
				logger.Warnf("Could not bump skipped_delete_markers counter in restore DB: %s", err)
			}
			if err := dbops.SetFileState(restoreDb, stmts, req.RestoreJobId, item.FileUuid, "skipped", "", now, now); err != nil {
				logger.Warnf("Could not set file state to 'skipped' in restore DB: %s", err)
			}
			continue
		}

		startTimeNs := time.Now().UnixNano()
		if err := dbops.SetFileState(restoreDb, stmts, req.RestoreJobId, item.FileUuid, "in_progress", "", startTimeNs, 0); err != nil {
			logger.Warnf("Could not set file state to 'in_progress' in restore DB: %s", err)
		}

		outcome := restoreOne(ctx, objStore, item, restoreDir, jobName, backupJobsState)
		endTimeNs := time.Now().UnixNano()

		switch outcome.state {
		case "done":
			if err := dbops.SetFileState(restoreDb, stmts, req.RestoreJobId, item.FileUuid, "done", "", startTimeNs, endTimeNs); err != nil {
				logger.Warnf("Could not set file state to 'done' in restore DB: %s", err)
			}
			switch item.Type {
			case "dir":
				_ = dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "restored_directories", 1)
			case "symlink":
				_ = dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "restored_symlinks", 1)
			case "file":
				_ = dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "restored_files", 1)
				if item.Size > 0 {
					_ = dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "bytes_restored", item.Size)
				}
			}
			restoredAny = true
		case "cancelled":
			// Context cancellation: leave the row with whatever state the last SetFileState
			// wrote (normally 'in_progress'). A later resume will retry it. We do not flip it
			// to 'cancelled' because the in-flight download may still be writing to disk.
			backupJobsState.UpdateStatsText(jobName, "current_operation", "", "", "")
			return Result{State: "cancelled", RestoredDirectory: restoreDir, TargetName: target.Name}
		default: // "failed"
			if err := dbops.SetFileState(restoreDb, stmts, req.RestoreJobId, item.FileUuid, "failed", outcome.errMsg, startTimeNs, endTimeNs); err != nil {
				logger.Warnf("Could not set file state to 'failed' in restore DB: %s", err)
			}
			if err := dbops.BumpCounter(restoreDb, stmts, req.RestoreJobId, "failed_to_restore_files", 1); err != nil {
				logger.Warnf("Could not bump failed_to_restore_files counter in restore DB: %s", err)
			}
		}
	}

	backupJobsState.UpdateStatsText(jobName, "current_operation", "", "", "")

	if ctx.Err() == context.Canceled {
		return Result{State: "cancelled", RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	if !restoredAny {
		return Result{State: "failed", Err: errors.New("all files failed to restore"), RestoredDirectory: restoreDir, TargetName: target.Name}
	}
	return Result{State: "finished", RestoredDirectory: restoreDir, TargetName: target.Name}
}

// initFreshRestoreJob writes the jobs-table row, resolves the manifest from the backup DB,
// applies exclusions, and persists each manifest item with state 'pending' in the restore DB.
func initFreshRestoreJob(restoreDb *sql.DB, stmts shared.RestoreDbPreparedStatements, backupDb *sql.DB,
	req Request, jobName string, targetName string, restoreDir string, backupJobsState *shared.BackupJobsState) error {

	jobStartTime, err := backupJobsState.GetStartTime(jobName, req.RestoreJobId, loggingContext+".initFreshRestoreJob")
	if err != nil {
		return err
	}

	requestFilesJson, err := json.Marshal(req.Files)
	if err != nil {
		return fmt.Errorf("could not encode request file list for restore job: %w", err)
	}
	exclusionsJson, err := json.Marshal(req.Exclusions)
	if err != nil {
		return fmt.Errorf("could not encode exclusions for restore job: %w", err)
	}
	if err := dbops.InsertRestoreJob(restoreDb, stmts, req.RestoreJobId, jobName, targetName, req.SourceBackupJobId,
		jobStartTime.UnixNano(), restoreDir, req.AllFiles, string(requestFilesJson), string(exclusionsJson), runtime.GOOS); err != nil {
		return fmt.Errorf("could not add restore job record to database: %w", err)
	}

	items, err := fetchItems(backupDb, req)
	if err != nil {
		return err
	}
	if len(req.Exclusions) > 0 {
		items, err = applyExclusions(items, req.Exclusions)
		if err != nil {
			return fmt.Errorf("error applying exclusions: %w", err)
		}
	}
	if len(items) == 0 {
		return errors.New("no files matched the restore request")
	}

	rows := make([]dbops.RestoreFileRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, dbops.RestoreFileRow{
			FileUuid:      it.uuid,
			LocalPath:     it.localPath,
			Type:          it.typ,
			LinkTarget:    it.linkTarget,
			Size:          it.size,
			Mtime:         it.mtime,
			Ctime:         it.ctime,
			Owner:         it.owner,
			Permissions:   it.permissions,
			Checksum:      it.checksum,
			ChecksumType:  it.checksumType,
			Encrypted:     it.encrypted,
			Version:       it.version,
			RemoteVersion: it.remoteVersion,
			DeleteMarker:  it.deleteMarker,
		})
	}
	if err := dbops.InsertManifestBatch(restoreDb, stmts, req.RestoreJobId, rows); err != nil {
		return fmt.Errorf("could not persist restore manifest: %w", err)
	}
	return nil
}

// resumeRestoreJob pre-seeds the in-memory counters from the restore DB and flips the job row
// state from 'crashed' (or whatever it was) back to 'started'.
func resumeRestoreJob(restoreDb *sql.DB, stmts shared.RestoreDbPreparedStatements, restoreJobId string,
	jobName string, backupJobsState *shared.BackupJobsState) error {

	counters, err := dbops.ReadCounters(restoreDb, stmts, restoreJobId)
	if err != nil {
		return fmt.Errorf("could not read counters from restore DB for resume: %w", err)
	}
	for k, v := range counters {
		backupJobsState.SeedCounter(jobName, restoreJobId, k, v)
	}
	if err := dbops.UpdateRestoreJobState(restoreDb, stmts, restoreJobId, "started"); err != nil {
		return fmt.Errorf("could not flip restore job state back to 'started': %w", err)
	}
	return nil
}

// Resume is a thin wrapper invoked by the scheduler when a caller asks to resume a
// previously-crashed restore. It reads the jobs row out of the per-target restore database,
// rebuilds a Request from its stored fields, and hands off to Do() which detects the existing
// row and skips the manifest resolution step. It MUST be invoked after MarkRestoreRunning so
// ctx/cancel/counters exist in Running[].
func Resume(jobName string, restoreJobId string, targetName string, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) Result {

	serverConfigCopy.Mutex.RLock()
	dataDir := serverConfigCopy.DataDir
	serverConfigCopy.Mutex.RUnlock()

	restoreDb, err := database.StartRestoreDb(dataDir, jobName, targetName, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not open restore database for backup '%s' target '%s': %w", jobName, targetName, err), TargetName: targetName}
	}
	restoreDbKey := database.GetRestoreDbKey(jobName, targetName)

	stmts, err := dbops.PrepareRestore(restoreDb)
	if err != nil {
		database.DisconnectFromDb(restoreDbKey, backupJobsState, restoreDb)
		return Result{State: "failed", Err: err, TargetName: targetName}
	}
	rec, err := dbops.FetchRestoreJob(restoreDb, stmts, restoreJobId)
	if err != nil {
		database.DisconnectFromDb(restoreDbKey, backupJobsState, restoreDb)
		if errors.Is(err, sql.ErrNoRows) {
			return Result{State: "failed", Err: fmt.Errorf("no restore job with id '%s' in restore database for backup '%s' target '%s'", restoreJobId, jobName, targetName), TargetName: targetName}
		}
		return Result{State: "failed", Err: err, TargetName: targetName}
	}
	if rec.State != "crashed" && rec.State != "stopped" {
		database.DisconnectFromDb(restoreDbKey, backupJobsState, restoreDb)
		return Result{State: "failed", Err: fmt.Errorf("restore job '%s' cannot be resumed because its current state is '%s' (expected 'crashed' or 'stopped')", restoreJobId, rec.State), TargetName: targetName}
	}
	// Close the DB before calling Do() because Do() will re-open it via StartRestoreDb.
	database.DisconnectFromDb(restoreDbKey, backupJobsState, restoreDb)

	var requestFiles []string
	if rec.RequestFiles != "" && rec.RequestFiles != "null" {
		if err := json.Unmarshal([]byte(rec.RequestFiles), &requestFiles); err != nil {
			return Result{State: "failed", Err: fmt.Errorf("could not decode stored request_files for resume: %w", err), TargetName: targetName}
		}
	}
	var exclusions []string
	if rec.Exclusions != "" && rec.Exclusions != "null" {
		if err := json.Unmarshal([]byte(rec.Exclusions), &exclusions); err != nil {
			return Result{State: "failed", Err: fmt.Errorf("could not decode stored exclusions for resume: %w", err), TargetName: targetName}
		}
	}

	req := Request{
		JobName:            jobName,
		RestoreJobId:       restoreJobId,
		SourceBackupJobId:  rec.SourceBackupJobId,
		TargetName:         rec.TargetName,
		Files:              requestFiles,
		AllFiles:           rec.AllFiles,
		RestoreDirOverride: rec.RestoreDir,
		Exclusions:         exclusions,
	}
	return Do(jobName, req, serverConfigCopy, backupJobsState)
}

// pickTarget returns the ConfigBackupTarget matching $targetName. If $targetName is empty it
// returns the first target in the backup definition. An error is returned only when a non-empty
// name does not match any target.
func pickTarget(backupConfig shared.ConfigBackup, targetName string) (shared.ConfigBackupTarget, error) {
	if len(backupConfig.Target) == 0 {
		return shared.ConfigBackupTarget{}, fmt.Errorf("backup definition '%s' has no targets configured", backupConfig.Name)
	}
	if targetName == "" {
		return backupConfig.Target[0], nil
	}
	for _, t := range backupConfig.Target {
		if t.Name == targetName {
			return t, nil
		}
	}
	return shared.ConfigBackupTarget{}, fmt.Errorf("no target named '%s' was found in backup definition '%s'", targetName, backupConfig.Name)
}

// resolveRestoreDir computes the absolute destination directory. Priority:
//  1. req.RestoreDirOverride (per-request override)
//  2. "<serverConfig.RestoreDir>/<jobName>/<restoreJobId>"
//  3. "<serverConfig.DataDir>/restores/<jobName>/<restoreJobId>" (when RestoreDir is unset)
func resolveRestoreDir(req Request, cfg shared.CfgTemplate, jobName string) (string, error) {
	if strings.TrimSpace(req.RestoreDirOverride) != "" {
		return filepath.Clean(req.RestoreDirOverride), nil
	}
	cfg.Mutex.RLock()
	base := cfg.RestoreDir
	dataDir := cfg.DataDir
	cfg.Mutex.RUnlock()
	if base == "" {
		base = filepath.Join(dataDir, "restores")
	}
	if base == "" {
		return "", errors.New("neither 'restore_dir' nor 'data_dir' is set in the server config")
	}
	return filepath.Join(base, jobName, req.RestoreJobId), nil
}

// validateSourceJobId ensures that the client-supplied source backup job id corresponds to a real
// row in the jobs table. This catches typos early, before we start walking remote_files.
func validateSourceJobId(db *sql.DB, jobName string, sourceJobId string) error {
	if sourceJobId == "" {
		return errors.New("source backup job id is required for the restore operation")
	}
	var count int
	row := db.QueryRow("SELECT count(*) FROM jobs WHERE id=? AND name=? AND type='backup'", sourceJobId, jobName)
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("while verifying source backup job id '%s': %w", sourceJobId, err)
	}
	if count != 1 {
		return fmt.Errorf("no backup job found with id '%s' for backup definition '%s'", sourceJobId, jobName)
	}
	return nil
}

type remoteItem struct {
	uuid          string
	localPath     string
	typ           string
	linkTarget    string
	size          int64
	mtime         int64
	ctime         int64
	owner         string
	permissions   string
	checksum      string
	checksumType  string
	encrypted     bool
	version       int64
	remoteVersion string
	deleteMarker  bool
}

// fetchItems returns the list of remote files associated with the source backup job id that match
// the restore request (all files, or a specific subset by local_path). When specific files are
// requested and any of them turn out to be directories, all children of those directories are
// included recursively.
func fetchItems(db *sql.DB, req Request) ([]remoteItem, error) {
	base := `SELECT rf.uuid, rf.local_path, rf.type, rf.link_target, rf.size, rf.mtime, rf.ctime,
		rf.owner, rf.permissions, rf.checksum, rf.checksum_type, rf.encrypted, rf.version,
		rf.remote_version, rf.delete_marker
		FROM remote_files rf INNER JOIN backup_collections bc ON bc.file_uuid = rf.uuid
		WHERE bc.job_id = ?`

	if req.AllFiles || len(req.Files) == 0 {
		return queryItems(db, base+" ORDER BY rf.type DESC, rf.local_path ASC", req.SourceBackupJobId)
	}

	// First pass: fetch items that match the requested paths exactly.
	placeholders := make([]string, len(req.Files))
	args := make([]interface{}, 0, len(req.Files)+1)
	args = append(args, req.SourceBackupJobId)
	for i, f := range req.Files {
		placeholders[i] = "?"
		args = append(args, f)
	}
	q := base + " AND rf.local_path IN (" + strings.Join(placeholders, ",") + ") ORDER BY rf.type DESC, rf.local_path ASC"
	exactItems, err := queryItems(db, q, args...) // #nosec - placeholders are hardcoded "?" characters
	if err != nil {
		return nil, err
	}

	// Collect directory paths so we can recursively include their children.
	var dirPaths []string
	for _, item := range exactItems {
		if item.typ == "dir" {
			dirPaths = append(dirPaths, item.localPath)
		}
	}
	if len(dirPaths) == 0 {
		return exactItems, nil
	}

	// Second pass: fetch all children of the discovered directories using prefix matching.
	// A child of "/foo/bar" is any row whose local_path starts with "/foo/bar/".
	seen := make(map[string]struct{}, len(exactItems))
	for _, item := range exactItems {
		seen[item.uuid] = struct{}{}
	}
	childArgs := make([]interface{}, 0, len(dirPaths)+1)
	childArgs = append(childArgs, req.SourceBackupJobId)
	childClauses := make([]string, len(dirPaths))
	// The escape character is '\', so any literal '\', '%', or '_' in the input must be escaped.
	// This applies to the path separator too: on Windows it is '\', and an unescaped '\' before
	// the trailing '%' would consume the wildcard and turn the pattern into a literal-percent match.
	likeEscaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	sep := likeEscaper.Replace(string(filepath.Separator))
	for i, dp := range dirPaths {
		childClauses[i] = "rf.local_path LIKE ? ESCAPE '\\'"
		childArgs = append(childArgs, likeEscaper.Replace(dp)+sep+"%")
	}
	childQuery := base + " AND (" + strings.Join(childClauses, " OR ") + ") ORDER BY rf.type DESC, rf.local_path ASC"
	childItems, err := queryItems(db, childQuery, childArgs...)
	if err != nil {
		return nil, err
	}

	// Merge, deduplicating in case an explicitly requested path was also a child of another
	// requested directory.
	for _, child := range childItems {
		if _, exists := seen[child.uuid]; !exists {
			seen[child.uuid] = struct{}{}
			exactItems = append(exactItems, child)
		}
	}
	return exactItems, nil
}

// queryItems executes a query and scans the rows into a slice of remoteItem.
func queryItems(db *sql.DB, query string, args ...interface{}) ([]remoteItem, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("while querying the database for files to restore: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logger.Warnf("error closing restore-file query rows: %s", closeErr)
		}
	}()

	results := make([]remoteItem, 0)
	for rows.Next() {
		var item remoteItem
		var encryptedInt int
		var deleteMarkerInt int
		if err := rows.Scan(&item.uuid, &item.localPath, &item.typ, &item.linkTarget, &item.size,
			&item.mtime, &item.ctime, &item.owner, &item.permissions, &item.checksum,
			&item.checksumType, &encryptedInt, &item.version, &item.remoteVersion, &deleteMarkerInt); err != nil {
			return nil, fmt.Errorf("while scanning remote_files row: %w", err)
		}
		item.encrypted = encryptedInt != 0
		item.deleteMarker = deleteMarkerInt != 0
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("while enumerating remote_files rows: %w", err)
	}
	return results, nil
}

// applyExclusions filters out items whose localPath matches any of the exclusion patterns. It uses
// the same doublestar glob matching as the backup exclusion logic (utils.IsPathExcluded) so that
// users can write the same patterns for both backup and restore exclusions.
func applyExclusions(items []remoteItem, exclusions []string) ([]remoteItem, error) {
	filtered := make([]remoteItem, 0, len(items))
	for _, item := range items {
		excluded, pattern, err := utils.IsPathExcluded(exclusions, item.localPath)
		if err != nil {
			return nil, fmt.Errorf("exclusion pattern error for path '%s': %w", item.localPath, err)
		}
		if excluded {
			logger.Debugf("excluding '%s' from restore (matched pattern '%s')", item.localPath, pattern)
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// sanitizeFilePaths strips trailing path separators from user-supplied file paths so that they
// match the way paths are stored in the database (without trailing separators). Both forward
// slashes and backslashes are stripped because the server OS may differ from the client OS that
// originally created the backup. Root paths ("/" on Unix, "X:\" on Windows) are preserved.
func sanitizeFilePaths(paths []string) []string {
	result := make([]string, len(paths))
	for i, p := range paths {
		result[i] = stripTrailingSeparators(p)
	}
	return result
}

// stripTrailingSeparators removes trailing '/' and '\' characters from a path while preserving
// root paths like "/" or "C:\".
func stripTrailingSeparators(path string) string {
	for len(path) > 0 {
		last := path[len(path)-1]
		if last != '/' && last != '\\' {
			return path
		}
		// Preserve Unix root "/"
		if path == "/" {
			return path
		}
		// Preserve Windows root like "C:\" or "c:/"
		if len(path) == 3 && path[1] == ':' {
			return path
		}
		path = path[:len(path)-1]
	}
	return path
}

// restoreOneOutcome describes how a single-item restore attempt ended. It lets the outer loop
// persist the correct per-file state and counter deltas in the restore DB without having to
// duplicate the in-memory counter logic that restoreOne already owns.
type restoreOneOutcome struct {
	state  string // "done", "failed", "cancelled"
	errMsg string
}

// restoreOne restores a single item. It continues to own the in-memory IncrementCounter calls
// (so existing watch streams keep working) and returns an outcome that the caller persists to
// the restore DB.
func restoreOne(ctx context.Context, objStore objectstore.ObjectStore, item dbops.RestoreFileRow,
	restoreDir string, jobName string, backupJobsState *shared.BackupJobsState) restoreOneOutcome {

	destPath := mapPathIntoRestoreDir(restoreDir, item.LocalPath)
	backupJobsState.UpdateStatsText(jobName, "current_file", item.LocalPath, "", "")

	switch item.Type {
	case "dir":
		if err := os.MkdirAll(destPath, 0o755); err != nil {
			logger.Warnf("could not create directory '%s' for restore: %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", err.Error())
			return restoreOneOutcome{state: "failed", errMsg: err.Error()}
		}
		backupJobsState.IncrementCounter(jobName, "restored_directories", item.LocalPath, item.Type, "restore", "")
		return restoreOneOutcome{state: "done"}
	case "symlink":
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			logger.Warnf("could not create parent directory for symlink '%s': %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", err.Error())
			return restoreOneOutcome{state: "failed", errMsg: err.Error()}
		}
		if err := os.Symlink(item.LinkTarget, destPath); err != nil && !errors.Is(err, os.ErrExist) {
			logger.Warnf("could not create symlink '%s' -> '%s': %s", destPath, item.LinkTarget, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", err.Error())
			return restoreOneOutcome{state: "failed", errMsg: err.Error()}
		}
		backupJobsState.IncrementCounter(jobName, "restored_symlinks", item.LocalPath, item.Type, "restore", "")
		return restoreOneOutcome{state: "done"}
	case "file":
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			logger.Warnf("could not create parent directory for file '%s': %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", err.Error())
			return restoreOneOutcome{state: "failed", errMsg: err.Error()}
		}
		dbRecord := shared.BackedUpFileProperties{
			Path:         item.LocalPath,
			Type:         item.Type,
			LinkTarget:   item.LinkTarget,
			Size:         item.Size,
			Owner:        item.Owner,
			Permissions:  item.Permissions,
			Checksum:     item.Checksum,
			ChecksumType: item.ChecksumType,
			Encrypted:    item.Encrypted,
		}
		// The object store Get() implementations are synchronous and do not accept a context.
		// Wrap the call in a goroutine so a cancel request returns immediately; the in-flight
		// download is deliberately allowed to leak until the HTTP client times out.
		type getResult struct {
			cancelled bool
			err       error
		}
		resCh := make(chan getResult, 1)
		go func() {
			cancelled, err := objStore.Get(dbRecord, destPath, item.Version, item.RemoteVersion, false)
			resCh <- getResult{cancelled: cancelled, err: err}
		}()
		select {
		case <-ctx.Done():
			logger.Infof("restore cancelled while downloading '%s'; allowing the in-flight download to leak", item.LocalPath)
			return restoreOneOutcome{state: "cancelled"}
		case res := <-resCh:
			if res.err != nil {
				logger.Warnf("failed to restore file '%s': %s", item.LocalPath, res.err)
				backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", res.err.Error())
				return restoreOneOutcome{state: "failed", errMsg: res.err.Error()}
			}
			if res.cancelled {
				return restoreOneOutcome{state: "cancelled"}
			}
			backupJobsState.IncrementCounter(jobName, "restored_files", item.LocalPath, item.Type, "restore", "")
			return restoreOneOutcome{state: "done"}
		}
	default:
		logger.Warnf("skipping restore of '%s' due to unknown type '%s'", item.LocalPath, item.Type)
		backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.LocalPath, item.Type, "restore", "unknown type")
		return restoreOneOutcome{state: "failed", errMsg: "unknown type"}
	}
}

// mapPathIntoRestoreDir translates an absolute source path into a destination path that lives
// under $restoreDir. The full source path is embedded so that distinct source trees do not clash.
// On Unix "/etc/hosts" becomes "<restoreDir>/etc/hosts"; on Windows "C:\foo\bar" becomes
// "<restoreDir>\C\foo\bar".
func mapPathIntoRestoreDir(restoreDir string, sourcePath string) string {
	clean := filepath.Clean(sourcePath)
	if runtime.GOOS == "windows" {
		if len(clean) >= 2 && clean[1] == ':' {
			drive := string(clean[0])
			rest := clean[2:]
			rest = strings.TrimPrefix(rest, string(filepath.Separator))
			return filepath.Join(restoreDir, drive, rest)
		}
		return filepath.Join(restoreDir, strings.TrimPrefix(clean, string(filepath.Separator)))
	}
	return filepath.Join(restoreDir, strings.TrimPrefix(clean, "/"))
}

// FinalizeJobRecord updates the jobs-table row in the per-target restore database after the
// restore finishes so that its state reflects the final outcome and a basic report is stored.
// It is the restore-side equivalent of dbops.UpdateJobDetails(..., "restore", ...).
func FinalizeJobRecord(cfg shared.CfgTemplate, jobName string, targetName string, restoreJobId string, state string,
	report string, backupJobsState *shared.BackupJobsState) error {

	cfg.Mutex.RLock()
	dataDir := cfg.DataDir
	cfg.Mutex.RUnlock()

	db, err := database.StartRestoreDb(dataDir, jobName, targetName, backupJobsState)
	if err != nil {
		return err
	}
	dbKey := database.GetRestoreDbKey(jobName, targetName)
	defer database.DisconnectFromDb(dbKey, backupJobsState, db)

	stmts, err := dbops.PrepareRestore(db)
	if err != nil {
		return err
	}
	return dbops.UpdateRestoreJobFinal(db, stmts, restoreJobId, time.Now().UnixNano(), state, report)
}
