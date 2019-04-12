package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"context"
	"github.com/satori/go.uuid"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// with a config RateLimit of "10 kb"
func TestSetupRateLimiterBucket1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "10 kb"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit, result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}
	if ratelimitNumeric != 10000 {
		t.Fatalf("setupRateLimiterBucket() returned %d but 10000 was expected.", ratelimitNumeric)
	}
	if burst < 1 {
		t.Fatalf("setupRateLimiterBucket() returned a ratelimit >0 but burst is < 1 (returned burst value is"+
			" '%d'). If ratelimit that >0 then burst needs to be >0", burst)
	}
	if bucket == nil {
		t.Fatalf("the bucket returned by setupRateLimiterBucket() is a nil pointer instead of a valid pointer")
	}
}

// with a config RateLimit of "0"
func TestSetupRateLimiterBucket2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "0"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit, result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}
	if ratelimitNumeric != 0 {
		t.Fatalf("setupRateLimiterBucket() returned %d but 0 was expected.", ratelimitNumeric)
	}
	if burst != 0 {
		t.Fatalf("setupRateLimiterBucket() returned a ratelimit 0 but burst != 0 (returned burst value is"+
			" '%d'). If ratelimit = 0 then we should get burst = 0", burst)
	}
	if bucket != nil {
		t.Fatalf("the bucket returned by setupRateLimiterBucket() should have been a a nil pointer but we got" +
			" a valid pointer")
	}
}

// with a config RateLimit of "5"
func TestSetupRateLimiterBucket3(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "5"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit, result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}
	if ratelimitNumeric != 5 {
		t.Fatalf("setupRateLimiterBucket() returned %d but 5 was expected.", ratelimitNumeric)
	}
	if burst != 1 {
		t.Fatalf("setupRateLimiterBucket() returned a ratelimit of %d but burst != 1 (returned burst value is"+
			" '%d'). If ratelimit that <10 then burst needs to be 1", ratelimitNumeric, burst)
	}
	if bucket == nil {
		t.Fatalf("the bucket returned by setupRateLimiterBucket() is a nil pointer instead of a valid pointer")
	}
}

// with a config RateLimit of "9223372036854775808"
func TestSetupRateLimiterBucket4(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "9223372036854775808"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit, result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}
	if ratelimitNumeric != 9223372036854775807 {
		t.Fatalf("setupRateLimiterBucket() returned %d but 9223372036854775807 was expected.", ratelimitNumeric)
	}
	if burst != MaxBufferSize {
		t.Fatalf("setupRateLimiterBucket() returned a ratelimit of %d but burst != %d (returned burst value is"+
			" '%d'). If burst > 2147483647 or burst > %d then burst needs to be lowered to whatever value of the "+
			"previous two is smaller", ratelimitNumeric, MaxBufferSize, burst, MaxBufferSize)
	}
	if bucket == nil {
		t.Fatalf("the bucket returned by setupRateLimiterBucket() is a nil pointer instead of a valid pointer")
	}
}

// test with a config RateLimit of "10" (10 bytes/sec) passing a file name which doesn't exist
func TestNewFileReader1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "10"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit, result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}
	fileToOpen := "a_missing_file_" + uuid.NewV4().String()
	_, err = NewFileReader(fileToOpen, bucket, backupJobsState, result.Config.Backup[0].Name,
		result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Target[0].Type, ratelimitNumeric, burst, 1000, context.TODO())
	if err == nil {
		t.Fatalf("NewFileReader() was supposed to return an error because file %s should not exist but it didn't return an error", fileToOpen)
	}
}

// test with a config RateLimit of "10" (10 bytes/sec) passing a file name which exists
func TestNewFileReader2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "10"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit,
		result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}

	// setup a 20 byte long file
	tmpPath := utils.SetupTmpDir("unittest_objectstore_common_test", t)
	fileToOpen := tmpPath + string(filepath.Separator) + "file.txt"
	err = ioutil.WriteFile(fileToOpen, []byte(`12345678901234567890`), 0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{tmpPath})
	statHandle, err := os.Stat(fileToOpen)
	if err != nil {
		t.Fatalf("Could not os.Stat() file %s", fileToOpen)
	}
	fileSize := statHandle.Size()

	reader, err := NewFileReader(fileToOpen, bucket, backupJobsState, result.Config.Backup[0].Name,
		result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Target[0].Type, ratelimitNumeric, burst,
		fileSize, context.TODO())
	if err != nil {
		t.Fatalf("while running NewFileReader() got error: %s ", err)
	}
	// sleep 3 seconds before starting the test in order to give the OS a chance to settle a bit as we measure how fast the operation took and if the OS is busy it could make the test flaky
	time.Sleep(3 * time.Second)
	// create a 1 KB buffer to hold read content
	p := make([]byte, 1024)
	startTime := time.Now()
	// fetch all file content
InfiniteLoop:
	for {
		_, err := reader.Read(p)
		if err != nil {
			switch err {
			// io.Reader reports io.EOF when reaching the end of the file. This is normal and expected
			case io.EOF:
				{
					duration := time.Since(startTime)
					if duration.Seconds() < 2 {
						t.Fatalf("Reading the rate limited file should have taken 2 seconds but it took %f",
							duration.Seconds())
					}
					if duration.Seconds() > 2.2 {
						t.Fatalf("Reading the rate limited file should have taken a bit over 2 seconds but it took %f",
							duration.Seconds())
					}
					reader.Close()
					break InfiniteLoop
				}
			case context.Canceled:
				{
					break InfiniteLoop
				}
			default:
				{
					t.Fatalf("While reading '%s' the following error was encountered: %s", path, err)
				}
			}
		}
	}
}

// test with a config RateLimit of "0" (unlimited) passing a file name which exists
func TestNewFileReader3(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].RateLimit = "0"
	err = config.Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}

	bucket, ratelimitNumeric, burst, err := setupRateLimiterBucket(result.Config.Backup[0].Target[0].RateLimit,
		result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("While trying to setup ratelimit bucket got error: %s", err)
	}

	// setup a 20 byte long file
	tmpPath := utils.SetupTmpDir("unittest_objectstore_common_test", t)
	fileToOpen := tmpPath + string(filepath.Separator) + "file.txt"
	err = ioutil.WriteFile(fileToOpen, []byte(`12345678901234567890`), 0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{tmpPath})

	statHandle, err := os.Stat(fileToOpen)
	if err != nil {
		t.Fatalf("Could not os.Stat() file %s", fileToOpen)
	}
	fileSize := statHandle.Size()

	reader, err := NewFileReader(fileToOpen, bucket, backupJobsState, result.Config.Backup[0].Name,
		result.Config.Backup[0].Target[0].Name, result.Config.Backup[0].Target[0].Type, ratelimitNumeric, burst,
		fileSize, context.TODO())
	if err != nil {
		t.Fatalf("while running NewFileReader() got error: %s ", err)
	}
	// create a 1 KB buffer to hold read content
	p := make([]byte, 1024)
	startTime := time.Now()
	// fetch all file content
InfiniteLoop:
	for {
		_, err := reader.Read(p)
		if err != nil {
			switch err {
			// io.Reader reports io.EOF when reaching the end of the file. This is normal and expected
			case io.EOF:
				{
					duration := time.Since(startTime)
					if duration.Seconds() > 0.1 {
						t.Fatalf("Reading the rate limited file should have taken under 0.1 seconds but it took %f",
							duration.Seconds())
					}
					reader.Close()
					break InfiniteLoop
				}
			case context.Canceled:
				{
					break InfiniteLoop
				}
			default:
				{
					t.Fatalf("While reading '%s' the following error was encountered: %s", path, err)
				}
			}
		}
	}
}

// valid config
func TestGetObjectStores1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].Type = "test_null"
	result.Config.Backup[0].Target[0].Name = "test1"
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	objectStores, err := GetObjectStores(context.TODO(), result.Config.Backup[0], backupJobsState)
	if err != nil {
		t.Fatalf("While running GetObjectStores() got error: %s", err)
	}
	// we expect to find one store of type  test_null having target name test1
	for _, store := range objectStores {
		StoreName, StoreType := store.GetStoreDetails()
		if StoreName != "test1" {
			t.Fatalf("Was expecting target(store) to have name 'test1' but it has name %s", StoreName)
		}
		if StoreType != "test_null" {
			t.Fatalf("Was expecting target(store) to have type 'test_null' but it has type %s", StoreType)
		}

	}
}

// invalid config
func TestGetObjectStores2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Target[0].Type = "test_inexisting"
	result.Config.Backup[0].Target[0].Name = "234sr"
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	_, err = GetObjectStores(context.TODO(), result.Config.Backup[0], backupJobsState)
	if err == nil {
		t.Fatal("While running GetObjectStores() an error was expected because an unknown backed was selected" +
			" but no error was returned")
	}
}

func TestCalculatePercent1(t *testing.T) {
	result := calculatePercent(100, 10)
	if result != 10 {
		t.Fatalf("Was expecting a result of '10' from calculatePercent() but got %d", result)
	}

	result = calculatePercent(100, 100)
	if result != 100 {
		t.Fatalf("1. Was expecting a result of '100' from calculatePercent() but got %d", result)
	}

	// if we pass in a read value larger than $filesize, we should get 100 as a result
	result = calculatePercent(100, 200)
	if result != 200 {
		t.Fatalf("2. Was expecting a result of '200' from calculatePercent() but got %d", result)
	}

	result = calculatePercent(100, 0)
	if result != 0 {
		t.Fatalf("Was expecting a result of '0' from calculatePercent() but got %d", result)
	}

	result = calculatePercent(0, 0)
	if result != 100 {
		t.Fatalf("1. Was expecting a result of '100' from calculatePercent() but got %d", result)
	}

	// this is an actual scenario (on Linux some special files have 0 size but attempting to read them may return content)
	result = calculatePercent(0, 20)
	if result != 100 {
		t.Fatalf("2. Was expecting a result of '100' from calculatePercent() but got %d", result)
	}

	result = calculatePercent(101101, 101100)
	if result != 99 {
		t.Fatalf("Was expecting a result of '99' from calculatePercent() but got %d", result)
	}
}
