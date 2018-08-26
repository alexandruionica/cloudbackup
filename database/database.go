package database

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/utils"
	_ "github.com/mattn/go-sqlite3"
	"database/sql"
	"errors"
	"path/filepath"
	"os"
)

const loggingContext = "database"
const DbOptions = "_foreign_keys=1"
var ErrCouldNotCreateDB = errors.New("could not create database")
var ErrCouldNotOpenDB = errors.New("could not open database")
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func CreateDb(dbfilepath string) error {
	db, err := OpenDb(dbfilepath)
	if err != nil {
		return ErrCouldNotCreateDB
	}

	sqlStmt := `
	CREATE TABLE files (path TEXT NOT NULL PRIMARY KEY, type TEXT, link_target TEXT, size INTEGER, mtime TEXT, 
	ctime TEXT, uid TEXT, gid TEXT, perm_mode TEXT, checksum TEXT, checksum_type, encrypted INTEGER, targets_ids TEXT);

	CREATE TABLE targets (id INTEGER NOT NULL PRIMARY KEY, backup_id TEXT, backup TEXT, name TEXT, type TEXT);

	CREATE TABLE jobs (id TEXT NOT NULL PRIMARY KEY, type TEXT, start_time TEXT, end_time TEXT, state TEXT, 
	processed_files INTEGER, processed_dirs INTEGER);

	CREATE TABLE remote_files (uuid NOT NULL PRIMARY KEY, remote_path TEXT, local_path TEXT, target_id INTEGER, 
	upload_date TEXT, job_id TEXT, current INTEGER , delete_marker INTEGER, version TEXT, type TEXT, 
	link_target TEXT, size INTEGER, mtime TEXT, ctime TEXT, uid TEXT, gid TEXT, perm_mode TEXT, checksum TEXT, 
	checksum_type, encrypted INTEGER, targets_ids TEXT, 
	FOREIGN KEY(target_id) REFERENCES targets(id), FOREIGN KEY(job_id) REFERENCES jobs(id));
	
	CREATE INDEX remote_files_job_id ON remote_files(job_id);
	CREATE INDEX remote_files_local_path ON remote_files(local_path);

	CREATE TABLE backup_collections (file_uuid TEXT, job_id TEXT, FOREIGN KEY(file_uuid) REFERENCES remote_files(uuid), 
	FOREIGN KEY(job_id) REFERENCES jobs(id));
	
	CREATE INDEX backup_collections_jobid ON backup_collections(job_id);

	`
	logger.Debugf("Creating tables in '%s' database", dbfilepath)
	_, err = db.Exec(sqlStmt)
	if err != nil {
		logger.Errorf("Encountered error while attempting to create database '%s' . The error is: %s",
			dbfilepath, err)
		logger.Debugf("Closing '%s' and removing the file", dbfilepath)
		// close connection to the db
		CloseDb(db, dbfilepath)
		// remove the incorrectly initialised db file
		if DbFileExists(dbfilepath) {
			err2 := os.Remove(dbfilepath)
			if err2 != nil {
				logger.Errorf("An additional error was encountered when trying to remove the incorrectly " +
					"initialised db file '%s'. The error was: %s", dbfilepath, err2)
			}
		}
		return err
	} else {
		// close connection to the db
		CloseDb(db, dbfilepath)
	}

	return nil
}

// opens a connection to the DB and if successful, it returns the *sql.DB
func OpenDb(dbfilepath string) (*sql.DB, error) {
	logger.Debugf("Opening database file '%s'")
	db, err := sql.Open("sqlite3", dbfilepath + "?" + DbOptions)
	if err != nil {
		logger.Errorf("Could not open database %s due to error: %s", dbfilepath, err)
		return &sql.DB{}, ErrCouldNotOpenDB
	}
	return db, nil
}

// close a database; the "name" parameter will generally be the name of the backup job or the full path to the DB file
func CloseDb(db *sql.DB, name string) {
	err := db.Close()
	if err != nil {
		logger.Errorf("While trying to close database '%s' encountered error: '%s'", name, err )
	}
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
			logger.Errorf("Backups for job '%s' are not possible as the database file can't be created or " +
				"initialised",
				backupName)
			return err2
		}
	}
	return nil
}

// Take care of starting the db connection;
// Parameters: "datadir" value from the config file and the name of the backup job
func Start(datadir string, backupName string) (*sql.DB, error) {
	// check if DB exists, if not then create it
	err := ValidateAndCreate(datadir, backupName, false)
	if err != nil {
		return &sql.DB{}, err
	}

	dbfilepath, err := GetDbFilePath(datadir, backupName)
	if err != nil {
		return &sql.DB{}, err
	}

	db, err := OpenDb(dbfilepath)
	if err != nil {
		return &sql.DB{}, err
	}

	return db, nil
}