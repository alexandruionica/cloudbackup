package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/database/dbops"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// setup filerecord for plain file which doesn't have a checksum
func TestPrepareFileRecord1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	if newDbRecord.Type != "file" {
		t.Fatalf("Expected Type to be 'file' string but got %s", newDbRecord.Type)
	}

	if newDbRecord.Size != stat.Size() {
		t.Fatalf("Size mismatch between on disk %d and what we got %d", stat.Size(), newDbRecord.Size)
	}
}

// setup filerecord for plain file which has a checksum
func TestPrepareFileRecord2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	if newDbRecord.Type != "file" {
		t.Fatalf("Expected Type to be 'file' string but got %s", newDbRecord.Type)
	}

	if newDbRecord.Size != stat.Size() {
		t.Fatalf("Size mismatch between on disk %d and what we got %d", stat.Size(), newDbRecord.Size)
	}
}

// setup filerecord for a symlink which has a real file as a target
func TestPrepareFileRecord3(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	symlinkPath := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
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

	if newDbRecord.Type != "symlink" {
		t.Fatalf("Expected Type to be 'symlink' string but got %s", newDbRecord.Type)
	}

	if newDbRecord.Size != stat.Size() {
		t.Fatalf("Size mismatch between on disk %d and what we got %d", stat.Size(), newDbRecord.Size)
	}
}

// setup filerecord for a symlink which has a broken target
func TestPrepareFileRecord4(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	symlinkPath := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(path, symlinkPath)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{symlinkPath})
	// remove symlink target so we get a broken link
	testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(symlinkPath, false)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	if newDbRecord.Type != "symlink" {
		t.Fatalf("Expected Type to be 'symlink' string but got %s", newDbRecord.Type)
	}

	if newDbRecord.Size != stat.Size() {
		t.Fatalf("Size mismatch between on disk %d and what we got %d", stat.Size(), newDbRecord.Size)
	}
}

// setup filerecord for directory
func TestPrepareFileRecord6(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, true)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	if newDbRecord.Type != "dir" {
		t.Fatalf("Expected Type to be 'dir' string but got %s", newDbRecord.Type)
	}

	if newDbRecord.Size != 0 {
		t.Fatalf("Expected Size to be 0 (as we always record directories to have size 0) but got %d", newDbRecord.Size)
	}
}

// test updating counters
func TestUpdateCounters1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
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
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
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
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

// test function works as expected when there is a DB match and also test getNewestRemoteFileUuid() and getRemoteVersionForVersion()
func TestGetRemoteFileVersionAndGetNewestRemoteFileUuid(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
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

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// we know from the above strconv.Itoa() the version to expect , meaning "1"
	remoteVersion, err := getRemoteVersionForVersion(dbData, dbtx, newDbRecord.Path, backupConfig.Target[0].Name, version)
	if err != nil {
		t.Fatalf("1. getRemoteVersionForVersion() returned error: %s", err)
	}
	if remoteVersion != "1" {
		t.Fatalf("1. was expecting getRemoteVersionForVersion() to return '1' but it returned: '%s'", remoteVersion)
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

	// we know from the above strconv.Itoa() the version to expect , meaning "2"
	remoteVersion, err = getRemoteVersionForVersion(dbData, dbtx, newDbRecord.Path, backupConfig.Target[0].Name, version)
	if err != nil {
		t.Fatalf("2. getRemoteVersionForVersion() returned error: %s", err)
	}
	if remoteVersion != "2" {
		t.Fatalf("2. was expecting getRemoteVersionForVersion() to return '2' but it returned: '%s'", remoteVersion)
	}

	// we know from the above strconv.Itoa() the version to expect for input "1" , meaning "1"
	remoteVersion, err = getRemoteVersionForVersion(dbData, dbtx, newDbRecord.Path, backupConfig.Target[0].Name, 1)
	if err != nil {
		t.Fatalf("3. getRemoteVersionForVersion() returned error: %s", err)
	}
	if remoteVersion != "1" {
		t.Fatalf("3. was expecting getRemoteVersionForVersion() to return '1' but it returned: '%s'", remoteVersion)
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

// on disk file - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum != ""
func TestNeedsUpload1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum == ""
func TestNeedsUpload2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are identical and compareChecksum=true
func TestNeedsUpload3(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum, err := utils.GetFileMD5Sum(path)
	if err != nil {
		t.Fatalf("While trying to calculate MD5 checksum, got error: %s", err)
	}

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != checksum {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to equal input checksum %s (as the file did not change) but instead got: %s", checksum, rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are identical except the checksum and compareChecksum=true
func TestNeedsUpload4(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum := "asdasdasdasd"

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum == checksum {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to differ input checksum %s (as the input was bogus) but instead got equality", checksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the Mtime and compareChecksum=false
func TestNeedsUpload5(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Mtime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as Mtime differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the Ctime and compareChecksum=false
func TestNeedsUpload6(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Ctime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if !metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==true' as Ctime differs, but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the file type differs and compareChecksum=false
func TestNeedsUpload7(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Type = "dir"

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file type differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the file size differs and compareChecksum=false
func TestNeedsUpload8(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Size = 1234567890123

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file size differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the $encrypt differs between the file and the Backup config variable
func TestNeedsUpload9(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Encrypted = false

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, true)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as encrypt settings differ, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false', but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum != ""
func TestNeedsUpload10(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum == ""
func TestNeedsUpload11(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are identical and compareChecksum=true
func TestNeedsUpload12(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum := ""

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != checksum {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to equal input checksum %s (as the file did not change) but instead got: %s", checksum, rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are identical except the checksum and compareChecksum=true ; this is a case where nothing should happen as DIR checksums should not be checked/used
func TestNeedsUpload13(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum := "asdasdasdasd"

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' because this is a Dir and checksum comparison doesn't make sense here")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum == checksum {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to differ input checksum %s (as the input was bogus) but instead got equality", checksum)
	}
}

// on disk dir - both DB data and newDbRecord are almost identical except the Mtime and compareChecksum=false
func TestNeedsUpload14(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Mtime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as Mtime differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are almost identical except the Ctime and compareChecksum=false
func TestNeedsUpload15(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Ctime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if !metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==true' as Ctime differs, but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are almost identical except the file type differs and compareChecksum=false
func TestNeedsUpload16(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Type = "file"

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file type differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are almost identical except the file size differs and compareChecksum=false
func TestNeedsUpload17(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Size = 1234567890123

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file size differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk dir - both DB data and newDbRecord are almost identical except the $encrypt differs between the file and the Backup config variable
func TestNeedsUpload18(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	// setup a tmpdir which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := utils.SetupTmpDir("cloudbackup_TestNeedsUpload_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{path})
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Encrypted = false

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, true)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' as encrypt settings differ but this is a 'dir' show they should not matter")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false', but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum != ""
func TestNeedsUpload19(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are identical and compareChecksum=false and data already saved in the DB has a checksum == ""
func TestNeedsUpload20(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are identical and compareChecksum=true
func TestNeedsUpload21(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum, err := utils.GetFileMD5Sum(path)
	if err != nil {
		t.Fatalf("While trying to calculate MD5 checksum, got error: %s", err)
	}

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to equal empty string (as the file is a symlink and for those we don't calculate the checksum) but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are identical except the checksum and compareChecksum=true
func TestNeedsUpload22(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}

	checksum := "asdasdads"

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

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecord, true, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=true so the returned checksum was expected "+
			"to equal empty string (as the file is a symlink and for those we don't calculate the checksum) but instead got: %s", rChecksum)
	}
}

// on disk file - both DB data and newDbRecord are almost identical except the Mtime and compareChecksum=false
func TestNeedsUpload23(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Mtime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as Mtime differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are almost identical except the Ctime and compareChecksum=false
func TestNeedsUpload24(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Ctime = time.Now()

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' but it didn't")
	}
	if !metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==true' as Ctime differs, but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are almost identical except the file type differs and compareChecksum=false
func TestNeedsUpload25(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Type = "file"

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file type differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are almost identical except the file size differs and compareChecksum=false
func TestNeedsUpload26(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Size = 1234567890123

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, false)
	if !contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==true' as file size differs, but it didn't")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false' but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

// on disk symlink - both DB data and newDbRecord are almost identical except the $encrypt differs between the file and the Backup config variable
func TestNeedsUpload27(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

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
	fpath, err := utils.SetupTmpFileWithContent([]byte(fileContent), "cloudbackup_TestAddEntryToRemoteFiles1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(fpath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", fpath, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{fpath})

	path := testutils.GenerateTmpFilePath(t, "cloudbackup_backup_test_symlink_", "_to_plain_File")
	err = os.Symlink(fpath, path)
	if err != nil {
		_ = os.RemoveAll(fpath) // #nosec
		t.Fatal(err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
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

	newDbRecordCopy := newDbRecord
	newDbRecordCopy.Encrypted = false

	contentChanged, metadataChanged, rCtime, rChecksum := needsUpload(path, stat, newDbRecordCopy, false, dereference, true)
	if contentChanged {
		t.Fatal("Was expecting that needsUpload() reports 'contentChanged==false' as encrypt settings differ but this is a symlink so they will be ignored")
	}
	if metadataChanged {
		t.Fatal("Was expecting that needsUpload() reports 'metadataChanged==false', but it didn't")
	}
	if ctime != rCtime {
		t.Fatalf("DB record ctime of %s does not match ctime %s returned by needsUpload()", ctime, rCtime)
	}
	if rChecksum != "" {
		t.Fatalf("needsUpload() was called with compareChecksum=false so the returned checksum was expected to be an empty string but instead got: %s", rChecksum)
	}
}

func TestUploadObject(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := "abcabfd3423de22"

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	for _, objectstore := range objectStores {
		rVersion, cancelled, err := UploadObject(newDbRecord, backupConfig, objectstore, backupJobsState, 1)
		if err != nil {
			t.Fatalf("UploadObject() returned error: %s", err)
		}
		if cancelled {
			t.Fatalf("UploadObject() signalled that it was cancelled but this should not have happend")
		}

		if rVersion != "1" {
			t.Fatalf("Was expecint that the object store used for testing returns '1' as rVersion passed back by UploadObject() but instead got: %s", rVersion)

		}
	}
}

// UploadAndUpdateDB with invalid operation
func TestUploadAndUpdateDB1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("blablaasdads", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to produce an error and return cancelled=false; as UploadAndUpdateDB() was called with an invalid $operation")
	}
	if err == nil {
		t.Fatal("Was expecting UploadAndUpdateDB() to produce an error as it was called with an invalid $operation")
	}
}

// UploadAndUpdateDB with "metadata-update" operation for a file which was not previously backed up
func TestUploadAndUpdateDB2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	oldOwner := newDbRecord.Owner
	newOwner := "a-bogus-owner"
	if oldOwner == newOwner {
		// makes no sense we would have an actual, on disk file with $newOwner as the owner so fail test
		t.Fatalf("Unexpected file owner of %s", oldOwner)
	}
	newDbRecord.Owner = newOwner
	cancelled, err := UploadAndUpdateDB("metadata-update", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err == nil {
		t.Fatal("Was expecting UploadAndUpdateDB() to produce an error but it didn't")
	}

}

// UploadAndUpdateDB with "new" and then "metadata-update" operation
func TestUploadAndUpdateDB3(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	oldOwner := newDbRecord.Owner
	newOwner := "a-bogus-owner"
	if oldOwner == newOwner {
		// makes no sense we would have an actual, on disk file with $newOwner as the owner so fail test
		t.Fatalf("Unexpected file owner of %s", oldOwner)
	}
	newDbRecord.Owner = newOwner
	cancelled, err = UploadAndUpdateDB("metadata-update", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, retrievedRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	if retrievedRecord.Owner != newOwner {
		t.Fatalf("Was expecting that the owner of %s is %s but got %s", path, newOwner, retrievedRecord.Owner)
	}
}

// UploadAndUpdateDB with "new" on an object store which always produces errors
func TestUploadAndUpdateDB4(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	errObjectStore := objectstore.InitialiseStoreError(ctx, backupConfig, "error_store", "store_error", 0)
	objectStores := []objectstore.ObjectStore{errObjectStore}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}

	// expected error is (a bit deceiving):    unsupported backend of type
	if err == nil {
		t.Fatal("Was expecting UploadAndUpdateDB() to produce an error but it didn't")
	}

}

// UploadAndUpdateDB with "new" on two object stores, with the second supposed to always produce errors
func TestUploadAndUpdateDB5(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	errObjectStore := objectstore.InitialiseStoreError(ctx, backupConfig, "error_store", "store_error", 0)
	objectStores = append(objectStores, errObjectStore)

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}

	// expected error is (a bit deceiving):    unsupported backend of type
	if err == nil {
		t.Fatal("Was expecting UploadAndUpdateDB() to produce an error but it didn't")
	}
}

// UploadAndUpdateDB with "new" and then "content-update" operation
func TestUploadAndUpdateDB6(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	oldSize := newDbRecord.Size
	newSize := oldSize + 10000
	newDbRecord.Size = newSize
	cancelled, err = UploadAndUpdateDB("content-update", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, retrievedRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	if retrievedRecord.Size != newSize {
		t.Fatalf("Was expecting that the size of %s is %d but got %d", path, newSize, retrievedRecord.Size)
	}

}

// markDeleted() for an already existing 1 DB entry but the path itself is no longer included in the paths to be backed up
func TestMarkDeleted1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
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

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	cancelled, err = markDeleted(newDbRecord, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting markDeleted() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting markDeleted() to not produce an error but it did: %s", err)
	}
	// because the file still exists on disk, the above should haven't actually changed anything
	found, _, err = getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if found {
		t.Fatalf("Found a record in the DB for path %s but there should no longer be one despite the file "+
			"still existing on disk; this is because there is no parent dir included  in the list of directories to backup", path)
	}
}

// markDeleted() for an already existing 1 DB entry which is under an included path and is not matched by an exclusion rule
func TestMarkDeleted2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	cancelled, err = markDeleted(newDbRecord, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting markDeleted() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting markDeleted() to not produce an error but it did: %s", err)
	}
	// because the file still exists on disk, the above should haven't actually changed anything
	found, _, err = getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("2. Did not find a record in the DB for path %s", path)
	}
}

// markDeleted() for an already existing 1 DB entry , parent dir is included in list of paths to be backed up but the file itself is under the exclusion list
func TestMarkDeleted3(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up and the the path itself is excluded
	result.Config.Backup[0].Paths = []string{dirPath}
	result.Config.Backup[0].Exclusions = []string{path}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	cancelled, err = markDeleted(newDbRecord, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting markDeleted() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting markDeleted() to not produce an error but it did: %s", err)
	}
	// because the file still exists on disk, the above should haven't actually changed anything
	found, _, err = getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if found {
		t.Fatalf("Found a record in the DB for path %s but there should no longer be one despite the file "+
			"still existing on disk; this is because while the parent dir is included in the list of directories to backup, the file itself is excluded", path)
	}
}

// markDeleted() for an already existing 1 DB entry which is under an included path and is not matched by an exclusion rule but using two datastores, both ok
func TestMarkDeleted4(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	result.Config.Backup[0].Target = append(result.Config.Backup[0].Target, result.Config.Backup[0].Target[0])
	result.Config.Backup[0].Target[1].Name = "2nd_store"
	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) != 2 {
		t.Fatalf("Was expecting 2 object stores but found: %d", len(objectStores))
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	// delete file from disk so Mark deleted has something to complain about
	testutils.DeleteTestFilesAndDirs([]string{path})

	cancelled, err = markDeleted(newDbRecord, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting markDeleted() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting markDeleted() to not produce an error but it did: %s", err)
	}
}

// markDeleted() for an already existing 1 DB entry which is under an included path and is not matched by an exclusion
// rule but using two datastores with the 2nd always giving errors
func TestMarkDeleted5(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	result.Config.Backup[0].Target = append(result.Config.Backup[0].Target, result.Config.Backup[0].Target[0])
	result.Config.Backup[0].Target[1].Name = "2nd_store"

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) != 2 {
		t.Fatalf("Was expecting 2 object stores but found: %d", len(objectStores))
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	// replace the 2nd object store with a broken one which has pretends to have the same name and same type (hint: it lies)
	errObjectStore := objectstore.InitialiseStoreError(ctx, backupConfig, "2nd_store", "test_null", 0)
	objectStores[1] = errObjectStore

	// delete file from disk so Mark deleted has something to complain about
	testutils.DeleteTestFilesAndDirs([]string{path})

	cancelled, err = markDeleted(newDbRecord, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting markDeleted() to return cancelled=false but it didn't")
	}
	if err == nil {
		t.Fatal("Was expecting markDeleted() to produce an error but it didn't")
	}
}

// FindAndMarkDeleted() for a path with 1 file which is under an included path and is not matched by an exclusion
// rule - more comprehensive tests are in backup_test.go (but coverage doesn't properly show them)
func TestFindAndMarkDeleted1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	err = dbops.EnsureTargetsInDb(dbData.Db, backupConfig)
	if err != nil {
		t.Fatalf("While trying to ensure all backup config targets have a DB entry, got error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	ctime, err := fileproperties.GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("Could not get ctime for %s due to error: %s", path, err)
	}
	checksum := ""

	newDbRecord, err := PrepareFileRecord(path, stat, backupConfig, ctime, checksum, jobId)
	if err != nil {
		t.Fatalf("PrepareFileRecord() returned error: %s", err)
	}

	cancelled, err := UploadAndUpdateDB("new", ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId, newDbRecord)
	if cancelled {
		t.Fatal("Was expecting UploadAndUpdateDB() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("Was expecting UploadAndUpdateDB() to not produce an error but it did: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find a record in the DB for path %s", path)
	}

	// should not actually mark anything as deleted
	cancelled = FindAndMarkDeleted(ctx, backupConfig, dbData, objectStores, backupJobsState, jobId, 10)
	if cancelled {
		t.Fatal("Was expecting FindAndMarkDeleted() to return cancelled=false but it didn't")
	}

	// because the file still exists on disk, the above should haven't actually changed anything
	found, _, err = getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("2. Did not find a record in the DB for path %s", path)
	}

	err = backupJobsState.MarkStopped(backupConfig.Name, "unittest_backup", jobId, true)
	if err != nil {
		t.Fatal(err)
	}
	dbops.CloseStatementsAndDb(dbData)

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// get a new JobID as this will force FindAndMarkDeleted() to think anything it finds needs deletion (as long as said thing has been deleted from disk too)
	jobId = u.String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}
	dbData, err = dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	ctx, err = backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}
	// delete backed up file
	testutils.DeleteTestFilesAndDirs([]string{path})
	// should mark above $path as deleted

	cancelled = FindAndMarkDeleted(ctx, backupConfig, dbData, objectStores, backupJobsState, jobId, 10)
	if cancelled {
		t.Fatal("Was expecting FindAndMarkDeleted() to return cancelled=false but it didn't")
	}

	// the above should have marked $path as deleted so this call should return an error
	found, _, err = getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("3. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if found {
		t.Fatalf("3. Found a record in the DB for path %s despite expecting to not find one", path)
	}
}

// backup plain file - without checksum - test backupNewItem() and backupExistingWithContentChange() and backupExistingWithMetadataChange()
func TestBackupNewItem1(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	result.Config.Backup[0].Checksum = false

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	err = dbops.EnsureTargetsInDb(dbData.Db, backupConfig)
	if err != nil {
		t.Fatalf("While trying to ensure all backup config targets have a DB entry, got error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	cancelled, err := backupNewItem(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting backupNewItem() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("backupNewItem() returned an error despite it not being expected: %s", err)
	}

	found, retrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1 .Did not find record in the DB for path %s despite it being expected", path)
	}

	retrievedDbRecordCopy := retrievedDbRecord
	retrievedDbRecordCopy.Owner = "the-new-owner"

	// we didn't actually have a content chage, we changed the Owner but none the less, this will lead to the DB record being updated
	cancelled, err = backupExistingWithContentChange(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, retrievedDbRecordCopy, jobId)
	if cancelled {
		t.Fatal("Was expecting backupExistingWithContentChange() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("backupExistingWithContentChange() returned an error despite it not being expected: %s", err)
	}

	found, SecondRetrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("2 .Did not find record in the DB for path %s despite it being expected", path)
	}
	if retrievedDbRecordCopy.Owner != SecondRetrievedDbRecord.Owner {
		t.Fatalf("backupExistingWithContentChange() doesn't seem to have updated the DB record, as expected")
	}

	SecondRetrievedDbRecord.Owner = "another-new-owner"
	cancelled, err = backupExistingWithMetadataChange(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, SecondRetrievedDbRecord, jobId)
	if cancelled {
		t.Fatal("Was expecting backupExistingWithMetadataChange() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("backupExistingWithMetadataChange() returned an error despite it not being expected: %s", err)
	}

	found, ThirdRetrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("3. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("3 .Did not find record in the DB for path %s despite it being expected", path)
	}
	if SecondRetrievedDbRecord.Owner != ThirdRetrievedDbRecord.Owner {
		t.Fatalf("backupExistingWithMetadataChange() doesn't seem to have updated the DB record, as expected")
	}
}

// backup plain file - with checksum enabled - test backupNewItem()
func TestBackupNewItem2(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	result.Config.Backup[0].Checksum = true

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	err = dbops.EnsureTargetsInDb(dbData.Db, backupConfig)
	if err != nil {
		t.Fatalf("While trying to ensure all backup config targets have a DB entry, got error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	cancelled, err := backupNewItem(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("Was expecting backupNewItem() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("backupNewItem() returned an error despite it not being expected: %s", err)
	}

	found, _, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("Did not find record in the DB for path %s despite it being expected", path)
	}
}

// backup plain file - with checksum disabled - test Do()
func TestDo(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	dereference := false

	result, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.Config.Backup[0].Dereference = dereference
	result.Config.Backup[0].Checksum = false

	// setup a tmpdir which then will be set in the config file as the path to be backed up
	dirPath := utils.SetupTmpDir("cloudbackup_TestMarkDeleted_", t)

	defer testutils.DeleteTestFilesAndDirs([]string{dirPath})
	// its critical for this test that the path used to test on has one of its parent directories mentioned in the config file as a path to be backed up
	result.Config.Backup[0].Paths = []string{dirPath}

	backupConfig := result.Config.Backup[0]

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}

	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_", jobId)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	if len(objectStores) < 1 {
		t.Fatal("No object stores defined in the config so there is nothing to test")
	}

	dbData, err := dbops.PrepareDbForBackup(backupConfig.Name, jobId, result.Config, backupJobsState, backupConfig)
	if err != nil {
		t.Fatalf("1. Could not setup DB prerequisite due to error: %s", err)
	}

	err = dbops.EnsureTargetsInDb(dbData.Db, backupConfig)
	if err != nil {
		t.Fatalf("While trying to ensure all backup config targets have a DB entry, got error: %s", err)
	}

	// cleanup
	defer func() {
		dbops.CloseStatementsAndDb(dbData)
	}()

	// setup a file which then will be fed to PrepareFileRecord() so we have a DB record to insert in the file table
	path := dirPath + string(filepath.Separator) + "a_test_file.txt"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Could not create file %s due to error: %s", path, err)
	}
	defer f.Close()
	defer testutils.DeleteTestFilesAndDirs([]string{path})
	_, err = f.WriteString("just a line with some text")
	if err != nil {
		t.Fatalf("Could not write %s due to error: %s", path, err)
	}
	f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	// new file, never backed up before
	cancelled, err := Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("1. Was expecting Do() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("1. Do() returned an error despite it not being expected: %s", err)
	}

	found, retrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("1. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("1. Did not find record in the DB for path %s despite it being expected", path)
	}

	// file previously backed up and hasn't changed since
	cancelled, err = Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("2. Was expecting Do() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("2. Do() returned an error despite it not being expected: %s", err)
	}

	found, secondRetrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("2. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("2. Did not find record in the DB for path %s despite it being expected", path)
	}

	if !reflect.DeepEqual(retrievedDbRecord, secondRetrievedDbRecord) {
		fmt.Println("########## REPORTED #############")
		utils.Pp(secondRetrievedDbRecord)
		fmt.Println("########## EXPECTED #############")
		utils.Pp(retrievedDbRecord)
		t.Fatal("2. Retrieved DB record after the second Do() run was supposed to equal the one retrieved before the second run but instead it differs")
	}

	// change the content of the file
	f, err = os.OpenFile(path, os.O_RDWR, 0755)
	if err != nil {
		t.Fatalf("Could not reopen %s due to err: %s", path, err)
	}
	defer f.Close()
	_, err = f.WriteString("just a line with some new text which should change its contents")
	if err != nil {
		t.Fatalf("2. Could not write %s due to error: %s", path, err)
	}
	f.Close()
	stat, err = os.Stat(path)
	if err != nil {
		t.Fatalf("While running os.Stat() got error: %s", err)
	}

	// file previously backed up and HAS had its contents changed since
	cancelled, err = Do(ctx, path, stat, backupConfig, dbData, objectStores, backupJobsState, jobId)
	if cancelled {
		t.Fatal("3. Was expecting Do() to return cancelled=false but it didn't")
	}
	if err != nil {
		t.Fatalf("3. Do() returned an error despite it not being expected: %s", err)
	}

	found, thirdRetrievedDbRecord, err := getBackedupObjectPropertiesFromDb(path, dbData)
	if err != nil {
		t.Fatalf("3. While retrieving from DB the record for path %s got error: %s", path, err)
	}
	if !found {
		t.Fatalf("3. Did not find record in the DB for path %s despite it being expected", path)
	}

	if reflect.DeepEqual(secondRetrievedDbRecord, thirdRetrievedDbRecord) {
		t.Fatal("3. Retrieved DB record after the third Do() is equal the one retrieved after the second run but they should differ")
	}
}
