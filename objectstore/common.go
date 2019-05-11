package objectstore

import (
	"cloudbackup/shared"
	"context"
	"errors"
	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"os"
)

const MaxBufferSize = 20971520 // 20 MiB = 20971520
const loggingContext = "objectstore"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

var CouldNotConvertRate = errors.New("could not convert rate to numeric value")

type ObjectStore interface {
	// uploads an item (if file; if not then just creates some kind of entry); it's up to the implementation to decide if
	// the provided $version is to be used;
	// in the returned values, $remoteVersion is the version as returned by the object store (some object stores provide
	// as a reply to a PUT a version which they generate) or it can also be the value of the input $version(converted to string)
	Upload(newDbRecord shared.BackedUpFileProperties, version int, backupJobsState shared.BackupJobsStateInterface) (remoteVersion string, cancelled bool, err error)
	// returned value $StoreName is the same as a target name in a Backup section of the config file; $StoreType represents target type
	GetStoreDetails() (StoreName string, StoreType string)
	// marks a given object, described by a $existingDbRecord.Path, as deleted. Depending on object store type this may have very different
	// implementations. This must NOT actually delete one or more versions belonging to a given object as depicted by a
	// path. For input parameter $version and returned $remoteVersion see the description for the Upload() method
	MarkDeleted(existingDbRecord shared.BackedUpFileProperties, version int) (remoteVersion string, cancelled bool, err error)
	// for a given $path, deletes a particular $version && $remote_version pair (it's up to the implementation to
	// decide which of the two makes sense to be used in order to remove the appropiate file)
	//  $objType is one of "dir"/"file"/"symlink"
	Delete(path string, objType string, version int, remoteVersion string) error
	// Validate that the config and credentials for a given object store are usable/work as expected
	// the Validate() function MUST NOT make any use of the $backupJobsState struct which is passed when an object
	// store is initialised. This is because when validate() is called, the object store is initialised with a "mock" $backupJobsState
	// Returns a message string (in case of success) and an error message if something went wrong
	Validate() (string, error)
}

// see description of the NewFileReader() function in order to understand the purpose of this type
type FileReader struct {
	// we need the original file reader in order to call Close() when we finished reading or want to close the file handle
	origFileReader *os.File
	// if rate limiting is not enabled than this will be a null pointer
	bucket *rate.Limiter
	// needed in order to increment the rate counters used for statistics/reporting
	backupJobsState shared.BackupJobsStateInterface
	// used for figuring out which stats counter to increment
	backupJobName string
	// used for figuring out which stats counter to increment and also for other cases where the Target Name is not
	// available from the caller of a particular function but the objectore object is passed in
	objectStoreName string
	// used for figuring out which stats counter to increment
	objectStoreType string
	rateLimit       uint64
	burst           uint64
	// used for error messages during file.Close()
	path string
	// used to calculate how many bytes can still be requested to be read from the file (makes sense only when rate limiting is in use)
	fileSize int64
	// how many bytes where read so far
	readBytes int64
	// for cancelling the job
	ctx context.Context
}

func GetObjectStores(ctx context.Context, backupConfig shared.ConfigBackup, backupJobsState shared.BackupJobsStateInterface) ([]ObjectStore, error) {
	results := make([]ObjectStore, 0)
	for _, backupTarget := range backupConfig.Target {

		switch backupTarget.Type {
		case "test_null":
			{
				store, err := InitialiseStoreTestNull(ctx, backupConfig, backupTarget, backupTarget.RateLimit, backupJobsState)
				if err != nil {
					return results, err
				}
				results = append(results, store)

			}
		// TODO: when implementing aws_s3 backend go back to the config file used for unit tests and add it back there too as it was removed due to tests failing because it was yet to be implemented
		// also update the config file used be the Python integration tests
		default:
			{
				logger.Errorf("unknown backend of type: '%s'", backupTarget.Type)
				return results, errors.New("unknown backend type")
			}
		}
	}
	return results, nil
}

// setup the io.Reader compliant type which records how many bytes were transferred and which also hooks up the
// rate limiter (if rate limiting is enabled)
func NewFileReader(path string, bucket *rate.Limiter, backupJobsState shared.BackupJobsStateInterface,
	BackupJobName string, ObjectStoreName string, ObjectStoreType string, rate uint64, burst uint64, fileSize int64, ctx context.Context) (FileReader, error) {
	fileReader, err := os.Open(path) // #nosec
	if err != nil {
		return FileReader{}, err
	}

	result := FileReader{
		origFileReader:  fileReader,
		bucket:          bucket,
		backupJobsState: backupJobsState,
		backupJobName:   BackupJobName,
		objectStoreName: ObjectStoreName,
		objectStoreType: ObjectStoreType,
		rateLimit:       rate,
		burst:           burst,
		path:            path,
		fileSize:        fileSize,
		readBytes:       0,
		ctx:             ctx,
	}

	return result, nil
}

// given a file size and how many bytes were read(or written) so far return a percent (as an int)
func calculatePercent(fileSize int64, readBytes int64) uint {
	if fileSize == 0 {
		return 100
	}
	if readBytes == 0 {
		return 0
	} else {
		return uint((readBytes * 100) / fileSize)
	}
}

// this reader just request reads from the actual os.file reader and forwards the result to the caller while also incrementing a counter
func (handle *FileReader) Read(p []byte) (int, error) {
	select {
	case <-handle.ctx.Done():
		{
			logger.Infof("Received cancellation request while reading content of '%s'", handle.path)
			return 0, context.Canceled
		}
	default:
		{
			newFile := false
			if handle.readBytes == 0 {
				newFile = true
			}
			if handle.rateLimit == 0 {
				readBytes, err := handle.origFileReader.Read(p)
				// update statistics
				handle.readBytes += int64(readBytes)
				// don't update counters if JustReadBytes == 0 && fileSIze > 0 && readBytesSoFar == fileSize . This is
				// needed because it seems there always is a 0 bytes read at the end of a file and this causes extra
				// messages to be sent to watch clients
				if !(readBytes == 0 && handle.fileSize > 0 && handle.readBytes == handle.fileSize) {
					handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName,
						handle.objectStoreType, int64(readBytes), handle.path,
						calculatePercent(handle.fileSize, handle.readBytes), newFile)
					handle.backupJobsState.AddBytesRead(handle.backupJobName, uint64(readBytes))
				}

				return readBytes, err
			} else {
				// bucket.WaitN() allows to read up to burst limit so we need to ensure we don't attempt larger values
				// than burst; we also need to be sure that we're not trying to read more bytes than the file has
				// because if we do so then the WaitN() will pause for more tokens to be available than needed
				if uint64(len(p)) <= handle.burst && (handle.fileSize-handle.readBytes) >= int64(len(p)) {
					// logger.Infof("1. waiting to read %10d for %s", len(p), handle.path)
					err := handle.bucket.WaitN(context.Background(), len(p))
					if err != nil {
						logger.Warningf("While pausing before attempting to read '%s' the following error was received "+
							"from the rate limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", handle.path, err)
					}
					readBytes, err := handle.origFileReader.Read(p)
					handle.readBytes += int64(readBytes)
					// don't update counters if JustReadBytes == 0 && fileSIze > 0 && readBytesSoFar == fileSize . This is
					// needed because it seems there always is a 0 bytes read at the end of a file and this causes extra
					// messages to be sent to watch clients
					if !(readBytes == 0 && handle.fileSize > 0 && handle.readBytes == handle.fileSize) {
						handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName,
							handle.objectStoreType, int64(readBytes), handle.path,
							calculatePercent(handle.fileSize, handle.readBytes), newFile)
						handle.backupJobsState.AddBytesRead(handle.backupJobName, uint64(readBytes))
					}
					return readBytes, err
				} else {
					var newBufSize int64
					// if we're trying to read more than it remains to be read then we need to adjust to what remains
					// or otherwise we'll pause for waiting more tokens then needed and slow down the whole backup
					if (handle.fileSize - handle.readBytes) <= int64(len(p)) {
						if (handle.fileSize - handle.readBytes) > 0 {
							newBufSize = handle.fileSize - handle.readBytes
						} else {
							// if we got 0 then probably this is the last read() as its going to return io.EOF .Set
							// to 1 in order to be able to get io.EOF
							newBufSize = 1
							// /etc/mtab is a symlink to /proc/self/mounts which is a 0 bytes file but when read it
							// actually returns content. So for 0 bytes files just attempt to do a 1KB read
							if handle.fileSize == 0 {
								newBufSize = 1000
							} else {
								if handle.fileSize-handle.readBytes < 0 {
									logger.Warningf("Remaining bytes to be read for '%s' is reported as %d which signifies a bug", handle.path, handle.fileSize-handle.readBytes)
								}
							}
						}
					} else {
						newBufSize = int64(len(p))
					}
					// handle.burst is forced somewhere else to be max size of a 32bit unsigned int so the conversion is safe
					if newBufSize > int64(handle.burst) {
						newBufSize = int64(handle.burst)
					}
					// byte slice $p which was passed over is larger than $burst size so we need to read $burst number of bytes and return that
					tmpP := make([]byte, newBufSize)
					// logger.Infof("2. waiting to read %10d for %s", newBufSize, handle.path)
					err := handle.bucket.WaitN(context.Background(), int(newBufSize))
					if err != nil {
						logger.Warningf("While pausing before attempting to read '%s' the following error was received "+
							"from the rate limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", handle.path, err)
					}
					readBytes, err := handle.origFileReader.Read(tmpP)
					handle.readBytes += int64(readBytes)
					// don't update counters if JustReadBytes == 0 && fileSIze > 0 && readBytesSoFar == fileSize . This is
					// needed because it seems there always is a 0 bytes read at the end of a file and this causes extra
					// messages to be sent to watch clients
					if !(readBytes == 0 && handle.fileSize > 0 && handle.readBytes == handle.fileSize) {
						handle.backupJobsState.IncrementRateCounter(handle.backupJobName, handle.objectStoreName,
							handle.objectStoreType, int64(readBytes), handle.path,
							calculatePercent(handle.fileSize, handle.readBytes), newFile)
						handle.backupJobsState.AddBytesRead(handle.backupJobName, uint64(readBytes))
					}
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

// sets up a rate limited bucket if $rateLimitStr converts to a value >0
// returns: pointer to rate limited bucket, converted $ratelimit, burst value, error
func setupRateLimiterBucket(rateLimitStr string, targetName string, backupConfigName string) (*rate.Limiter, uint64, uint64, error) {
	var rateLimitBucket *rate.Limiter

	ratelimit, err := humanize.ParseBytes(rateLimitStr)
	if err != nil {
		logger.Errorf("While trying to convert the rate limit '%s' to a "+
			"number the following error was encountered: %s", rateLimitStr, err)
		return rateLimitBucket, 0, 0, CouldNotConvertRate
	}
	if ratelimit > 0 {
		// if rateLimitVal > 9223372036854775807 conversion to int64 from uint64 will return a negative number
		if ratelimit > 9223372036854775807 {
			logger.Warningf("Rate is %d which is higher than ~ 9223 petabytes/sec and this would overflow "+
				"during a conversion from uint64 to int64. Lowering rate to %d", ratelimit, 9223372036854775807)
			// 9223.something petabytes/sec should be sufficient for the near future
			ratelimit = 9223372036854775807
		}
	}

	var burst uint64
	if ratelimit > 0 {
		// burst represents how much can be fetched in one iteration
		burst = ratelimit / 10
		// lower burst to ~2GB if burst is larger that the max positive value of a 32bit integer
		if burst > 2147483647 {
			burst = 2147483647
		}
		// the value of "burst" will be used to make a byte slice so whatever is the burst size will equal to allocated memory, at runtime
		if burst > MaxBufferSize {
			burst = MaxBufferSize
		}
		if burst < 1 {
			burst = 1
		}
		rateLimitBucket = rate.NewLimiter(rate.Limit(ratelimit), int(ratelimit/10))
	}

	if ratelimit > 0 {
		logger.Infof("Using rate limit %s/sec for target '%s' belonging to backup '%s'",
			humanize.Bytes(ratelimit), targetName, backupConfigName)
	}
	return rateLimitBucket, ratelimit, burst, nil
}
