package objectstore

import (
	"bytes"
	"cloudbackup/cbcrypto"
	"cloudbackup/shared"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"golang.org/x/time/rate"
)

type StoreAwsS3 struct {
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
	// object store specific parameters from here on
	awsAccessKeyId     string
	awsSecretAccessKey string
	// S3 storage class
	storageClass string
	region       string
	// AWS SDK v2 config
	awsCfg aws.Config
	// AWS managed uploader which does a lot of work (retries/multipart upload/etc)
	awsUploader *manager.Uploader
	// AWS S3 client
	awsS3Client *s3.Client
	encryptionState
}

func InitialiseStoreAwsS3(ctx context.Context, backupConfig shared.ConfigBackup, target shared.ConfigBackupTarget, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreAwsS3, error) {
	var rateLimitBucket *rate.Limiter

	rateLimitBucket, ratelimit, burst, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
	if err != nil {
		return &StoreAwsS3{}, err
	}

	result := &StoreAwsS3{
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
		encryptionState: encryptionState{
			enabled:  backupConfig.Encrypt,
			password: []byte(backupConfig.EncryptPass),
		},
	}

	// if any of those parameters was set then read its value and seed the struct
	GetStringParameter("AWS_ACCESS_KEY_ID", &result.awsAccessKeyId, target.Parameters, "")
	GetStringParameter("AWS_SECRET_ACCESS_KEY", &result.awsSecretAccessKey, target.Parameters, "")
	GetStringParameter("storage_class", &result.storageClass, target.Parameters, "")
	GetStringParameter("region", &result.region, target.Parameters, "")

	var loadOpts []func(*awsconfig.LoadOptions) error

	// if we have a key id and secret then use them
	if result.awsSecretAccessKey != "" && result.awsAccessKeyId != "" {
		logger.Debugf("Using user specified credentials")
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(result.awsAccessKeyId, result.awsSecretAccessKey, ""),
		))
		// we don't have a key id and secret so we'll fall back to the SDK's defaults
	} else {
		logger.Debugf("no credentials specified in the config, so defaulting to the defaults of the AWS SDK")
	}

	// set initial region; if the user didn't specify one, default to us-east-1 as we need a region hint for bucket region detection
	initialRegion := result.region
	if initialRegion == "" {
		initialRegion = "us-east-1"
	}
	loadOpts = append(loadOpts, awsconfig.WithRegion(initialRegion))

	// The v2 SDK accepts a standard *http.Client via config.WithHTTPClient, making rate limiting integration clean.
	httpClient := newRateLimitedHttpClientForAWS(ctx, rateLimitBucket, ratelimit, burst)
	loadOpts = append(loadOpts, awsconfig.WithHTTPClient(httpClient))

	result.awsCfg, err = awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return result, fmt.Errorf("could not setup AWS config for target '%s' belonging to backup '%s' due "+
			"to error: %s", target.Name, backupConfig.Name, err)
	}

	// create initial S3 client for region detection
	result.awsS3Client = s3.NewFromConfig(result.awsCfg)

	// try to determine programmatically the region; if not found then it will fallback to the user specified one; for
	// this specific call, we don't need an authenticated session
	err = result.getRegionFromBucket()
	if err != nil {
		return result, err
	}

	// re-create S3 client and uploader with the detected region
	result.awsS3Client = s3.NewFromConfig(result.awsCfg, func(o *s3.Options) {
		o.Region = result.region
	})
	result.awsUploader = manager.NewUploader(result.awsS3Client)

	return result, nil
}

// upload file and return remote version
func (objStore *StoreAwsS3) Upload(newDbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface, metadata bool) (remoteVersion string, cancelled bool, err error) {

	remotePath := calculateRemotePath(objStore.storePrefix, newDbRecord.Path, metadata)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", newDbRecord.Path, version, objStore.storeName, objStore.storeBucketName, remotePath)

	if newDbRecord.Type != "file" {
		// directories and symlinks DO NOT GET UPLOADED
		return strconv.FormatInt(version, 10), false, nil
	}

	// setup io.Reader (this handles reporting and optional rate limiting; rate is 0 here
	// because S3 traffic is rate-limited at the HTTP transport via http_rate_limiter.go).
	reader, err := NewFileReader(newDbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
		objStore.storeType, 0, objStore.burst, newDbRecord.Size, objStore.ctx, true)
	if err != nil {
		return strconv.FormatInt(version, 10), false, err
	}
	defer reader.Close()

	encrypting := objStore.EncryptionEnabled() && !newDbRecord.SkipEncryption

	if encrypting {
		return objStore.uploadEncrypted(newDbRecord, version, remotePath, &reader)
	}
	return objStore.uploadMultipart(newDbRecord, version, remotePath, &reader)
}

// uploadMultipart is the original (plaintext) path: hand the file to manager.Uploader
// which handles multipart upload, retries, and parallelism transparently.
func (objStore *StoreAwsS3) uploadMultipart(newDbRecord shared.BackedUpFileProperties, version int64, remotePath string, body io.Reader) (string, bool, error) {
	result, err := objStore.awsUploader.Upload(objStore.ctx, &s3.PutObjectInput{
		Bucket: aws.String(objStore.storeBucketName),
		Key:    aws.String(remotePath),
		Body:   body,
	})
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			msg := fmt.Sprintf("received cancellation request while uploading content to %s", remotePath)
			logger.Info(msg)
			return strconv.FormatInt(version, 10), true, errors.New(msg)
		}
		return strconv.FormatInt(version, 10), false, err
	}
	if result != nil && result.VersionID != nil {
		return *(result.VersionID), false, nil
	}
	msg := fmt.Sprintf("upload of '%s' was reported "+
		"successful but the upload response does not contain a file version. This means the backed up copy is "+
		"unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown", newDbRecord.Path)
	logger.Error(msg)
	return strconv.FormatInt(version, 10), false, errors.New(msg)
}

// uploadEncrypted streams the file through cbcrypto.EncryptingReader and uploads the
// ciphertext via single PutObject (NOT multipart): chunked AEAD must be processed as
// one sequential stream. Trade-off: the body is not seekable so the SDK cannot retry
// mid-upload on transient errors. A failed upload here returns an error and the
// caller will retry the whole file on the next backup run.
func (objStore *StoreAwsS3) uploadEncrypted(newDbRecord shared.BackedUpFileProperties, version int64, remotePath string, body io.Reader) (string, bool, error) {
	er, err := cbcrypto.NewEncryptingReader(body, objStore.KEK(), objStore.KeystoreUUID())
	if err != nil {
		return strconv.FormatInt(version, 10), false, fmt.Errorf("wrap with EncryptingReader: %w", err)
	}
	encryptedSize := cbcrypto.EncryptedSize(newDbRecord.Size)
	result, err := objStore.awsS3Client.PutObject(objStore.ctx, &s3.PutObjectInput{
		Bucket:        aws.String(objStore.storeBucketName),
		Key:           aws.String(remotePath),
		Body:          er,
		ContentLength: aws.Int64(encryptedSize),
	})
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			msg := fmt.Sprintf("received cancellation request while uploading content to %s", remotePath)
			logger.Info(msg)
			return strconv.FormatInt(version, 10), true, errors.New(msg)
		}
		return strconv.FormatInt(version, 10), false, err
	}
	if result != nil && result.VersionId != nil {
		return *(result.VersionId), false, nil
	}
	msg := fmt.Sprintf("encrypted upload of '%s' was reported successful but the "+
		"upload response does not contain a file version. This means the backed up copy "+
		"is unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown",
		newDbRecord.Path)
	logger.Error(msg)
	return strconv.FormatInt(version, 10), false, errors.New(msg)
}

func (objStore *StoreAwsS3) GetStoreDetails() (StoreName string, StoreType string) {
	return objStore.storeName, objStore.storeType
}

// S3 object-size limits.
//   - With multipart upload (manager.Uploader, the default for plaintext), an S3 object can be up to 5 TiB.
//   - Without multipart (single PutObject, used when client-side encryption is on because chunked AEAD
//     must be processed as a single sequential stream), the hard limit is 5 GiB per object.
const (
	s3MaxSinglePutBytes int64 = 5 * 1024 * 1024 * 1024        // 5 GiB
	s3MaxMultipartBytes int64 = 5 * 1024 * 1024 * 1024 * 1024 // 5 TiB
)

// MaxObjectSize: see ObjectStore.MaxObjectSize. When $encrypted is true we force single-PUT mode and
// therefore the 5 GiB cap applies.
func (objStore *StoreAwsS3) MaxObjectSize(encrypted bool) int64 {
	if encrypted {
		return s3MaxSinglePutBytes
	}
	return s3MaxMultipartBytes
}

// InitEncryption: see ObjectStore.InitEncryption.
func (objStore *StoreAwsS3) InitEncryption(opts EncryptionInitOptions) error {
	bump := func(name, msg string) {
		if objStore.backupJobsState != nil {
			objStore.backupJobsState.IncrementCounter(objStore.backupName, name, sidecarBucketKey(objStore.storePrefix), "file", "init", msg)
		}
	}
	return objStore.initEncryption(&awsS3SidecarIO{store: objStore}, opts, bump)
}

// awsS3SidecarIO bridges the shared keystore lifecycle to the S3 SDK.
// Receiver name is "sio" so the standard-library "io" package import isn't
// shadowed by the receiver identifier.
type awsS3SidecarIO struct {
	store *StoreAwsS3
}

// maxSidecarBytes guards against runaway sidecar reads; production sidecars
// are ~250 bytes so 64 KiB is far more than enough.
const maxSidecarBytes = 64 * 1024

func (sio *awsS3SidecarIO) key() string {
	return sidecarBucketKey(sio.store.storePrefix)
}

func (sio *awsS3SidecarIO) Fetch() ([]byte, error) {
	out, err := sio.store.awsS3Client.GetObject(sio.store.ctx, &s3.GetObjectInput{
		Bucket: aws.String(sio.store.storeBucketName),
		Key:    aws.String(sio.key()),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, errSidecarNotFound
		}
		// Some configurations surface "object not found" as a generic S3 error
		// with HTTP 404 rather than the typed NoSuchKey. Inspect API error code.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound") {
			return nil, errSidecarNotFound
		}
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(out.Body, maxSidecarBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxSidecarBytes {
		return nil, fmt.Errorf("sidecar object %q exceeds %d bytes; refusing to read more", sio.key(), maxSidecarBytes)
	}
	return body, nil
}

func (sio *awsS3SidecarIO) PutIfNotExists(body []byte) error {
	_, err := sio.store.awsS3Client.PutObject(sio.store.ctx, &s3.PutObjectInput{
		Bucket:      aws.String(sio.store.storeBucketName),
		Key:         aws.String(sio.key()),
		Body:        bytes.NewReader(body),
		IfNoneMatch: aws.String("*"),
	})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict":
			return errSidecarConflict
		}
	}
	return err
}

// place a delete marker on the newest version of a file. This is achieved by deleting the file without specifying a
// version. AWS does not allow a place marker operation so this is the only way to get a marker
func (objStore *StoreAwsS3) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	if existingDbRecord.Type != "file" {
		// directories and symlinks DO NOT GET UPLOADED so there is nothing to mark deleted
		return strconv.FormatInt(markerVersion, 10), false, nil
	}

	remotePath := calculateRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata)
	logger.Debugf("Marking as deleted: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, objStore.storeName, objStore.storeBucketName, remotePath)

	result, err := objStore.awsS3Client.DeleteObject(objStore.ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(objStore.storeBucketName),
		Key:    aws.String(remotePath),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return "0", false, fmt.Errorf("while trying to place a delete marker on '%s' "+
				"from S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", remotePath, objStore.storeBucketName, apiErr.ErrorCode(), apiErr.ErrorMessage())
		} else {
			return "0", false, fmt.Errorf("while trying to place a delete marker on '%s'"+
				" from S3 bucket '%s' received error "+
				"message: '%s'", remotePath, objStore.storeBucketName, err)
		}
	}

	if result == nil || result.VersionId == nil {
		return "0", false, fmt.Errorf("the AWS S3 operation to place a delete marker on '%s' "+
			"in S3 bucket '%s' did not return a version so "+
			"the delete operation can not proceed", remotePath, objStore.storeBucketName)
	}

	return *result.VersionId, false, nil
}

// delete a particular version for a given path
func (objStore *StoreAwsS3) Delete(existingDbRecord shared.BackedUpFileProperties, version int64, remoteVersion string, metadata bool) error {
	if existingDbRecord.Type != "file" {
		// directories and symlinks DO NOT GET UPLOADED so there is nothing to delete
		return nil
	}
	remotePath := calculateRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata)
	logger.Debugf("Deleting: '%s' having version: '%d' and remote version: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, version, remoteVersion, objStore.storeName, objStore.storeBucketName, remotePath)

	_, err := objStore.awsS3Client.DeleteObject(objStore.ctx, &s3.DeleteObjectInput{
		Bucket:    aws.String(objStore.storeBucketName),
		Key:       aws.String(remotePath),
		VersionId: aws.String(remoteVersion),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return fmt.Errorf("while trying to delete '%s' with version '%s' from S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", remotePath, remoteVersion, objStore.storeBucketName, apiErr.ErrorCode(), apiErr.ErrorMessage())
		} else {
			return fmt.Errorf("while trying to delete '%s' with version '%s' from S3 bucket '%s' received error "+
				"message: '%s'", remotePath, remoteVersion, objStore.storeBucketName, err)
		}
	}
	return nil
}

// Download a particular version of $existingDbRecord and save it at $restorePath; $version is ignored in this
// implementation but is specified due to being required by the interface specification
func (objStore *StoreAwsS3) Get(existingDbRecord shared.BackedUpFileProperties, restorePath string, version int64, remoteVersion string, metadata bool) (cancelled bool, err error) {
	if existingDbRecord.Type != "file" {
		return false, nil
	}

	remotePath := calculateRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata)
	logger.Debugf("Downloading: '%s' having remote version: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s' to local path: '%s'", existingDbRecord.Path, remoteVersion, objStore.storeName,
		objStore.storeBucketName, remotePath, restorePath)

	input := &s3.GetObjectInput{
		Bucket:    aws.String(objStore.storeBucketName),
		Key:       aws.String(remotePath),
		VersionId: aws.String(remoteVersion),
	}

	f, err := os.Create(restorePath) // #nosec G304 -- restore path is operator-supplied via REST API and validated upstream
	if err != nil {
		return false, fmt.Errorf("could not create file '%s' for restore: %s", restorePath, err)
	}
	defer f.Close()

	if existingDbRecord.Encrypted {
		return objStore.downloadEncrypted(existingDbRecord, restorePath, remotePath, remoteVersion, input, f)
	}
	return objStore.downloadMultipart(restorePath, remotePath, remoteVersion, input, f)
}

// downloadMultipart is the original (plaintext) path: parallel ranged downloads
// via manager.NewDownloader — fast for big objects, but incompatible with chunked AEAD
// which needs sequential read.
func (objStore *StoreAwsS3) downloadMultipart(restorePath, remotePath, remoteVersion string, input *s3.GetObjectInput, f *os.File) (bool, error) {
	downloader := manager.NewDownloader(objStore.awsS3Client)
	_, err := downloader.Download(objStore.ctx, f, input)
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			logger.Infof("received cancellation request while downloading '%s'", remotePath)
			return true, nil
		}
		return false, fmt.Errorf("while downloading '%s' with version '%s' from S3 bucket '%s': %s",
			remotePath, remoteVersion, objStore.storeBucketName, err)
	}
	return false, nil
}

// downloadEncrypted streams the object via single GetObject (sequential),
// validates the header's keystore_uuid against the cached sidecar UUID, and
// pipes the body through cbcrypto.NewDecryptingReader into the local file.
func (objStore *StoreAwsS3) downloadEncrypted(rec shared.BackedUpFileProperties, restorePath, remotePath, remoteVersion string, input *s3.GetObjectInput, f *os.File) (bool, error) {
	out, err := objStore.awsS3Client.GetObject(objStore.ctx, input)
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			logger.Infof("received cancellation request while downloading '%s'", remotePath)
			return true, nil
		}
		return false, fmt.Errorf("while downloading '%s' with version '%s' from S3 bucket '%s': %s",
			remotePath, remoteVersion, objStore.storeBucketName, err)
	}
	defer func() { _ = out.Body.Close() }()

	dr, err := cbcrypto.NewDecryptingReader(out.Body, objStore.KEK())
	if err != nil {
		return false, fmt.Errorf("wrap with DecryptingReader: %w", err)
	}
	hdr, err := dr.PeekHeader()
	if err != nil {
		return false, fmt.Errorf("parse encrypted header for '%s': %w", remotePath, err)
	}
	if hdr.KeystoreUUID != objStore.KeystoreUUID() {
		msg := fmt.Sprintf("encrypted object '%s' references a different keystore (header UUID %x, current sidecar UUID %x); cannot decrypt", remotePath, hdr.KeystoreUUID, objStore.KeystoreUUID())
		if objStore.backupJobsState != nil {
			objStore.backupJobsState.IncrementCounter(objStore.backupName, "decrypt_keystore_mismatch", rec.Path, "file", "restore", msg)
		}
		return false, errors.New(msg)
	}
	if _, err := io.Copy(f, dr); err != nil {
		if objStore.ctx.Err() == context.Canceled {
			logger.Infof("received cancellation request while decrypting '%s'", remotePath)
			return true, nil
		}
		return false, fmt.Errorf("while decrypting '%s' into '%s': %w", remotePath, restorePath, err)
	}
	return false, nil
}

// validate that the config of this object store is correct and that the credentials we have have sufficient access
// for a backup to be performed
func (objStore *StoreAwsS3) Validate() (string, error) {
	failedValidation := false
	failureMsg := ""

	// check Versioning is enabled and MFA Delete is not enabled
	versioningEnabled, mfaDeleteEnabled, err := objStore.checkBucketVersioningAndMFA()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While checking if S3 bucket '%s' has versioning enabled and MFA delete disabled, "+
			"encountered error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	} else {
		if !versioningEnabled {
			failureMsg += fmt.Sprintf("S3 bucket '%s' does not have versioning enabled and this is a required "+
				"setting. ", objStore.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
		if mfaDeleteEnabled {
			failureMsg += fmt.Sprintf("S3 bucket '%s' has MFA delete enabled and this setting needs to be "+
				"disabled as otherwise it will prevent proper operation of the backup "+
				"software. ", objStore.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
	}

	staticWebsiteEnabled, err := objStore.checkBucketHasStaticWebsiteEnabled()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While checking if S3 bucket '%s' has static website enabled encountered "+
			"error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	} else {
		if staticWebsiteEnabled {
			failureMsg += fmt.Sprintf("S3 bucket '%s' has static website hosting enabled and this setting needs to be "+
				"disabled as otherwise it can lead to backed up data being accessed by unauthorised parties. ",
				objStore.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
	}

	// check we can PUT and DELETE in the S3 bucket, directly under $prefix
	err = objStore.testUploadAndDelete()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While trying to upload and then delete a test file in S3 bucket '%s', "+
			"encountered error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	}

	if failedValidation {
		return failureMsg, errors.New(failureMsg)
	} else {
		return fmt.Sprintf("Target '%s' of type '%s' belonging to backup job '%s' passed validation",
			objStore.storeName, objStore.storeType, objStore.backupName), nil
	}
}

// checks if the S3 bucket has versioning enabled and MFA Delete disabled.
func (objStore *StoreAwsS3) checkBucketVersioningAndMFA() (versioningEnabled bool, mfaDeleteEnabled bool, err error) {
	var errMsg string
	logger.Debugf("Checking if S3 bucket '%s' has versioning enabled and MFA delete disabled", objStore.storeBucketName)

	result, err := objStore.awsS3Client.GetBucketVersioning(objStore.ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(objStore.storeBucketName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			errMsg = fmt.Sprintf("While checking if the S3 bucket '%s' has versioning enabled, encountered error: %s", objStore.storeBucketName, apiErr.ErrorMessage())
			logger.Error(errMsg)
			return versioningEnabled, mfaDeleteEnabled, fmt.Errorf("%s (code: %s)", apiErr.ErrorMessage(), apiErr.ErrorCode())
		} else {
			errMsg = fmt.Sprintf("While checking if the S3 bucket '%s' has versioning enabled, encountered error: %s", objStore.storeBucketName, err.Error())
			logger.Error(errMsg)
			return versioningEnabled, mfaDeleteEnabled, err
		}
	}

	if result != nil {
		if result.Status == types.BucketVersioningStatusEnabled {
			versioningEnabled = true
			logger.Debugf("S3 bucket '%s' has versioning enabled", objStore.storeBucketName)
		}
		if result.MFADelete == types.MFADeleteStatusEnabled {
			mfaDeleteEnabled = true
			logger.Debugf("S3 bucket '%s' has MFA delete enabled", objStore.storeBucketName)
		}
	}
	if !versioningEnabled {
		logger.Debugf("S3 bucket '%s' has versioning disabled", objStore.storeBucketName)
	}

	if !mfaDeleteEnabled {
		logger.Debugf("S3 bucket '%s' has MFA delete disabled", objStore.storeBucketName)
	}

	return versioningEnabled, mfaDeleteEnabled, nil
}

func (objStore *StoreAwsS3) checkBucketHasStaticWebsiteEnabled() (staticWebsiteEnabled bool, err error) {
	_, err = objStore.awsS3Client.GetBucketWebsite(objStore.ctx, &s3.GetBucketWebsiteInput{
		Bucket: aws.String(objStore.storeBucketName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "NoSuchWebsiteConfiguration" {
				// all is good as we don't want the bucket to have Static website support enabled
				return false, nil
			}
			errMsg := fmt.Sprintf("While checking that the S3 bucket '%s' does not have static website "+
				"hosting enabled, encountered error: %s", objStore.storeBucketName, apiErr.ErrorMessage())
			logger.Error(errMsg)
			return false, fmt.Errorf("%s (code: %s)", apiErr.ErrorMessage(), apiErr.ErrorCode())
		} else {
			errMsg := fmt.Sprintf("While checking that the S3 bucket '%s' does not have static website "+
				"hosting enabled, encountered error: %s", objStore.storeBucketName, err.Error())
			logger.Error(errMsg)
			return false, err
		}
	} else {
		// if we didn't get an error than static website hosting is enabled
		return true, nil
	}
}

// upload a test file and then delete it in order to validate permissions
func (objStore *StoreAwsS3) testUploadAndDelete() error {
	uploadPath := objStore.storePrefix + "/" + "test.txt"
	fakeReader := strings.NewReader(fmt.Sprintf("target privilege and settings validation - %s", time.Now().String()))

	logger.Debugf("Uploading test file to '%s' in order to validate PUT permission for S3 bucket '%s'", uploadPath, objStore.storeBucketName)
	result, err := objStore.awsS3Client.PutObject(objStore.ctx, &s3.PutObjectInput{
		Body:   fakeReader,
		Bucket: aws.String(objStore.storeBucketName),
		Key:    aws.String(uploadPath),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return fmt.Errorf("while trying to upload a test file to '%s' is S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", uploadPath, objStore.storeBucketName, apiErr.ErrorCode(), apiErr.ErrorMessage())
		} else {
			return fmt.Errorf("while trying to upload a test file to '%s' in S3 bucket '%s' received error "+
				"message: '%s'", uploadPath, objStore.storeBucketName, err)
		}
	} else {
		logger.Debugf("Successfully uploaded test file to '%s' in S3 bucket '%s'", uploadPath, objStore.storeBucketName)
	}

	if result == nil || result.VersionId == nil {
		return fmt.Errorf("AWS S3 upload for test file to '%s' in S3 bucket '%s' did not return a version so "+
			"the delete operation can not proceed", uploadPath, objStore.storeBucketName)
	} else {
		logger.Debugf("Deleting test file '%s' from S3 bucket '%s' in order to validate DELETE permissions", uploadPath, objStore.storeBucketName)
		_, err := objStore.awsS3Client.DeleteObject(objStore.ctx, &s3.DeleteObjectInput{
			Bucket:    aws.String(objStore.storeBucketName),
			Key:       aws.String(uploadPath),
			VersionId: aws.String(*(result.VersionId)),
		})
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) {
				return fmt.Errorf("while trying to delte test file '%s' from S3 bucket '%s' received error "+
					"code '%s' and error message: '%s'", uploadPath, objStore.storeBucketName, apiErr.ErrorCode(), apiErr.ErrorMessage())
			} else {
				return fmt.Errorf("while trying to delete test file '%s' from S3 bucket '%s' received error "+
					"message: '%s'", uploadPath, objStore.storeBucketName, err)
			}
		} else {
			logger.Debugf("Successfully deleted test file '%s' from S3 bucket '%s'.", uploadPath, objStore.storeBucketName)
		}
	}

	return nil
}

// queries the S3 bucket and tries to find its region; if it fails it will default to the one specified in the configuration file (if any)
func (objStore *StoreAwsS3) getRegionFromBucket() error {
	var errMsg string
	logger.Debugf("Attempting to figure out region for S3 bucket '%s'", objStore.storeBucketName)

	region, err := manager.GetBucketRegion(objStore.ctx, objStore.awsS3Client, objStore.storeBucketName)
	if err != nil {
		errMsg = fmt.Sprintf("unable to find bucket %s's region due to error: %s", objStore.storeBucketName, err)
		logger.Debug(errMsg)
		// if the user specified a region then we will use that and hope its the right one
		if objStore.region != "" {
			return nil
		} else {
			logger.Warn(errMsg)
			msg := fmt.Sprintf("unable to find bucket %s's region and there is no 'region' parameter defined in the "+
				"configuration file for this particular backup target. Please specify a 'region' parameter and a value"+
				" for it.", objStore.storeBucketName)
			logger.Error(msg)
			return errors.New(msg)
		}
	}
	logger.Debugf("Found S3 bucket '%s' to have AWS region '%s'", objStore.storeBucketName, region)
	if region != objStore.region {
		if objStore.region == "" {
			logger.Warnf("After querying the details of bucket '%s', it was reported that the bucket is hosted in "+
				"'%s' but you have not configured an AWS region. Please consider adjusting the configuration. "+
				"The region obtained from the bucket details will be used from now on",
				objStore.storeBucketName, region)
		} else {
			logger.Warnf("After querying the details of bucket '%s', it was reported that the bucket is hosted in "+
				"'%s' but you have configured AWS region '%s' . Please consider adjusting the configuration. "+
				"The region obtained from the bucket details will be used from now on",
				objStore.storeBucketName, region, objStore.region)
		}
	}
	objStore.region = region
	return nil
}

// returns an HTTP client with a rate-limiting transport that limits upload bandwidth.
// This client is passed to the AWS SDK v2 via config.WithHTTPClient
func newRateLimitedHttpClientForAWS(ctx context.Context, bucket *rate.Limiter, rateLimit uint64, burst uint64) *http.Client {
	logger.Debug("Setting up new HTTP client capable of rate limiting")

	wrappedTransport := &wrapAroundTransport{
		origTransport: http.DefaultTransport,
		ctx:           ctx,
		bucket:        bucket,
		rateLimit:     rateLimit,
		burst:         burst,
	}

	return &http.Client{Transport: wrappedTransport}
}
