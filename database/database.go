package database

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/utils"
	_ "github.com/mattn/go-sqlite3"
	"database/sql"
	"errors"
	"path/filepath"
)

const loggingContext = "database"
var ErrCouldNotCreateDB = errors.New("could not create database")
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func CreateDb(dbfilepath string) error {

	logger.Debugf("Opening database file '%s' used to store details for backup job '%s' and proceeding to " +
		"create tables")
	db, err := sql.Open("sqlite3", dbfilepath)
	if err != nil {
		logger.Errorf("Could not create database %s due to error: %s", dbfilepath, err)
		return ErrCouldNotCreateDB
	}
	defer func() {
		err := db.Close()
		if err != nil {
			logger.Warnf("While trying to close() the database '%s' the following error was encounterd: %s",
				dbfilepath, err)
		}
	}()

	sqlStmt := `
	CREATE TABLE files (path STRING NOT NULL PRIMARY KEY, type TEXT, link_target TEXT, size INTEGER, mtime TEXT, 
	ctime TEXT, uid TEXT, gid TEXT, perm_mode TEXT, checksum TEXT, checksum_type, encrypted INTEGER, targets_ids TEXT);
	`
	logger.Debugf("Creating tables")
	_, err = db.Exec(sqlStmt)
	if err != nil {
		logger.Errorf("Encountered error while attempting to create database '%s' . The error is: %s",
			dbfilepath, err)
		return err
	}

	return nil
}

// figure out the absolute path to the database file
func GetDbFilePath(datadir string, backupName string) (string, error){
	dbfilepath, err := filepath.Abs(datadir + string(filepath.Separator) + backupName + ".sqlite")
	if err != nil {
		logger.Errorf("While trying to establish the absolute path to the database holding information about " +
			" backup job '%s' the following error was encountered: %s", backupName, err)
		return "", errors.New("could not establish the absolute path to the database file")
	} else {
		return dbfilepath, nil
	}
}

func DbFileExists(dbfilepath string) bool {
	_, err := utils.FileExists(dbfilepath, true)
	if err != nil {
		if err != utils.ErrNoSuchFile {
			logger.Errorf("When attempting to read the properties for the database file '%s' the following " +
				"error was received: ", err)
		}
		return false
	} else {
		return true
	}
}

// if a db does not exist then it attempts to create one
// Parameters: "datadir" value from the config file; "backupName" is the name of the backup job; "configInit" if this
// is called during program startup/config reload and it encounters an error then log a specific error message which is
// different if this function is called during backup start
func ValidateAndCreate(datadir string, backupName string, configInit bool) error {
	// figure out name and absolute path to db file
	dbfilepath, err := GetDbFilePath(datadir, backupName)
	if err != nil {
		return err
	}
	// check if database file exists
	if ! DbFileExists(dbfilepath) {
		if configInit{
			logger.Warnf("Database file '%s' used to store details for backup job '%s' doesn't exist. This is " +
				"expected during the first start of the backup server or when configuration file changes affect the" +
				" 'data_dir' option or add a new backup job. If this is not the case then the root cause for the " +
				"missing database file should be established. Proceeding now to create the database.",
				dbfilepath, backupName)
		} else {
			logger.Errorf("Database file '%s' used to store details for backup job '%s' doesn't exist. This is " +
				"unexpected as the database file should already exist so the root cause for this issue should be " +
				"established. Creating the database now in order to proceed with the backup.",
				dbfilepath, backupName)
		}
		err2 := CreateDb(dbfilepath)
		if err2 != nil {
			logger.Errorf("Backups for job '%s' are not possible as the database file can't be created and " +
				"initialised",
				backupName)
			return err2
		}
	}
	return nil
}

// Take care of starting the db connection;
// Parameters: "datadir" value from the config file and the name of the backup job
func Start(datadir string, backupName string) error {
	err := ValidateAndCreate(datadir, backupName, false)
	if err != nil {
		return err
	}

	// TODO - connect to DB

	return nil
}