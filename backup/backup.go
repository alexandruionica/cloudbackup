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
			dbEntryFound, dbRecordProperties, err := getBackedupObjectPropertiesFromDb(path, dbData)
			if err != nil {
				return false, err
			}
			if dbEntryFound {
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
func getBackedupObjectPropertiesFromDb(path string, dbData shared.DbData) (bool, shared.BackedUpFileProperties, error){
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

// compares SQL DB obtained data about a file/dir/symlink with its current, on disk properties
//func objectPropertiesMatchDbRecord(path string, dbRecordProperties shared.BackedUpFileProperties, stat os.FileInfo) bool {
//	/*
//	type BackedUpFileProperties struct {
//	Path string
//	// one of: file / dir / symlink
//	Type string
//	// valid only for "symlink" type; otherwise it will be empty string
//	LinkTarget string
//	Size int64
//	// time object modified
//	Mtime time.Time
//	// time object metadata changed (ctime gets updated if file content gets changed too)
//	Ctime time.Time
//	// user id on *nix , Username on Windows (hence this is a string)
//	// TODO - validate that on Windows this is better than using a SID and also what to do in the Username or SID doesn't exist (on Windows only)
//	Uid string
//	// group id on *nix, Group name on Windows
//	// TODO - validate that on Windows this is better than using a SID and also what to do in the Groupname or SID doesn't exist (on Windows only)
//	Gid string
//	// on *nix this is the file mode (ex: 0755) ; on Windows some kind of basic permissions
//	// TODO - figure out file permissions on Windows
//	PermMode string
//	// if checksuming is enabled then this will be non empty
//	Checksum string
//	// if checksuming is enabled then this will hold whatever algorithm was used for checksumming
//	ChecksumType string
//	Encrypted bool
//	// references the "name" of one or more entries in "targets" table ; multiple entries will be comma separated
//	Targets string
//}
//	 */
//	onDiskObjectProperties := shared.BackedUpFileProperties{
//		Path: path,
//		Size: stat.Size(),
//		Mtime: stat.ModTime(),
//		// TODO - implement ctime - see possible options like https://github.com/djherbis/times/issues/1 (and library it provides)
//		Ctime: time.Time{},
//		Uid:
//
//	}
//
//}

// uploads an object (file / dir / symlink) to the remote object storage. For dirs/symlinks it only uploads metadata
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func UploadObject (ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup) (bool, error) {
	return false, nil
}