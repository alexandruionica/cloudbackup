package objectstore

import (
	"bytes"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/mattn/go-ieproxy"
	"golang.org/x/time/rate"
)

// Microsoft Azure Blob storage account
type StoreAzureBlob struct {
	ctx        context.Context
	backupName string
	storeName  string
	storeType  string
	// name of the object store bucket used for storing content. In Azure language a "container" is what AWS S3 and GCP storage call a "bucket"
	storeBucketName string
	// prefix to prepend to all backed up items. This normally "backup.target.prefix" + $separator + "backup.name";
	// ANY CHANGE TO THIS MAY BREAK ALREADY MADE BACKUPS
	storePrefix string
	// this is the rate limiter bucket (token bucket); not to be confused with the above $storeBucketName
	bucket          *rate.Limiter
	rateLimit       uint64
	burst           uint64
	backupJobsState shared.BackupJobsStateInterface
	// Azure storage account name - this is concept which is somewhat similar to what a GCP account is and to what an
	// AWS account would be if the only service AWS had was S3 . In your Azure account you create different storage
	// accounts and withing the storage accounts you create containers (which are like S3 buckets) and then withing a
	// container to upload / download contant to/from
	azureAccountName string
	// Azure SDK client used for all blob operations (upload, download, delete) and service-level queries
	azureClient *azblob.Client
}

func InitialiseStoreAzureBlob(ctx context.Context, backupConfig shared.ConfigBackup, target shared.ConfigBackupTarget, rateLimitStr string, backupJobsState shared.BackupJobsStateInterface) (*StoreAzureBlob, error) {
	var rateLimitBucket *rate.Limiter

	rateLimitBucket, ratelimit, burst, err := setupRateLimiterBucket(rateLimitStr, target.Name, backupConfig.Name)
	if err != nil {
		return &StoreAzureBlob{}, err
	}

	result := &StoreAzureBlob{
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

	var storageAccessKey string
	GetStringParameter("storage_account", &result.azureAccountName, target.Parameters, "")
	GetStringParameter("storage_access_key", &storageAccessKey, target.Parameters, "")
	// TODO - implement also Azure Metadata service lookup and make key based auth optional

	credential, err := azblob.NewSharedKeyCredential(result.azureAccountName, storageAccessKey)
	if err != nil {
		logger.Errorf("invalid credentials. When initialising the Azure blob storage account, received error: %s", err)
	}

	// The new Azure SDK accepts a standard *http.Client as Transport in ClientOptions, which makes rate limiting
	// integration much cleaner than the old pipeline factory approach. We pass our rate-limited HTTP client directly.
	httpClient := newRateLimitedHttpClientForAzure(ctx, rateLimitBucket, ratelimit, burst)
	clientOptions := &azblob.ClientOptions{}
	clientOptions.Transport = httpClient

	var serviceURL string
	var primaryBlobServiceEndpoint string
	GetStringParameter("primary_blob_service_endpoint", &primaryBlobServiceEndpoint, target.Parameters, "")
	if primaryBlobServiceEndpoint != "" {
		serviceURL = primaryBlobServiceEndpoint
	} else {
		// Using the default storage account blob service URL endpoint as the user did not specify any
		serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net/", result.azureAccountName)
	}

	// according to https://blogs.msdn.microsoft.com/windowsazurestorage/2011/02/17/windows-azure-blob-md5-overview/ MD5
	// checksumming is not needed if HTTPS is used as transfport (and we enforce it's use)

	logger.Debugf("Setting up connection to Azure Blob Storage")
	result.azureClient, err = azblob.NewClientWithSharedKeyCredential(serviceURL, credential, clientOptions)
	if err != nil {
		return result, fmt.Errorf("while creating the Azure Blob Storage client, encountered error: %s", err)
	}
	logger.Debugf("Done setting up connection to Azure Blob Storage")

	return result, nil
}

func (objStore *StoreAzureBlob) Upload(DbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {

	// Azure Blob doesn't support versioning so we use our own scheme of  "v" + $version appended together with a "." to the file name
	remotePath, remoteVersion := calculateAzureStorageRemotePath(objStore.storePrefix, DbRecord.Path, metadata, version, false)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", DbRecord.Path, version, objStore.storeName, objStore.storeBucketName, remotePath)

	if DbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(DbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
			objStore.storeType, 0, objStore.burst, DbRecord.Size, objStore.ctx, true) // we pass ratelimit as 0 because the rate limiting will be done (if needed) by the http.Client
		if err != nil {
			return remoteVersion, false, err
		}
		defer reader.Close()

		// upload
		_, err = objStore.azureClient.UploadStream(objStore.ctx, objStore.storeBucketName, remotePath, &reader,
			&azblob.UploadStreamOptions{
				BlockSize:   5 * 1024 * 1024,
				Concurrency: 3,
			})
		if err != nil {
			if objStore.ctx.Err() == context.Canceled {
				msg := fmt.Sprintf("received cancellation request while uploading content of '%s'", DbRecord.Path)
				logger.Info(msg)
				return remoteVersion, true, errors.New(msg)
			}
			return remoteVersion, false, err
		}
		// if we got here then upload worked as expected
		return remoteVersion, false, nil
	} else {
		// directories and symlinks DO NOT GET UPLOADED
		return remoteVersion, false, nil
	}
}

// GetStoreDetails()(StoreName string, StoreType string)
func (objStore *StoreAzureBlob) GetStoreDetails() (StoreName string, StoreType string) {
	return objStore.storeName, objStore.storeType
}

// Mark a file as deleted by uploading a 0 bytes file which has the same name but has the suffix ".d${markerVersion}"
//
//	($markerVersion gets replaces with the value of the parameter). This is needed because Azure Blobs does not
//	support versioning (it supports snapshots but that is not the same thing as versioning and it has limitations)
func (objStore *StoreAzureBlob) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	// Azure Blob doesn't support versioning so we use our own scheme of  "d" + $version appended together with a "." to the file name
	remotePath, remoteVersion := calculateAzureStorageRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata, markerVersion, true)
	if existingDbRecord.Type != "file" {
		// directories and symlinks DO NOT GET UPLOADED so there is nothing to mark deleted
		return remotePath, false, nil
	}

	logger.Debugf("Marking as deleted: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, objStore.storeName, objStore.storeBucketName, remotePath)

	fakeReader := strings.NewReader("") // zero bytes content
	// upload aka place the marker which is a 0 bytes file
	_, err = objStore.azureClient.UploadStream(objStore.ctx, objStore.storeBucketName, remotePath, fakeReader,
		&azblob.UploadStreamOptions{
			BlockSize:   5 * 1024,
			Concurrency: 3,
		})
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			msg := fmt.Sprintf("received cancellation request while placing a delete marker for '%s'", existingDbRecord.Path)
			logger.Info(msg)
			return remoteVersion, true, errors.New(msg)
		}
		return remoteVersion, false, err
	}
	// if we got here then upload(marker placement) worked as expected
	return remoteVersion, false, nil

}

func (objStore *StoreAzureBlob) Delete(existingDbRecord shared.BackedUpFileProperties, version int64, remoteVersion string, metadata bool) error {
	// the Upload() function prefixes the returned version with "v" while the MarkDeleted() prefixes with "d"
	deleteMarker := strings.HasPrefix(remoteVersion, "d")
	// Azure Blob doesn't support versioning so we use our own scheme of  "d" + $version appended together with a "." to the file name
	remotePath, remoteVersion := calculateAzureStorageRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata, version, deleteMarker)

	if existingDbRecord.Type != "file" {
		// directories and symlinks DO NOT GET UPLOADED so there is nothing to delete
		return nil
	}
	logger.Debugf("Deleting: '%s' having version: '%d' and remote version: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, version, remoteVersion, objStore.storeName, objStore.storeBucketName, remotePath)

	_, err := objStore.azureClient.DeleteBlob(objStore.ctx, objStore.storeBucketName, remotePath, nil)
	if err != nil {
		return fmt.Errorf("while trying to delete the file '%s' from Azure Blobs container(bucket)"+
			" '%s' received error message: '%s'", existingDbRecord.Path, objStore.storeBucketName, err)
	}

	return nil
}

func (objStore *StoreAzureBlob) Get(existingDbRecord shared.BackedUpFileProperties, restorePath string, version int64, remoteVersion string, metadata bool) (cancelled bool, err error) {
	if existingDbRecord.Type != "file" {
		return false, nil
	}

	isDeleteMarker := len(remoteVersion) > 0 && remoteVersion[0] == 'd'
	remotePath, _ := calculateAzureStorageRemotePath(objStore.storePrefix, existingDbRecord.Path, metadata, version, isDeleteMarker)
	logger.Debugf("Downloading: '%s' having remote version: '%s' from object store: '%s' using container: '%s' and"+
		" full remote path: '%s' to local path: '%s'", existingDbRecord.Path, remoteVersion, objStore.storeName,
		objStore.storeBucketName, remotePath, restorePath)

	downloadResponse, err := objStore.azureClient.DownloadStream(objStore.ctx, objStore.storeBucketName, remotePath, nil)
	if err != nil {
		if objStore.ctx.Err() == context.Canceled {
			logger.Infof("received cancellation request while downloading '%s'", remotePath)
			return true, nil
		}
		return false, fmt.Errorf("while trying to download '%s' with version '%s' from Azure container '%s': %s",
			remotePath, remoteVersion, objStore.storeBucketName, err)
	}

	bodyStream := downloadResponse.Body
	defer bodyStream.Close()

	f, err := os.Create(restorePath)
	if err != nil {
		return false, fmt.Errorf("could not create file '%s' for restore: %s", restorePath, err)
	}
	defer f.Close()

	buf := make([]byte, 1024*1024)
	for {
		n, readErr := bodyStream.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return false, fmt.Errorf("while writing restored content to '%s': %s", restorePath, writeErr)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if objStore.ctx.Err() == context.Canceled {
				logger.Infof("received cancellation request while downloading '%s'", remotePath)
				return true, nil
			}
			return false, fmt.Errorf("while reading download stream for '%s': %s", remotePath, readErr)
		}
	}

	return false, nil
}

func (objStore *StoreAzureBlob) Validate() (string, error) {
	failedValidation := false
	failureMsg := ""

	// check static website hosting is not enabled
	serviceClient := objStore.azureClient.ServiceClient()
	serviceProperties, err := serviceClient.GetProperties(objStore.ctx, nil)
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While fetching the properties of Azure storage account '%s', encountered "+
			"error: %s ", objStore.azureAccountName, err)
	} else {
		if serviceProperties.StaticWebsite != nil && serviceProperties.StaticWebsite.Enabled != nil && *serviceProperties.StaticWebsite.Enabled {
			failedValidation = true
			failureMsg += "Static website hosting is enabled on the container (bucket) which is going to used for " +
				"backups. Refusing to start as this may lead to the backups data being accessed by third parties. "
		}
	}

	// check we can PUT, GET and DELETE in the Azure container(bucket), directly under $prefix
	err = objStore.testUploadGetDelete()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While trying to upload and then delete a test file in Azure container(bucket) '%s', "+
			"encountered error: %s. ", objStore.storeBucketName, err) // must leave one whitespace at end of sentence
	}

	if failedValidation {
		return failureMsg, errors.New(failureMsg)
	} else {
		return fmt.Sprintf("Target '%s' of type '%s' belonging to backup job '%s' passed validation",
			objStore.storeName, objStore.storeType, objStore.backupName), nil
	}
}

// testUploadGetDelete
func (objStore *StoreAzureBlob) testUploadGetDelete() error {
	uploadPath := objStore.storePrefix + "/" + "test.txt"
	uploadMsg := fmt.Sprintf("target privilege and settings validation - %s", time.Now().String())
	fakeReader := strings.NewReader(uploadMsg)

	// upload
	logger.Debugf("Uploading test file to '%s' in order to validate PUT permission for Azure container(bucket) '%s'",
		uploadPath, objStore.storeBucketName)
	_, err := objStore.azureClient.UploadStream(objStore.ctx, objStore.storeBucketName, uploadPath, fakeReader,
		&azblob.UploadStreamOptions{
			BlockSize:   2 * 1024 * 1024,
			Concurrency: 3,
		})
	if err != nil {
		return fmt.Errorf("while finishing up the upload of a test file to '%s' in Azure container(bucket) '%s' "+
			"received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	logger.Debugf("Successfully uploaded test file to '%s' in Azure container(bucket) '%s'",
		uploadPath, objStore.storeBucketName)

	// download
	logger.Debugf("Downloading test file '%s' from Azure container(bucket) '%s'", uploadPath, objStore.storeBucketName)
	downloadResponse, err := objStore.azureClient.DownloadStream(objStore.ctx, objStore.storeBucketName, uploadPath, nil)
	if err != nil {
		return fmt.Errorf("while trying to setup a download for a test file from '%s' in Azure container(bucket)"+
			" '%s' received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	} else {
		bodyStream := downloadResponse.Body
		// read the body into a buffer
		downloadedData := bytes.Buffer{}
		_, err = downloadedData.ReadFrom(bodyStream)
		if err != nil {
			return fmt.Errorf("while trying downloading a test file from '%s' in Azure container(bucket)"+
				" '%s' received error message: '%s'", uploadPath, objStore.storeBucketName, err)
		}
		logger.Debugf("Successfully downloaded test file '%s' from Azure container(bucket) '%s'", uploadPath, objStore.storeBucketName)

		if downloadedData.String() != uploadMsg {
			return fmt.Errorf("downloaded test file content from '%s' in Azure container(bucket) '%s' did not match the uploaded "+
				"content", uploadPath, objStore.storeBucketName)
		}
	}

	// delete
	logger.Debugf("Deleting test file '%s' from Azure container(bucket) '%s'", uploadPath, objStore.storeBucketName)
	_, err = objStore.azureClient.DeleteBlob(objStore.ctx, objStore.storeBucketName, uploadPath, nil)
	if err != nil {
		return fmt.Errorf("while trying to delete the test file from '%s' in Azure container(bucket)"+
			" '%s' received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	} else {
		logger.Debugf("Successfully deleted test file '%s' from Azure container(bucket) '%s'", uploadPath, objStore.storeBucketName)
	}

	return nil
}

// return the remote path and remote version for a given $prefix , $path and $metadata (true if file is metadata, false if not), $version is
// needed as Azure Blobs doesn't natively support versioning so we need to use our scheme and $delete_marker influences the version scheme result
func calculateAzureStorageRemotePath(prefix string, path string, metadata bool, version int64, deleteMarker bool) (string, string) {
	// Azure Blob doesn't support versioning so we use our own scheme of:
	//   "v" or "d" + $version appended together with a "." to the file name
	//   "v" is for "version" and "d" for "delete marker"
	var remoteVersion string
	if deleteMarker {
		remoteVersion = "d" + strconv.FormatInt(version, 10)
	} else {
		remoteVersion = "v" + strconv.FormatInt(version, 10)
	}

	if metadata {
		// when dealing with metadata, we want to store on the remote only the filename, excluding the rest of the local path
		filename := filepath.Base(path) + "." + remoteVersion
		// ensure MS Windows paths are converted to forward slash; otherwise filepath.ToSlash() should not affect Unixes
		remotePath := filepath.ToSlash(prefix + "/" + MetaDataPrepend + "/" + filename)
		// ensure we don't have double forward slashes
		return utils.SquashForwardSlashes(remotePath), remoteVersion
	} else {
		// ensure MS Windows paths are converted to forward slash; otherwise filepath.ToSlash() should not affect Unixes
		remotePath := filepath.ToSlash(prefix + "/" + DataPrepend + "/" + path + "." + remoteVersion)
		// ensure we don't have double forward slashes
		return utils.SquashForwardSlashes(remotePath), remoteVersion
	}
}

//

// returns an HTTP client with a rate-limiting transport that limits upload bandwidth.
// This client is passed to the Azure SDK via ClientOptions.Transport
func newRateLimitedHttpClientForAzure(ctx context.Context, bucket *rate.Limiter, rateLimit uint64, burst uint64) *http.Client {
	logger.Debug("Setting up new HTTP client capable of rate limiting")

	wrappedTransport := &wrapAroundTransport{
		origTransport: &http.Transport{
			Proxy: ieproxy.GetProxyFunc(),
			// We use Dial instead of DialContext as DialContext has been reported to cause slower performance.
			Dial /*Context*/ : (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).Dial, /*Context*/
			MaxIdleConns:           0, // No limit
			MaxIdleConnsPerHost:    100,
			IdleConnTimeout:        90 * time.Second,
			TLSHandshakeTimeout:    10 * time.Second,
			ExpectContinueTimeout:  1 * time.Second,
			DisableKeepAlives:      false,
			DisableCompression:     false,
			MaxResponseHeaderBytes: 0,
		},
		ctx:       ctx,
		bucket:    bucket,
		rateLimit: rateLimit,
		burst:     burst,
	}

	return &http.Client{Transport: wrappedTransport}
}
