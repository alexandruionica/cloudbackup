package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"fmt"
	"github.com/gofrs/uuid"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// wipes the "Parameters" of the first target belonging to the first backup and sets new parameter values based on
// environment variables. If the environment variables don't exist then it will fail as no test can be done without them
func getAndSetAzureBlobStorageConfigFromEnv(srvCfg *shared.RuntimeConfig, t *testing.T, newStoreName string) {
	// empty the parameters slice so we start from scratch
	srvCfg.Config.Backup[0].Target[0].Parameters = []shared.ConfigBackupTargetParams{}
	// force target type to be Azure blob
	srvCfg.Config.Backup[0].Target[0].Type = "azure_blob"
	srvCfg.Config.Backup[0].Target[0].Name = newStoreName
	// given that tests run in parallel on multiple OS's , ensure each has a different prefix so we don't have a clash
	srvCfg.Config.Backup[0].Target[0].Prefix = "tests/" + runtime.GOOS + "/" + t.Name()
	storageAccount := os.Getenv("CLD_AZURE_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Fatalf("Environment variable 'CLD_AZURE_STORAGE_ACCOUNT' is not set so the test doesn't know what " +
			"Azure Storage account to use")
	} else {
		srvCfg.Config.Backup[0].Target[0].Parameters = append(srvCfg.Config.Backup[0].Target[0].Parameters,
			shared.ConfigBackupTargetParams{
				Name:  "storage_account",
				Value: storageAccount,
			},
		)
	}
	StorageAccessKey := os.Getenv("CLD_AZURE_STORAGE_ACCESS_KEY")
	if StorageAccessKey == "" {
		t.Fatalf("Environment variable 'CLD_AZURE_STORAGE_ACCESS_KEY' is not set so the test doesn't know what " +
			"Azure Storage account key to use")
	} else {
		srvCfg.Config.Backup[0].Target[0].Parameters = append(srvCfg.Config.Backup[0].Target[0].Parameters,
			shared.ConfigBackupTargetParams{
				Name:  "storage_access_key",
				Value: StorageAccessKey,
			},
		)
	}
	aBucket := os.Getenv("CLD_AZURE_STORAGE_CONTAINER")
	if aBucket == "" {
		t.Fatalf("Environment variable 'CLD_AZURE_STORAGE_CONTAINER' is not set so the test doesn't know what " +
			"Azure Storage container(bucket) to use")
	} else {
		srvCfg.Config.Backup[0].Target[0].Bucket = aBucket
	}

	err := config.ValidateBackup(srvCfg.Config.Backup, true)
	if err != nil {
		t.Fatalf("Failed to validate the configuration file after adjusting the settings based on environment "+
			"variables specified by the user. The encountered error was: %s", err)
	}

}

// tests Validate() WITHOUT rate limiting enabled
func TestAzureBlobValidateUploadDelete1(t *testing.T) {
	const targetName string = "azure_blob_1"
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// disable rate limiting so multi-part uploads are enable and the S3 client is with signing enabled
	result.Config.Backup[0].Target[0].RateLimit = "0"

	// fetch settings from environment variables (if specified by caller) and adjust backup[0].target[0]
	getAndSetAzureBlobStorageConfigFromEnv(result, t, targetName)

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := shared.NewJobsState()

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	foundMatch := false
	var objStore ObjectStore
	for _, store := range objectStores {
		storeName, StoreType := store.GetStoreDetails()
		if storeName == targetName && StoreType == "azure_blob" {
			foundMatch = true
			objStore = store
			_, err = store.Validate()
			if err != nil {
				t.Fatalf("While validating the Azure Blob object store, encountered error: %s", err)
			}
		}
	}
	if !foundMatch {
		t.Fatalf("Did not find any backup target matching expected type 'azure_blob' and name '%s'", targetName)
	}

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAzureBlobValidateUploadDelete1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(path)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", path, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	// partially populated DB record to use for passing to Upload()
	fileDbRecord := shared.BackedUpFileProperties{
		Path: path,
		Type: utils.FileType(stat),
		Size: stat.Size(),
	}

	if objStore == nil {
		t.Fatalf("Someone broke the test (not the tested functions), $objStore is a nil pointer")
	}
	remoteVersion1, cancelled, err := objStore.Upload(fileDbRecord, 1, backupJobsState, false)
	if err != nil {
		t.Fatalf("1. Upload() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("1. Upload() returned that it was cancelled but this was not expected")
	}

	err = objStore.Delete(fileDbRecord, 1, remoteVersion1, false)
	if err != nil {
		t.Fatalf("Delete() for version 1 returned error: %s", err)
	}

	// delete again (what is now a missing object) - should return an error as this version of the file no longer exists
	err = objStore.Delete(fileDbRecord, 1, remoteVersion1, false)
	if err == nil {
		t.Fatal("Delete() for version 1 should have returned an error but didn't")
	}

	if strings.HasSuffix(err.Error(), "Code: BlobNotFound") {
		t.Fatalf("Was expecting error contains: 'Code: BlobNotFound' but instead got error: %s", err)
	}

	err = objStore.Delete(fileDbRecord, 1, "1231313123", false)
	if err == nil {
		t.Fatal("Calling Delete() for an incorrect version did not return an error")
	}

	// upload again so we can use Mark deleted on something
	remoteVersion2, cancelled, err := objStore.Upload(fileDbRecord, 2, backupJobsState, false)
	if err != nil {
		t.Fatalf("2. Upload() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("2. Upload() returned that it was cancelled but this was not expected")
	}

	remoteVersion2DeleteMarker, cancelled, err := objStore.MarkDeleted(fileDbRecord, 3, false)
	if err != nil {
		t.Fatalf("MarkDeleted() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("1. MarkDeleted() returned that it was cancelled but this was not expected")
	}

	// delete version two of file
	err = objStore.Delete(fileDbRecord, 2, remoteVersion2, false)
	if err != nil {
		t.Fatalf("Delete() for version 2 returned error: %s", err)
	}

	// delete delete marker
	err = objStore.Delete(fileDbRecord, 3, remoteVersion2DeleteMarker, false)
	if err != nil {
		t.Fatalf("Delete() of the delete marker returned error: %s", err)
	}

}

// tests Validate() WITH rate limiting enabled
func TestAzureBlobValidateUploadDelete2(t *testing.T) {
	const targetName string = "azure_blob_1"
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// enable rate limiting so multi-part uploads are disabled and the S3 client is with signing disabled
	result.Config.Backup[0].Target[0].RateLimit = "100000 KB"

	// fetch settings from environment variables (if specified by caller) and adjust backup[0].target[0]
	getAndSetAzureBlobStorageConfigFromEnv(result, t, targetName)

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := shared.NewJobsState()

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	foundMatch := false
	var objStore ObjectStore
	for _, store := range objectStores {
		storeName, StoreType := store.GetStoreDetails()
		if storeName == targetName && StoreType == "azure_blob" {
			foundMatch = true
			objStore = store
			_, err = store.Validate()
			if err != nil {
				t.Fatalf("While validating the Azure Blob object store, encountered error: %s", err)
			}
		}
	}
	if !foundMatch {
		t.Fatalf("Did not find any backup target matching expected type 'azure_blob' and name '%s'", targetName)
	}

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAzureBlobValidateUploadDelete1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(path)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", path, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	// partially populated DB record to use for passing to Upload()
	fileDbRecord := shared.BackedUpFileProperties{
		Path: path,
		Type: utils.FileType(stat),
		Size: stat.Size(),
	}

	if objStore == nil {
		t.Fatalf("Someone broke the test (not the tested functions), $objStore is a nil pointer")
	}
	remoteVersion1, cancelled, err := objStore.Upload(fileDbRecord, 1, backupJobsState, false)
	if err != nil {
		t.Fatalf("1. Upload() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("1. Upload() returned that it was cancelled but this was not expected")
	}

	err = objStore.Delete(fileDbRecord, 1, remoteVersion1, false)
	if err != nil {
		t.Fatalf("Delete() for version 1 returned error: %s", err)
	}

	// delete again (what is now a missing object) - should return an error as this version of the file no longer exists
	err = objStore.Delete(fileDbRecord, 1, remoteVersion1, false)
	if err == nil {
		t.Fatal("Delete() for version 1 should have returned an error but didn't")
	}

	if strings.HasSuffix(err.Error(), "Code: BlobNotFound") {
		t.Fatalf("Was expecting error contains: 'Code: BlobNotFound' but instead got error: %s", err)
	}

	err = objStore.Delete(fileDbRecord, 1, "1231313123", false)
	if err == nil {
		t.Fatal("Calling Delete() for an incorrect version did not return an error")
	}

	// upload again so we can use Mark deleted on something
	remoteVersion2, cancelled, err := objStore.Upload(fileDbRecord, 2, backupJobsState, false)
	if err != nil {
		t.Fatalf("2. Upload() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("2. Upload() returned that it was cancelled but this was not expected")
	}

	remoteVersion2DeleteMarker, cancelled, err := objStore.MarkDeleted(fileDbRecord, 3, false)
	if err != nil {
		t.Fatalf("MarkDeleted() returned error: %s", err)
	}
	if cancelled {
		t.Fatalf("1. MarkDeleted() returned that it was cancelled but this was not expected")
	}

	if cancelled {
		t.Fatalf("2. MarkDeleted() returned that it was cancelled but this was not expected")
	}

	// delete version two of file
	err = objStore.Delete(fileDbRecord, 2, remoteVersion2, false)
	if err != nil {
		t.Fatalf("Delete() for version 2 returned error: %s", err)
	}

	// delete delete marker
	err = objStore.Delete(fileDbRecord, 3, remoteVersion2DeleteMarker, false)
	if err != nil {
		t.Fatalf("Delete() of the delete marker returned error: %s", err)
	}
}
