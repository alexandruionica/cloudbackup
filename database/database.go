package database

import (
	"cloudbackup/shared"
	"cloudbackup/utils"
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

const loggingContext = "database"

// cache=shared - according to https://www.sqlite.org/sharedcache.html this improves performance
// _foreign_keys=1 - enable foreign keys support and enforcement
// _journal_mode=WAL - use Write-Ahead Logging as it's more performant  - should be safe and faster - https://www.sqlite.org/pragma.html#pragma_journal_mode
// synchronous=NORMAL - WAL mode is safe from corruption with synchronous=NORMAL - https://www.sqlite.org/pragma.html#pragma_synchronous
// both of the above "_journal_mode" and "synchronous" ensure good performance from Sqlite. Removing those settings will
// decrease performance by 8 times to roughly 100 transactions/sec for our usage cases
const DbOptions = "_foreign_keys=1&cache=shared&_journal_mode=WAL&_synchronous=NORMAL"

var ErrCouldNotCreateDB = errors.New("could not create database")
var ErrCouldNotOpenDB = errors.New("could not open database")
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func CreateDb(db *sql.DB, dbfilepath string) error {

	sqlStmt := `
	CREATE TABLE files (path TEXT NOT NULL PRIMARY KEY, type TEXT, link_target TEXT, size INTEGER, mtime INTEGER, 
	ctime INTEGER, owner TEXT, permissions TEXT, checksum TEXT, checksum_type, encrypted INTEGER, job_id TEXT,
	FOREIGN KEY(job_id) REFERENCES jobs(id));
	
	CREATE INDEX files_job_id ON files(job_id);

	CREATE TABLE targets (name TEXT NOT NULL PRIMARY KEY, backup_name TEXT, type TEXT, bucket TEXT, prefix TEXT, date_added INTEGER);

	CREATE TABLE jobs (id TEXT NOT NULL PRIMARY KEY, name TEXT, type TEXT, start_time INTEGER, end_time INTEGER, state TEXT, 
	report TEXT);

	CREATE TABLE remote_files (uuid NOT NULL PRIMARY KEY, local_path TEXT, target TEXT, 
	upload_date INTEGER, job_id TEXT, delete_marker INTEGER, version INTEGER, remote_version TEXT, src_os TEXT, type TEXT, 
	link_target TEXT, size INTEGER, mtime INTEGER, ctime INTEGER, owner TEXT, permissions TEXT, checksum TEXT, 
	checksum_type, encrypted INTEGER,
	FOREIGN KEY(target) REFERENCES targets(name), FOREIGN KEY(job_id) REFERENCES jobs(id));
	
	CREATE INDEX remote_files_job_id ON remote_files(job_id);
	CREATE INDEX remote_files_local_path ON remote_files(local_path);
	CREATE INDEX remote_files_upload_date ON remote_files(upload_date);

	CREATE TABLE backup_collections (file_uuid TEXT, job_id TEXT, target TEST, FOREIGN KEY(file_uuid) REFERENCES remote_files(uuid), 
	FOREIGN KEY(job_id) REFERENCES jobs(id), FOREIGN KEY(target) REFERENCES targets(name));
	
	CREATE INDEX backup_collections_jobid ON backup_collections(job_id);

	`
	logger.Debugf("Creating tables in '%s' database", dbfilepath)
	_, err := db.Exec(sqlStmt)
	if err != nil {
		logger.Errorf("Encountered error while attempting to create database '%s' . The error is: %s",
			dbfilepath, err)
		logger.Debugf("Closing '%s' and removing the file", dbfilepath)
		// close connection to the db
		err = db.Close()
		if err != nil {
			logger.Errorf("Could not close database '%s' due to error: %s", dbfilepath, err)
		}
		fExists, _ := DbFileExists(dbfilepath) // #nosec
		// remove the incorrectly initialised db file
		if fExists {
			err2 := os.Remove(dbfilepath)
			if err2 != nil {
				logger.Errorf("An additional error was encountered when trying to remove the incorrectly "+
					"initialised db file '%s'. The error was: %s", dbfilepath, err2)
			}
		}
		return err
	} else {
		err = db.Close()
		if err != nil {
			logger.Errorf("Could not close database '%s' due to error: %s", dbfilepath, err)
		}
	}

	return nil
}

// opens a connection to the DB and if successful, it returns the *sql.DB
// params: $datadir is the folder containing the database file; $backupName is the name of the backup (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// if $fileExists == false then don't attempt to "ping" the DB as it will error;
// $backupJobsState is used to signal that a given database is opened (so it should not be attempted to copy the DB)
func OpenDb(datadir string, backupName string, fileExists bool, backupJobsState *shared.BackupJobsState) (*sql.DB, error) {
	backupJobsState.MarkOpenDb(backupName)
	dbfilepath, err := GetDbFilePath(datadir, backupName)
	if err != nil {
		backupJobsState.UnMarkOpenDb(backupName)
		return &sql.DB{}, err
	}

	logger.Debugf("Opening database file '%s'", dbfilepath)
	db, err := sql.Open("sqlite3", dbfilepath+"?"+DbOptions)
	if err != nil {
		backupJobsState.UnMarkOpenDb(backupName)
		logger.Errorf("Could not open database %s due to error: %s", dbfilepath, err)
		return &sql.DB{}, ErrCouldNotOpenDB
	}
	/* according to https://github.com/mattn/go-sqlite3
	Error: database is locked
		When you get an database is locked. Please use the following options.
		Add to DSN: cache=shared

		Example:
		db, err := sql.Open("sqlite3", "file:locked.sqlite?cache=shared")
		Second please set the database connections of the SQL package to 1.

		db.SetMaxOpenConn(1)
	*/
	db.SetMaxOpenConns(1)

	if fileExists {
		err = db.Ping()
		if err != nil {
			backupJobsState.UnMarkOpenDb(backupName)
			logger.Errorf("Connection test to the database %s returned error: %s", dbfilepath, err)
			return &sql.DB{}, ErrCouldNotOpenDB
		}
	}

	return db, nil
}

// close a database; the $name parameter MUST be the name of the backup job as defined in the configuration file
// $backupJobsState is used to signal that a given database is opened (so it should not be attempted to copy the DB)
func CloseDb(db *sql.DB, BackupJobName string, backupJobsState *shared.BackupJobsState) {
	err := db.Close()
	// purposely remove the lock (the mark that the DB was open). This may lead to issues but otherwise any attempt to make a copy of the DB would hang the whole program
	backupJobsState.UnMarkOpenDb(BackupJobName)
	if err != nil {
		logger.Errorf("While trying to close database '%s' encountered error: '%s'. This may lead to any "+
			"attempts to copy the database (in order to back it up to the remote object store) to produce corrupted DB "+
			"copies (the original should remain unaffected).", BackupJobName, err)
	}
}

// figure out the absolute path to the database file
func GetDbFilePath(datadir string, backupName string) (string, error) {
	dbfilepath, err := filepath.Abs(datadir + string(filepath.Separator) + backupName + ".sqlite")
	if err != nil {
		logger.Errorf("While trying to establish the absolute path to the database holding information about "+
			" backup job '%s' the following error was encountered: %s", backupName, err)
		return "", errors.New("could not establish the absolute path to the database file")
	} else {
		return dbfilepath, nil
	}
}

func DbFileExists(dbfilepath string) (bool, error) {
	_, err := utils.FileExists(dbfilepath, true)
	if err != nil {
		if err != utils.ErrNoSuchFile {
			logger.Errorf("When attempting to read the properties for the database file '%s' the following "+
				"error was received: ", err)
			return false, err
		}
		return false, nil
	} else {
		return true, nil
	}
}

// if a db does not exist then it attempts to create one
// Parameters: "datadir" value from the config file; $backupName is the name of the backup (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// "configInit" if this is called during program startup/config reload and it encounters an error then log a specific
// error message which is different if this function is called during backup start
// $backupJobsState is used to signal that a given database is opened (so it should not be attempted to copy the DB)
func ValidateAndCreate(datadir string, backupName string, configInit bool, backupJobsState *shared.BackupJobsState) error {
	// figure out name and absolute path to db file
	dbfilepath, err := GetDbFilePath(datadir, backupName)
	if err != nil {
		return err
	}
	// check if database file exists
	fExists, err := DbFileExists(dbfilepath)
	if err != nil {
		return err
	}
	if !fExists {
		if configInit {
			logger.Warnf("Database file '%s' used to store details for backup job '%s' doesn't exist. This is "+
				"expected during the first start of the backup server or when configuration file changes affect the"+
				" 'data_dir' option or add a new backup job. If this is not the case then the root cause for the "+
				"missing database file should be established. Proceeding now to create the database.",
				dbfilepath, backupName)
		} else {
			logger.Errorf("Database file '%s' used to store details for backup job '%s' doesn't exist. This is "+
				"unexpected as the database file should already exist so the root cause for this issue should be "+
				"established. Creating the database now in order to proceed with the backup.",
				dbfilepath, backupName)
		}
		db, err := OpenDb(datadir, backupName, false, backupJobsState)
		if err != nil {
			return ErrCouldNotCreateDB
		}

		err = CreateDb(db, backupName)
		if err != nil {
			logger.Errorf("Backups for job '%s' are not possible as the database file can't be created or "+
				"initialised",
				backupName)
			return err
		}
	}
	return nil
}

// Take care of starting the db connection;
// Parameters: $datadir value from the config file; $backupName is the name of the backup (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// $backupJobsState is used to signal that a given database is opened (so it should not be attempted to copy the DB)
func Start(datadir string, backupName string, backupJobsState *shared.BackupJobsState) (*sql.DB, error) {
	// check if DB exists, if not then create it
	err := ValidateAndCreate(datadir, backupName, false, backupJobsState)
	if err != nil {
		return &sql.DB{}, err
	}

	db, err := OpenDb(datadir, backupName, true, backupJobsState)
	if err != nil {
		return &sql.DB{}, err
	}

	return db, nil
}
