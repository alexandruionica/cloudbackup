package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
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
	// if rate limiting is not enabled than this will be a null pointer
	bucket *rate.Limiter
	// needed in order to increment the rate counters used for statistics/reporting
	backupJobsState shared.BackupJobsStateInterface
	// used for figuring out which stats counter to increment
	backupJobName string
	// used for figuring out which stats counter to increment
	objectStoreName string
	// used for figuring out which stats counter to increment
	objectStoreType string
	rateLimit uint64
	burst uint64
	// used for error messages during file.Close()
	path string
	//
	ctx context.Context
}

func GetObjectStores (ctx context.Context, backupConfig config.Backup, backupJobsState shared.BackupJobsStateInterface) ([]ObjectStore, error) {
	results := make([]ObjectStore, 0)
	for _, backupTarget := range backupConfig.Target {

		switch backupTarget.Type {
			case "test_null": {
				store, err := InitialiseStoreTestNull(ctx, backupConfig, backupTarget, backupTarget.RateLimit, backupJobsState)
				if err != nil {
					return results, err
				}
				results = append(results, store)

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
func NewFileReader (path string, bucket *rate.Limiter, backupJobsState shared.BackupJobsStateInterface,
	BackupJobName string, ObjectStoreName string, ObjectStoreType string, rate uint64, burst uint64, ctx context.Context)(FileReader, error) {
	fileReader, err := os.Open(path) // #nosec
	if err != nil {
		return FileReader{}, err
	}

	result := FileReader{
		origFileReader: fileReader,
		bucket: bucket,
		backupJobsState: backupJobsState,
		backupJobName: BackupJobName,
		objectStoreName: ObjectStoreName,
		objectStoreType: ObjectStoreType,
		rateLimit:rate,
		burst: burst,
		path: path,
		ctx: ctx,
	}

	return result, nil
}

// this reader just request reads from the actual os.file reader and forwards the result to the caller while also incrementing a counter
func (handle *FileReader) Read(p []byte) (int, error) {
	select {
	case <-handle.ctx.Done(): {
		logger.Infof("Received cancellation request while reading content of '%s'", handle.path)
		return 0, context.Canceled
	}
	default:
		{
			if handle.rateLimit == 0 {
				readBytes, err := handle.origFileReader.Read(p)
				handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName, handle.objectStoreType, int64(readBytes))
				return readBytes, err
			} else {
				// bucket.WaitN() allows to read up to burst limit so we need to ensure we don't attempt larger values than burst
				if uint64(len(p)) <= handle.burst {
					err := handle.bucket.WaitN(context.Background(), len(p))
					if err != nil {
						logger.Warningf("While pausing before attempting to read '%s' the following error was received " +
							"from the rate limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", handle.path, err)
					}
					readBytes, err := handle.origFileReader.Read(p)
					handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName, handle.objectStoreType, int64(readBytes))
					return readBytes, err
				} else {
					// byte slice $p which was passed over is larger than $burst size so we need to read $burst number of bytes and return that
					tmpP := make([]byte, handle.burst)
					err := handle.bucket.WaitN(context.Background(), int(handle.burst))
					if err != nil {
						logger.Warningf("While pausing before attempting to read '%s' the following error was received " +
							"from the rate limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", handle.path, err)
					}
					readBytes, err := handle.origFileReader.Read(tmpP)
					handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName, handle.objectStoreType, int64(readBytes))
					// copy read data to the original slice ;  func copy(dst, src []Type) int
					copiedBytes := copy(p, tmpP)
					if copiedBytes != len(tmpP) {
						logger.Errorf("Internal error when reading rate limited file '%s'", handle.path)
						return copiedBytes, errors.New("internal error when reading rate limited file - tmp slice content was not fully copied to requested slice")
					}
					return readBytes, err
				}
			}
		}
	}
}

// close the underlying file handle
func (handle *FileReader) Close() {
	err := handle.origFileReader.Close()
	if err != nil {
		logger.Warningf("Could not close file descriptor for '%s'", handle.path)
	}
}