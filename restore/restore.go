package restore

import (
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"database/sql"
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
}

// Do runs a restore job end to end. It opens the backup's SQLite database, resolves the list of
// files to restore, downloads each file into RestoredDirectory, and updates counters in
// backupJobsState as it goes. It MUST be invoked after backupJobsState.MarkRestoreRunning has been
// called for (jobName, restoreJobId) so that ctx/cancel/counters exist in Running[].
func Do(jobName string, req Request, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) Result {

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

	// Only the chosen target is initialised — no reason to spin up connections to stores we are not
	// going to read from for this restore.
	singleTargetConfig := shared.CopyConfigBackupStruct(backupConfig)
	singleTargetConfig.Target = []shared.ConfigBackupTarget{target}
	objectStores, err := objectstore.GetObjectStores(ctx, singleTargetConfig, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not initialise the object store for target '%s': %w", target.Name, err)}
	}
	if len(objectStores) != 1 {
		return Result{State: "failed", Err: fmt.Errorf("expected exactly one object store for target '%s' but got %d", target.Name, len(objectStores))}
	}
	objStore := objectStores[0]

	restoreDir, err := resolveRestoreDir(req, serverConfigCopy, jobName)
	if err != nil {
		return Result{State: "failed", Err: err}
	}
	if err := os.MkdirAll(restoreDir, 0o755); err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not create restore directory '%s': %w", restoreDir, err)}
	}

	db, err := database.Start(serverConfigCopy.DataDir, jobName, backupJobsState)
	if err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not open database for backup definition '%s': %w", jobName, err), RestoredDirectory: restoreDir}
	}
	defer database.DisconnectFromDb(jobName, backupJobsState, db)

	if err := validateSourceJobId(db, jobName, req.SourceBackupJobId); err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir}
	}

	jobStartTime, err := backupJobsState.GetStartTime(jobName, req.RestoreJobId, loggingContext+".Do")
	if err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir}
	}
	if err := dbops.AddJobDetails(db, req.RestoreJobId, jobName, "restore", jobStartTime); err != nil {
		return Result{State: "failed", Err: fmt.Errorf("could not add restore job record to database: %w", err), RestoredDirectory: restoreDir}
	}

	items, err := fetchItems(db, req)
	if err != nil {
		return Result{State: "failed", Err: err, RestoredDirectory: restoreDir}
	}

	if len(req.Exclusions) > 0 {
		items, err = applyExclusions(items, req.Exclusions)
		if err != nil {
			return Result{State: "failed", Err: fmt.Errorf("error applying exclusions: %w", err), RestoredDirectory: restoreDir}
		}
	}

	if len(items) == 0 {
		return Result{State: "failed", Err: errors.New("no files matched the restore request"), RestoredDirectory: restoreDir}
	}

	backupJobsState.UpdateStatsText(jobName, "current_operation", "Restoring files", "", "")

	var restoredAny bool
	for _, item := range items {
		select {
		case <-ctx.Done():
			return Result{State: "cancelled", RestoredDirectory: restoreDir}
		default:
		}

		if item.deleteMarker {
			backupJobsState.IncrementCounter(jobName, "skipped_delete_markers", item.localPath, item.typ, "restore", "")
			continue
		}

		if restoreOne(ctx, objStore, item, restoreDir, jobName, backupJobsState) {
			restoredAny = true
		}
	}

	backupJobsState.UpdateStatsText(jobName, "current_operation", "", "", "")

	if ctx.Err() == context.Canceled {
		return Result{State: "cancelled", RestoredDirectory: restoreDir}
	}
	if !restoredAny {
		return Result{State: "failed", Err: errors.New("all files failed to restore"), RestoredDirectory: restoreDir}
	}
	return Result{State: "finished", RestoredDirectory: restoreDir}
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
	for i, dp := range dirPaths {
		childClauses[i] = "rf.local_path LIKE ? ESCAPE '\\'"
		// Escape any existing '%', '_', or '\' in the directory path so they are matched literally.
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(dp)
		childArgs = append(childArgs, escaped+string(filepath.Separator)+"%")
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

// restoreOne restores a single item and returns true on success. All errors are recorded into
// the restore job counters; failures do not abort the whole restore (per the "per-file failure"
// design decision).
func restoreOne(ctx context.Context, objStore objectstore.ObjectStore, item remoteItem,
	restoreDir string, jobName string, backupJobsState *shared.BackupJobsState) bool {

	destPath := mapPathIntoRestoreDir(restoreDir, item.localPath)
	backupJobsState.UpdateStatsText(jobName, "current_file", item.localPath, "", "")

	switch item.typ {
	case "dir":
		if err := os.MkdirAll(destPath, 0o755); err != nil {
			logger.Warnf("could not create directory '%s' for restore: %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", err.Error())
			return false
		}
		backupJobsState.IncrementCounter(jobName, "restored_directories", item.localPath, item.typ, "restore", "")
		return true
	case "symlink":
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			logger.Warnf("could not create parent directory for symlink '%s': %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", err.Error())
			return false
		}
		if err := os.Symlink(item.linkTarget, destPath); err != nil && !errors.Is(err, os.ErrExist) {
			logger.Warnf("could not create symlink '%s' -> '%s': %s", destPath, item.linkTarget, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", err.Error())
			return false
		}
		backupJobsState.IncrementCounter(jobName, "restored_symlinks", item.localPath, item.typ, "restore", "")
		return true
	case "file":
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			logger.Warnf("could not create parent directory for file '%s': %s", destPath, err)
			backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", err.Error())
			return false
		}
		dbRecord := shared.BackedUpFileProperties{
			Path:         item.localPath,
			Type:         item.typ,
			LinkTarget:   item.linkTarget,
			Size:         item.size,
			Owner:        item.owner,
			Permissions:  item.permissions,
			Checksum:     item.checksum,
			ChecksumType: item.checksumType,
			Encrypted:    item.encrypted,
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
			cancelled, err := objStore.Get(dbRecord, destPath, item.version, item.remoteVersion, false)
			resCh <- getResult{cancelled: cancelled, err: err}
		}()
		select {
		case <-ctx.Done():
			logger.Infof("restore cancelled while downloading '%s'; allowing the in-flight download to leak", item.localPath)
			return false
		case res := <-resCh:
			if res.err != nil {
				logger.Warnf("failed to restore file '%s': %s", item.localPath, res.err)
				backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", res.err.Error())
				return false
			}
			if res.cancelled {
				return false
			}
			backupJobsState.IncrementCounter(jobName, "restored_files", item.localPath, item.typ, "restore", "")
			return true
		}
	default:
		logger.Warnf("skipping restore of '%s' due to unknown type '%s'", item.localPath, item.typ)
		backupJobsState.IncrementCounter(jobName, "failed_to_restore_files", item.localPath, item.typ, "restore", "unknown type")
		return false
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

// FinalizeJobRecord updates the jobs table row after the restore finishes so that its state
// reflects the final outcome and a basic report is stored.
func FinalizeJobRecord(cfg shared.CfgTemplate, jobName string, restoreJobId string, state string, report string,
	backupJobsState *shared.BackupJobsState) error {
	db, err := database.Start(cfg.DataDir, jobName, backupJobsState)
	if err != nil {
		return err
	}
	defer database.DisconnectFromDb(jobName, backupJobsState, db)
	return dbops.UpdateJobDetails(db, restoreJobId, jobName, "restore", time.Now(), state, report)
}
