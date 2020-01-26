package scan

import (
	"cloudbackup/backup"
	"cloudbackup/daemon/globals"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

const loggingContext = "backup.scan"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// Path descends into the file tree rooted at $path, calls walk() if $path is a directory and otherwise backup function
// return: first parameter will be "true" only if it was signalled via closeChan to stop the running backup; second
// parameter signifies errors
func Path(ctx context.Context, path string, backupConfig shared.ConfigBackup, backupJobsState shared.BackupJobsStateInterface,
	dryRun bool, dbData shared.DbData, objectStores []objectstore.ObjectStore, jobUuid string) (bool, error) {
	globals.Stats.IncrementFunctions("scan.Path")
	defer globals.Stats.DecrementFunctions("scan.Path")

	var stat os.FileInfo
	var err error
	backupJobsState.IncrementSequence(backupConfig.Name)
	if backupConfig.Dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if err != nil {
		logger.Errorf("While trying to get properties of %s encountered error '%s'", path, err)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine", path, "unknown",
			"examine", err.Error())
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
				err := AddTopLevelJobPath(dbData, jobUuid, path, "dir")
				if err != nil {
					return false, err
				}
				exiting, err := walk(ctx, path, stat, backupConfig, backupJobsState, dryRun, dbData, objectStores, jobUuid)
				// backup job was signalled to exit - Examine FIRST $exiting and then $err
				if exiting {
					return true, err
				}
				if err != nil {
					logger.Warnf("While backing up the contents of directory %s the following error was "+
						"encountered: %s", path, err)
				}
			} else {
				var fileType string
				switch utils.FileType(stat) {
				case "file":
					{
						backupJobsState.IncrementCounter(backupConfig.Name, "examined_files", path,
							"file", "examine", "")
						fileType = "file"
					}
				case "symlink":
					{
						backupJobsState.IncrementCounter(backupConfig.Name, "examined_symlinks", path,
							"symlink", "examine", "")
						fileType = "symlink"
					}
				default:
					{
						backupJobsState.IncrementCounter(backupConfig.Name, "examined_unknown", path,
							"unknown", "examine", "")
						fileType = "unknown"
					}
				}
				err = AddTopLevelJobPath(dbData, jobUuid, path, fileType)
				if err != nil {
					return false, err
				}
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", path, "", "")
				if !dryRun {
					// call to function dealing with backing up individual files
					cancelled, err := backup.Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid)
					if cancelled {
						return true, nil
					}
					if err != nil {
						logger.Warnf("While trying to backup '%s' the following error was encountered: %s", path, err)
						backup.MarkItemAsFailed(path, fileType, jobUuid, dbData)
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
func walk(ctx context.Context, path string, stat os.FileInfo, backupConfig shared.ConfigBackup,
	backupJobsState shared.BackupJobsStateInterface, dryRun bool, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, jobUuid string) (bool, error) {
	if !dryRun {
		// call to backup the folder entry itself (this can lead to a scenario where the directory entry itself is
		// backed up but when attempting to walk the directory an error could be triggered)
		cancelled, err := backup.Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid)
		if cancelled {
			return true, nil
		}
		if err != nil {
			logger.Warnf("While trying to backup '%s' the following error was encountered: %s", path, err)
			backup.MarkItemAsFailed(path, "dir", jobUuid, dbData)
		}
		// if no error was reported and no cancellation was reported either then we continue to process all files and
		// folders part of the just "backed up" directory
	}

	// set current file examined to empty as otherwise output will look inconsistent if we descend a different folder
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "", "", "")
	backupJobsState.IncrementCounter(backupConfig.Name, "examined_directories", path, "dir",
		"examine", "")
	logger.Debugf("Getting list of files and directories part of %s", path)
	names, topLevelErr := readDirNames(path)
	if topLevelErr != nil {
		logger.Warnf("While trying to get directory listing for '%s' encountered error '%s'", path, topLevelErr)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_enumerate", path, "dir",
			"enumerate", topLevelErr.Error())
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
				return true, nil
			}
		default:
			backupJobsState.IncrementSequence(backupConfig.Name)
			childPath := filepath.Join(path, name)
			logger.Debugf("Getting details for %s", childPath)
			excluded, excludedExpr, err := utils.IsPathExcluded(backupConfig.Exclusions, childPath)
			if err != nil {
				logger.Warnf("While trying to check if %s should be excluded from being backed up, the following "+
					"error was encountered '%s'", childPath, err)
				backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine", childPath,
					"unknown", "examine", err.Error())
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown", childPath, "",
					err.Error())
				continue
			}
			if excluded {
				logger.Debugf("Skipping from backup %s as it is excluded by expression %s", childPath, excludedExpr)
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown",
					childPath, excludedExpr, "")
				backupJobsState.IncrementCounter(backupConfig.Name, "excluded", childPath,
					"unknown", "excluded", "")
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
				backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_examine", childPath,
					"unknown", "examine", err.Error())
				backupJobsState.UpdateStatsText(backupConfig.Name, "unknown", childPath,
					"", err.Error())
			} else {
				if fileInfo.IsDir() {
					exiting, _ := walk(ctx, childPath, fileInfo, backupConfig, backupJobsState, dryRun, dbData,
						objectStores, jobUuid) // #nosec
					// lower level walk() was signalled to exit
					if exiting {
						return true, nil
					}
					// set current directory back to what we're in right now as the above walk() has set it to $childPath
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", path,
						"", "")
				} else {
					var fileType string
					switch utils.FileType(fileInfo) {
					case "file":
						{
							backupJobsState.IncrementCounter(backupConfig.Name, "examined_files", childPath,
								"file", "examine", "")
							fileType = "file"
						}
					case "symlink":
						{
							backupJobsState.IncrementCounter(backupConfig.Name, "examined_symlinks", childPath,
								"symlink", "examine", "")
							fileType = "symlink"
						}
					default:
						{
							backupJobsState.IncrementCounter(backupConfig.Name, "examined_unknown", childPath,
								"unknown", "examine", "")
							fileType = "unknown"
						}
					}
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", childPath,
						"", "")
					if !dryRun {
						// call to function dealing with backing up of files
						cancelled, err := backup.Do(ctx, childPath, fileInfo, backupConfig, dbData, objectStores, backupJobsState, jobUuid)
						if cancelled {
							return true, nil
						}
						if err != nil {
							logger.Warnf("While trying to backup '%s' the following error was encountered: %s", childPath, err)
							backup.MarkItemAsFailed(childPath, fileType, jobUuid, dbData)
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
	return false, nil
}

// adds a new record in the "top_items" table which represents a top level path (meaning which was mentioned in the
// config file in the "paths" section of a backup job) for a given backup job run
func AddTopLevelJobPath(dbData shared.DbData, jobId string, path string, fileType string) error {
	_, err := dbData.Db.Exec(dbData.PreparedStatements.TopItemsInsert, jobId, path, fileType)
	if err != nil {
		logger.Errorf("While trying to add information about path %s to the "+
			"database, the following error was encountered: '%s'", path, err)
		return err
	}
	return nil
}
