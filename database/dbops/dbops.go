package dbops

import (
	"cloudbackup/config"
	"cloudbackup/database"
	"cloudbackup/shared"
	"database/sql"
	"errors"
	log "github.com/sirupsen/logrus"
	"time"
)

const loggingContext = "database.dbops"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func CloseStatementsAndDb(dbData shared.DbData) {
	if dbData.Connected {
		// close common used prepare statements
		ClosePreparedStatements(dbData.PreparedStatements)
		// close db connection
		database.CloseDb(dbData.Db, dbData.Name)
	}
}

// prepare the most used SQL statements. This should increase performance and also help with SQL injection prevention
// returns: a shared.DbPreparedStatements and an error object
func Prepare(db *sql.DB) (shared.DbPreparedStatements, error) {
	var err error
	var PreparedStatements shared.DbPreparedStatements
	// query statement - having it as text to as it will be used in transactions too
	PreparedStatements.FilesQuery = "SELECT path, type, link_target, size, mtime, ctime, owner, permissions, checksum, " +
		"checksum_type, encrypted, job_id FROM files WHERE path = ?"
	PreparedStatements.FilesQueryStmt, err = db.Prepare(PreparedStatements.FilesQuery)
	if err != nil {
		logger.Errorf("While trying to prepare an SQL query statement, encountered error: '%s'", err)
		return PreparedStatements, err
	}

	// insert statement - having it as text to as it will be used in transactions too
	PreparedStatements.FilesInsert = "INSERT INTO files (path, type, link_target, size, mtime, " +
		"ctime, owner, permissions, checksum, checksum_type, encrypted, job_id) VALUES " +
		"(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	PreparedStatements.FilesInsertStmt, err = db.Prepare(PreparedStatements.FilesInsert)
	if err != nil {
		logger.Errorf("While trying to prepare an SQL insert statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.FilesQueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesQueryStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	// delete statement - having it text only as its used once per a given class of transactions
	PreparedStatements.FilesDelete = "DELETE FROM files where path=?"

	// update statement - having it as text to as it will be used in transactions too
	PreparedStatements.FilesUpdate = "UPDATE files SET type=?, link_target=?, size=?, mtime=?, " +
		"ctime=?, owner=?, permissions=?, checksum=?, checksum_type=?, encrypted=?, job_id=? WHERE path=?"
	PreparedStatements.FilesUpdateStmt, err = db.Prepare(PreparedStatements.FilesUpdate)
	if err != nil {
		logger.Errorf("While trying to prepare an SQL update statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.FilesQueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesQueryStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesInsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesInsertStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	// insert statement - having it as text only and not an actual prepared statement (as this will be used only in transactions, and called generally once per transaction)
	PreparedStatements.RemoteFilesInsert = "INSERT INTO remote_files (uuid, remote_path, local_path, target, upload_date, " +
		"job_id, delete_marker, version, src_os, type, link_target, size, mtime, ctime, owner, permissions, " +
		"checksum, checksum_type, encrypted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	//  query statement used to figure out the largest version for a particular path and target name pair
	PreparedStatements.RemoteFilesQueryNewestVersion = "SELECT version FROM remote_files WHERE local_path = ? AND target = ? ORDER BY version DESC LIMIT 1"

	// query statement used to figure out the uuid of the largest version for a particular path and target name pair as long as they are not a delete marker
	PreparedStatements.RemoteFilesQueryNewestVersionUuidStmt, err = db.Prepare("SELECT uuid FROM remote_files WHERE local_path=? AND target = ? AND delete_marker=0 ORDER BY version DESC LIMIT 1")
	if err != nil {
		logger.Errorf("While trying to prepare an SQL update statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.FilesQueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesQueryStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesInsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesInsertStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesUpdateStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesInsertStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	// insert statement - having it as text and prepared as it is going to be used in transactions and outside transactions
	PreparedStatements.BackupCollectionsInsert = "INSERT INTO backup_collections (file_uuid, job_id, target) VALUES (?, ?, ?)"
	PreparedStatements.BackupCollectionsInsertStmt, err = db.Prepare(PreparedStatements.BackupCollectionsInsert)
	if err != nil {
		logger.Errorf("While trying to prepare an SQL update statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.FilesQueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesQueryStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesInsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesInsertStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesUpdateStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesUpdateStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.RemoteFilesQueryNewestVersionUuidStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'RemoteFilesQueryNewestVersionUuidStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	// find which items are not listed in the last backup but still mentioned in the "files" table
	PreparedStatements.FindDeletedItemsStmt, err = db.Prepare("SELECT path FROM files EXCEPT SELECT local_path FROM remote_files rf INNER JOIN backup_collections bc ON bc.file_uuid == rf.uuid  WHERE bc.job_id=? AND bc.target=? LIMIT ?")
	if err != nil {
		logger.Errorf("While trying to prepare an SQL update statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.FilesQueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesQueryStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesInsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesInsertStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.FilesUpdateStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'FilesUpdateStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.RemoteFilesQueryNewestVersionUuidStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'RemoteFilesQueryNewestVersionUuidStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.BackupCollectionsInsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'BackupCollectionsInsertStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	return PreparedStatements, nil
}

// close a shared.DbPreparedStatements object
func ClosePreparedStatements(dbPreparedStatements shared.DbPreparedStatements) {
	if dbPreparedStatements.FilesQueryStmt != nil {
		err := dbPreparedStatements.FilesQueryStmt.Close()
		if err != nil {
			logger.Warnf("Could not close the db query statement for common operations")
		}
	}

	if dbPreparedStatements.FilesInsertStmt != nil {
		err := dbPreparedStatements.FilesInsertStmt.Close()
		if err != nil {
			logger.Warnf("Could not close the db insert statement for common operations")
		}
	}

	if dbPreparedStatements.FilesUpdateStmt != nil {
		err := dbPreparedStatements.FilesUpdateStmt.Close()
		if err != nil {
			logger.Warnf("Could not close the db update statement for common operations")
		}
	}
}

// TODO - when a config update changes anything regarding targets ; specially deleting targets, we need to ensure no lingering entries remain in the db and decide what to do with remote files

// TODO - when a config update deletes a backup section we need to ensure no lingering entries remain in the db and decide what to do with remote files

// ensures that the database has a record of all targets mentioned in the config file (for a particular backup name)
// given that a DB is allocated to one backup name only, this should not be an issue
func EnsureTargetsInDb(db *sql.DB, backupConfig config.Backup) error {
	logger.Debug("Checking the database has a record for each target mentioned in the config file")
	var (
		targetName string
		backupName string
	)
	dbFoundTargetNames := make([]string, 0)
	// build list of targets from the Database
	rows, err := db.Query("SELECT name, backup_name from targets")
	if err != nil {
		logger.Errorf("While trying to get from the database the list of targets, the following error was "+
			"encountered: '%s'", err)
		return err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a db.Query for retrieving target list, the following error "+
				"was encountered: '%s'", err)
		}
	}()
	for rows.Next() {
		err := rows.Scan(&targetName, &backupName)
		if err != nil {
			logger.Errorf("While enumerating from the database the list of targets, the following error was "+
				"encountered: '%s'", err)
			return err
		}
		dbFoundTargetNames = append(dbFoundTargetNames, targetName)
		if backupName != backupConfig.Name {
			logger.Warningf("Found in the database belonging to backup job '%s' target '%s' having back job "+
				"name '%s' which is different than what the config file has. This inconsistency may have been caused by"+
				" adjusting the configuration file and then manually renaming the sql database file",
				backupConfig.Name, targetName, backupName)
		}
	}
	err = rows.Err()
	if err != nil {
		logger.Errorf("Could not enumerate the list of all targets from the database due to the following "+
			"error: '%s'", err)
		return err
	}

	// build list of targets in the config file
	configFoundTargetNames := make([]string, 0)
	for _, target := range backupConfig.Target {
		configFoundTargetNames = append(configFoundTargetNames, target.Name)
	}

	targetNamesToAdd := make([]string, 0)
	// check if the number of targets in the DB includes all targets mentioned in the config file
	if len(dbFoundTargetNames) > 0 {
		for _, configTarget := range configFoundTargetNames {
			found := false
			for _, dbTarget := range dbFoundTargetNames {
				if dbTarget == configTarget {
					found = true
				}
			}
			if !found {
				targetNamesToAdd = append(targetNamesToAdd, configTarget)
			}
		}
		// else just add all target names in the config to the list of names to insert in the DB
	} else {
		for _, configTarget := range configFoundTargetNames {
			targetNamesToAdd = append(targetNamesToAdd, configTarget)
		}
	}

	// if any targets were found to be mentioned only in the config file then add them to the DB too
	if len(targetNamesToAdd) > 0 {
		logger.Debugf("Adding to DB the following target names: '%+v'", targetNamesToAdd)
		dateAdded := time.Now()
		for _, targetToAdd := range targetNamesToAdd {
			// walk each target in the config file until a match is found
			for _, targetInCfg := range backupConfig.Target {
				if targetToAdd == targetInCfg.Name {
					_, err := db.Exec("INSERT INTO targets (name, backup_name, type, date_added) VALUES "+
						"(?, ?, ?, ?)", targetToAdd, backupConfig.Name, targetInCfg.Type, dateAdded)
					if err != nil {
						logger.Errorf("While trying to add information about target '%s' to the database, the "+
							"following error was encountered: '%s'", targetToAdd, err)
						return err
					}
					break
				}
			}
		}
	}
	return nil
}

// for a given UUID it check that no job exists with its value. Job type is not relevant
// returns: bool with true if a match was found, false otherwise; err field if error is encountered (if an error is
// found then the bool field value is to be ignored)
func CheckJobUuidExists(db *sql.DB, jobid string) (bool, error) {
	logger.Debugf("Checking if the database has a record of job id '%s'", jobid)
	var jobIdInDb string
	// build list of targets from the Database
	rows, err := db.Query("SELECT id FROM jobs WHERE id = ?", jobid)
	if err != nil {
		logger.Errorf("While trying to get from the database any job id with uuid '%s', the following error was "+
			"encountered: '%s'", jobid, err)
		return false, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a db.Query for retrieving any job id with a given uuid, the "+
				"following error was encountered: '%s'", err)
		}
	}()
	for rows.Next() {
		err := rows.Scan(&jobIdInDb)
		if err != nil {
			logger.Errorf("While enumerating from the database the list of jobs with a given uuid, the "+
				"following error was encountered: '%s'", err)
			return false, err
		}
		// any result row means we had a match
		logger.Debugf("Found in the database a record of job id '%s'", jobid)
		return true, nil
	}
	err = rows.Err()
	if err != nil {
		logger.Errorf("Could not enumerate the list of all targets from the database due to the following "+
			"error: '%s'", err)
		return false, err
	}
	// if we got here there there wasn't any match and no error was encountered
	logger.Debugf("Did not find in the database a record of job id '%s'", jobid)
	return false, nil
}

// adds a new record in the "jobs" table for a new job
func AddJobDetails(db *sql.DB, jobId string, jobName string, jobType string, startTime time.Time) error {
	/*
			CREATE TABLE jobs (id TEXT NOT NULL PRIMARY KEY, name TEXT, type TEXT, start_time TEXT, end_time TEXT, state TEXT,
		report TEXT);
	*/
	_, err := db.Exec("INSERT INTO jobs (id, name, type, start_time, end_time, state, report) "+
		"VALUES "+
		"(?, ?, ?, ?, ?, ?, ?)", jobId, jobName, jobType, startTime, "", "started", "")
	if err != nil {
		logger.Errorf("While trying to add information about %s job having name '%s' and id '%s' to the "+
			"database, the following error was encountered: '%s'", jobType, jobName, jobId, err)
		return err
	}
	return nil
}

// updates an existing job record in the "jobs" table
func UpdateJobDetails(db *sql.DB, jobId string, jobName string, jobType string, endTime time.Time, JobState string, report string) error {
	/*
			CREATE TABLE jobs (id TEXT NOT NULL PRIMARY KEY, name TEXT, type TEXT, start_time TEXT, end_time TEXT, state TEXT,
		report TEXT);
	*/
	result, err := db.Exec("UPDATE jobs SET end_time = ?, state = ?, report = ? WHERE id = ? AND name = ? "+
		"AND type = ?", endTime, JobState, report, jobId, jobName, jobType)
	if err != nil {
		logger.Errorf("While trying to update information about %s job having name '%s' and id '%s' in the "+
			"database, the following error was encountered: '%s'", jobType, jobName, jobId, err)
		return err
	}
	rowsUpdated, err := result.RowsAffected()
	if err != nil {
		logger.Errorf("While trying to check if updating the database entry for %s job having name '%s' and id"+
			" '%s' was successful, the following error was encountered: '%s'", jobType, jobName, jobId, err)
		return err
	}
	if rowsUpdated != 1 {
		if rowsUpdated < 1 {
			logger.Errorf("Did not manage to update the database entry for %s job having name '%s' and id '%s' as"+
				" no matching entries were found. Job status and integrity may be affected.", jobType, jobName, jobId)
			return errors.New("could not find a matching database entry")
		} else {
			logger.Errorf("Found '%d' database entries instead of 1 for %s job having name '%s' and id '%s'. "+
				"Job status may be incorrect for the other jobs which had matching id and name and the status report "+
				"will be incorrect for them  ", rowsUpdated, jobType, jobName, jobId)
			return errors.New("more than one matching database entry")
		}

	}
	return nil
}
