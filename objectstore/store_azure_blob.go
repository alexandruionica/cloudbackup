package objectstore

import (
	"bytes"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"golang.org/x/time/rate"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	// Azure Pipeline- think of it as the base client wrapper which is then used to upload / download / perform other operations
	azurePipeline pipeline.Pipeline
	// Azure Blob Storage URL - this thing is used to interact with the container (but not to upload / download from containers)
	azureServiceURL azblob.ServiceURL
	// Azure Container URL object which wraps the above Azure Pipeline - this is what is used to upload / download from containers
	azureContainerURL azblob.ContainerURL
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
	result.azurePipeline = azblob.NewPipeline(credential, azblob.PipelineOptions{})

	var primaryBlobServiceEndpoint string
	GetStringParameter("primary_blob_service_endpoint", &primaryBlobServiceEndpoint, target.Parameters, "")
	var ContainerURL, ServiceURL *url.URL
	if primaryBlobServiceEndpoint != "" {
		ServiceURL, err = url.Parse(primaryBlobServiceEndpoint)
		if err != nil {
			return result, fmt.Errorf("while constructing the Azure Storage Service URL using the supplied "+
				"'primary_blob_service_endpoint' parameter, the following error was encountered: %s", err)
		}

		ContainerURL, err = url.Parse(primaryBlobServiceEndpoint + "/" + result.storeBucketName)
		if err != nil {
			return result, fmt.Errorf("while constructing the Azure Storage Container URL using the supplied "+
				"'primary_blob_service_endpoint' parameter, the following error was encountered: %s", err)
		}
	} else {
		// Using the default storage account blob service URL endpoint as the user did not specify any
		ServiceURL, err = url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net", result.azureAccountName))
		if err != nil {
			return result, fmt.Errorf("while constructing the Azure Storage Service URL, encountered error: %s", err)
		}

		ContainerURL, err = url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/%s",
			result.azureAccountName, result.storeBucketName))
		if err != nil {
			return result, fmt.Errorf("while constructing the Azure Storage Container URL, encountered error: %s", err)
		}
	}

	// Create an ServiceURL object that wraps the service URL and a request pipeline. This is used to make request to
	// the overall storage account
	logger.Debugf("Setting up connection to Azure Blob Storage")
	result.azureServiceURL = azblob.NewServiceURL(*ServiceURL, result.azurePipeline)

	// Create a ContainerURL object that wraps the container URL and a request pipeline to make requests.
	result.azureContainerURL = azblob.NewContainerURL(*ContainerURL, result.azurePipeline)
	logger.Debugf("Done setting up connection to Azure Blob Storage")

	return result, nil
}

func (objStore *StoreAzureBlob) Upload(DbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {

	// Azure Blob doesn't support versioning so we use our own scheme of  "v" + $version appended together with a "." to the file name
	remotePath, remoteVersion := calculateAzureStorageRemotePath(objStore.storePrefix, DbRecord.Path, metadata, version, false)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", DbRecord.Path, version, objStore.storeName, objStore.storeBucketName, remotePath)
	blobURL := objStore.azureContainerURL.NewBlockBlobURL(remotePath)

	if DbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(DbRecord.Path, objStore.bucket, objStore.backupJobsState, objStore.backupName, objStore.storeName,
			objStore.storeType, 0, objStore.burst, DbRecord.Size, objStore.ctx, true) // we pass ratelimit as 0 because the rate limiting will be done (if needed) by the http.Client
		if err != nil {
			return remoteVersion, false, err
		}
		defer reader.Close()

		// upload
		_, err = azblob.UploadStreamToBlockBlob(objStore.ctx, &reader, blobURL, azblob.UploadStreamToBlockBlobOptions{
			BufferSize: 5 * 1024 * 1024,
			MaxBuffers: 3})
		if err != nil {
			if objStore.ctx.Err() == context.Canceled {
				msg := fmt.Sprintf("received cancellation request while uploading content of %s", DbRecord.Path)
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

func (objStore *StoreAzureBlob) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreAzureBlob) Delete(existingDbRecord shared.BackedUpFileProperties, version int64, remoteVersion string, metadata bool) error {
	return fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreAzureBlob) Get(existingDbRecord shared.BackedUpFileProperties, restorePath string, version int64, remoteVersion string, metadata bool) (cancelled bool, err error) {
	return false, fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreAzureBlob) Validate() (string, error) {
	failedValidation := false
	failureMsg := ""

	// check static website hosting is not enabled
	serviceProperties, err := objStore.azureServiceURL.GetProperties(objStore.ctx)
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While fetching the properties of Azure storage account '%s', encountered "+
			"error: %s ", objStore.azureAccountName, err)
	} else {
		if serviceProperties.StaticWebsite != nil && serviceProperties.StaticWebsite.Enabled {
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

	// prepares target filename in the object store
	blobURL := objStore.azureContainerURL.NewBlockBlobURL(uploadPath)

	// upload
	logger.Debugf("Uploading test file to '%s' in order to validate PUT permission for Azure container(bucket) '%s'",
		uploadPath, objStore.storeBucketName)
	_, err := azblob.UploadStreamToBlockBlob(objStore.ctx, fakeReader, blobURL, azblob.UploadStreamToBlockBlobOptions{
		BufferSize: 2 * 1024 * 1024,
		MaxBuffers: 3})
	if err != nil {
		return fmt.Errorf("while finishing up the upload of a test file to '%s' in Azure container(bucket) '%s' "+
			"received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	}
	logger.Debugf("Successfully uploaded test file to '%s' in Azure container(bucket) '%s'",
		uploadPath, objStore.storeBucketName)

	// download
	logger.Debugf("Downloading test file '%s' from Azure container(bucket) '%s'", uploadPath, objStore.storeBucketName)
	downloadResponse, err := blobURL.Download(objStore.ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false)
	if err != nil {
		return fmt.Errorf("while trying to setup a download for a test file from '%s' in Azure container(bucket)"+
			" '%s' received error message: '%s'", uploadPath, objStore.storeBucketName, err)
	} else {
		// NOTE: automatically retries are performed if the connection fails
		bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
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
	_, err = blobURL.Delete(objStore.ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
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
