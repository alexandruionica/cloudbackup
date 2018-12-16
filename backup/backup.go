package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"os"
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
				return false, err
			}
			// if a db entry is found then this object has been previously backed up so it needs to be verified if the
			// object has changed
			if dbEntryFound {
				// check if properties match between DB record and os.FileInfo
				logger.Infof("Found db record for '%s' with properties '%+v'", path, dbRecordProperties)
				contentChanged, metadataChanged, ctime, checksum := needsUpload(path, stat, dbRecordProperties, backupConfig.Checksum)
				newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum)
				if err != nil {
					// something bad enough happened that we don't have a usable db record so we can't proceed to
					// backup this file
					return false, err
				}
				if contentChanged {
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
						return false, encounteredErrorObject
					} else {
						if newDbRecord.Type == "file" {
							backupJobsState.IncrementCounter(backupConfig.Name, "uploaded_files")
						} else {
							backupJobsState.IncrementCounter(backupConfig.Name, "uploaded_non_files")
						}
					}

					// backup successful
					return false, nil

				} else {
					if metadataChanged {
						encounteredError := 0
						var encounteredErrorObject error
						// back up the object metadata to one or more remote object stores
						for _, objectStore := range objectStores {
							// TODO - proceed to update file metadata in the DB and on the remote ???? (to decide what to do with
							// the remote: changed owner is probably something we want to flag, but not much else)
							// back up the object to one or more remote object stores
							cancelled, err := UpdateObjectMetadata(ctx, path, newDbRecord, backupConfig, objectStore)
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
							return false, encounteredErrorObject
						}

						// backup successful
						return false, nil
					}
				}
			// no db record found so this is the first time this object is backed up
			}else{
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
					// backup this file
					return false, err
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
					return false, encounteredErrorObject
				} else {
					if newDbRecord.Type == "file" {
						backupJobsState.IncrementCounter(backupConfig.Name, "uploaded_files")
					} else {
						backupJobsState.IncrementCounter(backupConfig.Name, "uploaded_non_files")
					}
				}

				// backup successful
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
		err := rows.Scan(&dbRecord.Path, &dbRecord.Type, &dbRecord.LinkTarget, &dbRecord.Size, &dbRecord.Mtime,
			&dbRecord.Ctime, &dbRecord.Owner, &dbRecord.Permissons, &dbRecord.Checksum,
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
	backupConfig config.Backup, objectStore objectstore.ObjectStore) (bool, error) {
	// TODO - use the context and pass it further down
	logger.Debugf("Updating remote stored metadata for previously backed up and unchanged '%s'", path)

	// TODO - insert / update db records


	return false, nil
}