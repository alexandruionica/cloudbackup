package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const loggingContext = "backup"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// performs backup of a file or dir
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func Do(ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string) (bool, error) {
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
				updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
				return false, errors.New(fmt.Sprintf("While searching the database for an object record,"+
					" encountered error: %s", err))
			}
			// if a db entry is found then this object has been previously backed up so it needs to be verified if the
			// object has changed
			if dbEntryFound {
				logger.Debugf("Found DB entry for %s", path)
				// check if properties match between DB record and os.FileInfo
				contentChanged, metadataChanged, ctime, checksum := needsUpload(path, stat, dbRecordProperties, backupConfig.Checksum)
				updatedDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobUuid)
				if err != nil {
					// something bad enough happened that we don't have a usable db record so we can't proceed to
					// backup this file
					updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
					return false, errors.New(fmt.Sprintf("Could not prepare an updated db record due to "+
						"error: %s", err))
				}
				if contentChanged {
					logger.Debugf("Content change detected for %s", path)
					cancelled, err := backupExistingWithContentChange(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, updatedDbRecord, jobUuid)
					return cancelled, err
				} else {
					if metadataChanged {
						logger.Debugf("Metadata change detected for %s", path)
						cancelled, err := backupExistingWithMetadataChange(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, updatedDbRecord, jobUuid)
						return cancelled, err
					} else {
						// object is up to date (aka we got a copy in the backup)
						// add entry to backup_collections so this file would also be included in a restore
						var foundErr error
						for _, objStore := range objectStores {
							targetName, _ := objStore.GetStoreDetails()
							remoteFilesUuid, err := getNewestRemoteFileUuid(dbData, path, targetName)
							_, err = dbData.PreparedStatements.BackupCollectionsInsertStmt.Exec(remoteFilesUuid, jobUuid, targetName)
							if err != nil {
								foundErr = errors.New(fmt.Sprintf("Despite no change detected for '%s', could not add entry to backup_collections"+
									" table due to error %s . This means that if a restore is selected for this particular backup job id, then this file "+
									"won't be restored despite the fact that a previous run ensured it is backed up", path, err))
								logger.Error(foundErr)
							}
						}
						updateCounters(backupJobsState, backupConfig.Name, "up_to_date", updatedDbRecord.Type, path, foundErr)
					}
				}
				// no db record found so this is the first time this object is backed up
			} else {
				logger.Debugf("Did not find a DB entry for %s , this was not a previously backed up", path)
				cancelled, err := backupNewItem(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid)
				return cancelled, err
			}
		}
	}
	return false, nil
}

// Performs the backup for an existing file/dir/folder for which it has been established that it's metadata changed
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func backupExistingWithMetadataChange(ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, updatedDbRecord shared.BackedUpFileProperties, jobUuid string) (bool, error) {
	if updatedDbRecord.Type == "unknown" {
		// report it as a "failed_to_upload_unknown" instead of updated_metadata as we don't support "unknown" files but we want to report somehow this issue
		updateCounters(backupJobsState, backupConfig.Name, "update", updatedDbRecord.Type, path, errors.New("unsupported file type"))
		return false, errors.New("unsupported file type")
	}

	cancelled, err := UploadAndUpdateDB("metadata-update", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid, updatedDbRecord)
	return cancelled, err
}

// Performs the backup for an existing file/dir/folder for which it has been established that it's content changed
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func backupExistingWithContentChange(ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, updatedDbRecord shared.BackedUpFileProperties, jobUuid string) (bool, error) {

	if updatedDbRecord.Type == "unknown" {
		updateCounters(backupJobsState, backupConfig.Name, "upload", updatedDbRecord.Type,
			path, errors.New("unsupported file type"))
		return false, errors.New("unsupported file type")
	}

	cancelled, err := UploadAndUpdateDB("content-update", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid, updatedDbRecord)
	return cancelled, err
}

// Performs the backup for a new file/dir/folder. This function being called means its established this item has never
// been backed up before
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func backupNewItem(ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string) (bool, error) {
	var err error

	checksum := ""
	if backupConfig.Checksum && utils.FileType(stat) == "file" {
		checksum, err = utils.GetFileMD5Sum(path)
	}
	ctime, err := fileproperties.GetCtime(path)
	if err != nil {
		ctime = time.Time{}
	}
	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobUuid)
	if err != nil {
		// something bad enough happened that we don't have a usable db record so we can't proceed to
		// backup this file.
		updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
		return false, err
	}
	if newDbRecord.Type == "unknown" {
		updateCounters(backupJobsState, backupConfig.Name, "upload", newDbRecord.Type, path,
			errors.New("unsupported file type"))
		return false, errors.New("unsupported file type")
	}
	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobUuid, newDbRecord)
	return cancelled, err
}

// Wraps around the DB transaction needed and also the file/dir upload & metadata update code
// $operation must be one of "new", "content-update", "metadata-update"; $dbData is the constructed DB rectord struct needed for the "files" table.
func UploadAndUpdateDB(operation string, ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string, DbRecord shared.BackedUpFileProperties) (bool, error) {
	// $operationType is used for counters and status messages
	var operationType string
	switch operation {
	case "new":
		operationType = "upload"
	case "content-update":
		operationType = "upload"
	case "metadata-update":
		operationType = "update"
	default:
		return false, errors.New(fmt.Sprintf("Unknown operation: %s . Allowed operations are:  'new', 'content-update', 'metadata-update'", operation))
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to backup this file.
		updateCounters(backupJobsState, backupConfig.Name, operationType, utils.FileType(stat), path, err)
		return false, errors.New(fmt.Sprintf("While trying to start a database transaction "+
			"encountered error: %s", err))
	}
	if operation == "new" {
		_, err = dbtx.Exec(dbData.PreparedStatements.FilesInsert, DbRecord.Path, DbRecord.Type,
			DbRecord.LinkTarget, DbRecord.Size, DbRecord.Mtime.Format(time.RFC3339Nano),
			DbRecord.Ctime.Format(time.RFC3339Nano), DbRecord.Owner,
			DbRecord.Permissons, DbRecord.Checksum, DbRecord.ChecksumType, DbRecord.Encrypted,
			DbRecord.JobUuid)
	} else {
		_, err = dbtx.Exec(dbData.PreparedStatements.FilesUpdate, DbRecord.Type,
			DbRecord.LinkTarget, DbRecord.Size, DbRecord.Mtime.Format(time.RFC3339Nano),
			DbRecord.Ctime.Format(time.RFC3339Nano), DbRecord.Owner,
			DbRecord.Permissons, DbRecord.Checksum, DbRecord.ChecksumType, DbRecord.Encrypted,
			DbRecord.JobUuid, DbRecord.Path)
	}

	if err != nil {
		logger.Errorf("function passed uuid %s vs dbrecord obj uuid: %s", jobUuid, DbRecord.JobUuid)
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", path, txerr)
		}
		// could not add dbentry to the database so we can't proceed to backup this file.
		updateCounters(backupJobsState, backupConfig.Name, operationType, utils.FileType(stat), path, err)
		return false, errors.New(fmt.Sprintf("While trying to add new file object DB entry "+
			"encountered error: %s", err))
	}
	encounteredError := 0
	var encounteredErrorObject error
	var JobCancelled bool
	// back up the object to one or more remote object stores
	for _, objectStore := range objectStores {
		remotePath, cancelled, err := UploadObject(ctx, path, DbRecord, backupConfig, objectStore, backupJobsState)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		if cancelled {
			JobCancelled = true
			break
		}
		targetName, _ := objectStore.GetStoreDetails()
		// TODO - if $operation == "content-update" then ensure that $remotePath + $version is unique ; if not and another one or figure out what to do
		remoteUuid, err := addEntryToRemoteFiles(remotePath, targetName, jobUuid, 0, dbData, dbtx, DbRecord)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		_, err = dbtx.Exec(dbData.PreparedStatements.BackupCollectionsInsert, remoteUuid, jobUuid, targetName)
		if err != nil {
			encounteredError++
			encounteredErrorObject = errors.New(fmt.Sprintf("For '%s' could not add entry to backup_collections"+
				" table due to error %s", path, err))
			break
		}
	}
	if encounteredError > 0 || JobCancelled {
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", path, txerr)
		}
		if len(objectStores) > 1 {
			// TODO - ENSURE THAT ANY SUCCESSFULLY UPLOADED FILE/DIR/SYMLINK IS REMOVED
			logger.Warnf("Failed upload of '%s' to %d out of %d targets. All targets are be "+
				"considered failed (even the ones where the backup was successful) for this item.",
				path, encounteredError, len(objectStores))
		}
		if JobCancelled {
			return true, nil
		} else {
			updateCounters(backupJobsState, backupConfig.Name, operationType, DbRecord.Type, path, encounteredErrorObject)
			return false, encounteredErrorObject
		}
	}

	txerr := dbtx.Commit()
	if txerr != nil {
		return false, errors.New(fmt.Sprintf("Could not commit transaction due to error: %s", err))
	}
	// if we got here then all was good
	updateCounters(backupJobsState, backupConfig.Name, operationType, DbRecord.Type, path, nil)
	return false, nil
}

// $remotePath is the path on the remote object store, as returned by objectstore/Upload() or objectstore/MetadataUpdate()
// $target is one target name for the backup section definition in use. This target is to the objecstore where this file/dir/symlink got sent/updated
// $deleteMarker: 1 for true, 0 for false. If 1 it means the file was deleted from the local filesystem
//
// Returns the uuid value for this entry and if an error was encountered or not. If err then ignore the uuid value.
func addEntryToRemoteFiles(remotePath string, target string, jobUuid string, deleteMarker int, dbData shared.DbData,
	dbtx *sql.Tx, fileDbRecord shared.BackedUpFileProperties) (string, error) {
	entryUuid := uuid.NewV4().String()
	// TODO - calculate version (select from remote_files all entries matching "local_path" with the current item, and then look at their version field and increment the largest value found
	version, err := getRemoteFileVersion(dbData, dbtx, fileDbRecord.Path, target)
	// logger.Debugf("Adding entry to remote_files for %s", fileDbRecord.Path)
	if err != nil {
		return "", err
	}
	_, err = dbtx.Exec(dbData.PreparedStatements.RemoteFilesInsert, entryUuid, remotePath, fileDbRecord.Path, target,
		time.Now().UnixNano(), jobUuid, deleteMarker, version, runtime.GOOS, fileDbRecord.Type,
		fileDbRecord.LinkTarget, fileDbRecord.Size, fileDbRecord.Mtime.Format(time.RFC3339Nano),
		fileDbRecord.Ctime.Format(time.RFC3339Nano), fileDbRecord.Owner,
		fileDbRecord.Permissons, fileDbRecord.Checksum, fileDbRecord.ChecksumType, fileDbRecord.Encrypted)
	if err != nil {
		return "", errors.New(fmt.Sprintf("While trying to add a db record for '%s' in the remote_files table, "+
			"encountered error: %s", fileDbRecord.Path, err))
	}
	return entryUuid, nil
}

// For a given file path and a backup target name calculate version
// returns an increment of the largest found version and nil; if an error is encountered then it returns 0 and the error; if no entry is found then it returns 1 and nil
func getRemoteFileVersion(dbData shared.DbData, dbtx *sql.Tx, localPath string, targetName string) (int, error) {
	// rows, err := dbData.PreparedStatements.RemoteFilesQueryNewestVersionStmt.Query(localPath, targetName)
	rows, err := dbtx.Query(dbData.PreparedStatements.RemoteFilesQueryNewestVersion, localPath, targetName)
	if err != nil {
		logger.Errorf("While querying the database in order to calculate a version number for '%s' the "+
			"following error was encountered: %s", localPath, err)
		return 0, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement used for querying the database in order "+
				"to calculate a version number for '%s' the following error was encountered: %s", localPath, err)
		}
	}()
	var version int
	entryFound := false
	for rows.Next() {
		entryFound = true
		err := rows.Scan(&version)
		if err != nil {
			logger.Errorf("While retrieving the database record for '%s' the following error was encountered:"+
				" '%s'", targetName, err)
			return 0, err
		}
		break
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order calculate a version number"+
			" for '%s' , the following error was encountered: %s", localPath, err)
		return 0, err
	}
	if entryFound {
		return version + 1, nil
	} else {
		return 1, nil
	}
}

// for a given pair of file/dir/symlink path and backup config target, find the uuid for the newest backed up version which is not a delete marker
func getNewestRemoteFileUuid(dbData shared.DbData, localPath string, targetName string) (string, error) {
	rows, err := dbData.PreparedStatements.RemoteFilesQueryNewestVersionUuidStmt.Query(localPath, targetName)
	if err != nil {
		logger.Errorf("While querying the database in order to get the uuid of the newest remote version for "+
			"'%s', the following error was encountered: %s", localPath, err)
		return "", err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement for querying the database in order to "+
				"get the uuid of the newest remote version for %s, encountered error: %s", localPath, err)
		}
	}()
	var remoteUuid string
	entryFound := false
	for rows.Next() {
		entryFound = true
		// the sqlite3 driver produces an error when fetching a string and converting it to time.time so we have to
		// manually do the conversion
		err := rows.Scan(&remoteUuid)
		if err != nil {
			logger.Errorf("While retrieving from the database the remote uuid for '%s' the following error was encountered:"+
				" '%s'", localPath, err)
			return "", err
		}
		break
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database for the remote uuid for '%s' "+
			", the following error was encountered: %s", localPath, err)
		return "", err
	}
	if !entryFound {
		return "", errors.New(fmt.Sprintf("Could not find the uuid for previously backed up object '%s'", localPath))
	}

	if remoteUuid == "" {
		return "", errors.New(fmt.Sprintf("Found an empty uuid for previously backed up object '%s'", localPath))
	}
	return remoteUuid, nil
}

// check if a given path exists in the Database;
// returns the following values: bool depicting if an entry was found or not; if found a populated
// shared.BackedUpFileProperties object containing all of the properties of given object as extracted from the DB
// record; an error object is an error is encountered
func getBackedupObjectPropertiesFromDb(path string, dbData shared.DbData) (bool, shared.BackedUpFileProperties, error) {
	rows, err := dbData.PreparedStatements.FilesQueryStmt.Query(path)
	if err != nil {
		logger.Errorf("While querying the database in order to check if '%s' has been previously backed"+
			" up, the following error was encountered: %s", path, err)
		return false, shared.BackedUpFileProperties{}, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement for checking if '%s' has been"+
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
		// the sqlite3 driver produces an error when fetching a string and converting it to time.time so we have to
		// manually do the conversion
		var tmpMtime, tmpCtime string
		err := rows.Scan(&dbRecord.Path, &dbRecord.Type, &dbRecord.LinkTarget, &dbRecord.Size, &tmpMtime,
			&tmpCtime, &dbRecord.Owner, &dbRecord.Permissons, &dbRecord.Checksum,
			&dbRecord.ChecksumType, &dbRecord.Encrypted, &dbRecord.JobUuid)
		if err != nil {
			logger.Errorf("While retrieving the database record for '%s' the following error was encountered:"+
				" '%s'", path, err)
			return false, shared.BackedUpFileProperties{}, err
		} else {
			// convert string to time for  mtime  and  ctime
			if tmpMtime != "" {
				dbRecord.Mtime, err = time.Parse(time.RFC3339Nano, tmpMtime)
				if err != nil {
					logger.Error("While converting mtime property of database record for '%s' the following "+
						"error was encountered: %s", path, err)
					return false, shared.BackedUpFileProperties{}, errors.New(fmt.Sprintf("While converting "+
						"mtime property encountered error: %s", err))
				}
			}
			if tmpCtime != "" {
				dbRecord.Ctime, err = time.Parse(time.RFC3339Nano, tmpCtime)
				if err != nil {
					logger.Error("While converting ctime property of database record for '%s' the following "+
						"error was encountered: %s", path, err)
					return false, shared.BackedUpFileProperties{}, errors.New(fmt.Sprintf("While converting "+
						"ctime property encountered error: %s", err))
				}
			}
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to check if '%s' "+
			"has been previously backed up, the following error was encountered: %s", path, err)
		return false, shared.BackedUpFileProperties{}, err
	}
	if !entryFound {
		logger.Debugf("Did not find in the DB a match for %s", path)
		return false, shared.BackedUpFileProperties{}, nil
	}
	// if we got here, all was fine
	return true, dbRecord, nil
}

// compares on disk state vs db and returns:
// bool with value true if file changed and it needs upload (this implies a metadata upload is needed too); bool with
// value true when a metadata change was detected but the file content itself remains unchanged  ; time.Time containing
// ctime populated when either file content or metadata change was detected. This is done because it is expensive to
// get ctime (1 system call) and we want to avoid calling this again later; $checksum empty if an error was encountered
// while trying to calculate it or if checksum comparison was not requested, otherwise ascii string with md5 sum
func needsUpload(path string, stat os.FileInfo, dbRecordProperties shared.BackedUpFileProperties, compareChecksum bool) (contentChanged bool,
	metadataChanged bool, ctime time.Time, checksum string) {
	var err error
	objectType := utils.FileType(stat)
	if compareChecksum && objectType == "file" {
		checksum, err = utils.GetFileMD5Sum(path)
		if err != nil {
			// if we got any errors means we could not calculate the checksum so to be safe, we consider that the file needs to be uploaded
			contentChanged = true
		} else if checksum != dbRecordProperties.Checksum {
			contentChanged = true
		}
		// if size or mtime differs then we got a file change
	} else if stat.Size() != dbRecordProperties.Size || stat.ModTime() != dbRecordProperties.Mtime {
		contentChanged = true
		// if type changed then we need to back it up (for example in the DB it's marked as a symlink but on disk it's a file now
	} else if objectType != dbRecordProperties.Type {
		contentChanged = true
	}
	ctime, err = fileproperties.GetCtime(path)
	// in case of error we just treat it as the metadata changed as we can't know for sure if it didn't and it's better to be safe and just back it up
	if err != nil {
		metadataChanged = true
	} else {
		if ctime != dbRecordProperties.Ctime {
			metadataChanged = true
		}
	}
	// if we have a symlink, check if the symlink target has changed and if so then update metadata
	if !metadataChanged && objectType == "symlink" {
		linkTarget, err := os.Readlink(path)
		// in case of error we just treat it as the metadata changed as we can't know for sure if it didn't and it's better to be safe and just back it up
		if err != nil {
			metadataChanged = true
		} else {
			if linkTarget != dbRecordProperties.LinkTarget {
				metadataChanged = true
			}
		}
	}
	return
}

// prepares sql entry for a file record
// parameters: $path is the full path to the object, $stat is the result from os.lstat or os.stat; $ctime is the change
// time of the object and is passed in as it's an expensive system call and other function will most likely already
// have obtained the value
func PrepareFileRecord(path string, stat os.FileInfo, backupConfig config.Backup, ctime time.Time, checksum string, jobUuid string) (shared.BackedUpFileProperties, error) {
	ctime, err := fileproperties.GetCtime(path)
	if err != nil {
		ctime = time.Time{}
	}

	// even if we get an error (and we don't have complete or any file properties) we will still attempt to back it up
	owner, permissions, _ := fileproperties.GetObjectPermissions(path, stat) // #nosec
	onDiskObjectProperties := shared.BackedUpFileProperties{
		Path:       path,
		Type:       utils.FileType(stat),
		Size:       stat.Size(),
		Mtime:      stat.ModTime(),
		Ctime:      ctime,
		Owner:      owner,
		Permissons: permissions,
		Checksum:   checksum,
		JobUuid:    jobUuid,
		Encrypted:  backupConfig.Encrypt,
	}
	if checksum != "" {
		// for now we support only md5 checksumming, but we have room to implement something else, if needed, later
		onDiskObjectProperties.ChecksumType = "md5"
	}
	if onDiskObjectProperties.Type == "symlink" {
		// get symlink target
		onDiskObjectProperties.LinkTarget, err = os.Readlink(path)
		return onDiskObjectProperties, err
	}

	// if we got here than all was fine
	return onDiskObjectProperties, nil
}

// uploads an object (file / dir / symlink) to the remote object storage. For dirs/symlinks it only uploads metadata
// for files it uploads both content and metadata. This function is also responsible for adding data in the
// remote_files DB table
// return values: string with the remote object path(for example bucket_name/path/to/file), bool with true if backup
// got cancelled, false otherwise ; error if error encountered
func UploadObject(ctx context.Context, path string, newDbRecord shared.BackedUpFileProperties,
	backupConfig config.Backup, objectStore objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface) (string, bool, error) {
	// TODO - use the context and pass it further down
	if newDbRecord.Type == "file" {
		logger.Debugf("Uploading '%s'", path)
	} else {
		logger.Debugf("Uploading metadata for '%s' which is of type '%s'", path, newDbRecord.Type)
	}

	result, cancelled, err := objectStore.Upload(path, newDbRecord, backupJobsState)
	if cancelled {
		return "", true, err
	}
	if err != nil {
		return "", false, err
	}

	// $result represents the remote path (in the object store) where the object has been backed up
	storeName, _ := objectStore.GetStoreDetails()
	logger.Debugf("'%s' successfully uploaded to object store %s at remote location '%s'", path, storeName, result)

	return result, false, nil
}

// updates remote metadata for an object (file / dir / symlink) to the remote object storage. The object must already
// have been uploaded
// params: $ctx for canceable context; $path with absolute path to object being backed up; $newDbRecord has all of the
// details about the object which will be partially used for the metadata; $backupConfig is the struct with the details
// of this backup as represented in the config file
// return values: string with the remote object path(for example bucket_name/path/to/file), bool with true if backup
// got cancelled, false otherwise ; error if error encountered
func UpdateObjectMetadata(ctx context.Context, path string, newDbRecord shared.BackedUpFileProperties,
	backupConfig config.Backup, objectStore objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface) (string, bool, error) {
	// TODO - use the context and pass it further down
	logger.Debugf("Updating remote stored metadata for previously backed up and unchanged '%s'", path)

	result, cancelled, err := objectStore.MetadataUpdate(path, newDbRecord)
	if cancelled {
		return "", true, err
	}
	if err != nil {
		return "", false, err
	}

	// $result represents the remote path (in the object store) where the object has been backed up
	storeName, _ := objectStore.GetStoreDetails()
	logger.Debugf("'%s' successfully uploaded to object store %s at remote location '%s'", path, storeName, result)

	return result, false, nil
}

// update counters in the backup job state struct. Params: $backupJobsState is the shared struct (pointer to it)
// where all the state is kept; $backupName corresponds to the name of the backup job as defined in the configuration
// file; $operationType must be one of "upload" or "update"; fileType is one of: "file", "dir", "symlink" or "unknown";
// $path os used only for logging errors and it represents the full path of the file/dir/symlink being backed up; if
// $err != nil then the counter to be updated will be one of "failure" type and otherwise its a "success" one
func updateCounters(backupJobsState shared.BackupJobsStateInterface, backupName string, operationType string, fileType string, path string, err error) {
	switch operationType {
	case "upload":
		{
			if err != nil {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "failed_to_upload_files", path, fileType, "upload", err.Error())
				case "dir":
					backupJobsState.IncrementCounter(backupName, "failed_to_upload_directories", path, fileType, "metadata", err.Error())
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "failed_to_upload_symlinks", path, fileType, "metadata", err.Error())
				default:
					{
						backupJobsState.IncrementCounter(backupName, "failed_to_upload_unknown", path, fileType, "metadata", err.Error())
						logger.Warningf("'%s' is of an unknown type. Only directories, regular files and "+
							"symlinks are supported for backup. Consider excluding this file from backup in order "+
							"to prevent future warnings.", path)
					}
				}
			} else {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "uploaded_files", path, fileType, "upload", "")
				case "dir":
					backupJobsState.IncrementCounter(backupName, "uploaded_directories", path, fileType, "metadata", "")
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "uploaded_symlinks", path, fileType, "metadata", "")
				default:
					logger.Warningf("Tried to increment 'uploaded' counter for '%s' of type: '%s'. "+
						"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
				}
			}
		}
	case "update":
		{
			if err != nil {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_files", path, fileType, "metadata", err.Error())
				case "dir":
					backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_directories", path, fileType, "metadata", err.Error())
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_symlinks", path, fileType, "metadata", err.Error())
				default:
					logger.Warningf("Tried to increment 'failed_to_update_metadata' counter for '%s' of type: '%s'. "+
						"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
				}
			} else {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "updated_metadata_for_files", path, fileType, "metadata", "")
				case "dir":
					backupJobsState.IncrementCounter(backupName, "updated_metadata_for_directories", path, fileType, "metadata", "")
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "updated_metadata_for_symlinks", path, fileType, "metadata", "")
				default:
					logger.Warningf("Tried to increment 'updated_metadata' counter for '%s' of type: '%s'. "+
						"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
				}
			}
		}
	case "up_to_date":
		{
			switch fileType {
			case "file":
				backupJobsState.IncrementCounter(backupName, "up_to_date_files", path, fileType, "up_to_date", "")
			case "dir":
				backupJobsState.IncrementCounter(backupName, "up_to_date_directories", path, fileType, "up_to_date", "")
			case "symlink":
				backupJobsState.IncrementCounter(backupName, "up_to_date_symlinks", path, fileType, "up_to_date", "")
			}
		}
	case "mark_deleted":
		{
			if err != nil {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "failed_to_mark_deleted_files", path, fileType, "mark_deleted", err.Error())
				case "dir":
					backupJobsState.IncrementCounter(backupName, "failed_to_mark_deleted_directories", path, fileType, "mark_deleted", err.Error())
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "failed_to_mark_deleted_symlinks", path, fileType, "mark_deleted", err.Error())
				}
			} else {
				switch fileType {
				case "file":
					backupJobsState.IncrementCounter(backupName, "marked_deleted_files", path, fileType, "mark_deleted", "")
				case "dir":
					backupJobsState.IncrementCounter(backupName, "marked_deleted_directories", path, fileType, "mark_deleted", "")
				case "symlink":
					backupJobsState.IncrementCounter(backupName, "marked_deleted_symlinks", path, fileType, "mark_deleted", "")
				}
			}
		}
	default:
		logger.Errorf("Tried to update counters for operation of type '%s' during a backup having name '%s' "+
			"for object '%s'. This is bug, please report it.", operationType, backupName, path)
	}
}

// runs a PreRunScript or a PostRunScript
func RunPrePostScript(path string, scriptType string, backupName string, jobId string) error {
	logger.Infof("Running %s_run_script '%s'", scriptType, path)
	logger.Debugf("Running (without the single quotes): '%s' '%s'", path, jobId)
	var cmd *exec.Cmd
	// on Windows, to run Powershell scripts, you need to call powershell.exe itself
	if strings.ToLower(filepath.Ext(path)) == ".ps1" && runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-File", path, jobId) // #nosec
	} else {
		cmd = exec.Command(path, jobId) // #nosec
	}
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		msg := fmt.Sprintf("While executing %s_run_script '%s', encountered error: %s\nScript "+
			"output was: %s", scriptType, path, err, stdoutStderr)
		logger.Error(msg)
		return errors.New(msg)
	}
	// if we got here, all was good
	return nil
}

// this must be ran after a backup job has completed and it will mark deleted all files/dir/symlinks which are not
// listed in the current backup and also don't exist any more on this but are mentioned in the "files" table
// $maxResults represents how many records should be fetched from the DB for processing. If the limit is hit then
// after processing the results, the function will recurse, calling itself again with said limit until all DB records
// are processed
// returns: true if cancelled (via context)
func FindAndMarkDeleted(ctx context.Context, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string, maxResults int) bool {
	// locate all objects which are not mentioned in the current backup

	var foundEntries []string
	// the first target name should be sufficient as both targets
	rows, err := dbData.PreparedStatements.FindDeletedItemsStmt.Query(jobUuid, backupConfig.Target[0].Name, maxResults)
	if err != nil {
		logger.Errorf("While querying the database in order to find files which are deleted, encountered error: %s", err)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", "", "", "mark_deleted", err.Error())
		return false
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement for finding deleted items, the "+
				"following error was encountered: %s", err)
		}
	}()
	for rows.Next() {
		select {
		case <-ctx.Done():
			{
				logger.Infof("cancelling running backup job '%s'", backupConfig.Name)
				err := rows.Close()
				if err != nil {
					logger.Warningf("While trying to close/cancel a DB query, encountered error: %s", err)
				}
				return true
			}
		default:
			var path string
			err := rows.Scan(&path)
			if err != nil {
				logger.Errorf("While retrieving the database record for deleted item, the following error "+
					"was encountered: '%s'", err)
				backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", "", "", "mark_deleted", err.Error())
				continue
			}
			// we build first the list of found items (in a slice) and then process them (after the DB query is completed)
			// in order to avoid blocking (forever) when we try to start a new DB transaction due to this query still
			// being in progress. This is due to various limitations of Sqlite and how we work with it. There are
			// alternatives but they have caveats.
			foundEntries = append(foundEntries, path)
		}
	}
	for _, path := range foundEntries {
		select {
		case <-ctx.Done():
			{
				logger.Infof("cancelling running backup job '%s'", backupConfig.Name)
				err := rows.Close()
				if err != nil {
					logger.Warningf("While trying to close/cancel a DB query, encountered error: %s", err)
				}
				return true
			}
		default:
			{
				dbEntryFound, dbRecordProperties, err := getBackedupObjectPropertiesFromDb(path, dbData)
				if err != nil {
					backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", path, "", "mark_deleted", err.Error())
					logger.Warningf("While searching the database for an object record for path: '%s',"+
						" encountered error: %s", path, err)
					continue
				}
				if dbEntryFound {
					// process path (mark it as deleted in the files table and in remote object store)
					logger.Debugf("Marking '%s' '%s' as deleted as it no longer exists on disk", dbRecordProperties.Type, path)
					backupJobsState.IncrementSequence(backupConfig.Name) // <-- needed so Watch clients consider the message as for a different item than the previous one
					cancelled, err := markDeleted(dbRecordProperties, backupConfig, dbData, objectStores, backupJobsState, jobUuid)
					if cancelled {
						return true
					}
					updateCounters(backupJobsState, backupConfig.Name, "mark_deleted", dbRecordProperties.Type, dbRecordProperties.Path, err)
				} else {
					backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", path, "", "mark_deleted", err.Error())
					logger.Warningf("Path '%s' no longer appears in the file table so it can't be properly marked as deleted", path)
					continue
				}
			}
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to find deleted items, "+
			"the following error was encountered: %s", err)
		backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", "", "", "mark_deleted", err.Error())
		return false
	}

	// if the DB query returned the max number of results then recurse until we get less that $maxResults results
	if len(foundEntries) >= maxResults {
		cancelled := FindAndMarkDeleted(ctx, backupConfig, dbData, objectStores, backupJobsState, jobUuid, maxResults)
		return cancelled
	}
	return false
}

// for a given $path, it marks it as deleted by adjusting the DB entries and also updating metadata on the objectstore(s)
// returns true if the function was cancelled, false otherwise; encountered error if any
func markDeleted(ObjectDbRecord shared.BackedUpFileProperties, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string) (bool, error) {
	// for each found object check if it exists on disk. This must be done because there is a chance that the backup
	// failed for some items and so they don't appear in the list of backed up items but they still exist on disk so
	// their reference from the "files" table should not be removed
	if ObjectDbRecord.Type == "dir" {
		// in this context, dereferencing does not make sense no matter what is in the config file
		_, err := utils.DirExists(ObjectDbRecord.Path, false)
		if err == nil {
			// dir still exists on disk , skip marking it deleted
			return false, nil
		}
	} else {
		// in this context, dereferencing does not make sense no matter what is in the config file
		_, err := utils.FileExists(ObjectDbRecord.Path, false)
		if err == nil {
			// file/symlink path still exists on disk,, skip marking it deleted
			return false, nil
		}
	}
	/*
		There is a chance that the above will cause an inconsistency if the object type changed from dir to file/symlink
		(or the other way around) and also during the backup this path failed to be uploaded. In this case it will be marked
		as deleted and the next backup should pick it up. The inconsistencu is that in the DB it will appear as dir -> deleted -> file
		for that path (or the other way around) but in reality the "deleted" state may have been very short lived. In order
		to avoid this we would have to double the amount of "stat" system calls and that is expensive for getting rid of
		this edge case
	*/

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to backup this file.
		return false, errors.New(fmt.Sprintf("While trying to start a database transaction for deletion marking, "+
			"encountered error: %s", err))
	}

	// delete path entry for this path, from the "files" table
	_, err = dbtx.Exec(dbData.PreparedStatements.FilesDelete, ObjectDbRecord.Path)
	if err != nil {
		logger.Errorf("While trying to delete from the 'files' table the entry for '%s', encountered error: %s", ObjectDbRecord.Path, err)
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", ObjectDbRecord.Path, txerr)
		}
		return false, err
	}

	encounteredError := 0
	var encounteredErrorObject error
	var JobCancelled bool
	// back up the object to one or more remote object stores
	for _, objectStore := range objectStores {
		remotePath, cancelled, err := objectStore.MarkDeleted(ObjectDbRecord.Path, ObjectDbRecord)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		if cancelled {
			JobCancelled = true
			break
		}
		targetName, _ := objectStore.GetStoreDetails()
		_, err = addEntryToRemoteFiles(remotePath, targetName, jobUuid, 1, dbData, dbtx, ObjectDbRecord)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		// We will NOT add an entries to backup collections as during a restore there is nothing to do with a delete
		// marker (and the purpose of the backup_collections table is to list what needs to be restored)
	}
	if encounteredError > 0 || JobCancelled {
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", ObjectDbRecord.Path, txerr)
		}
		if len(objectStores) > 1 {
			// TODO - ENSURE THAT ANY SUCCESSFULLY ADDED DELETE_MARKER (FOR OTHER OBJECT STORES WHICH SUCCEEDED) IS REMOVED
			logger.Warnf("Failed adding delete marker for '%s' to %d out of %d targets. All targets are to be "+
				"considered failed (even the ones where adding the delete marker was successful) for this item.",
				ObjectDbRecord.Path, encounteredError, len(objectStores))
		}
		if JobCancelled {
			return true, nil
		} else {
			return false, encounteredErrorObject
		}
	}

	// end, all was good until here
	txerr := dbtx.Commit()
	if txerr != nil {
		return false, errors.New(fmt.Sprintf("Could not commit transaction due to error: %s", err))
	}
	return false, nil
}
