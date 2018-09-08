package backup

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"os"
)

const loggingContext = "backup"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// performs backup of a file or dir
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func Do (ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData) (bool, error) {
	select {
	case <-ctx.Done():
		{
			logger.Infof("cancelling processing for backup of '%s'", path)
			return true, nil
		}
	// perform backup work
	default:
		{
			dBentryFound, dbRecordProperties, err := getBackedupObjectDataFromDb(path, dbData)
			if err != nil {
				return false, err
			}
			if dBentryFound {
				// check if properties match between DB record and os.FileInfo
				logger.Infof("Found db record for '%s' with properties '%+v'", path, dbRecordProperties)

			}else{
				// proceed to upload file and then insert DB record
			}
		}
	}
	return false, nil
}

// check if a given path exists in the Database;
// returns the following values: bool depicting if an entry was found or not; if found a populated
// shared.BackedUpFileProperties object containing all of the properties of given object as extracted from the DB
// record; an error object is an error is encountered
func getBackedupObjectDataFromDb (path string, dbData shared.DbData) (bool, shared.BackedUpFileProperties, error){
	rows, err := dbData.PreparedStatements.QueryStmt.Query(path)
	if err != nil {
		logger.Errorf("While querying the database in order to check if '%s' has been previously backed" +
			" up, the following error was encountered: %s", path, err)
		return false, shared.BackedUpFileProperties{}, err
	}
	defer func (){
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement for checking if '%s' has been" +
				" previously backed up, the following error was encountered: %s", path, err)
		}
	}()
	entryFound := false
	dbRecord := shared.BackedUpFileProperties{}
	for rows.Next() {
		if entryFound {
			logger.Errorf("Found duplicate database record for '%s' in the 'files' table.", path)
			return false, shared.BackedUpFileProperties{}, errors.New("duplicate database record in 'files' table")
		}
		entryFound = true
		err := rows.Scan(&dbRecord.Path, &dbRecord.Type, &dbRecord.LinkTarget, &dbRecord.Size, &dbRecord.Mtime,
			&dbRecord.Ctime, &dbRecord.Uid, &dbRecord.Gid, &dbRecord.PermMode, &dbRecord.Checksum,
			&dbRecord.ChecksumType, &dbRecord.Encrypted, &dbRecord.Targets)
		if err != nil {
			logger.Errorf("While retrieving the database record for '%s' the following error was encountered:" +
				" '%s'", path, err)
			return false, shared.BackedUpFileProperties{}, err
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to check if '%s' " +
			"has been previously backed up, the following error was encountered: %s", path, err)
		return false, shared.BackedUpFileProperties{}, err
	}
	if ! entryFound {
		logger.Debugf("Did not find in the DB a match for %s", path)
		return false, shared.BackedUpFileProperties{}, nil
	}
	// if we got here, all was fine
	return false, dbRecord, nil
}