package scan

import (
	"os"
	"path/filepath"

	"cloudbackup/config"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "backup.scan"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// Path descends into the file tree rooted at $path, calls walk() if $path is a directory and otherwise backup function
func Path(path string, backupConfig config.Backup) error {
	stat, err := os.Lstat(path)
	if backupConfig.Dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if err != nil {
		// TODO - increment a counter in order to note that errors were encountered during backup
		logger.Errorf("While trying to get properties of %s encountered error '%s'", path, err)
		return err
	} else {
		if stat.IsDir() {
			err = walk(path, stat, backupConfig)
			if err != nil {
				return err
			}
		} else {
			// TODO - add call to function dealing with backing up individual files
		}
	}
	return nil
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

// walk recursively descends path, calling walkFn.
func walk(path string, stat os.FileInfo, backupConfig config.Backup) error {
	// TODO - call to backup the folder entry itself ($stat will ge used here)
	logger.Debugf("Getting list of files and directories part of %s", path)
	names, topLevelErr := readDirNames(path)
	if topLevelErr != nil {
		logger.Warnf("While trying to get directory listing for '%s' encountered error '%s'", path, topLevelErr)
	}

	// even if $topLevelErr != nil it is possible that readDirNames() returned a partial list of directory contents
	var err error
	for _, name := range names {
		childPath := filepath.Join(path, name)
		logger.Debugf("Getting details for %s", childPath)
		// TODO - add function to check if this path is excluded from backup
		var fileInfo os.FileInfo
		if backupConfig.Dereference {
			fileInfo, err = os.Stat(childPath)
		} else {
			fileInfo, err = os.Lstat(childPath)
		}
		if err != nil {
			logger.Warnf("While trying to get properties of %s encountered error '%s'", childPath, err)
		} else {
			if fileInfo.IsDir() {
				_ = walk(childPath, fileInfo, backupConfig) // #nosec
			} else {
				// TODO - add call to function dealing with backing up individual files
			}
		}
	}
	return topLevelErr
}
