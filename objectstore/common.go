package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/juju/ratelimit"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
)

const loggingContext = "objectstore"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ObjectStore interface {
	Upload (path string, newDbRecord shared.BackedUpFileProperties, backupJobsState shared.BackupJobsStateInterface) (result string, cancelled bool, err error)
	// update or create a metadata entry
	MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties) (result string, cancelled bool, err error)
	//
	GetStoreDetails()(StoreName string, StoreType string)
}

type FileReader struct {
	// we need the original file reader in order to call Close() when we finished reading or want to close the file handle
	origFileReader *os.File
	// if rate limiting is not requested than this will be a copy of the above *os.File pointer (io.Reader is an interface)
	rateLimitedReader io.Reader
	// needed in order to increment the rate counters used for statistics/reporting
	backupJobsState shared.BackupJobsStateInterface
	// used for figuring out which stats counter to increment
	backupJobName string
	// used for figuring out which stats counter to increment
	objectStoreName string
	// used for figuring out which stats counter to increment
	objectStoreType string
	// used for error messages during file.Close()
	path string
}

func GetObjectStores (ctx context.Context, backupConfig config.Backup, backupJobsState shared.BackupJobsStateInterface) ([]ObjectStore, error) {
	results := make([]ObjectStore, 0)
	for _, backupTarget := range backupConfig.Target {
		ratelimit, err := humanize.ParseBytes(backupTarget.RateLimit)
		if err != nil {
			return results, errors.New(fmt.Sprintf("While trying to convert the rate limit '%s' to a number " +
				"the following error was encountered: %s", backupTarget.RateLimit, err))
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

		switch backupTarget.Type {
			case "test_null": {
				results = append(results, InitialiseStoreTestNull(ctx, backupConfig, backupTarget, int64(ratelimit), backupJobsState))
				if ratelimit > 0 {
					logger.Infof("Using rate limit %s for target '%s' belonging to backup '%s'",
						humanize.Bytes(ratelimit), backupTarget.Name, backupConfig.Name)
				}

			}
			// TODO: when implementing aws_s3 backend go back to the config file used for unit tests and add it back there too as it was removed due to tests failing because it was yet to be implemented
			// also update the config file used be the Python integration tests
			default: {
				logger.Errorf("unknown backend of type: '%s'", backupTarget.Type)
				return results, errors.New("unknown backend type")
			}
		}
	}
	return results, nil
}

// setup the io.Reader compliant type which records how many bytes were transferred and which also hooks up the
// rate limiter (if rate limiting is enabled)
func NewFileReader (path string, bucket *ratelimit.Bucket, backupJobsState shared.BackupJobsStateInterface,
	BackupJobName string, ObjectStoreName string, ObjectStoreType string)(FileReader, error) {
	fileReader, err := os.Open(path) // #nosec
	if err != nil {
		return FileReader{}, err
	}

	var finalReader io.Reader
	if bucket != nil {
		// func Reader(r io.Reader, bucket *Bucket) io.Reader
		finalReader = ratelimit.Reader(fileReader, bucket)
	} else {
		finalReader = fileReader
	}

	result := FileReader{
		origFileReader: fileReader,
		rateLimitedReader: finalReader,
		backupJobsState: backupJobsState,
		backupJobName: BackupJobName,
		objectStoreName: ObjectStoreName,
		objectStoreType: ObjectStoreType,
	}

	return result, nil
}

// this reader just request reads from the actual os.file reader and forwards the result to the caller while also incrementing a counter
func (handle *FileReader) Read(p []byte) (int, error) {
	readBytes, err := handle.rateLimitedReader.Read(p)
	handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName, handle.objectStoreType, int64(readBytes))
	return readBytes, err
}

// close the underlying file handle
func (handle *FileReader) Close() {
	err := handle.origFileReader.Close()
	if err != nil {
		logger.Warningf("Could not close file descriptor for '%s'", handle.path)
	}
}