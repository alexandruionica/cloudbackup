package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"errors"
	"fmt"
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
func Do (ctx context.Context, path string, stat os.FileInfo, backupConfig config.Backup, dbData shared.DbData,
	objectStores []objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface) (bool, error) {
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
				return false, errors.New(fmt.Sprintf("While searching the database for a file object record," +
					" encountered error: %s", err))
			}
			// if a db entry is found then this object has been previously backed up so it needs to be verified if the
			// object has changed
			if dbEntryFound {
				logger.Debugf("Found DB entry for %s", path)
				// check if properties match between DB record and os.FileInfo
				contentChanged, metadataChanged, ctime, checksum := needsUpload(path, stat, dbRecordProperties, backupConfig.Checksum)
				updatedDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum)
				if err != nil {
					// something bad enough happened that we don't have a usable db record so we can't proceed to
					// backup this file
					updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
					return false, errors.New(fmt.Sprintf("Could not prepare an updated db record due to " +
						"error: %s", err))
				}
				if contentChanged {
					logger.Debugf("Content change detected for %s", path)
					encounteredError := 0
					if updatedDbRecord.Type == "unknown" {
						updateCounters(backupJobsState, backupConfig.Name, "upload", updatedDbRecord.Type,
							path, errors.New("unsupported file type"))
						return false, errors.New("unsupported file type")
					}
					var encounteredErrorObject error
					// back up the object to one or more remote object stores
					for _, objectStore := range objectStores {
						cancelled, err := UploadObject(ctx, path, updatedDbRecord, backupConfig, objectStore, backupJobsState)
						if err != nil {
							encounteredError ++
							encounteredErrorObject = err
						}
						if cancelled {
							return true, nil
						}
					}
					if encounteredError > 0 {
						if len(objectStores) > 1{
							logger.Warnf("Failed upload of '%s' to %d out of %d targets", path, encounteredError,  len(objectStores))
						}
						updateCounters(backupJobsState, backupConfig.Name, "upload", updatedDbRecord.Type, path, encounteredErrorObject)
						return false, encounteredErrorObject
					}

					// backup successful
					updateCounters(backupJobsState, backupConfig.Name, "upload", updatedDbRecord.Type, path, nil)
					return false, nil

				} else {
					if metadataChanged {
						logger.Debugf("Metadata change detected for %s", path)
						if updatedDbRecord.Type == "unknown" {
							// report it as a "failed_to_upload_unknown" instead of updated_metadata as we don't support "unknown" files but we want to report somehow this issue
							updateCounters(backupJobsState, backupConfig.Name, "upload", updatedDbRecord.Type, path, errors.New("unsupported file type"))
							return false, errors.New("unsupported file type")
						}
						encounteredError := 0
						var encounteredErrorObject error
						// back up the object metadata to one or more remote object stores
						for _, objectStore := range objectStores {
							// TODO - proceed to update file metadata in the DB and on the remote ???? (to decide what to do with
							// the remote: changed owner is probably something we want to flag, but not much else)
							// back up the object to one or more remote object stores
							cancelled, err := UpdateObjectMetadata(ctx, path, updatedDbRecord, backupConfig, objectStore, backupJobsState)
							if err != nil {
								encounteredError ++
								encounteredErrorObject = err
							}
							if cancelled {
								return true, nil
							}
						}
						if encounteredError > 0 {
							if len(objectStores) > 1{
								logger.Warnf("Failed upload metadata changes of of '%s' to %d out of %d targets",
									path, encounteredError,  len(objectStores))
							}
							updateCounters(backupJobsState, backupConfig.Name, "update", updatedDbRecord.Type, path, encounteredErrorObject)
							return false, encounteredErrorObject
						}
						// backup successful
						updateCounters(backupJobsState, backupConfig.Name, "update", updatedDbRecord.Type, path, nil)
						return false, nil
					}
				}
			// no db record found so this is the first time this object is backed up
			}else{
				logger.Debugf("Did not find a DB entry for %s , this was not previously backed up", path)
				checksum := ""
				if backupConfig.Checksum && utils.FileType(stat) == "file" {
					checksum, err = utils.GetFileMD5Sum(path)
				}
				ctime, err := fileproperties.GetCtime(path)
				if err != nil {
					ctime = time.Time{}
				}
				newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum)
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

				// TODO - seems the targets property is not filled in yet which is a problem
				_, err = dbData.PreparedStatements.InsertStmt.Exec(newDbRecord.Path, newDbRecord.Type,
					newDbRecord.LinkTarget, newDbRecord.Size, newDbRecord.Mtime.Format(time.RFC3339Nano),
					newDbRecord.Ctime.Format(time.RFC3339Nano), newDbRecord.Owner,
					newDbRecord.Permissons, newDbRecord.Checksum, newDbRecord.ChecksumType, newDbRecord.Encrypted,
					newDbRecord.Targets)
				if err != nil {
					// could not add dbentry to the database so we can't proceed to backup this file.
					updateCounters(backupJobsState, backupConfig.Name, "upload", utils.FileType(stat), path, err)
					return false, errors.New(fmt.Sprintf("While trying to add new file object DB entry " +
						"encountered error: %s", err))
				}

				encounteredError := 0
				var encounteredErrorObject error
				// back up the object to one or more remote object stores
				for _, objectStore := range objectStores {
					cancelled, err := UploadObject(ctx, path, newDbRecord, backupConfig, objectStore, backupJobsState)
					if err != nil {
						encounteredError ++
						encounteredErrorObject = err
					}
					if cancelled {
						return true, nil
					}
				}
				if encounteredError > 0 {
					if len(objectStores) > 1{
						logger.Warnf("Failed upload of '%s' to %d out of %d targets", path, encounteredError,  len(objectStores))
					}
					updateCounters(backupJobsState, backupConfig.Name, "upload", newDbRecord.Type, path, encounteredErrorObject)
					return false, encounteredErrorObject
				}

				// backup successful
				updateCounters(backupJobsState, backupConfig.Name, "upload", newDbRecord.Type, path, nil)
				return false, nil
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
		// the sqlite3 driver produces an error when fetching a string and converting it to time.time so we have to
		// manually do the conversion
		var tmpMtime, tmpCtime string
		err := rows.Scan(&dbRecord.Path, &dbRecord.Type, &dbRecord.LinkTarget, &dbRecord.Size, &tmpMtime,
			&tmpCtime, &dbRecord.Owner, &dbRecord.Permissons, &dbRecord.Checksum,
			&dbRecord.ChecksumType, &dbRecord.Encrypted, &dbRecord.Targets)
		if err != nil {
			logger.Errorf("While retrieving the database record for '%s' the following error was encountered:" +
				" '%s'", path, err)
			return false, shared.BackedUpFileProperties{}, err
		} else {
			// convert string to time for  mtime  and  ctime
			if tmpMtime != "" {
				dbRecord.Mtime, err = time.Parse(time.RFC3339Nano, tmpMtime)
				if err != nil {
					logger.Error("While converting mtime property of database record for '%s' the following " +
						"error was encountered: %s", path, err)
					return false, shared.BackedUpFileProperties{}, errors.New(fmt.Sprintf("While converting " +
						"mtime property encountered error: %s", err))
				}
			}
			if tmpCtime != "" {
				dbRecord.Ctime, err = time.Parse(time.RFC3339Nano, tmpCtime)
				if err != nil {
					logger.Error("While converting ctime property of database record for '%s' the following " +
						"error was encountered: %s", path, err)
					return false, shared.BackedUpFileProperties{}, errors.New(fmt.Sprintf("While converting " +
						"ctime property encountered error: %s", err))
				}
			}
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
	if ! metadataChanged && objectType == "symlink" {
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
func PrepareFileRecord(path string, stat os.FileInfo, backupConfig config.Backup, ctime time.Time, checksum string) (shared.BackedUpFileProperties, error) {
	/*
	type BackedUpFileProperties struct {
	Path string
	// one of: file / dir / symlink / unknown
	Type string
	// valid only for "symlink" type; otherwise it will be empty string
	LinkTarget string
	Size int64
	// time object modified
	Mtime time.Time
	// time object metadata changed (ctime gets updated if file content gets changed too)
	Ctime time.Time
	// user id on *nix , Username on Windows (hence this is a string)
	// Actual name (not account id / SID) of the file owner
	Owner string
	// Json encoded string. To decode use type from cloudbackup/backup/fileproperties  FilePermissions struct
	Permissions string
	// if checksuming is enabled then this will be non empty
	Checksum string
	// if checksuming is enabled then this will hold whatever algorithm was used for checksumming
	ChecksumType string
	Encrypted bool
	// references the "name" of one or more entries in "targets" table ; multiple entries will be comma separated
	Targets string
}
	 */
	 ctime, err := fileproperties.GetCtime(path)
	 if err != nil {
	 	ctime = time.Time{}
	 }

	 // even if we get an error (and we don't have complete or any file properties) we will still attempt to back it up
	 owner, permissions, _ := fileproperties.GetObjectPermissions(path, stat) // #nosec
	 onDiskObjectProperties := shared.BackedUpFileProperties{
		Path: path,
		Type: utils.FileType(stat),
		Size: stat.Size(),
		Mtime: stat.ModTime(),
		Ctime: ctime,
		Owner: owner,
		Permissons: permissions,
		Checksum: checksum,
		Encrypted: backupConfig.Encrypt,
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
// for files it uploads both content and metadata
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func UploadObject(ctx context.Context, path string, newDbRecord shared.BackedUpFileProperties,
	backupConfig config.Backup, objectStores objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface) (bool, error) {
	// TODO - use the context and pass it further down
	if newDbRecord.Type == "file" {
		logger.Debugf("Uploading '%s'", path)
	} else {
		logger.Debugf("Uploading metadata for '%s' which is of type '%s'", path, newDbRecord.Type)
	}

	result, cancelled, err := objectStores.Upload(path, newDbRecord, backupJobsState)
	if cancelled {
		return true, err
	}
	if err != nil {
		return false, err
	}

	// $result represents the remote path (in the object store) where the object has been backed up
	storeName, _ := objectStores.GetStoreDetails()
	logger.Debugf("'%s' successfully uploaded to object store %s at remote location '%s'", path, storeName, result)

	// TODO - add "Target" to DB record and then update record in db

	return false, nil
}

// updates remote metadata for an object (file / dir / symlink) to the remote object storage. The object must already
// have been uploaded
// params: $ctx for canceable context; $path with absolute path to object being backed up; $newDbRecord has all of the
// details about the object which will be partially used for the metadata; $backupConfig is the struct with the details
// of this backup as represented in the config file
// return values: bool with true if backup got cancelled, false otherwise ; error if error encountered
func UpdateObjectMetadata(ctx context.Context, path string, newDbRecord shared.BackedUpFileProperties,
	backupConfig config.Backup, objectStore objectstore.ObjectStore, backupJobsState shared.BackupJobsStateInterface) (bool, error) {
	// TODO - use the context and pass it further down
	logger.Debugf("Updating remote stored metadata for previously backed up and unchanged '%s'", path)
	// TODO - insert / update db records


	return false, nil
}

// update counters in the backup job state struct. Params: $backupJobsState is the shared struct (pointer to it)
// where all the state is kept; $backupName corresponds to the name of the backup job as defined in the configuration
// file; $operationType must be one of "upload" or "update"; fileType is one of: "file", "dir", "symlink" or "unknown";
// $path os used only for logging errors and it represents the full path of the file/dir/symlink being backed up; if
// $err != nil then the counter to be updated will be one of "failure" type and otherwise its a "success" one
func updateCounters(backupJobsState shared.BackupJobsStateInterface, backupName string, operationType string, fileType string, path string, err error) {
	switch operationType {
		case "upload": {
			if err != nil {
				switch fileType {
					case "file":
						backupJobsState.IncrementCounter(backupName, "failed_to_upload_files", path, fileType, "upload", err.Error())
					case "dir":
						backupJobsState.IncrementCounter(backupName, "failed_to_upload_directories", path, fileType, "metadata", err.Error())
					case "symlink":
						backupJobsState.IncrementCounter(backupName, "failed_to_upload_symlinks", path, fileType, "metadata", err.Error())
					default: {
						backupJobsState.IncrementCounter(backupName, "failed_to_upload_unknown", path, fileType, "metadata", err.Error())
						logger.Warningf("'%s' is of an unknown type. Only directories, regular files and " +
							"symlinks are supported for backup. Consider excluding this file from backup in order " +
							"to prevent future warnings.", path)
					}
				}
			} else {
				switch fileType {
					case "file":
						backupJobsState.IncrementCounter(backupName, "uploaded_files", path, fileType, "upload","")
					case "dir":
						backupJobsState.IncrementCounter(backupName, "uploaded_directories", path, fileType, "metadata","")
					case "symlink":
						backupJobsState.IncrementCounter(backupName, "uploaded_symlinks", path, fileType, "metadata","")
					default:
						logger.Warningf("Tried to increment 'uploaded' counter for '%s' of type: '%s'. " +
							"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
					}
			}
		}
		case "update": {
			if err != nil {
				switch fileType {
					case "file":
						backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_files", path, fileType, "metadata",err.Error())
					case "dir":
						backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_directories", path, fileType, "metadata", err.Error())
					case "symlink":
						backupJobsState.IncrementCounter(backupName, "failed_to_update_metadata_for_symlinks", path, fileType, "metadata", err.Error())
					default:
						logger.Warningf("Tried to increment 'failed_to_update_metadata' counter for '%s' of type: '%s'. " +
							"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
				}
			} else {
				switch fileType {
					case "file":
						backupJobsState.IncrementCounter(backupName, "updated_metadata_for_files", path, fileType, "metadata","")
					case "dir":
						backupJobsState.IncrementCounter(backupName, "updated_metadata_for_directories", path, fileType, "metadata","")
					case "symlink":
						backupJobsState.IncrementCounter(backupName, "updated_metadata_for_symlinks", path, fileType, "metadata","")
					default:
						logger.Warningf("Tried to increment 'updated_metadata' counter for '%s' of type: '%s'. " +
							"This is a bug as this type should be skipped from being backed up. Please report it.", path, fileType)
				}
			}
		}
		default:
			logger.Errorf("Tried to update counters for operation of type '%s' during a backup having name '%s' " +
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
		msg := fmt.Sprintf("While executing %s_run_script '%s', encountered error: %s\nScript " +
			"output was: %s", scriptType, path, err, stdoutStderr)
		logger.Error(msg)
		return errors.New(msg)
	}
	// if we got here, all was good
	return nil
}