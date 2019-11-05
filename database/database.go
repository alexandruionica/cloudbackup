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
	"sync/atomic"
	"time"
)

const loggingContext = "database"
const ErrTimedOut = "timed out"

// cache=shared - according to https://www.sqlite.org/sharedcache.html this improves performance
// _foreign_keys=1 - enable foreign keys support and enforcement
// _journal_mode=WAL - use Write-Ahead Logging as it's more performant  - should be safe and faster - https://www.sqlite.org/pragma.html#pragma_journal_mode
// synchronous=NORMAL - WAL mode is safe from corruption with synchronous=NORMAL - https://www.sqlite.org/pragma.html#pragma_synchronous
// busy_timeout=5000 - if a table is locked then retry up to 5 seconds the given operation - https://www.sqlite.org/pragma.html#pragma_busy_timeout; afterwards the OP will return an error
// both of the above "_journal_mode" and "synchronous" ensure good performance from Sqlite. Removing those settings will
// decrease performance by 8 times to roughly 100 transactions/sec for our usage cases
const DbOptions = "_foreign_keys=1&cache=shared&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"

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

	CREATE TABLE failed_files (uuid NOT NULL PRIMARY KEY, job_id TEXT, path TEXT, type TEXT,
	FOREIGN KEY(job_id) REFERENCES jobs(id));

	CREATE INDEX failed_files_job_id ON failed_files(job_id);

	CREATE TABLE backup_collections (file_uuid TEXT, job_id TEXT, target TEST, FOREIGN KEY(file_uuid) REFERENCES remote_files(uuid), 
	FOREIGN KEY(job_id) REFERENCES jobs(id), FOREIGN KEY(target) REFERENCES targets(name));
	
	CREATE INDEX backup_collections_jobid ON backup_collections(job_id);
    CREATE INDEX backup_collections_target ON backup_collections(target);

	`
	logger.Debugf("Creating tables in '%s' database", dbfilepath)
	_, err := db.Exec(sqlStmt)
	if err != nil {
		logger.Errorf("Encountered error while attempting to create database '%s' . The error is: %s",
			dbfilepath, err)
		logger.Debugf("Closing '%s' and removing the file", dbfilepath)
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
	}
	return nil
}

// opens a connection to the DB and if successful, it returns the *sql.DB
// params: $datadir is the folder containing the database file; $backupName is the name of the backup (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// if $fileExists == false then don't attempt to "ping" the DB as it will error;
func openDbFile(datadir string, backupName string, fileExists bool) (*sql.DB, error) {
	dbfilepath, err := GetDbFilePath(datadir, backupName)
	if err != nil {
		return &sql.DB{}, err
	}

	logger.Debugf("Opening database file '%s'", dbfilepath)
	db, err := sql.Open("sqlite3", dbfilepath+"?"+DbOptions)
	if err != nil {
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
	/* According to https://github.com/mattn/go-sqlite3 's tickets regarding locking related issues, setting db.SetMaxOpenConn(1)
	should still allow multiple connections (from the same program) as they will be multiplexed over the existing 1 connection
	*/
	db.SetMaxOpenConns(1)

	if fileExists {
		err = db.Ping()
		if err != nil {
			logger.Errorf("Connection test to the database %s returned error: %s", dbfilepath, err)
			return &sql.DB{}, ErrCouldNotOpenDB
		}
	}

	return db, nil
}

// opens a connection to the DB and if successful, it returns the *sql.DB
// params: $datadir is the folder containing the database file; dbName is the name of the backup or the name of the restore (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// if $fileExists == false then don't attempt to "ping" the DB as it will error;
// $backupJobsState is used to signal that a given database is opened and increment number of connected clients
// $timeout is max duration to wait for getting the lock wihich governs DB access. If it times out then the error $ErrTimedOut will be returned together with an empty sql.DB{}
func OpenDb(datadir string, dbName string, fileExists bool, backupJobsState *shared.BackupJobsState, timeout time.Duration) (*sql.DB, error) {
	logger.Debugf("Getting a DB connection for database '%s'", dbName)
	backupJobsState.Lock.RLock()
	entry, ok := backupJobsState.DbOpenAllowed[dbName]
	backupJobsState.Lock.RUnlock()
	if ok { // the map has an entry for our database
		if timeout != 0 {
			timedOut := entry.DbOpenAllowed.GetLockWithTimeout(timeout)
			if timedOut {
				return &sql.DB{}, errors.New(ErrTimedOut)
			}
		} else {
			entry.DbOpenAllowed.GetLock()
		}
		atomic.AddUint32(&entry.NumClients, 1)
		if entry.DB != nil {
			logger.Debugf("Found an existing DB connection for database '%s'", dbName)
			entry.DbOpenAllowed.ReleaseLock()
			return entry.DB, nil
		} else { // we have an entry by no active opened DB (pointer nil) so we need to open the DB file
			logger.Debugf("Did not find an existing DB connection for database '%s' despite "+
				"this database having been opened before.Proceeding to setup a new connection", dbName)
			db, err := openDbFile(datadir, dbName, fileExists)
			if err != nil {
				atomic.AddUint32(&entry.NumClients, ^uint32(0)) // decrements NumClients
				entry.DbOpenAllowed.ReleaseLock()
				return db, err
			} else {
				entry.DB = db
				atomic.AddUint32(&entry.NumClients, ^uint32(0)) // decrements NumClients
				entry.DbOpenAllowed.ReleaseLock()
				return db, err
			}
		}
	} else { // $BackupJobName was not yet added to the map
		logger.Debugf("Did not find an existing DB connection for database '%s' so "+
			"proceeding to setup one", dbName)
		db, err := openDbFile(datadir, dbName, fileExists)
		if err != nil {
			return db, err
		}

		if !ok {
			backupJobsState.Lock.Lock() // lock BackupJobsState while adding a new map member in DbOpenAllowed
			backupJobsState.DbOpenAllowed[dbName] = &shared.DbAccess{
				DbOpenAllowed: utils.NewMutexWithTimeout(),
				NumClients:    1,
				DB:            db,
			}
			backupJobsState.Lock.Unlock() // close as soon as possible as if the struct is Locked, actual backups may stop
		}
		return db, nil
	}
}

// removes a client from the list of clients connected to a database; the $dbName parameter MUST be either the name of the
// backup job as defined in the configuration file or the name of a restore job;
// $backupJobsState is used get the struct which contains number of connected clients and then proceed to decrement it
// This function must be safe to run even on a closed / not opened DB
func DisconnectFromDb(dbName string, backupJobsState *shared.BackupJobsState) {
	logger.Debugf("Disconnecting from database '%s'", dbName)
	backupJobsState.Lock.RLock()
	entry, ok := backupJobsState.DbOpenAllowed[dbName]
	backupJobsState.Lock.RUnlock()
	if ok { // the map has an entry for our database
		atomic.AddUint32(&entry.NumClients, ^uint32(0)) // decrements NumClients
	} else { // no entry exists for this DB which is unexpected (an error)
		logger.Warnf("Tried to disconnect from database '%s' but no connections to the database are recorded to exist", dbName)
		return
	}
}

// waits for all clients to disconnect from the database and then closes down the DB connection. If $releaseLock is
// set to false the the DB lock won't be released which mean no one can re-open the database until the lock is released.
func CloseDb(dbName string, backupJobsState *shared.BackupJobsState, releaseLock bool) {
	logger.Debugf("Closing database '%s'", dbName)
	backupJobsState.Lock.RLock()
	entry, ok := backupJobsState.DbOpenAllowed[dbName]
	backupJobsState.Lock.RUnlock()
	if ok { // the map has an entry for our database
		entry.DbOpenAllowed.GetLock() // acquire lock so no new DB connections be setup to this DB
		for {                         // sleep 50 ms until all DB clients have disconnected
			numClients := atomic.LoadUint32(&entry.NumClients)
			if numClients > 0 {
				time.Sleep(50 * time.Millisecond)
			} else {
				break
			}
		}
		db := entry.DB
		entry.DB = nil // this makes is clear that any new attempts to use this DB will have to setup a new connection
		err := db.Close()
		if err != nil {
			logger.Errorf("While trying to close database '%s' encountered error: '%s'. This may lead to any "+
				"attempts to copy the database (in order to back it up to the remote object store) to produce corrupted DB "+
				"copies (the original should remain unaffected).", dbName, err)
		}
		if releaseLock {
			entry.DbOpenAllowed.ReleaseLock()
		}
		return
	} else { // no entry exists for this DB which is unexpected (an error)
		logger.Errorf("Tried to close database '%s' but no record of the database being open exists. This may lead to any "+
			"attempts to copy the database (in order to back it up to the remote object store) to produce corrupted DB "+
			"copies (the original should remain unaffected).", dbName)
		return
	}
}

// unlocks a DB which was let locked by CloseDb()
func UnlockDb(dbName string, backupJobsState *shared.BackupJobsState) {
	logger.Debugf("Removing Lock of database '%s'", dbName)
	backupJobsState.Lock.RLock()
	entry, ok := backupJobsState.DbOpenAllowed[dbName]
	backupJobsState.Lock.RUnlock()
	if ok { // the map has an entry for our database
		entry.DbOpenAllowed.ReleaseLock()
	} else { // no entry exists for this DB which is unexpected (an error)
		logger.Errorf("Tried to remove lock of database '%s' but no lock for the database can be found", dbName)
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
		db, err := OpenDb(datadir, backupName, false, backupJobsState, 0)
		if err != nil {
			return ErrCouldNotCreateDB
		}

		err = CreateDb(db, backupName)
		if err != nil {
			DisconnectFromDb(backupName, backupJobsState)
			logger.Errorf("Backups for job '%s' are not possible as the database file can't be created or "+
				"initialised",
				backupName)
			return err
		}
		// close db connection
		DisconnectFromDb(backupName, backupJobsState)
	}
	return nil
}

// Take care of starting the db connection;
// Parameters: $datadir value from the config file; $backupName is the name of the backup (we use it to
// figure the sql file path) and this name must match the name of the backup job as defined in the configuration file;
// $backupJobsState is used to signal that a given database is opened (so it should not be attempted to copy the DB)
func Start(datadir string, backupName string, backupJobsState *shared.BackupJobsState) (*sql.DB, error) {
	// check if DB exists, if not then create it
	err := ValidateAndCreate(datadir, backupName, false, backupJobsState) // this does not increment the number of connected DB clients
	if err != nil {
		return &sql.DB{}, err
	}

	db, err := OpenDb(datadir, backupName, true, backupJobsState, 0) // if successful, increments number of DB clients
	if err != nil {
		return &sql.DB{}, err
	}

	return db, nil
}
