package dbops

import (
	"cloudbackup/config"
	"cloudbackup/database"
	"cloudbackup/shared"
	"database/sql"
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
	// query statement
	PreparedStatements.QueryStmt, err = db.Prepare("SELECT path, type, link_target, size, mtime, ctime, uid, gid, perm_mode, " +
		"checksum, checksum_type, encrypted, targets FROM files WHERE path = ?")
	if err != nil {
		logger.Errorf("While trying to prepare an SQL query statement, encountered error: '%s'", err)
		return PreparedStatements, err
	}

	// insert statement
	PreparedStatements.InsertStmt, err = db.Prepare("INSERT INTO files (path, type, link_target, size, mtime, ctime, uid, gid, " +
		"perm_mode, checksum, checksum_type, encrypted, targets) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		logger.Errorf("While trying to prepare an SQL insert statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.QueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'QueryStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	// update statement
	PreparedStatements.UpdateStmt, err = db.Prepare("UPDATE files SET type=?, link_target=?, size=?, mtime=?, ctime=?, uid=?, " +
		"gid=?, perm_mode=?, checksum=?, checksum_type=?, encrypted=?, targets=? WHERE path=?")
	if err != nil {
		logger.Errorf("While trying to prepare an SQL update statement, encountered error: '%s'", err)
		// close other opened statements before returning
		err2 := PreparedStatements.QueryStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'QueryStmt' received error: '%s'", err2)
		}
		err2 = PreparedStatements.InsertStmt.Close()
		if err2 != nil {
			logger.Warnf("While trying to early close 'InsertStmt' received error: '%s'", err2)
		}
		return PreparedStatements, err
	}

	return PreparedStatements, nil
}

// close a shared.DbPreparedStatements object
func ClosePreparedStatements(dbPreparedStatements shared.DbPreparedStatements){
	if dbPreparedStatements.QueryStmt != nil {
		err := dbPreparedStatements.QueryStmt.Close()
		if err != nil {
			logger.Warnf("Could not close the db query statement for common operations")
		}
	}

	if dbPreparedStatements.InsertStmt != nil {
		err := dbPreparedStatements.InsertStmt.Close()
		if err != nil {
			logger.Warnf("Could not close the db insert statement for common operations")
		}
	}

	if dbPreparedStatements.UpdateStmt != nil {
		err := dbPreparedStatements.UpdateStmt.Close()
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
		targetName         string
		backupName         string
	)
	dbFoundTargetNames := make([]string, 0)
	// build list of targets from the Database
	rows, err := db.Query("SELECT name, backup_name from targets")
	if err != nil {
		logger.Errorf("While trying to get from the database the list of targets, the following error was " +
			"encountered: '%s'", err)
		return err
	}
	defer func (){
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a db.Query for retrieving target list, the following error " +
				"was encountered: '%s'", err)
		}
	}()
	for rows.Next() {
		err := rows.Scan(&targetName, &backupName)
		if err != nil {
			logger.Errorf("While enumerating from the database the list of targets, the following error was " +
				"encountered: '%s'", err)
			return err
		}
		dbFoundTargetNames = append(dbFoundTargetNames, targetName)
		if backupName != backupConfig.Name {
			logger.Warningf("Found in the database belonging to backup job '%s' target '%s' having back job " +
				"name '%s' which is different than what the config file has. This inconsistency may have been caused by" +
				" adjusting the configuration file and then manually renaming the sql database file",
				backupConfig.Name, targetName, backupName)
		}
	}
	err = rows.Err()
	if err != nil {
		logger.Errorf("Could not enumerate the list of all targets from the database due to the following " +
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
			if ! found {
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
					_, err := db.Exec("INSERT INTO targets (name, backup_name, type, date_added) VALUES " +
						"(?, ?, ?, ?)", targetToAdd, backupConfig.Name, targetInCfg.Type, dateAdded)
					if err != nil {
						logger.Errorf("While trying to add information about target '%s' to the database, the " +
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