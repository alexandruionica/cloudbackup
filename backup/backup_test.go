package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/database/dbops"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"errors"
	"fmt"
	"github.com/satori/go.uuid"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// setup filerecord for plain file which doesn't have a checksum
func TestPrepareFileRecord1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	jobId := uuid.NewV4().String()

	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestPrepareFileRecord1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}
	if newDbRecord.JobUuid != jobId {
		t.Fatalf("Expected jobid %s but got %s", jobId, newDbRecord.JobUuid)
	}
	if newDbRecord.Path != path {
		t.Fatalf("Expected path %s but got %s", path, newDbRecord.Path)
	}
	if newDbRecord.ChecksumType != "" {
		t.Fatalf("Expected ChecksumType to be empty string but got %s", newDbRecord.ChecksumType)
	}
	if newDbRecord.LinkTarget != "" {
		t.Fatalf("Expected LinkTarget to be empty string but got %s", newDbRecord.LinkTarget)
	}
}

// setup filerecord for plain file which has a checksum
func TestPrepareFileRecord2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	jobId := uuid.NewV4().String()

	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestPrepareFileRecord1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := "abcabfd3423de22"

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}
	if newDbRecord.JobUuid != jobId {
		t.Fatalf("Expected jobid %s but got %s", jobId, newDbRecord.JobUuid)
	}
	if newDbRecord.Path != path {
		t.Fatalf("Expected path %s but got %s", path, newDbRecord.Path)
	}
	if newDbRecord.ChecksumType != "md5" {
		t.Fatalf("Expected ChecksumType to be 'md5' string but got %s", newDbRecord.ChecksumType)
	}
	if newDbRecord.LinkTarget != "" {
		t.Fatalf("Expected LinkTarget to be empty string but got %s", newDbRecord.LinkTarget)
	}
}

// setup filerecord for a symlink which has a real file as a target
func TestPrepareFileRecord3(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	jobId := uuid.NewV4().String()

	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestPrepareFileRecord1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(path)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", path, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	// ########
	symlinkPath := testutils.GenerateTmpFilePath("cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(path, symlinkPath)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{symlinkPath})

	stat, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(symlinkPath, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(symlinkPath, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}
	if newDbRecord.JobUuid != jobId {
		t.Fatalf("Expected jobid %s but got %s", jobId, newDbRecord.JobUuid)
	}
	if newDbRecord.Path != symlinkPath {
		t.Fatalf("Expected path %s but got %s", symlinkPath, newDbRecord.Path)
	}
	if newDbRecord.ChecksumType != "" {
		t.Fatalf("Expected ChecksumType to be empty string but got %s", newDbRecord.ChecksumType)
	}
	if newDbRecord.LinkTarget != path {
		t.Fatalf("Expected LinkTarget to be %s string but got %s", path, newDbRecord.LinkTarget)
	}
}

// setup filerecord for a symlink which has a broken target
func TestPrepareFileRecord4(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	jobId := uuid.NewV4().String()

	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestPrepareFileRecord1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(path)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", path, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	// ########
	symlinkPath := testutils.GenerateTmpFilePath("cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(path, symlinkPath)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	//defer testutils.DeleteTestFilesAndDirs([]string{symlinkPath})
	// remove symlink target so we get a broken link
	testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(symlinkPath, false)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(symlinkPath, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}
	if newDbRecord.JobUuid != jobId {
		t.Fatalf("Expected jobid %s but got %s", jobId, newDbRecord.JobUuid)
	}
	if newDbRecord.Path != symlinkPath {
		t.Fatalf("Expected path %s but got %s", symlinkPath, newDbRecord.Path)
	}
	if newDbRecord.ChecksumType != "" {
		t.Fatalf("Expected ChecksumType to be empty string but got %s", newDbRecord.ChecksumType)
	}
	if newDbRecord.LinkTarget != path {
		t.Fatalf("Expected LinkTarget to be %s string but got %s", path, newDbRecord.LinkTarget)
	}
}

// test updating counters
func TestUpdateCounters1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup", jobId)
	if err != nil {
		t.Fatal(err)
	}
	// file upload without error
	if backupJobsState.Running[0].StatsCounters["uploaded_files"] != 0 {
		t.Fatal("uploaded_files is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "file", "a_file_path1", nil)
	updateCounters(backupJobsState, backupConfig.Name, "upload", "file", "a_file_path2", nil)
	if backupJobsState.Running[0].StatsCounters["uploaded_files"] != 2 {
		t.Fatalf("uploaded_files is not 2 but is %d", backupJobsState.Running[0].StatsCounters["uploaded_files"])
	}

	// dir upload without error
	if backupJobsState.Running[0].StatsCounters["uploaded_directories"] != 0 {
		t.Fatal("uploaded_directories is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "dir", "a_dir_path1", nil)
	updateCounters(backupJobsState, backupConfig.Name, "upload", "dir", "a_dir_path2", nil)
	if backupJobsState.Running[0].StatsCounters["uploaded_directories"] != 2 {
		t.Fatalf("uploaded_directories is not 2 but is %d", backupJobsState.Running[0].StatsCounters["uploaded_directories"])
	}

	// symlink upload without error
	if backupJobsState.Running[0].StatsCounters["uploaded_symlinks"] != 0 {
		fmt.Printf("%+v", backupJobsState.Running[0].StatsText)
		t.Fatal("uploaded_symlinks is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "symlink", "a_symlink_path1", nil)
	updateCounters(backupJobsState, backupConfig.Name, "upload", "symlink", "a_symlink_path2", nil)
	if backupJobsState.Running[0].StatsCounters["uploaded_symlinks"] != 2 {
		t.Fatalf("uploaded_symlinks is not 2 but is %d", backupJobsState.Running[0].StatsCounters["uploaded_symlinks"])
	}

	// file upload with error
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_files"] != 0 {
		t.Fatal("failed_to_upload_files is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "file", "a_file_path1", errors.New("an error"))
	updateCounters(backupJobsState, backupConfig.Name, "upload", "file", "a_file_path2", errors.New("an error"))
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_files"] != 2 {
		t.Fatalf("failed_to_upload_files is not 2 but is %d", backupJobsState.Running[0].StatsCounters["failed_to_upload_files"])
	}

	// dir upload with error
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_directories"] != 0 {
		t.Fatal("failed_to_upload_directories is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "dir", "a_dir_path1", errors.New("an error"))
	updateCounters(backupJobsState, backupConfig.Name, "upload", "dir", "a_dir_path2", errors.New("an error"))
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_directories"] != 2 {
		t.Fatalf("failed_to_upload_directories is not 2 but is %d", backupJobsState.Running[0].StatsCounters["failed_to_upload_directories"])
	}

	// symlink with without error
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_symlinks"] != 0 {
		fmt.Printf("%+v", backupJobsState.Running[0].StatsText)
		t.Fatal("failed_to_upload_symlinks is not 0")
	}
	updateCounters(backupJobsState, backupConfig.Name, "upload", "symlink", "a_symlink_path1", errors.New("an error"))
	updateCounters(backupJobsState, backupConfig.Name, "upload", "symlink", "a_symlink_path2", errors.New("an error"))
	if backupJobsState.Running[0].StatsCounters["failed_to_upload_symlinks"] != 2 {
		t.Fatalf("failed_to_upload_symlinks is not 2 but is %d", backupJobsState.Running[0].StatsCounters["failed_to_upload_symlinks"])
	}
}

// test  addEntryToRemoteFiles() and getBackedupObjectPropertiesFromDb() and updateDbEntryInFiles()
func TestAddEntryToRemoteFilesAndGetBackedupObjectPropertiesFromDb(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]

	jobId := uuid.NewV4().String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("1. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}
	// cleanup
	defer func() {
		_ = dbtx.Rollback() //nolint:errcheck
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := "abcabfd3423de22"

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	err = addDbEntryToFiles(dbData, dbtx, newDbRecord)
	if err != nil {
		t.Fatalf("Could not add an entry to the 'files' table due to error: %s", err)
	}
	version, err := calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("Could not calculate remote version for %s due to error: %s", path, err)
	}
	_, err = addDbEntryToRemoteFiles(backupConfig.Target[0].Name, jobId, 0, dbData, dbtx, newDbRecord, version, strconv.Itoa(version))
	if err != nil {
		t.Fatalf("Failed to addDbEntryToRemoteFiles() due to error: %s", err)
	}

	//check that we have only 1 entry in the remote_files table
	rows, err := dbtx.Query("SELECT count(*) FROM remote_files")
	if err != nil {
		t.Fatalf("While trying to count number of entries in 'remote_files' table, got error '%s'", err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			t.Fatalf("While trying to Close() a db.Query the following error was encountered: '%s'", err)
		}
	}()
	numEntries := 0
	for rows.Next() {
		err := rows.Scan(&numEntries)
		if err != nil {
			t.Fatalf("while fetchig the DB query result, the following error was encountered: '%s'", err)
		}
		continue
	}
	err = rows.Err()
	if err != nil {
		t.Fatalf("Could not enumerate the db query result due to the following error: '%s'", err)
	}
	if numEntries != 1 {
		t.Fatalf("Was expecting 1 row to be found in the DB but instead %d were found", numEntries)
	}

	err = dbtx.Commit()
	if err != nil {
		t.Fatalf("1. Could not commit transaction due to error: %s", err)
	}
	found, retrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	if !newDbRecord.Ctime.Equal(retrievedDbRecord.Ctime) || !newDbRecord.Mtime.Equal(retrievedDbRecord.Mtime) {
		fmt.Println("########## RETRIEVED #############")
		utils.Pp(retrievedDbRecord)
		fmt.Println("########## EXPECTED  #############")
		utils.Pp(newDbRecord)
		t.Fatal("1. Retrieved DB record Ctime or Mtime doesn't match what we've sent (see above for details)")
	} else {
		// reflect.DeepEqual below will fail due to some fields in time.Time not matching so we just overwrite the
		// fields (once we know that Time.Equal is true) in order to have a proper comparison done by reflect
		retrievedDbRecord.Ctime = newDbRecord.Ctime
		retrievedDbRecord.Mtime = newDbRecord.Mtime
	}
	if !reflect.DeepEqual(newDbRecord, retrievedDbRecord) {
		fmt.Println("########## RETRIEVED #############")
		utils.Pp(retrievedDbRecord)
		fmt.Println("########## EXPECTED #############")
		utils.Pp(newDbRecord)
		t.Fatal("1. Retrieved DB record doesn't match what we've sent (see above for details)")
	}

	dbtx, err = dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("2. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}
	// cleanup
	defer func() {
		_ = dbtx.Rollback() //nolint:errcheck
		dbops.CloseStatementsAndDb(dbData)
	}()

	// change Size so we have something to update in the DB and then validate the update worked
	newDbRecord.Size = 123455432
	err = updateDbEntryInFiles(dbData, dbtx, newDbRecord)
	if err != nil {
		t.Fatalf("1. Failed to update db record due to error: %s", err)
	}

	err = dbtx.Commit()
	if err != nil {
		t.Fatalf("2. Could not commit transaction due to error: %s", err)
	}

	found, retrievedDbRecord2, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("2. Did not find a record in the DB for path %s", path)
	}
	if !reflect.DeepEqual(newDbRecord, retrievedDbRecord2) {
		fmt.Println("########## RETRIEVED #############")
		utils.Pp(retrievedDbRecord2)
		fmt.Println("########## EXPECTED #############")
		utils.Pp(newDbRecord)
		t.Fatal("2. Retrieved DB record doesn't match what we've sent (see above for details)")
	}

	dbtx, err = dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("3. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}
	// cleanup
	defer func() {
		_ = dbtx.Rollback() //nolint:errcheck
		dbops.CloseStatementsAndDb(dbData)
	}()
	// try to change for an inexisting path, should return an error
	newDbRecord.Path = "a_path_which_does_not_exist"
	err = updateDbEntryInFiles(dbData, dbtx, newDbRecord)
	if err == nil {
		t.Fatal("updateDbEntryInFiles() should have returned an error but didn't")
	} else {
		if !strings.Contains(err.Error(), "update should have changed 1 row for but it changed 0 rows") {
			t.Fatalf("Was expecting updateDbEntryInFiles() to return an error containing: 'update should "+
				"have changed 1 row for but it changed 0 rows' but instead it returned: %s", err)
		}
	}

}

// test function works as expected when there isn't a DB match
func TestGetRemoteFileVersion1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]

	jobId := uuid.NewV4().String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("Could not setup DB prerequisite due to error: %s", err)
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("could not start a database transaction so we can't proceed to test; error was: %s", err)
	}

	// cleanup
	defer func() {
		_ = dbtx.Rollback() //nolint:errcheck
		dbops.CloseStatementsAndDb(dbData)
	}()

	path := "a_file_which_does_not_exist"
	// if the file does not exist in the DB then we should get a version of 1
	version, err := calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("1. While trying to get a version, encountered error: %s", err)
	}
	if version != 1 {
		t.Fatalf("1. Was expecting returned version to be 1 but instead we got: %d", version)
	}

	// execute again, should give same result
	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("1. While trying to get a version, encountered error: %s", err)
	}
	if version != 1 {
		t.Fatalf("1. Was expecting returned version to be 1 but instead we got: %d", version)
	}

}

// test function works as expected when there is a DB match and also test getNewestRemoteFileUuid()
func TestGetRemoteFileVersionAndGetNewestRemoteFileUuid(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	// ensure we have at least 2 targets by copying the first target
	backupConfig.Target = append(backupConfig.Target, backupConfig.Target[0])
	newTargetName := "another_target"
	backupConfig.Target[len(backupConfig.Target)-1].Name = newTargetName

	jobId := uuid.NewV4().String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("Could not setup DB prerequisite due to error: %s", err)
	}

	dbtx, err := dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("1. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}

	// cleanup
	defer func() {
		_ = dbtx.Rollback() //nolint:errcheck
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	fileContent := "just a string"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
		ctime = time.Time{}
	}
	checksum := "abcabfd3423de22"

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	err = addDbEntryToFiles(dbData, dbtx, newDbRecord)
	if err != nil {
		t.Fatalf("Could not add an entry to the 'files' table due to error: %s", err)
	}

	version, err := calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("Could not calculate remote version for %s due to error: %s", path, err)
	}
	file1stTimeUuid, err := addDbEntryToRemoteFiles(backupConfig.Target[0].Name, jobId, 0, dbData, dbtx, newDbRecord, version, strconv.Itoa(version))
	if err != nil {
		t.Fatalf("Failed to addDbEntryToRemoteFiles() due to error: %s", err)
	}
	if file1stTimeUuid == "" {
		t.Fatal("1. addDbEntryToRemoteFiles() returned an empty uuid")
	}

	// if the file has only 1 entry in the DB then we should get a version of 2
	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("2. While trying to get a version, encountered error: %s", err)
	}
	if version != 2 {
		t.Fatalf("2. Was expecting returned version to be 2 but instead we got: %d", version)
	}

	// execute again, should give same result
	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("3. While trying to get a version, encountered error: %s", err)
	}
	if version != 2 {
		t.Fatalf("3. Was expecting returned version to be 2 but instead we got: %d", version)
	}

	err = dbtx.Commit()
	if err != nil {
		t.Fatalf("1. Could not commit transaction due to error: %s", err)
	}

	retrieveUuid, err := getNewestRemoteFileUuid(dbData, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("While running getNewestRemoteFileUuid() for path %s got error: %s", path, err)
	}

	if retrieveUuid != file1stTimeUuid {
		t.Fatalf("1. remote file uuid retrieved by getNewestRemoteFileUuid() is %s and it doesn't match what we expected it to be: %s", retrieveUuid, file1stTimeUuid)
	}

	dbtx, err = dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("2. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}

	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("2. Could not calculate remote version for %s due to error: %s", path, err)
	}
	file2ndTimeUuid, err := addDbEntryToRemoteFiles(backupConfig.Target[0].Name, jobId, 0, dbData, dbtx, newDbRecord, version, strconv.Itoa(version))
	if err != nil {
		t.Fatalf("Failed to addDbEntryToRemoteFiles() due to error: %s", err)
	}
	if file2ndTimeUuid == "" {
		t.Fatal("2. addDbEntryToRemoteFiles() returned an empty uuid")
	}
	if file1stTimeUuid == file2ndTimeUuid {
		t.Fatalf("addDbEntryToRemoteFiles() returned two idential UUIDs, for 2 runs. The uuid was: %s", file1stTimeUuid)
	}

	// if the file has only 2 entries in the DB then we should get a version of 3
	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("4. While trying to get a version, encountered error: %s", err)
	}
	if version != 3 {
		t.Fatalf("4. Was expecting returned version to be 3 but instead we got: %d", version)
	}

	err = dbtx.Commit()
	if err != nil {
		t.Fatalf("2. Could not commit transaction due to error: %s", err)
	}

	retrieveUuid, err = getNewestRemoteFileUuid(dbData, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("While running getNewestRemoteFileUuid() for path %s got error: %s", path, err)
	}
	if retrieveUuid != file2ndTimeUuid {
		t.Fatalf("2. remote file uuid retrieved by getNewestRemoteFileUuid() is %s and it doesn't match what we expected it to be: %s", retrieveUuid, file1stTimeUuid)
	}

	// add another entry to the table, this time for a different target so the uuid should now appear in the return of getNewestRemoteFileUuid()
	dbtx, err = dbData.Db.Begin()
	if err != nil {
		// could not start a database transaction so we can't proceed to test
		t.Fatalf("3. could not start a database transaction so we can't proceed to test; error was: %s", err)
	}

	version, err = calcRemoteFileVersion(dbData, dbtx, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("3. Could not calculate remote version for %s due to error: %s", path, err)
	}
	file3ndTimeUuid, err := addDbEntryToRemoteFiles(newTargetName, jobId, 0, dbData, dbtx, newDbRecord, version, strconv.Itoa(version))
	if err != nil {
		t.Fatalf("Failed to addDbEntryToRemoteFiles() due to error: %s", err)
	}

	if file1stTimeUuid == file3ndTimeUuid || file2ndTimeUuid == file3ndTimeUuid {
		t.Fatalf("addDbEntryToRemoteFiles() returned for a new target name uuid: %s but it equals a previous uuid", file1stTimeUuid)
	}

	err = dbtx.Commit()
	if err != nil {
		t.Fatalf("3. Could not commit transaction due to error: %s", err)
	}

	retrieveUuid, err = getNewestRemoteFileUuid(dbData, path, backupConfig.Target[0].Name)
	if err != nil {
		t.Fatalf("While running getNewestRemoteFileUuid() for path %s got error: %s", path, err)
	}
	if retrieveUuid != file2ndTimeUuid {
		t.Fatalf("3. remote file uuid retrieved by getNewestRemoteFileUuid() is %s and it doesn't match what we expected it to be: %s", retrieveUuid, file1stTimeUuid)
	}

}
