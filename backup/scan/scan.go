package scan

import (
	"cloudbackup/backup"
	"cloudbackup/objectstore"
	"os"
	"path/filepath"

	"cloudbackup/config"
	"cloudbackup/daemon/globals"
	"cloudbackup/shared"
	"context"
	"github.com/bmatcuk/doublestar"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "backup.scan"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// Path descends into the file tree rooted at $path, calls walk() if $path is a directory and otherwise backup function
// return: first parameter will be "true" only if it was signalled via closeChan to stop the running backup; second
// parameter signifies errors
func Path(ctx context.Context, path string, backupConfig config.Backup, backupJobsState shared.BackupJobsStateInterface,
	dryRun bool, dbData shared.DbData, objectStores []objectstore.ObjectStore) (bool, error) {
	globals.Stats.IncrementFunctions("scan.Path")
	defer globals.Stats.DecrementFunctions("scan.Path")

	var stat os.FileInfo
	var err error
	if backupConfig.Dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if err != nil {
		logger.Errorf("While trying to get properties of %s encountered error '%s'", path, err)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine")
		return false, err
	} else {
		select {
		case <-ctx.Done():
			{
				logger.Infof("cancelling running backup job '%s'", backupConfig.Name)
				return true, nil
			}
		default:
			if stat.IsDir() {
				exiting, err := walk(ctx, path, stat, backupConfig, backupJobsState, dryRun, dbData, objectStores)
				// backup job was signalled to exit - Examine FIRST $exiting and then $err
				if exiting {
					return true, err
				}
				if err != nil {
					return false, err
				}
			} else {
				backupJobsState.IncrementCounter(backupConfig.Name, "examined_files")
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", path, "", "")
				if ! dryRun {
					// call to function dealing with backing up individual files
					cancelled, err := backup.Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState)
					if cancelled{
						return true, nil
					}
					if err != nil {
						return false, err
					}
				}

			}
		}
	}
	// set to empty examined directory and file stats as we've completed the "run"
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", "", "", "")
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "", "", "")
	return false, nil
}

// readDirNames reads the directory named by dirname and returns
// a list of directory entries.
func readDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname) // #nosec
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	err2 := f.Close()
	if err2 != nil {
		logger.Warnf("Could not close descriptor after reading properties of directory %s", dirname)
	}
	if err != nil {
		return nil, err
	}
	return names, nil
}

// walk recursively path
// parameters: ctx is context for cancellation of the walk(), the 'db' parameters is the DB pointer used for sql ops
// return: first value will be "true" only if it was signalled via the ctx context to stop the running backup;
// second value signifies errors
func walk(ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup,
	backupJobsState shared.BackupJobsStateInterface, dryRun bool, dbData shared.DbData,
	objectStores []objectstore.ObjectStore) (bool, error) {
	if ! dryRun {
		// call to backup the folder entry itself
		cancelled, err := backup.Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState)
		if cancelled{
			return true, nil
		}
		if err != nil {
			return false, err
		}
		// if no error was reported and no cancellation was reported either then we continue to process all files and
		// folders part of the just "backed up" directory
	}

	// set current file examined to empty as otherwise output will look inconsistent if we descend a different folder
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "", "", "")
	backupJobsState.IncrementCounter(backupConfig.Name, "examined_directories")
	logger.Debugf("Getting list of files and directories part of %s", path)
	names, topLevelErr := readDirNames(path)
	if topLevelErr != nil {
		logger.Warnf("While trying to get directory listing for '%s' encountered error '%s'", path, topLevelErr)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine")
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", path,
			"", topLevelErr.Error())
	} else {
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", path,
			"", "")
	}

	// even if $topLevelErr != nil it is possible that readDirNames() returned a partial list of directory contents
	for _, name := range names {
		select {
		case <-ctx.Done():
			{
				logger.Infof("while processing '%s' received request to cancel running backup job '%s'",
					path, backupConfig.Name)
				return true, topLevelErr
			}
		default:
			childPath := filepath.Join(path, name)
			logger.Debugf("Getting details for %s", childPath)
			excluded, excludedExpr, err := isExcluded(backupConfig.Exclusions, childPath)
			if err != nil {
				logger.Warnf("While trying to check if %s should be excluded from being backed up, the following " +
					"error was encountered '%s'", childPath, err)
				backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine")
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown", childPath, "",
					err.Error())
				continue
			}
			if excluded {
				logger.Debugf("Skipping from backup %s as it is excluded by expression %s", childPath, excludedExpr)
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown",
					childPath, excludedExpr, "")
				backupJobsState.IncrementCounter(backupConfig.Name, "excluded")
				continue
			}

			var fileInfo os.FileInfo
			if backupConfig.Dereference {
				fileInfo, err = os.Stat(childPath)
			} else {
				fileInfo, err = os.Lstat(childPath)
			}
			if err != nil {
				logger.Warnf("While trying to get properties of %s encountered error '%s'", childPath, err)
				backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine")
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown", childPath,
					"", err.Error())
			} else {
				if fileInfo.IsDir() {
					exiting, _ := walk(ctx, childPath, fileInfo, backupConfig, backupJobsState, dryRun, dbData,
						objectStores) // #nosec
					// lower level walk() was signalled to exit
					if exiting {
						return true, topLevelErr
					}
				} else {
					backupJobsState.IncrementCounter(backupConfig.Name, "examined_files")
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", childPath,
						"","")
					if ! dryRun {
						// call to function dealing with backing up of files
						cancelled, err := backup.Do(ctx, childPath, fileInfo, backupConfig, dbData, objectStores, backupJobsState)
						if cancelled{
							return true, nil
						}
						if err != nil {
							return false, err
						}
					}
					// mark current examined file as none as we don't know if the next iteration of the main for loop
					//  will next encounter a directory or not
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "",
						"", "")
				}
			}
		}
	}
	return false, topLevelErr
}

// check if $path is matches any of the Globstar elements of the $exclusions array. If a match is found then true
// is returned followed also by the exclusion rule which matched and nil; if an error is encountered then the last
// element will be the error message
func isExcluded(exclusions []string, path string) (bool, string, error){
	for _, excludedPath := range exclusions {
		match, err := doublestar.PathMatch(excludedPath, path)
		if err != nil {
			return false, "", err
		}
		if match {
			return true, excludedPath, nil
		}
	}
	return false, "", nil
}