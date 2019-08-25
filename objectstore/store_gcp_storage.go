package objectstore

import (
	gcpStorage "cloud.google.com/go/storage"
	"cloudbackup/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	"io"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
}

func InitialiseStoreGcpStorage(ctx context.Context, backupConfig shared.ConfigBackup, target shared.ConfigBackupTarget, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreGcpStorage, error) {
	var rateLimitBucket *rate.Limiter

	rateLimitBucket, ratelimit, burst, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
	if err != nil {
		return &StoreGcpStorage{}, err
	}

	// these will be used for rate limiting via the http.Client and we want them separated from the above, just in
	// case we end up using both (in maybe slightly different ways)
	rateLimitBucket2, ratelimit2, burst2, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
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

	// TODO - what to do about enabling MD5 checksum hash + send in order to validate that the uploaded file matches the signature given the performance implications

	GetStringParameter("storage_class", &result.storageClass, target.Parameters, "")

	// if we got the credentials in the config then attempt to use them
	if foundGcpCredentialsInTargetConfig(target.Parameters) {
		credentialsJsonBlob, err := makeCredentialsJson(target.Parameters)
		if err != nil {
			return &StoreGcpStorage{}, err
		}
		rateLimitedHttpClient := newRateLimitedHttpClientForGcp(ctx, rateLimitBucket2, ratelimit2, burst2, credentialsJsonBlob)
		result.gcpStorageClient, err = gcpStorage.NewClient(ctx, option.WithCredentialsJSON(credentialsJsonBlob), option.WithHTTPClient(rateLimitedHttpClient))
		if err != nil {
			return &StoreGcpStorage{}, fmt.Errorf("failed to create GCP client using provided credentials due to error: %s", err)
		}
		// trying to loging using the sdk for GCP's own default rules for locating credentials
	} else {
		rateLimitedHttpClient := newRateLimitedHttpClientForGcp(ctx, rateLimitBucket2, ratelimit2, burst2, []byte{})
		result.gcpStorageClient, err = gcpStorage.NewClient(ctx, option.WithHTTPClient(rateLimitedHttpClient))
		if err != nil {
			return &StoreGcpStorage{}, fmt.Errorf("failed to create GCP client due to error: %s", err)
		}
	}

	result.gcpBucketObj = result.gcpStorageClient.Bucket(result.storeBucketName)

	return result, nil
}

func (objStore *StoreGcpStorage) Upload(newDbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {
	remotePath := calculateGcpStorageRemotePath(objStore.storePrefix, newDbRecord.Path, metadata)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", newDbRecord.Path, version, objStore.storeName, objStore.storeBucketName, remotePath)

	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(newDbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
			objStore.storeType, 0, objStore.burst, newDbRecord.Size, objStore.ctx, true) // we pass ratelimit as 0 because the rate limiting will be done (if needed) by the http.Client
		if err != nil {
			return strconv.FormatInt(version, 10), false, err
		}
		defer reader.Close()

		// upload
		wc := objStore.gcpBucketObj.Object(remotePath).NewWriter(objStore.ctx)
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
				"unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown", newDbRecord.Path)
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

func (objStore *StoreGcpStorage) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("function MarkDeleted() note yet implemented for backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreGcpStorage) Delete(path string, objType string, version int64, remoteVersion string, metadata bool) error {
	return fmt.Errorf("function Delete() note yet implemented for backend of type: '%s'", objStore.storeType)
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
