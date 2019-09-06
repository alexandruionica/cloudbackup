package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gcpStorage "cloud.google.com/go/storage"
	"cloudbackup/shared"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	gcpTransport "google.golang.org/api/transport/http"
)

type StoreGcpStorage struct {
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
	// GCP storage class
	storageClass string
	// GCP client session
	gcpStorageClient *gcpStorage.Client
	// GCP bucket object used for various API calls
	gcpBucketObj *gcpStorage.BucketHandle
	// if to disable the use of CRC32c hashing of files before uploading them (in order for GCP to validate no corruption happened in flight)
	disableCrc32cHash bool
}

func InitialiseStoreGcpStorage(ctx context.Context, backupConfig shared.ConfigBackup, target shared.ConfigBackupTarget, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreGcpStorage, error) {
	var rateLimitBucket *rate.Limiter

	rateLimitBucket, ratelimit, burst, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
	if err != nil {
		return &StoreGcpStorage{}, err
	}

	result := &StoreGcpStorage{
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
	}

	GetStringParameter("storage_class", &result.storageClass, target.Parameters, "")
	GetBoolParameter("disable_crc32c_hash", &result.disableCrc32cHash, target.Parameters, false)

	// if we got the credentials in the config then attempt to use them
	if foundGcpCredentialsInTargetConfig(target.Parameters) {
		credentialsJsonBlob, err := makeCredentialsJson(target.Parameters)
		if err != nil {
			return &StoreGcpStorage{}, err
		}
		// This rate limiter limits the HTTP POST method which means in practice it limits uploads only
		rateLimitedHttpClient := newRateLimitedHttpClientForGcp(ctx, rateLimitBucket, ratelimit, burst, credentialsJsonBlob)
		result.gcpStorageClient, err = gcpStorage.NewClient(ctx, option.WithCredentialsJSON(credentialsJsonBlob), option.WithHTTPClient(rateLimitedHttpClient))
		if err != nil {
			return &StoreGcpStorage{}, fmt.Errorf("failed to create GCP client using provided credentials due to error: %s", err)
		}
		// trying to login using the SDK for GCP's own default rules for locating credentials
	} else {
		// This rate limiter limits the HTTP POST method which means in practice it limits uploads only
		rateLimitedHttpClient := newRateLimitedHttpClientForGcp(ctx, rateLimitBucket, ratelimit, burst, []byte{})
		result.gcpStorageClient, err = gcpStorage.NewClient(ctx, option.WithHTTPClient(rateLimitedHttpClient))
		if err != nil {
			return &StoreGcpStorage{}, fmt.Errorf("failed to create GCP client due to error: %s", err)
		}
	}

	result.gcpBucketObj = result.gcpStorageClient.Bucket(result.storeBucketName)

	return result, nil
}

func (objStore *StoreGcpStorage) Upload(DbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {
	remotePath := calculateGcpStorageRemotePath(objStore.storePrefix, DbRecord.Path, metadata)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", DbRecord.Path, version, objStore.storeName, objStore.storeBucketName, remotePath)

	if DbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(DbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
			objStore.storeType, 0, objStore.burst, DbRecord.Size, objStore.ctx, true) // we pass ratelimit as 0 because the rate limiting will be done (if needed) by the http.Client
		if err != nil {
			return strconv.FormatInt(version, 10), false, err
		}
		defer reader.Close()

		// upload
		wc := objStore.gcpBucketObj.Object(remotePath).NewWriter(objStore.ctx)

		// this leads to an extra read of a file, before it is uploaded, in order to compute the hash
		if !objStore.disableCrc32cHash {
			wc.CRC32C, err = crc32Hash(DbRecord.Path)
			if err != nil {
				return "", false, fmt.Errorf("could not calculate CRC32c checksum for '%s'", DbRecord.Path)
			}
			wc.SendCRC32C = true
		}

		// this operation is async (according to GCP library's docs) so a nil error may still mean a failure will happen.
		// Use the wc.Close() below in order to be sure the upload is complete
		if _, err := io.Copy(wc, &reader); err != nil {
			if objStore.ctx.Err() == context.Canceled {
				msg := fmt.Sprintf("received cancellation request while uploading content to %s", remotePath)
				logger.Info(msg)
				return strconv.FormatInt(version, 10), true, errors.New(msg)
			}
			return strconv.FormatInt(version, 10), false, err
		}
		if err := wc.Close(); err != nil {
			if objStore.ctx.Err() == context.Canceled {
				msg := fmt.Sprintf("received cancellation request while uploading content to %s", remotePath)
				logger.Info(msg)
				return strconv.FormatInt(version, 10), true, errors.New(msg)
			}
			msg := fmt.Sprintf("while finishing up the upload to '%s' in GCP bucket '%s' received error "+
				"message: '%s'", remotePath, objStore.storeBucketName, err)
			logger.Error(msg)
			return strconv.FormatInt(version, 10), false, errors.New(msg)
		}

		attrs := wc.Attrs()

		if attrs != nil {
			return strconv.FormatInt(attrs.Generation, 10), false, nil
		} else {
			msg := fmt.Sprintf("upload of '%s' was reported "+
				"successful but the upload response does not contain a file version. This means the backed up copy is "+
				"unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown", DbRecord.Path)
			logger.Errorf(msg)
			return strconv.FormatInt(version, 10), false, errors.New(msg)
		}
	} else {
		// directories and symlinks DO NOT GET UPLOADED
		return strconv.FormatInt(version, 10), false, nil
	}
}

// GetStoreDetails()(StoreName string, StoreType string)
func (objStore *StoreGcpStorage) GetStoreDetails() (StoreName string, StoreType string) {
	return objStore.storeName, objStore.storeType
}

// place a delete marker on the newest version of a file. This is achieved by deleting the file without specifying a
// version. GCP does not actually have a concept of a delete marker.
func (objStore *StoreGcpStorage) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	remotePath := calculateGcpStorageRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata)
	logger.Debugf("Marking as deleted: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, objStore.storeName, objStore.storeBucketName, remotePath)
	err = objStore.gcpBucketObj.Object(remotePath).Delete(objStore.ctx)
	if err != nil {
		if err == gcpStorage.ErrObjectNotExist {
			logger.Errorf("while trying to place a delete marker on '%s'"+
				" from GCP storage bucket '%s' received error "+
				"message: '%s' . None the less this item will be marked as deleted but you must investigate what "+
				"happened on the object store side as your backup may be compromised and be invalid if files no longer "+
				"exist on the object store side, despite being expected to exist", remotePath, objStore.storeBucketName, err)
			return string(markerVersion) + "_delete_marker", false, nil
		}
		return "0", false, fmt.Errorf("while trying to place a delete marker on '%s'"+
			" from GCP storage bucket '%s' received error "+
			"message: '%s'", remotePath, objStore.storeBucketName, err)
	}
	// append "_delete_marker" to the version so later when it is requested to delete the delete marker itself we know
	// to not do anything (as GCP storage doesn't have a concept of delete markers)
	return string(markerVersion) + "_delete_marker", false, nil
}

func (objStore *StoreGcpStorage) Delete(path string, objType string, version int64, remoteVersion string, metadata bool) error {
	remotePath := calculateGcpStorageRemotePath(objStore.storePrefix, path, metadata)
	logger.Debugf("Deleting: '%s' having version: '%d' and remote version: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", path, version, remoteVersion, objStore.storeName, objStore.storeBucketName, remotePath)
	// if this a request to delete a "delete marker" then return success as this object store does not have the concept
	//   of delete markers so there is nothing to delete
	if strings.HasSuffix(remoteVersion, "_delete_marker") {
		return nil
	}

	generation, err := strconv.ParseInt(remoteVersion, 10, 64)
	if err != nil {
		return fmt.Errorf("could not convert '%s' to an int64 due to error: %s . Because of this, deletion of "+
			"'%s' with version '%s' from object store: '%s' using bucket: '%s' and full remote path: '%s' is not possible",
			remoteVersion, err, path, remoteVersion, objStore.storeName, objStore.storeBucketName, remotePath)
	}
	err = objStore.gcpBucketObj.Object(remotePath).Generation(generation).Delete(objStore.ctx)
	if err != nil {
		return fmt.Errorf("while trying to delete '%s' with version '%s' from GCP storage bucket '%s' received error "+
			"message: '%s'", remotePath, remoteVersion, objStore.storeBucketName, err)
	}
	return nil
}

func (objStore *StoreGcpStorage) Validate() (string, error) {
	failedValidation := false
	failureMsg := ""

	attrs, err := objStore.gcpBucketObj.Attrs(objStore.ctx)
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While fetching the properties of GCP storage bucket '%s' "+
			"encountered error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	} else {
		if !attrs.VersioningEnabled {
			failureMsg += fmt.Sprintf("GCP bucket '%s' does not have versioning enabled and this is a required "+
				"setting. ", objStore.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
	}

	// check we can PUT and DELETE in the GCP bucket, directly under $prefix
	err = objStore.testUploadGetDelete()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While trying to upload and then delete a test file in GCP bucket '%s', "+
			"encountered error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	}

	if failedValidation {
		return failureMsg, errors.New(failureMsg)
	} else {
		return fmt.Sprintf("%s passed validation", objStore.storeName), nil
	}
}

// upload a test file, retrieve it back and then delete it in order to validate permissions
func (objStore *StoreGcpStorage) testUploadGetDelete() error {
	uploadPath := objStore.storePrefix + "/" + "test.txt"
	uploadMsg := fmt.Sprintf("target privilege and settings validation - %s", time.Now().String())
	fakeReader := strings.NewReader(uploadMsg)

	// upload
	logger.Debugf("Uploading test file to '%s' in order to validate PUT permission for GCP bucket '%s'", uploadPath, objStore.storeBucketName)
	wc := objStore.gcpBucketObj.Object(uploadPath).NewWriter(objStore.ctx)
	// this operation is async (according to GCP library's docs) so a nil error may still mean a failure will happen.
	// Use the wc.Close() below in order to be sure the upload is complete
	if _, err := io.Copy(wc, fakeReader); err != nil {
		return fmt.Errorf("while trying to upload a test file to '%s' in GCP bucket '%s' received error "+
			"message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("while finishing up the upload of a test file to '%s' in GCP bucket '%s' received error "+
			"message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	logger.Debugf("Successfully uploaded test file to '%s' in GCP bucket '%s'", uploadPath, objStore.storeBucketName)

	// download
	rc, err := objStore.gcpBucketObj.Object(uploadPath).NewReader(objStore.ctx)
	if err != nil {
		if rc != nil {
			err2 := rc.Close()
			if err2 != nil {
				logger.Warnf("could not close handle on GCP uploaded test file '%s' located in GCP bucket '%s', "+
					"due to received error message: '%s'", uploadPath, objStore.storeBucketName, err2)
			}
		}
		return fmt.Errorf("while trying to setup a download for a test file from '%s' in GCP bucket '%s' received error "+
			"message: '%s'", uploadPath, objStore.storeBucketName, err)
	}

	logger.Debugf("Downloading test file '%s' from GCP bucket '%s'", uploadPath, objStore.storeBucketName)
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		err2 := rc.Close()
		if err2 != nil {
			logger.Warnf("could not close handle on GCP uploaded test file '%s' located in GCP bucket '%s', "+
				"due to received error message: '%s'", uploadPath, objStore.storeBucketName, err2)
		}
		return fmt.Errorf("while trying to download a test file from '%s' in GCP bucket '%s' received error "+
			"message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	logger.Debugf("Successfully downloaded test file '%s' from GCP bucket '%s'", uploadPath, objStore.storeBucketName)

	err = rc.Close()
	if err != nil {
		logger.Warnf("Could not close handle on GCP uploaded test file '%s' located in GCP bucket '%s', "+
			"due to received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	}

	if uploadMsg != string(data) {
		return fmt.Errorf("downloaded test file content from '%s' in GCP bucket '%s' did not match the uploaded "+
			"content", uploadPath, objStore.storeBucketName)

	}

	// delete
	logger.Debugf("Deleting test file '%s' from GCP bucket '%s'", uploadPath, objStore.storeBucketName)
	err = objStore.gcpBucketObj.Object(uploadPath).Delete(objStore.ctx)
	if err != nil {
		return fmt.Errorf("could not delete GCP uploaded test file '%s' located in GCP bucket '%s', due to "+
			"received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	logger.Debugf("Successfully deleted test file '%s' from GCP bucket '%s'", uploadPath, objStore.storeBucketName)

	return nil

}

// reads the passed parameters and assembles a json object. Returns json object and error (if any then json object needs to be ignored)
func makeCredentialsJson(params []shared.ConfigBackupTargetParams) ([]byte, error) {
	//credentialParameters := [...]string{"type", "project_id", "private_key_id", "private_key", "client_email", "client_id",
	//	"auth_uri", "token_uri", "auth_provider_x509_cert_url", "client_x509_cert_url"}
	type GcpCredJson struct {
		Type                    string `yaml:"type" json:"type"`
		ProjectId               string `yaml:"project_id" json:"project_id"`
		PrivateKeyId            string `yaml:"private_key_id" json:"private_key_id"`
		PrivateKey              string `yaml:"private_key" json:"private_key"`
		ClientEmail             string `yaml:"client_email" json:"client_email"`
		ClientId                string `yaml:"client_id" json:"client_id"`
		AuthUri                 string `yaml:"auth_uri" json:"auth_uri"`
		TokenUri                string `yaml:"token_uri" json:"token_uri"`
		AuthProviderX509Certurl string `yaml:"auth_provider_x509_cert_url" json:"auth_provider_x509_cert_url"`
		ClientX509CertUrl       string `yaml:"client_x509_cert_url" json:"client_x509_cert_url"`
	}
	var gcpCred GcpCredJson
	for _, param := range params {
		switch sanitisedParam := strings.ToLower(param.Name); sanitisedParam {
		case "type":
			gcpCred.Type = param.Value
		case "project_id":
			gcpCred.ProjectId = param.Value
		case "private_key_id":
			gcpCred.PrivateKeyId = param.Value
		case "private_key":
			gcpCred.PrivateKey = param.Value
		case "client_email":
			gcpCred.ClientEmail = param.Value
		case "client_id":
			gcpCred.ClientId = param.Value
		case "auth_uri":
			gcpCred.AuthUri = param.Value
		case "token_uri":
			gcpCred.TokenUri = param.Value
		case "auth_provider_x509_cert_url":
			gcpCred.AuthProviderX509Certurl = param.Value
		case "client_x509_cert_url":
			gcpCred.ClientX509CertUrl = param.Value
		}
	}
	result, err := json.Marshal(gcpCred)
	if err != nil {
		logger.Warningf("Could not json encode provided GCP credentials due to error: %s", err)
		return nil, err
	}
	return result, nil
}

// returns true if in the Target definition (in the Params section) the GCP private key is specified
func foundGcpCredentialsInTargetConfig(params []shared.ConfigBackupTargetParams) bool {
	for _, entry := range params {
		if strings.ToLower(entry.Name) == "private_key" {
			return true
		}
	}
	return false
}

// for a given $prefix , $path and $metadata (true if file is metadata, false if not) return the remote path
func calculateGcpStorageRemotePath(prefix string, path string, metadata bool) string {
	if metadata {
		// when dealing with metadata, we want to store on the remote only the filename, excluding the rest of the local path
		filename := filepath.Base(path)
		// ensure MS Windows paths are converted to forward slash; otherwise filepath.ToSlash() should not affect Unixes
		return filepath.ToSlash(prefix + "/" + MetaDataPrepend + "/" + filename)
	} else {
		// ensure MS Windows paths are converted to forward slash; otherwise filepath.ToSlash() should not affect Unixes
		return filepath.ToSlash(prefix + "/" + DataPrepend + "/" + path)
	}
}

// returns a HTTP client which can then be passed to the GCP sdk, when initialising the SDK. For this to work,
// the implementation of the the http.Transport interface must be the one used by the GCP SDK as it takes care of
// authentication and probably other things. Unfortunately this means that upgrades of the GCP SDK can lead to issues
// if they start doing things differently.
// This rate limiter limits the HTTP POST method which means in practice it limits uploads only
func newRateLimitedHttpClientForGcp(ctx context.Context, bucket *rate.Limiter, rateLimit uint64, burst uint64, credentialBlob []byte) *http.Client {
	logger.Debug("Setting up new HTTP client capable of rate limiting")
	var httpTransport http.RoundTripper
	var err error
	// how we call gcpTransport.NewTransport is tied deeply to the implementation of the GCP APIs in GO. If that library changes, it may affect us
	if len(credentialBlob) > 0 {
		httpTransport, err = gcpTransport.NewTransport(ctx, http.DefaultTransport, option.WithScopes(gcpStorage.ScopeFullControl), option.WithCredentialsJSON(credentialBlob))
	} else {
		httpTransport, err = gcpTransport.NewTransport(ctx, http.DefaultTransport, option.WithScopes(gcpStorage.ScopeFullControl))
	}
	if err != nil {
		logger.Errorf("While trying to setup the GCP rate limited http client, got error: %s", err)
	}

	wrappedTransport := &wrapAroundTransport{
		origTransport: httpTransport,
		ctx:           ctx,
		bucket:        bucket,
		rateLimit:     rateLimit,
		burst:         burst,
	}

	return &http.Client{Transport: wrappedTransport}
}
