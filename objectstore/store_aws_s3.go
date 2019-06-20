package objectstore

import (
	"cloudbackup/shared"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/time/rate"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	// multipart uploads are enabled
	multipartAllowed bool
	region           string
	// AWS client session
	awsSess *session.Session
	// AWS managed uploader which does a lot of work (retries/multipart upload/etc)
	awsUploader *s3manager.Uploader
	// AWS S3 session
	awsS3Svc *s3.S3
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
	}

	// if any of those parameters was set then read its value and seed the struct
	GetStringParameter("AWS_ACCESS_KEY_ID", &result.awsAccessKeyId, target.Parameters, "")
	GetStringParameter("AWS_SECRET_ACCESS_KEY", &result.awsSecretAccessKey, target.Parameters, "")
	GetStringParameter("storage_class", &result.storageClass, target.Parameters, "")
	GetStringParameter("region", &result.region, target.Parameters, "")
	// if we have a key id and secret then use them
	if result.awsSecretAccessKey != "" && result.awsAccessKeyId != "" {
		logger.Debugf("Using user specified credentials")
		result.awsSess, err = session.NewSessionWithOptions(session.Options{
			Config: aws.Config{Region: aws.String(result.region),
				Credentials: credentials.NewStaticCredentials(result.awsAccessKeyId, result.awsSecretAccessKey, ""),
			},
		})
		// we don't have a key id and session so we'll fall back to the SDK's defaults
	} else {
		logger.Debugf("no credentials specified in the config, so defaulting to the defaults of the AWS SDK")
		result.awsSess, err = session.NewSessionWithOptions(session.Options{
			Config: aws.Config{
				Region: aws.String(result.region),
			},
		})
	}
	if err != nil {
		return result, fmt.Errorf("could not setup AWS session for target '%s' belonging to backup '%s' due "+
			"to error: %s", target.Name, backupConfig.Name, err)
	}

	// try to determine programatically the region; if not found then it will fallback to the user specified one; for
	// this specific call, we don't need an authenticated session
	err = result.getRegionFromBucket()
	if err != nil {
		return result, err
	}

	// unfortunately the way the AWS SDK implements multipart uploads is incompatible with the rate limiter. What the
	// S3 SDK does is buffer 5MB worth of file (or less if size of item is smaller) in 5 GO routines and only then proceed
	// to upload. This makes the rate limiter mostly useless as the rate limiter operates on the io.Reader interface
	// provided to the AWS SDK
	if ratelimit == 0 {
		result.multipartAllowed = true
	} else {
		logger.Info("Not using multipart uploads as rate limiting is enabled")
	}

	result.awsUploader = s3manager.NewUploader(result.awsSess)
	// Create a S3 client with additional configuration
	if result.multipartAllowed {
		result.awsS3Svc = s3.New(result.awsSess,
			aws.NewConfig().WithRegion(result.region))
	} else {
		// disable MD5 signing as it requires reading each file twice
		result.awsS3Svc = s3.New(result.awsSess, aws.NewConfig().WithS3DisableContentMD5Validation(true),
			aws.NewConfig().WithRegion(result.region))
		// the S3 sdk is braindead and will by default read the whole file , compute a SHA256 checksum and then include it
		// in the call to the API (HTTP header X-Amz-Content-Sha256). It proved to be extremely difficult to figure out
		// how to disable this. The downside to disabling this is that if the file gets corrupted in flight, you won't be notified
		result.awsS3Svc.Handlers.Sign.PushFrontNamed(v4.BuildNamedHandler(v4.SignRequestHandler.Name, func(s *v4.Signer) {
			s.UnsignedPayload = true
		}))
	}

	return result, nil
}

// pretend to upload file (actually discarding all read content)
func (object *StoreAwsS3) Upload(newDbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface, metadata bool) (remoteVersion string, cancelled bool, err error) {

	remotePath := calculateAwsS3RemotePath(object.storePrefix, newDbRecord.Path, metadata)
	logger.Debugf("Uploading: '%s' having version: '%d' to object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", newDbRecord.Path, version, object.storeName, object.storeBucketName, remotePath)

	if newDbRecord.Type == "file" {
		// setup io.Reader (this handles reporting and optional rate limiting)
		reader, err := NewFileReader(newDbRecord.Path, object.bucket, object.backupJobsState, object.backupName, object.storeName,
			object.storeType, object.rateLimit, object.burst, newDbRecord.Size, object.ctx, true)
		if err != nil {
			return strconv.FormatInt(version, 10), false, err
		}
		defer reader.Close()

		// upload using Multipart uploads and the S3 upload manager
		if object.multipartAllowed {
			result, err := object.awsUploader.UploadWithContext(object.ctx, &s3manager.UploadInput{
				Bucket: aws.String(object.storeBucketName),
				Key:    aws.String(remotePath),
				Body:   &reader,
			})
			if err != nil {
				return strconv.FormatInt(version, 10), false, err
			}
			if result != nil && result.VersionID != nil {
				return *(result.VersionID), false, nil
			} else {
				msg := fmt.Sprintf("upload of '%s' was reported "+
					"successful but the upload response does not contain a file version. This means the backed up copy is "+
					"unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown", newDbRecord.Path)
				logger.Errorf(msg)
				return strconv.FormatInt(version, 10), false, errors.New(msg)
			}
			// upload using streaming upload (without multipart upload)
		} else {
			result, err := object.awsS3Svc.PutObject(&s3.PutObjectInput{
				Body:   aws.ReadSeekCloser(&reader),
				Bucket: aws.String(object.storeBucketName),
				Key:    aws.String(remotePath),
			})
			if err != nil {
				return strconv.FormatInt(version, 10), false, err
			}
			if result != nil && result.VersionId != nil {
				return *(result.VersionId), false, nil
			} else {
				msg := fmt.Sprintf("upload of '%s' was reported "+
					"successful but the upload response does not contain a file version. This means the backed up copy is "+
					"unusable and it's unsafe to delete it as the 'version' of the uploaded item is unknown", newDbRecord.Path)
				logger.Errorf(msg)
				return strconv.FormatInt(version, 10), false, errors.New(msg)
			}
		}
	} else {
		// directories and symlinks DO NOT GET UPLOADED
		return strconv.FormatInt(version, 10), false, nil
	}
}

func (object *StoreAwsS3) GetStoreDetails() (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}

// place a delete marker on the newest version of a file. This is achieved by deleting the file without specifying a
// version. AWS does not allow a place marker operation so this is the only way to get a marker
func (object *StoreAwsS3) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, metadata bool) (remoteVersion string, cancelled bool, err error) {
	remotePath := calculateAwsS3RemotePath(object.storePrefix, existingDbRecord.Path, metadata)
	logger.Debugf("Marking as deleted: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", existingDbRecord.Path, object.storeName, object.storeBucketName, remotePath)

	inputDelete := &s3.DeleteObjectInput{
		Bucket: aws.String(object.storeBucketName),
		Key:    aws.String(remotePath),
	}
	result, err := object.awsS3Svc.DeleteObject(inputDelete)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "0", false, fmt.Errorf("while trying to place a delete marker on '%s' "+
				"from S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", remotePath, object.storeBucketName, aerr.Code(), aerr.Message())
		} else {
			return "0", false, fmt.Errorf("while trying to place a delete marker on '%s'"+
				" from S3 bucket '%s' received error "+
				"message: '%s'", remotePath, object.storeBucketName, err)
		}
	}

	if result == nil || result.VersionId == nil {
		return "0", false, fmt.Errorf("the AWS S3 operation to place a delete marker on '%s' "+
			"in S3 bucket '%s' did not return a version so "+
			"the delete operation can not proceed", remotePath, object.storeBucketName)
	}

	return *result.VersionId, false, nil
}

// delete a particular version for a given path; $objType is one of "dir"/"file"/"symlink"
func (object *StoreAwsS3) Delete(path string, objType string, version int64, remoteVersion string, metadata bool) error {
	remotePath := calculateAwsS3RemotePath(object.storePrefix, path, metadata)
	logger.Debugf("Marking as deleted: '%s' having version: '%d' and remote version: '%s' from object store: '%s' using bucket: '%s' and"+
		" full remote path: '%s'", path, version, remoteVersion, object.storeName, object.storeBucketName, remotePath)

	inputDelete := &s3.DeleteObjectInput{
		Bucket:    aws.String(object.storeBucketName),
		Key:       aws.String(remotePath),
		VersionId: aws.String(remoteVersion),
	}
	_, err := object.awsS3Svc.DeleteObject(inputDelete)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("while trying to delete '%s' with version '%s' from S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", remotePath, remoteVersion, object.storeBucketName, aerr.Code(), aerr.Message())
		} else {
			return fmt.Errorf("while trying to delete '%s' with version '%s' from S3 bucket '%s' received error "+
				"message: '%s'", remotePath, remoteVersion, object.storeBucketName, err)
		}
	}
	return nil
}

// validate that the config of this object store is correct and that the credentials we have have sufficient access
// for a backup to be performed
func (object *StoreAwsS3) Validate() (string, error) {
	failedValidation := false
	failureMsg := ""

	// check Versioning is enabled and MFA Delete is not enabled
	versioningEnabled, mfaDeleteEnabled, err := object.checkBucketVersioningAndMFA()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While checking if S3 bucket '%s' has versioning enabled  MFA delete disabled, "+
			"encountered error: %s. ", object.storeBucketName, err) // must leave one whitespace at end of sentence
	} else {
		if !versioningEnabled {
			failureMsg += fmt.Sprintf("S3 bucket '%s' does not have versioning enabled and this is a required "+
				"setting. ", object.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
		if mfaDeleteEnabled {
			failureMsg += fmt.Sprintf("S3 bucket '%s' has MFA delete enabled and this setting needs to be "+
				"disabled as otherwise it will prevent proper operation of the backup "+
				"software. ", object.storeBucketName) // must leave one whitespace at end of sentence
			failedValidation = true
		}
	}

	// check we can PUT and DELETE in the S3 bucket, directly under $prefix
	err = object.testUploadAndDelete()
	if err != nil {
		failedValidation = true
		failureMsg += fmt.Sprintf("While trying to upload and then delete a test file in S3 bucket '%s', "+
			"encountered error: %s. ", object.storeBucketName, err) // must leave one whitespace at end of sentence
	}

	if failedValidation {
		return failureMsg, errors.New(failureMsg)
	} else {
		return fmt.Sprintf("%s passed validation", object.storeName), nil
	}
}

// checks if the S3 bucket has versioning enabled and MFA Delete disabled.
func (object *StoreAwsS3) checkBucketVersioningAndMFA() (versioningEnabled bool, mfaDeleteEnabled bool, err error) {
	var errMsg string
	logger.Debugf("Checking if S3 bucket '%s' has versioning enabled and MFA delete disabled", object.storeBucketName)

	input := &s3.GetBucketVersioningInput{
		Bucket: aws.String(object.storeBucketName),
	}

	result, err := object.awsS3Svc.GetBucketVersioning(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				errMsg = fmt.Sprintf("While checking if the S3 bucket '%s' has versioning enabled, encountered error: %s", object.storeBucketName, aerr.Error())
				logger.Errorf(errMsg)
				return versioningEnabled, mfaDeleteEnabled, errors.New(errMsg)
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
			errMsg = fmt.Sprintf("While checking if the S3 bucket '%s' has versioning enabled, encountered error: %s", object.storeBucketName, err.Error())
			logger.Errorf(errMsg)
			return versioningEnabled, mfaDeleteEnabled, errors.New(errMsg)
		}
	}

	if result != nil {
		if result.Status != nil {
			if strings.ToLower(*(result.Status)) == "enabled" {
				versioningEnabled = true
				logger.Debugf("S3 bucket '%s' has versioning enabled", object.storeBucketName)
			}
		}
		if result.MFADelete != nil {
			if strings.ToLower(*(result.MFADelete)) == "enabled" {
				mfaDeleteEnabled = true
				logger.Debugf("S3 bucket '%s' has MFA delete enabled", object.storeBucketName)
			}
		}
	}
	if !versioningEnabled {
		logger.Debugf("S3 bucket '%s' has versioning disabled", object.storeBucketName)
	}

	if !mfaDeleteEnabled {
		logger.Debugf("S3 bucket '%s' has MFA delete disabled", object.storeBucketName)
	}

	return versioningEnabled, mfaDeleteEnabled, nil
}

// upload a test file and then delete it in order to validate permissions
func (object *StoreAwsS3) testUploadAndDelete() error {
	uploadPath := object.storePrefix + "/" + "test.txt"
	fakeReader := strings.NewReader(fmt.Sprintf("target privilege and settings validation - %s", time.Now().String()))
	input := &s3.PutObjectInput{
		Body:   aws.ReadSeekCloser(fakeReader),
		Bucket: aws.String(object.storeBucketName),
		Key:    aws.String(uploadPath),
	}
	logger.Debugf("Uploading test file to '%s' in order to validate PUT permission for S3 bucket '%s'", uploadPath, object.storeBucketName)
	result, err := object.awsS3Svc.PutObject(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("while trying to upload a test file to '%s' is S3 bucket '%s' received error "+
				"code '%s' and error message: '%s'", uploadPath, object.storeBucketName, aerr.Code(), aerr.Message())
		} else {
			return fmt.Errorf("while trying to upload a test file to '%s' in S3 bucket '%s' received error "+
				"message: '%s'", uploadPath, object.storeBucketName, err)
		}
	} else {
		logger.Debugf("Successfully uploaded test file to '%s' in S3 bucket '%s'", uploadPath, object.storeBucketName)
	}

	if result == nil || result.VersionId == nil {
		return fmt.Errorf("AWS S3 upload for test file to '%s' in S3 bucket '%s' did not return a version so "+
			"the delete operation can not proceed", uploadPath, object.storeBucketName)
	} else {
		inputDelete := &s3.DeleteObjectInput{
			Bucket:    aws.String(object.storeBucketName),
			Key:       aws.String(uploadPath),
			VersionId: aws.String(*(result.VersionId)),
		}
		logger.Debugf("Deleting test file '%s' from S3 bucket '%s' in order to validate DELETE permissions", uploadPath, object.storeBucketName)
		_, err := object.awsS3Svc.DeleteObject(inputDelete)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				return fmt.Errorf("while trying to delte test file '%s' from S3 bucket '%s' received error "+
					"code '%s' and error message: '%s'", uploadPath, object.storeBucketName, aerr.Code(), aerr.Message())
			} else {
				return fmt.Errorf("while trying to delete test file '%s' from S3 bucket '%s' received error "+
					"message: '%s'", uploadPath, object.storeBucketName, err)
			}
		} else {
			logger.Debugf("Successfully deleted test file '%s' from S3 bucket '%s'.", uploadPath, object.storeBucketName)
		}
	}

	return nil
}

// queries the S3 bucket and tries to find its region; if it fails it will default to the one specified in the configuration file (if any)
func (object *StoreAwsS3) getRegionFromBucket() error {
	var errMsg, region string
	var err error
	logger.Debugf("Attempting to figure out region for S3 bucket '%s'", object.storeBucketName)
	// if the user did not specify a region, default to us-east-1 as we need to specify a region hint
	if object.region == "" {
		region, err = s3manager.GetBucketRegion(object.ctx, object.awsSess, object.storeBucketName, "us-east-1")
	} else {
		region, err = s3manager.GetBucketRegion(object.ctx, object.awsSess, object.storeBucketName, object.region)
	}
	if err != nil {
		errMsg = fmt.Sprintf("unable to find bucket %s's region due to error: %s", object.storeBucketName, err)
		logger.Debug(errMsg)
		// if the user specified a region then we will use that and hope its the right one
		if object.region != "" {
			return nil
		} else {
			logger.Warn(errMsg)
			msg := fmt.Sprintf("unable to find bucket %s's region and there is no 'region' parameter defined in the "+
				"configuration file for this particular backup target. Please specify a 'region' parameter and a value"+
				" for it.", object.storeBucketName)
			logger.Errorf(msg)
			return errors.New(msg)
		}
	}
	logger.Debugf("Found S3 bucket '%s' to have AWS region '%s'", object.storeBucketName, region)
	if region != object.region {
		if object.region == "" {
			logger.Warnf("After querying the details of bucket '%s', it was reported that the bucket is hosted in "+
				"'%s' but you have not configured an AWS region. Please consider adjusting the configuration. "+
				"The region obtained from the bucket details will be used from now on",
				object.storeBucketName, region)
		} else {
			logger.Warnf("After querying the details of bucket '%s', it was reported that the bucket is hosted in "+
				"'%s' but you have configured AWS region '%s' . Please consider adjusting the configuration. "+
				"The region obtained from the bucket details will be used from now on",
				object.storeBucketName, region, object.region)
		}
	}
	object.region = region
	return nil
}

// for a given $prefix , $path and $metadata (true if file is metadata, false if not) return the remote path
func calculateAwsS3RemotePath(prefix string, path string, metadata bool) string {
	var prepend string
	if metadata {
		prepend = MetaDataPrepend
	} else {
		prepend = DataPrepend
	}
	// ensure MS Windows paths are converted to forward slash; otherwise filepath.ToSlash() should not affect Unixes
	return filepath.ToSlash(prefix + "/" + prepend + "/" + path)
}
