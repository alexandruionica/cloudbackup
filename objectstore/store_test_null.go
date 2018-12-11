package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	"github.com/dustin/go-humanize"
	"golang.org/x/time/rate"
	"io"
	"fmt"
)

type StoreTestNull struct {
	ctx context.Context
	backupName string
	storeName string
	storeType string
	bucket *rate.Limiter
	rateLimit uint64
	burst uint64
	backupJobsState shared.BackupJobsStateInterface
}

func InitialiseStoreTestNull (ctx context.Context, backupConfig config.Backup, target config.Target, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreTestNull, error) {
	var rateLimitBucket *rate.Limiter

	ratelimit, err := humanize.ParseBytes(rateLimitStr)
	if err != nil {
		return &StoreTestNull{}, errors.New(fmt.Sprintf("While trying to convert the rate limit '%s' to a " +
			"number the following error was encountered: %s", rateLimitStr, err))
	}
	if ratelimit > 0 {
		// if rateLimitVal > 9223372036854775807 conversion to int64 from uint64 will return a negative number
		if ratelimit > 9223372036854775807 {
			logger.Warningf("Rate is %d which is higher than ~ 9223 petabytes/sec and this would overflow " +
				"during a conversion from uint64 to int64. Lowering rate to %d", ratelimit, 9223372036854775807)
			// 9223.something petabytes/sec should be sufficient for the near future
			ratelimit = 9223372036854775807
		}
	}

	var burst uint64
	if ratelimit > 0 {
		// burst represents how much can be fetched in one iteration
		burst = ratelimit/10
		// lower burst to ~2GB if burst is larger that the max positive value of a 32bit integer
		if burst > 2147483647 {
			burst = 2147483647
		}
		if burst < 1 {
			burst = 1
		}
		rateLimitBucket = rate.NewLimiter(rate.Limit(ratelimit), int(ratelimit/10))
	}

	if ratelimit > 0 {
		logger.Infof("Using rate limit %s for target '%s' belonging to backup '%s'",
			humanize.Bytes(ratelimit), target.Name, backupConfig.Name)
	}

	result := &StoreTestNull{
		ctx: ctx,
		backupName: backupConfig.Name,
		storeName: target.Name,
		storeType: target.Type,
		bucket: rateLimitBucket,
		rateLimit: ratelimit,
		burst: burst,
		backupJobsState: backupJobsState,
	}
	// actual backends will also setup the connection client in this section
	return result, nil
}

// pretend to upload file (actually discarding all read content)
func (object *StoreTestNull) Upload (path string, newDbRecord shared.BackedUpFileProperties, backupJobsState shared.BackupJobsStateInterface)  (result string, cancelled bool, err error) {
	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(path, object.bucket, object.backupJobsState, object.backupName, object.storeName, object.storeType, object.rateLimit, object.burst, object.ctx)
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
				switch err {
				// io.Reader reports io.EOF when reaching the end of the file. This is normal and expected
					case io.EOF: {
						break
					}
					case context.Canceled: {
						return "", true, nil
					}
					default: {
						logger.Warningf("While reading '%s' the following error was encountered: %s", path, err)
						return "", false, err
					}
				}
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