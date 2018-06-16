package scan

import (
	"os"
	"path/filepath"

	"cloudbackup/config"
	log "github.com/sirupsen/logrus"
	"cloudbackup/shared"
	"github.com/bmatcuk/doublestar"
)

const loggingContext = "backup.scan"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// Path descends into the file tree rooted at $path, calls walk() if $path is a directory and otherwise backup function
// return: first parameter will be "true" only if it was signalled via closeChan to stop the running backup; second
// parameter signifies errors
func Path(path string, backupConfig config.Backup, backupJobsState *shared.BackupJobsState,
	closeChan chan bool) (bool, error) {
	var stat os.FileInfo
	var err error
	if backupConfig.Dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if err != nil {
		logger.Errorf("While trying to get properties of %s encountered error '%s'", path, err)
		backupJobsState.IncrementCounter(backupConfig.Name, "examine_produced_errors")
		return false, err
	} else {
		select {
		case <-closeChan:
			{
				logger.Infof("cancelling running backup job '%s'", backupConfig.Name)
				return true, nil
			}
		default:
			if stat.IsDir() {
				backupJobsState.IncrementCounter(backupConfig.Name, "examined_directories")
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", path)
				exiting, err := walk(path, stat, backupConfig, backupJobsState, closeChan)
				// backup job was signalled to exit - Examine FIRST $exiting and then $err
				if exiting {
					return true, err
				}
				if err != nil {
					return false, err
				}
			} else {
				backupJobsState.IncrementCounter(backupConfig.Name, "examined_files")
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", path)
				// TODO - add call to function dealing with backing up individual files
			}
		}
	}
	// set to empty examined directory and file stats as we've completed the "run"
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", "")
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "")
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
// return: first parameter will be "true" only if it was signalled via closeChan to stop the running backup; second
// parameter signifies errors
func walk(path string, stat os.FileInfo, backupConfig config.Backup, backupJobsState *shared.BackupJobsState,
	closeChan chan bool) (bool, error) {
	// TODO - call to backup the folder entry itself ($stat will ge used here)

	// set current file examined to empty as otherwise output will look inconsistent if we descend a different folder
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "")
	logger.Debugf("Getting list of files and directories part of %s", path)
	names, topLevelErr := readDirNames(path)
	if topLevelErr != nil {
		logger.Warnf("While trying to get directory listing for '%s' encountered error '%s'", path, topLevelErr)
		backupJobsState.IncrementCounter(backupConfig.Name, "examine_produced_errors")
	}

	// even if $topLevelErr != nil it is possible that readDirNames() returned a partial list of directory contents
	for _, name := range names {

		select {
		case <-closeChan:
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
				backupJobsState.IncrementCounter(backupConfig.Name, "examine_produced_errors")
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "")
				continue
			}
			if excluded {
				logger.Debugf("Skipping from backup %s as it is excluded by expression %s", childPath, excludedExpr)
				backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "")
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
				backupJobsState.IncrementCounter(backupConfig.Name, "examine_produced_errors")
			} else {
				if fileInfo.IsDir() {
					backupJobsState.IncrementCounter(backupConfig.Name, "examined_directories")
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_directory", path)
					exiting, _ := walk(childPath, fileInfo, backupConfig, backupJobsState, closeChan) // #nosec
					// lower level walk() was signalled to exit
					if exiting {
						return true, topLevelErr
					}
				} else {
					backupJobsState.IncrementCounter(backupConfig.Name, "examined_files")
					backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", path)
					// TODO - add call to function dealing with backing up individual files

					backupJobsState.UpdateStatsText(backupConfig.Name, "current_file", "")
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