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
	"github.com/gofrs/uuid"
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

// struct use for temporarily accounting what upload or mark_deleted operations where successful
type successfullyProcessed struct {
	Path string
	// one of "file"/"dir"/"symlink"
	ObjType       string
	Version       int
	RemoteVersion string
	Objectstore   objectstore.ObjectStore
}

// performs backup of a file or dir
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func Do(ctx context.Context, path string, stat os.FileInfo, backupConfig config.ConfigBackup, dbData shared.DbData,
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
				return false, fmt.Errorf("while searching the database for an object record, encountered error: %s", err)
			}
			// if a db entry is found then this object has been previously backed up so it needs to be verified if the
			// object has changed
			if dbEntryFound {
				logger.Debugf("Found DB entry for %s", path)
				// check if properties match between DB record and os.FileInfo
				contentChanged, metadataChanged, ctime, checksum := needsUpload(path, stat, dbRecordProperties, backupConfig.Checksum, backupConfig.Dereference, backupConfig.Encrypt)
				updatedDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobUuid)
				if err != nil {
					// something bad enough happened that we don't have a usable db record so we can't proceed to
					// backup this file
					updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
					return false, fmt.Errorf("could not prepare an updated db record due to error: %s", err)
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
						logger.Debugf("No change detected(it's up to date) for %s", path)
						// object is up to date (aka we got a copy in the backup)
						// add entry to backup_collections so this file would also be included in a restore
						var foundErr error
						for _, objStore := range objectStores {
							targetName, _ := objStore.GetStoreDetails()
							remoteFilesUuid, err := getNewestRemoteFileUuid(dbData, path, targetName)
							if err != nil {
								foundErr = err
								logger.Error(foundErr)
							} else {
								_, err = dbData.PreparedStatements.BackupCollectionsInsertStmt.Exec(remoteFilesUuid, jobUuid, targetName)
								if err != nil {
									foundErr = fmt.Errorf("despite no change detected for '%s', could not add entry to backup_collections"+
										" table due to error %s . This means that if a restore is selected for this particular backup job id, then this file "+
										"won't be restored despite the fact that a previous run ensured it is backed up", path, err)
									logger.Error(foundErr)
								}
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
func backupExistingWithMetadataChange(ctx context.Context, path string, stat os.FileInfo, backupConfig config.ConfigBackup, dbData shared.DbData,
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
func backupExistingWithContentChange(ctx context.Context, path string, stat os.FileInfo, backupConfig config.ConfigBackup, dbData shared.DbData,
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
func backupNewItem(ctx context.Context, path string, stat os.FileInfo, backupConfig config.ConfigBackup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string) (bool, error) {
	var err error

	checksum := ""
	if backupConfig.Checksum && utils.FileType(stat) == "file" {
		checksum, err = utils.GetFileMD5Sum(path)
		if err != nil {
			logger.Errorf("While trying to calculate MD5 for '%s' encountered error: %s . Will use a UUID as a "+
				"checksum in order to have the correct one added during the next backup run", path, err)
			u, err := uuid.NewV4()
			if err != nil {
				logger.Errorf("Could not generate a UUID so the backup for item '%s' can't proceed. Encountered error is: %s", path, err)
				return false, err
			}
			checksum = u.String()
		}
	}
	ctime, err := fileproperties.GetCtime(path, backupConfig.Dereference)
	if err != nil {
		logger.Debugf("For '%s' could not establish ctime due to error: %s ; using current time as ctime", path, err)
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
func UploadAndUpdateDB(operation string, ctx context.Context, path string, stat os.FileInfo, backupConfig config.ConfigBackup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string, DbRecord shared.BackedUpFileProperties) (bool, error) {
	// $operationType is used for counters, status messages and also for deciding if the file needs sending to the
	// remote or if just a DB updated for properties only is needed
	var operationType string
	switch operation {
	case "new":
		operationType = "upload"
	case "content-update":
		operationType = "upload"
	case "metadata-update":
		operationType = "update"
	default:
		return false, fmt.Errorf("unknown operation: %s . Allowed operations are:  'new', 'content-update', 'metadata-update'", operation)
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to backup this file.
		updateCounters(backupJobsState, backupConfig.Name, operationType, utils.FileType(stat), path, err)
		return false, fmt.Errorf("while trying to start a database transaction "+
			"encountered error: %s", err)
	}
	if operation == "new" {
		err = addDbEntryToFiles(dbData, dbtx, DbRecord)
	} else {
		err = updateDbEntryInFiles(dbData, dbtx, DbRecord)
	}

	if err != nil {
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", path, txerr)
		}
		// could not add dbentry to the database so we can't proceed to backup this file.
		updateCounters(backupJobsState, backupConfig.Name, operationType, utils.FileType(stat), path, err)
		return false, fmt.Errorf("while trying to add new file object DB entry encountered error: %s", err)
	}
	encounteredError := 0
	var encounteredErrorObject error
	var JobCancelled bool
	var processed []successfullyProcessed
	// back up the object to one or more remote object stores
	for _, objectStore := range objectStores {
		targetName, _ := objectStore.GetStoreDetails()
		// figure out the next version number
		version, err := calcRemoteFileVersion(dbData, dbtx, path, targetName)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		var cancelled bool
		var remoteVersion string
		if operationType == "upload" {
			remoteVersion, cancelled, err = UploadObject(DbRecord, backupConfig, objectStore, backupJobsState, version)
			if err != nil {
				encounteredError++
				encounteredErrorObject = err
				break
			}
			if cancelled {
				JobCancelled = true
				break
			}
			// append upload details here in case we need to roll back (as we will have to delete the uploaded content)
			processed = append(processed, successfullyProcessed{
				Path:          DbRecord.Path,
				ObjType:       DbRecord.Type,
				Version:       version,
				RemoteVersion: remoteVersion,
				Objectstore:   objectStore,
			})
		}
		// in case of operationType == update we don't have to actually send/update the object store; we'll just update the DB entry
		if operationType == "update" {
			// use whatever was the last remote_version instead of a new one
			if version > 1 {
				// get from db the "remote_version" of the previous $version
				remoteVersion, err = getRemoteVersionForVersion(dbData, dbtx, path, targetName, version-1)
				if err != nil {
					encounteredError++
					encounteredErrorObject = err
					break
				}
			} else {
				err = fmt.Errorf("object %s was detected to have had a metadata change but no previous backed "+
					"up version can be found for it in the database", path)
				log.Error(err)
				encounteredError++
				encounteredErrorObject = err
				break
			}
		}

		remoteUuid, err := addDbEntryToRemoteFiles(targetName, jobUuid, 0, dbData, dbtx, DbRecord, version, remoteVersion)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		_, err = dbtx.Exec(dbData.PreparedStatements.BackupCollectionsInsert, remoteUuid, jobUuid, targetName)
		if err != nil {
			encounteredError++
			encounteredErrorObject = fmt.Errorf("for '%s' could not add entry to backup_collections"+
				" table due to error %s", path, err)
			break
		}
	}
	if encounteredError > 0 || JobCancelled {
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", path, txerr)
		}
		if len(objectStores) > 1 && operationType == "upload" {
			logger.Warnf("Failed upload of '%s' to %d out of %d targets. All targets are be "+
				"considered failed (even the ones where the backup was successful) for this item.",
				path, encounteredError, len(objectStores))
		}
		// ensure that any successfully uploaded file/dir/symlink is removed
		for _, entry := range processed {
			err = entry.Objectstore.Delete(entry.Path, entry.ObjType, entry.Version, entry.RemoteVersion)
			if err != nil {
				logger.Warnf("After failed upload for '%s', while trying to cleanup, encountered error: %s", path, err)
			}
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
		return false, fmt.Errorf("could not commit transaction due to error: %s", err)
	}
	// if we got here then all was good
	updateCounters(backupJobsState, backupConfig.Name, operationType, DbRecord.Type, path, nil)
	return false, nil
}

// $target is one target name for the backup section definition in use. This target is to the objecstore where this file/dir/symlink got sent/updated
// $deleteMarker: 1 for true, 0 for false. If 1 it means the file was deleted from the local filesystem
// $version represents the object version number to add in the database for this item while $remoteVersion represents
// the version as returned by the object store after the PUT operation (there may be a difference between the two, depending on Object Store capabilities)
//
// Returns the uuid value for this entry and if an error was encountered or not. If err then ignore the uuid value.
func addDbEntryToRemoteFiles(target string, jobUuid string, deleteMarker int, dbData shared.DbData,
	dbtx *sql.Tx, fileDbRecord shared.BackedUpFileProperties, version int, remoteVersion string) (string, error) {
	u, err := uuid.NewV4()
	if err != nil {
		logger.Errorf("Could not generate a UUID so the backup for '%s' can't complete. Encountered error is: %s", fileDbRecord.Path, err)
		return "", err
	}
	entryUuid := u.String()

	_, err = dbtx.Exec(dbData.PreparedStatements.RemoteFilesInsert, entryUuid, fileDbRecord.Path, target,
		time.Now().UnixNano(), jobUuid, deleteMarker, version, remoteVersion, runtime.GOOS, fileDbRecord.Type,
		fileDbRecord.LinkTarget, fileDbRecord.Size, fileDbRecord.Mtime.UnixNano(),
		fileDbRecord.Ctime.UnixNano(), fileDbRecord.Owner,
		fileDbRecord.Permissions, fileDbRecord.Checksum, fileDbRecord.ChecksumType, fileDbRecord.Encrypted)
	if err != nil {
		return "", fmt.Errorf("while trying to add a db record for '%s' in the remote_files table, "+
			"encountered error: %s", fileDbRecord.Path, err)
	}
	return entryUuid, nil
}

// adds one entry to "files" table and returns whatever error is encountered
func addDbEntryToFiles(dbData shared.DbData, dbtx *sql.Tx, fileDbRecord shared.BackedUpFileProperties) error {
	_, err := dbtx.Exec(dbData.PreparedStatements.FilesInsert, fileDbRecord.Path, fileDbRecord.Type,
		fileDbRecord.LinkTarget, fileDbRecord.Size, fileDbRecord.Mtime.UnixNano(),
		fileDbRecord.Ctime.UnixNano(), fileDbRecord.Owner,
		fileDbRecord.Permissions, fileDbRecord.Checksum, fileDbRecord.ChecksumType, fileDbRecord.Encrypted,
		fileDbRecord.JobUuid)
	return err
}

// updates and entry in the "files" table and returns whatever error is encountered
func updateDbEntryInFiles(dbData shared.DbData, dbtx *sql.Tx, fileDbRecord shared.BackedUpFileProperties) error {
	result, err := dbtx.Exec(dbData.PreparedStatements.FilesUpdate, fileDbRecord.Type,
		fileDbRecord.LinkTarget, fileDbRecord.Size, fileDbRecord.Mtime.UnixNano(),
		fileDbRecord.Ctime.UnixNano(), fileDbRecord.Owner,
		fileDbRecord.Permissions, fileDbRecord.Checksum, fileDbRecord.ChecksumType, fileDbRecord.Encrypted,
		fileDbRecord.JobUuid, fileDbRecord.Path)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("update should have changed 1 row for but it changed %d rows", rows)
	}
	return nil
}

// For a given file path and a backup target name calculate version
// returns an increment of the largest found version and nil; if an error is encountered then it returns 0 and the error; if no entry is found then it returns 1 and nil
func calcRemoteFileVersion(dbData shared.DbData, dbtx *sql.Tx, localPath string, targetName string) (int, error) {
	rows, err := dbtx.Query(dbData.PreparedStatements.RemoteFilesQueryNewestVersion, localPath, targetName)
	if err != nil {
		logger.Errorf("While querying the database in order to calculate a version number for '%s' the "+
			"following error was encountered: %s", localPath, err)
		return 0, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a the database query used "+
				"to calculate a version number for '%s' the following error was encountered: %s", localPath, err)
		}
	}()
	var version int
	entryFound := false
	for rows.Next() {
		if entryFound {
			logger.Warnf("While querying the database in order to calculate a version number for '%s' belonging"+
				" to target '%s' found more than one 'highest version' number."+
				"Using first match and ignoring the rest", localPath, targetName)
			continue
		}
		entryFound = true
		err := rows.Scan(&version)
		if err != nil {
			logger.Errorf("While retrieving the database record for version belonging to '%s', the"+
				" following error was encountered: '%s'", targetName, err)
			return 0, err
		}
	}
	if err = rows.Err(); err != nil {
		logger.Warnf("While trying to Close() a the database query used "+
			"to calculate a version number for '%s' the following error was encountered: %s", localPath, err)
		return 0, err
	}
	if entryFound {
		return version + 1, nil
	} else {
		return 1, nil
	}
}

// for a given $version && $targetName && $localPath return from the DB the "remote_version" field
func getRemoteVersionForVersion(dbData shared.DbData, dbtx *sql.Tx, localPath string, targetName string, version int) (string, error) {
	rows, err := dbtx.Query(dbData.PreparedStatements.RemoteFilesQueryRemoteVersion, localPath, targetName, version)
	if err != nil {
		logger.Errorf("While querying the database in order to find the remote_version for '%s' and version '%d' belonging to target '%s' the "+
			"following error was encountered: %s", localPath, version, targetName, err)
		return "", err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a prepared statement used for querying the database in order "+
				"to calculate a version number for '%s' the following error was encountered: %s", localPath, err)
		}
	}()
	var remoteVersion string
	entryFound := false
	for rows.Next() {
		if entryFound {
			logger.Warnf("More than one database record for '%s' and version '%d' belonging to target '%s'. "+
				"Using first match and ignoring the rest", localPath, version, targetName)
			continue
		}
		entryFound = true
		err := rows.Scan(&remoteVersion)
		if err != nil {
			logger.Errorf("While retrieving the database record for remote_file version belonging to '%s', the"+
				" following error was encountered: '%s'", targetName, err)
			return "", err
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to find the remote_version"+
			" for '%s' , the following error was encountered: %s", localPath, err)
		return "", err
	}
	if entryFound {
		return remoteVersion, nil
	} else {
		return "", fmt.Errorf("could not find remote_version for %s", localPath)
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
		break //nolint:staticcheck
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database for the remote uuid for '%s' "+
			", the following error was encountered: %s", localPath, err)
		return "", err
	}
	if !entryFound {
		return "", fmt.Errorf("could not find the uuid for previously backed up object '%s'", localPath)
	}

	if remoteUuid == "" {
		return "", fmt.Errorf("found an empty uuid for previously backed up object '%s'", localPath)
	}
	return remoteUuid, nil
}

// check if a given path exists in the Database;
// returns the following values: bool depicting if an entry was found or not; if found a populated
// shared.BackedUpFileProperties object containing all of the properties of given object as extracted from the DB
// record; an error object if an error is encountered
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
		var tmpMtime, tmpCtime int64
		err := rows.Scan(&dbRecord.Path, &dbRecord.Type, &dbRecord.LinkTarget, &dbRecord.Size, &tmpMtime,
			&tmpCtime, &dbRecord.Owner, &dbRecord.Permissions, &dbRecord.Checksum,
			&dbRecord.ChecksumType, &dbRecord.Encrypted, &dbRecord.JobUuid)
		if err != nil {
			logger.Errorf("While retrieving the database record for '%s' the following error was encountered:"+
				" '%s'", path, err)
			return false, shared.BackedUpFileProperties{}, err
		} else {
			// convert string to time for  mtime  and  ctime
			if tmpMtime > 0 {
				dbRecord.Mtime = time.Unix(0, tmpMtime)
			} else {
				logger.Errorf("The database record for '%s' has Mtime in nanoseconds %d which is an invalid value", path, tmpMtime)
				return false, shared.BackedUpFileProperties{}, errors.New("mtime read from DB is <= 0")
			}
			if tmpCtime > 0 {
				dbRecord.Ctime = time.Unix(0, tmpCtime)
			} else {
				logger.Errorf("The database record for '%s' has Ctime in nanoseconds %d which is an invalid value", path, tmpCtime)
				return false, shared.BackedUpFileProperties{}, errors.New("ctime read from DB is <= 0")
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

// compares on disk state vs db ; in terms of parameters $dereference is true if symlinks should be dereferenced
// (this basically corresponds to the entry in the backup configuration file); $encrypt corresponds to the Backup config
// section so we'll check if the already stored file matches the encryption state set in the config
// and returns:
// bool with value true if file changed and it needs upload (this implies a metadata upload is needed too); bool with
// value true when a metadata change was detected but the file content itself remains unchanged  ; time.Time containing
// ctime populated when either file content or metadata change was detected. This is done because it is expensive to
// get ctime (1 system call) and we want to avoid calling this again later; $checksum empty if an error was encountered
// while trying to calculate it or if checksum comparison was not requested, otherwise ascii string with md5 sum
func needsUpload(path string, stat os.FileInfo, dbRecordProperties shared.BackedUpFileProperties, compareChecksum bool, dereference bool, encrypt bool) (contentChanged bool,
	metadataChanged bool, ctime time.Time, checksum string) {
	var err error
	objectType := utils.FileType(stat)
	if dbRecordProperties.Encrypted != encrypt && objectType == "file" {
		logger.Debugf("For '%s' we got a mismatch between what the config file says regarding encryption and"+
			" the already stored DB record so we'll consider the content of the file changed and have it backed up", path)
		contentChanged = true
	} else if compareChecksum && objectType == "file" {
		checksum, err = utils.GetFileMD5Sum(path)
		if err != nil {
			// if we got any errors means we could not calculate the checksum so to be safe, we consider that the file needs to be uploaded
			contentChanged = true
			logger.Debugf("Checksum could not be calculated for '%s' so its safer to consider the content to be changed", path)
		} else if checksum != dbRecordProperties.Checksum {
			logger.Debugf("Checksum change detected for '%s'", path)
			contentChanged = true
		}
		// if size or mtime differs then we got a file change (we exclude directories from Size check as that represents the number of items in a dir, not a property of the dir itself)
	} else if dbRecordProperties.Type != "dir" && stat.Size() != dbRecordProperties.Size {
		logger.Debugf("Size change detected for '%s'", path)
		contentChanged = true
	} else if !stat.ModTime().Equal(dbRecordProperties.Mtime) {
		logger.Debugf("Mtime change detected for '%s'", path)
		contentChanged = true
		// if type changed then we need to back it up (for example in the DB it's marked as a symlink but on disk it's a file now
	} else if objectType != dbRecordProperties.Type {
		contentChanged = true
	}

	ctime, err = fileproperties.GetCtime(path, dereference)
	// Ctime change signals that one of Owner of permissions has changed for this item (so we won't also test for
	// individual changed owner or permissions)
	// in case of error we just treat it as the metadata changed as we can't know for sure if it didn't and it's better to be safe and just back it up
	if err != nil {
		logger.Debugf("Could not get ctime so to be safe considering that Ctime change detected for '%s'", path)
		metadataChanged = true
	} else {
		if !ctime.Equal(dbRecordProperties.Ctime) {
			logger.Debugf("Ctime change detected for '%s'", path)
			metadataChanged = true
		}
	}
	// if we have a symlink, check if the symlink target has changed and if so then update metadata
	if !metadataChanged && objectType == "symlink" {
		linkTarget, err := os.Readlink(path)
		// in case of error we just treat it as the metadata changed as we can't know for sure if it didn't and it's better to be safe and just back it up
		if err != nil {
			logger.Debugf("Could not get link target so to be safe considering that link target change detected for '%s'", path)
			metadataChanged = true
		} else {
			if linkTarget != dbRecordProperties.LinkTarget {
				logger.Debugf("Link target change detected for '%s'", path)
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
func PrepareFileRecord(path string, stat os.FileInfo, backupConfig config.ConfigBackup, ctime time.Time, checksum string, jobUuid string) (shared.BackedUpFileProperties, error) {
	var err error
	// even if we get an error (and we don't have complete or any file properties) we will still attempt to back it up
	owner, permissions, err := fileproperties.GetObjectPermissions(path, stat)
	if err != nil {
		logger.Warnf("Could not get permissions for '%s' due to error: %s", path, err)
	}
	onDiskObjectProperties := shared.BackedUpFileProperties{
		Path:        path,
		Type:        utils.FileType(stat),
		Size:        stat.Size(),
		Mtime:       stat.ModTime(),
		Ctime:       ctime,
		Owner:       owner,
		Permissions: permissions,
		Checksum:    checksum,
		JobUuid:     jobUuid,
		Encrypted:   backupConfig.Encrypt,
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

	// directories should be recorded as having size 0 as the size value makes no sense for a backup
	// purpose and size changes would trigger new backups (at least on Linux a directories size will increase as inodes are added)
	if onDiskObjectProperties.Type == "dir" {
		onDiskObjectProperties.Size = 0
	}

	// if we got here than all was fine
	return onDiskObjectProperties, nil
}

// uploads an object (file / dir / symlink) to the remote object storage. For dirs/symlinks it only uploads metadata
// for files it uploads both content and metadata.
// return values: the version of the stored item, as returned by the object store;
// bool with true if backup got cancelled, false otherwise ; error if error encountered
func UploadObject(newDbRecord shared.BackedUpFileProperties,
	backupConfig config.ConfigBackup, objectStore objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, version int) (string, bool, error) {
	if newDbRecord.Type == "file" {
		logger.Debugf("Uploading '%s'", newDbRecord.Path)
	} else {
		logger.Debugf("Uploading metadata for '%s' which is of type '%s'", newDbRecord.Path, newDbRecord.Type)
	}

	remoteVersion, cancelled, err := objectStore.Upload(newDbRecord, version, backupJobsState)
	if cancelled {
		return remoteVersion, true, err
	}
	if err != nil {
		return remoteVersion, false, err
	}

	// $result represents the remote path (in the object store) where the object has been backed up
	storeName, _ := objectStore.GetStoreDetails()
	logger.Debugf("'%s' successfully uploaded to object store %s", newDbRecord.Path, storeName)

	return remoteVersion, false, nil
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
func FindAndMarkDeleted(ctx context.Context, backupConfig config.ConfigBackup, dbData shared.DbData,
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
					backupJobsState.IncrementCounter(backupConfig.Name, "failed_to_find_deleted", path, "", "mark_deleted", "")
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
func markDeleted(ObjectDbRecord shared.BackedUpFileProperties, backupConfig config.ConfigBackup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface, jobUuid string) (bool, error) {
	// establish it path is part of exclusions. This is to cover an edge case where a previously backed up path has been
	// added to the exclusions list. If this is the case then we want to have it mark deleted (and skip checking further
	// below if the path exists on disk)
	isExcluded, _, _ := utils.IsPathExcluded(backupConfig.Exclusions, ObjectDbRecord.Path) // #nosec
	// establish if path is included in paths to be backed up. If not then we want to have it marked as deleted as this
	// is an edge case where a a particular path has been removed from the Backup configuration but previously backed
	// up files located underneath the path are still mentioned in the DB. Even if the files are still on disk, we want
	// to remove mentions from the "files" table and also from the object store as for all intents and purposes the
	// file is as "deleted" from that point of view
	isIncluded, _ := utils.IsPathIncluded(backupConfig.Paths, ObjectDbRecord.Path)

	if (!isExcluded) && isIncluded {
		// for each found object check if it exists on disk. This must be done because there is a chance that the backup
		// failed for some items and so they don't appear in the list of backed up items but they still exist on disk so
		// their reference from the "files" table should not be removed
		logger.Debugf("'%s' is not excluded and is also included so checking if it exists on disk before marking it as deleted", ObjectDbRecord.Path)
		if ObjectDbRecord.Type == "dir" {
			// in this context, dereferencing does not make sense no matter what is in the config file
			_, err := utils.DirExists(ObjectDbRecord.Path, false)
			if err == nil {
				logger.Debugf("Directory '%s' still exists on disk, skipping marking it deleted", ObjectDbRecord.Path)
				return false, nil
			}
		} else {
			// in this context, dereferencing does not make sense no matter what is in the config file
			_, err := utils.FileExists(ObjectDbRecord.Path, false)
			if err == nil {
				logger.Debugf("%s '%s' still exists on disk, skipping marking it deleted", ObjectDbRecord.Type, ObjectDbRecord.Path)
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
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to backup this file.
		return false, fmt.Errorf("while trying to start a database transaction for deletion marking, "+
			"encountered error: %s", err)
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
	var processed []successfullyProcessed
	// back up the object to one or more remote object stores
	for _, objectStore := range objectStores {

		targetName, _ := objectStore.GetStoreDetails()
		version, err := calcRemoteFileVersion(dbData, dbtx, ObjectDbRecord.Path, targetName)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}

		remoteVersion, cancelled, err := objectStore.MarkDeleted(ObjectDbRecord, version)
		if err != nil {
			logger.Debugf("The object store returned an error for the mark deleted operation: %s", err)
			encounteredError++
			encounteredErrorObject = err
			break
		}
		if cancelled {
			JobCancelled = true
			break
		}
		// append upload(mark deleted generally uploads a "marker") details here in case we need to roll back
		// (as we will have to delete the uploaded content)
		processed = append(processed, successfullyProcessed{
			Path:          ObjectDbRecord.Path,
			ObjType:       ObjectDbRecord.Type,
			Version:       version,
			RemoteVersion: remoteVersion,
			Objectstore:   objectStore,
		})
		_, err = addDbEntryToRemoteFiles(targetName, jobUuid, 1, dbData, dbtx, ObjectDbRecord, version, remoteVersion)
		if err != nil {
			encounteredError++
			encounteredErrorObject = err
			break
		}
		// We will NOT add an entries to backup collections as during a restore there is nothing to do with a delete
		// marker (and the purpose of the backup_collections table is to list what needs to be restored)
	}
	if encounteredError > 0 || JobCancelled {
		logger.Warnf("Could not mark '%s' as deleted as an error was encountered: %s", ObjectDbRecord.Path, encounteredErrorObject)
		txerr := dbtx.Rollback()
		if txerr != nil {
			logger.Warningf("Could not rollback transaction for '%s' due to error: %s", ObjectDbRecord.Path, txerr)
		}
		if len(objectStores) > 1 {
			logger.Warnf("Failed adding delete marker for '%s' to %d out of %d targets. All targets are to be "+
				"considered failed (even the ones where adding the delete marker was successful) for this item.",
				ObjectDbRecord.Path, encounteredError, len(objectStores))
		}
		// ensure that any successfully already added delete markers are removed
		for _, entry := range processed {
			err = entry.Objectstore.Delete(entry.Path, entry.ObjType, entry.Version, entry.RemoteVersion)
			if err != nil {
				logger.Warnf("While trying to cleanup delete markers, encountered error: %s", err)
			}
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
		return false, fmt.Errorf("could not commit transaction due to error: %s", err)
	}
	logger.Debugf("DB transaction for marking '%s' as deleted was committed", ObjectDbRecord.Path)
	return false, nil
}
