package objectstore

import (
	"cloudbackup/cbcrypto"
	"cloudbackup/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/time/rate"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type StoreTestNull struct {
	ctx        context.Context
	backupName string
	storeName  string
	storeType  string
	// name of the object store bucket used for storing content
	storeBucketName string
	// prefix to prepend to all backed up items. This normally "backup.target.prefix" + $separator + "backup.name";
	// ANY CHANGE TO THIS MAY BREAK ALREADY MADE BACKUPS
	storePrefix string
	// this is the rate limiter bucket (token bucket); not to be confused with the above $storeBucketName
	bucket          *rate.Limiter
	rateLimit       uint64
	burst           uint64
	backupJobsState shared.BackupJobsStateInterface
	// In-memory store for plaintext bucket-side objects (currently only the
	// keystore sidecar). Test backend keeps these in process so unit tests
	// can exercise the encryption lifecycle without a real cloud bucket.
	memObjects map[string][]byte
	memMu      sync.Mutex
	// When the target parameter "persist_dir" is set, objects are written to (and read back
	// from) that directory instead of memObjects, so they outlive the store instance. The
	// daemon builds a fresh store per job, which is why integration tests need this to run a
	// restore against what an earlier backup job uploaded. Each object lives in one file named
	// by the hex SHA-256 of its remote key: keys carry absolute source paths (Unicode, colons,
	// long names) that are not safe to mirror as filenames on every supported OS.
	persistDir string
	encryptionState
}

func InitialiseStoreTestNull(ctx context.Context, backupConfig shared.ConfigBackup, target shared.ConfigBackupTarget, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreTestNull, error) {
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
		storeBucketName: target.Bucket,
		storePrefix:     target.Prefix + "/" + backupConfig.Name,
		bucket:          rateLimitBucket,
		rateLimit:       ratelimit,
		burst:           burst,
		backupJobsState: backupJobsState,
		memObjects:      make(map[string][]byte),
		encryptionState: encryptionState{
			enabled:  backupConfig.Encrypt,
			password: []byte(backupConfig.EncryptPass),
		},
	}
	resolveStringParameter("persist_dir", &result.persistDir, target.Parameters, "")
	if result.persistDir != "" {
		if err := os.MkdirAll(result.persistDir, 0o750); err != nil {
			return &StoreTestNull{}, fmt.Errorf("target '%s' of backup '%s': could not create persist_dir '%s': %w",
				target.Name, backupConfig.Name, result.persistDir, err)
		}
	}
	// actual backends will also setup the connection client in this section
	return result, nil
}

// InitEncryption: see ObjectStore.InitEncryption.
func (objStore *StoreTestNull) InitEncryption(opts EncryptionInitOptions) error {
	bump := func(name, msg string) {
		if objStore.backupJobsState != nil {
			objStore.backupJobsState.IncrementCounter(objStore.backupName, name, sidecarBucketKey(objStore.storePrefix), "file", "init", msg)
		}
	}
	return objStore.initEncryption(&testNullSidecarIO{store: objStore}, opts, bump)
}

// testNullSidecarIO is an in-memory sidecarIO backed by the StoreTestNull's
// memObjects map. Lets unit tests exercise the encryption lifecycle without
// touching real cloud buckets.
type testNullSidecarIO struct {
	store *StoreTestNull
}

func (io *testNullSidecarIO) key() string {
	return sidecarBucketKey(io.store.storePrefix)
}

func (io *testNullSidecarIO) Fetch() ([]byte, error) {
	body, ok, err := io.store.blobGet(io.key())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errSidecarNotFound
	}
	return body, nil
}

func (io *testNullSidecarIO) PutIfNotExists(body []byte) error {
	return io.store.blobPutIfNotExists(io.key(), body)
}

// pretend to upload file (actually discarding all read content)
func (objStore *StoreTestNull) Upload(newDbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface, metadata bool) (remoteVersion string, cancelled bool, err error) {
	var prepend string
	if metadata {
		prepend = MetaDataPrepend
	} else {
		prepend = DataPrepend
	}
	logger.Debugf("Pretending to upload: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", newDbRecord.Path, version, objStore.storeName, objStore.storeBucketName, objStore.storePrefix+"/"+prepend+"/"+newDbRecord.Path)

	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(newDbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
			objStore.storeType, objStore.rateLimit, objStore.burst, newDbRecord.Size, objStore.ctx, true)
		if err != nil {
			return strconv.FormatInt(version, 10), false, err
		}
		defer reader.Close()

		// Integration tests that rely on the test_null backend's "instant upload"
		// behavior set a non-zero rate limit; under rate limiting, draining the
		// reader for a metadata=true upload (DB copy or config copy at backup
		// end) would crawl through the limiter and stall backup shutdown. Keep
		// the original short-circuit for that case — production behaviour is
		// unchanged (DB+config copies are always SkipEncryption=true plaintext
		// anyway, so there's no encryption read-path to exercise here).
		if metadata && objStore.rateLimit > 0 {
			return strconv.FormatInt(version, 10), false, nil
		}

		// Wrap with EncryptingReader when encryption is on AND this isn't an
		// internal cloudbackup state file (DB copy / config copy ship plaintext).
		var src io.Reader = &reader
		if objStore.EncryptionEnabled() && !newDbRecord.SkipEncryption {
			er, encErr := cbcrypto.NewEncryptingReader(&reader, objStore.KEK(), objStore.KeystoreUUID())
			if encErr != nil {
				return strconv.FormatInt(version, 10), false, fmt.Errorf("wrap with EncryptingReader: %w", encErr)
			}
			src = er
		}

		// Drain all bytes (ciphertext when encryption is on; plaintext otherwise).
		// Captured under the would-be remote path so unit tests can read them
		// back and validate roundtrip (e.g. encrypt → decrypt).
		remotePath := objStore.storePrefix + "/" + prepend + "/" + newDbRecord.Path
		var captured []byte
		p := make([]byte, 102400)
		for {
			n, err := src.Read(p)
			if n > 0 {
				captured = append(captured, p[:n]...)
			}
			if err != nil {
				switch err {
				case io.EOF:
					if putErr := objStore.blobPut(remotePath, captured); putErr != nil {
						return strconv.FormatInt(version, 10), false, putErr
					}
					return strconv.FormatInt(version, 10), false, nil
				case context.Canceled:
					logger.Infof("Received cancellation request while uploading '%s'", newDbRecord.Path)
					return strconv.FormatInt(version, 10), true, nil
				default:
					logger.Warningf("While reading '%s' the following error was encountered: %s", newDbRecord.Path, err)
					return strconv.FormatInt(version, 10), false, err
				}
			}
		}
	} else {
		return strconv.FormatInt(version, 10), false, nil
	}
}

func (objStore *StoreTestNull) GetStoreDetails() (StoreName string, StoreType string) {
	return objStore.storeName, objStore.storeType
}

// MaxObjectSize: see ObjectStore.MaxObjectSize. The test-null backend imposes no limit so
// upstream size-cap logic can be exercised by tests without hitting a synthetic ceiling.
func (objStore *StoreTestNull) MaxObjectSize(encrypted bool) int64 {
	return math.MaxInt64
}

// pretend to place a delete marker
func (objStore *StoreTestNull) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	var prepend string
	if metadata {
		prepend = MetaDataPrepend
	} else {
		prepend = DataPrepend
	}
	logger.Debugf("Pretending to mark as deleted: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, objStore.storeName, objStore.storeBucketName,
		objStore.storePrefix+"/"+prepend+"/"+existingDbRecord.Path)
	return strconv.FormatInt(markerVersion, 10), false, nil
}

// pretend to delete a particular version for a given path; $objType is one of "dir"/"file"/"symlink"
func (objStore *StoreTestNull) Delete(existingDbRecord shared.BackedUpFileProperties, version int64, remoteVersion string, metadata bool) error {
	var prepend string
	if metadata {
		prepend = MetaDataPrepend
	} else {
		prepend = DataPrepend
	}
	logger.Debugf("Pretending to delete: '%s' having version: '%d' and remote version: '%s' from object store:"+
		" '%s' using bucket: '%s' and full remote path: '%s'", existingDbRecord.Path, version, remoteVersion,
		objStore.storeName, objStore.storeBucketName, objStore.storePrefix+"/"+prepend+"/"+existingDbRecord.Path)
	return nil
}

// pretend to download a particular version
func (objStore *StoreTestNull) Get(existingDbRecord shared.BackedUpFileProperties, restorePath string, version int64, remoteVersion string, metadata bool) (cancelled bool, err error) {
	if existingDbRecord.Type != "file" {
		return false, nil
	}
	var prepend string
	if metadata {
		prepend = MetaDataPrepend
	} else {
		prepend = DataPrepend
	}

	// Pace the "download" when a rate limit is configured on the target. A real backend spends
	// wall-clock time moving bytes over the network; this one serves objects out of memory or a
	// local directory, so an unthrottled restore of a handful of small files finishes
	// before an API client has any chance to attach to /restore/watch. Integration tests that need
	// an observable, still-running restore set a non-zero ratelimit for exactly this reason - the
	// same knob the upload path already honors via NewFileReader(). Metadata downloads (DB copy /
	// config copy) stay unthrottled, mirroring the short-circuit in Upload().
	if objStore.rateLimit > 0 && !metadata {
		if cancelled := objStore.throttleDownload(existingDbRecord.Size, existingDbRecord.Path); cancelled {
			return true, nil
		}
	}

	remoteKey := objStore.storePrefix + "/" + prepend + "/" + existingDbRecord.Path
	body, ok, err := objStore.blobGet(remoteKey)
	if err != nil {
		return false, err
	}
	if !ok {
		// Historically TestNull's Get was a no-op; preserve that behavior for
		// callers that never uploaded anything (e.g., tests that only exercise
		// validation paths).
		return false, nil
	}

	f, err := os.Create(restorePath) // #nosec G304 -- restore path is test-controlled
	if err != nil {
		return false, fmt.Errorf("could not create file '%s' for restore: %s", restorePath, err)
	}
	defer f.Close()

	src := bytesReaderOf(body)
	if existingDbRecord.Encrypted {
		dr, err := cbcrypto.NewDecryptingReader(src, objStore.KEK())
		if err != nil {
			return false, fmt.Errorf("wrap with DecryptingReader: %w", err)
		}
		hdr, err := dr.PeekHeader()
		if err != nil {
			return false, fmt.Errorf("parse encrypted header for '%s': %w", remoteKey, err)
		}
		if hdr.KeystoreUUID != objStore.KeystoreUUID() {
			msg := fmt.Sprintf("encrypted object '%s' references a different keystore (header UUID %x, current sidecar UUID %x); cannot decrypt", remoteKey, hdr.KeystoreUUID, objStore.KeystoreUUID())
			if objStore.backupJobsState != nil {
				objStore.backupJobsState.IncrementCounter(objStore.backupName, "decrypt_keystore_mismatch", existingDbRecord.Path, "file", "restore", msg)
			}
			return false, fmt.Errorf("%s", msg)
		}
		src = dr
	}
	if _, err := io.Copy(f, src); err != nil {
		return false, err
	}
	return false, nil
}

// throttleDownload consumes $size bytes worth of tokens from the target's rate limiter before a
// (pretend) download is served, so that test_null restores progress at the configured rate instead
// of completing instantly. Tokens are consumed in chunks no larger than the limiter's burst because
// rate.Limiter.WaitN refuses any request bigger than that.
// returns: true when the job was cancelled while waiting for tokens
func (objStore *StoreTestNull) throttleDownload(size int64, path string) (cancelled bool) {
	if objStore.bucket == nil || objStore.bucket.Burst() < 1 {
		return false
	}
	maxChunk := int64(objStore.bucket.Burst())
	for remaining := size; remaining > 0; {
		chunk := remaining
		if chunk > maxChunk {
			chunk = maxChunk
		}
		if err := objStore.bucket.WaitN(objStore.ctx, int(chunk)); err != nil {
			if objStore.ctx.Err() != nil {
				logger.Infof("Received cancellation request while downloading '%s'", path)
				return true
			}
			logger.Warningf("While pausing before downloading '%s' the following error was received from the "+
				"rate limiting token bucket: %s . Proceeding to download while ignoring the rate limiting", path, err)
			return false
		}
		remaining -= chunk
	}
	return false
}

// bytesReaderOf returns an io.Reader over a copy of b — safe for use across
// goroutines and decoupled from later memObjects mutations.
func bytesReaderOf(b []byte) io.Reader {
	dup := make([]byte, len(b))
	copy(dup, b)
	return &bytesSliceReader{buf: dup}
}

type bytesSliceReader struct{ buf []byte }

func (r *bytesSliceReader) Read(p []byte) (int, error) {
	if len(r.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// blobPath maps a remote key to its file under persistDir.
func (objStore *StoreTestNull) blobPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(objStore.persistDir, hex.EncodeToString(sum[:]))
}

// blobGet returns a private copy of the object stored under $key. $found is false when nothing was
// ever stored there; $err reports I/O failures of the persistent backend only.
func (objStore *StoreTestNull) blobGet(key string) (body []byte, found bool, err error) {
	if objStore.persistDir == "" {
		objStore.memMu.Lock()
		defer objStore.memMu.Unlock()
		b, ok := objStore.memObjects[key]
		if !ok {
			return nil, false, nil
		}
		return append([]byte(nil), b...), true, nil
	}
	b, err := os.ReadFile(objStore.blobPath(key)) // #nosec G304 -- path derived from a hash inside persistDir
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("test_null persist_dir read of '%s': %w", key, err)
	}
	return b, true, nil
}

// blobPut stores (or overwrites) the object under $key. The persistent backend writes to a temp
// file and renames it so a concurrent reader never observes a partial object.
func (objStore *StoreTestNull) blobPut(key string, body []byte) error {
	if objStore.persistDir == "" {
		objStore.memMu.Lock()
		defer objStore.memMu.Unlock()
		objStore.memObjects[key] = append([]byte(nil), body...)
		return nil
	}
	final := objStore.blobPath(key)
	tmp, err := os.CreateTemp(objStore.persistDir, ".upload-*")
	if err != nil {
		return fmt.Errorf("test_null persist_dir write of '%s': %w", key, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("test_null persist_dir write of '%s': %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("test_null persist_dir write of '%s': %w", key, err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("test_null persist_dir write of '%s': %w", key, err)
	}
	return nil
}

// blobPutIfNotExists stores the object under $key only when nothing is there yet, returning
// errSidecarConflict otherwise. The persistent backend relies on O_EXCL so two daemons racing on
// the same persist_dir see the same conflict a cloud conditional PUT would report.
func (objStore *StoreTestNull) blobPutIfNotExists(key string, body []byte) error {
	if objStore.persistDir == "" {
		objStore.memMu.Lock()
		defer objStore.memMu.Unlock()
		if _, ok := objStore.memObjects[key]; ok {
			return errSidecarConflict
		}
		objStore.memObjects[key] = append([]byte(nil), body...)
		return nil
	}
	f, err := os.OpenFile(objStore.blobPath(key), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- hashed name inside persistDir
	if errors.Is(err, os.ErrExist) {
		return errSidecarConflict
	}
	if err != nil {
		return fmt.Errorf("test_null persist_dir conditional write of '%s': %w", key, err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("test_null persist_dir conditional write of '%s': %w", key, err)
	}
	return f.Close()
}

// validated that the config of this object store is correct
func (objStore *StoreTestNull) Validate() (string, error) {
	// given this is for tests and it discards data, always return nil (meaning OK)
	return fmt.Sprintf("%s passed validation", objStore.storeName), nil
}
