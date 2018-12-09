package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"github.com/juju/ratelimit"
	"io"
)

type StoreTestNull struct {
	ctx context.Context
	backupName string
	storeName string
	storeType string
	bucket *ratelimit.Bucket
	backupJobsState shared.BackupJobsStateInterface
}

func InitialiseStoreTestNull (ctx context.Context, backupConfig config.Backup, target config.Target, rateLimitVal int64, backupJobsState shared.BackupJobsStateInterface) (*StoreTestNull) {
	var rateLimitBucket *ratelimit.Bucket
	if rateLimitVal > 0 {
		rateLimitBucket = ratelimit.NewBucketWithRate(float64(rateLimitVal), rateLimitVal)
	}

	result := &StoreTestNull{
		ctx: ctx,
		backupName: backupConfig.Name,
		storeName: target.Name,
		storeType: target.Type,
		bucket: rateLimitBucket,
		backupJobsState: backupJobsState,
	}
	// actual backends will also setup the connection client in this section
	return result
}

// pretend to upload file (actually discarding all read content)
func (object *StoreTestNull) Upload (path string, newDbRecord shared.BackedUpFileProperties, backupJobsState shared.BackupJobsStateInterface)  (result string, cancelled bool, err error) {
	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(path, object.bucket, object.backupJobsState, object.backupName, object.storeName, object.storeType)
		if err != nil {
			return "", false, err
		}
		defer reader.Close()

		// create a 1 MiB buffer to hold read content
		p := make([]byte, 1048576)
		// fake work of uploading file - read all bytes and discard them. Report errors
		for {
			_, err := reader.Read(p)
			if err != nil{
				// io.Reader reports io.EOF when reaching the end of the file. This is normal and expected
				if err == io.EOF {
					break
				}
				logger.Warningf("While reading '%s' the following error was encountered: %s", path, err)
				return "", false, err
			}

		}
		return "test_null_discared:" + path, false, nil
	} else {
		// TODO - build metadata for dir / symlink and then proceed to discard it
		return "test_null_discarded_" + newDbRecord.Type + "_" + path, false, nil
	}



}

func (object *StoreTestNull) MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, nil
}

func (object *StoreTestNull) GetStoreDetails ()  (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}