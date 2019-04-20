package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"golang.org/x/time/rate"
	"io"
	"strconv"
)

type StoreTestNull struct {
	ctx             context.Context
	backupName      string
	storeName       string
	storeType       string
	bucket          *rate.Limiter
	rateLimit       uint64
	burst           uint64
	backupJobsState shared.BackupJobsStateInterface
}

func InitialiseStoreTestNull(ctx context.Context, backupConfig config.Backup, target config.Target, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreTestNull, error) {
	var rateLimitBucket *rate.Limiter

	rateLimitBucket, ratelimit, burst, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
	if err != nil {
		return &StoreTestNull{}, err
	}

	result := &StoreTestNull{
		ctx:             ctx,
		backupName:      backupConfig.Name,
		storeName:       target.Name,
		storeType:       target.Type,
		bucket:          rateLimitBucket,
		rateLimit:       ratelimit,
		burst:           burst,
		backupJobsState: backupJobsState,
	}
	// actual backends will also setup the connection client in this section
	return result, nil
}

// pretend to upload file (actually discarding all read content)
func (object *StoreTestNull) Upload(path string, newDbRecord shared.BackedUpFileProperties, version int, backupJobsState shared.BackupJobsStateInterface) (remoteVersion string, cancelled bool, err error) {
	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(path, object.bucket, object.backupJobsState, object.backupName, object.storeName,
			object.storeType, object.rateLimit, object.burst, newDbRecord.Size, object.ctx)
		if err != nil {
			return strconv.Itoa(version), false, err
		}
		defer reader.Close()

		// create a 1000 KiB buffer to hold read content
		p := make([]byte, 102400)
		// fake work of uploading file - read all bytes and discard them. Report errors
		for {
			_, err := reader.Read(p)
			// logger.Infof("read %d bytes for %s", readyBytes, path)
			if err != nil {
				switch err {
				// io.Reader reports io.EOF when reaching the end of the file. This is normal and expected
				case io.EOF:
					{
						return strconv.Itoa(version), false, nil
					}
				case context.Canceled:
					{
						return strconv.Itoa(version), true, nil
					}
				default:
					{
						logger.Warningf("While reading '%s' the following error was encountered: %s", path, err)
						return strconv.Itoa(version), false, err
					}
				}
			}
		}
	} else {
		// TODO - build metadata for dir / symlink and then proceed to discard it
		return strconv.Itoa(version), false, nil
	}
}

func (object *StoreTestNull) GetStoreDetails() (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}

// pretend to place a delete marker
func (object *StoreTestNull) MarkDeleted(path string, existingDbRecord shared.BackedUpFileProperties, version int) (remoteVersion string, cancelled bool, err error) {
	return strconv.Itoa(version), false, nil
}
