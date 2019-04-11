package backup

import (
	"cloudbackup/backup/fileproperties"
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"errors"
	"fmt"
	"github.com/satori/go.uuid"
	"os"
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
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "TestPrepareFileRecord1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path)
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
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "TestPrepareFileRecord1_sample_file_")
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

	ctime, err := fileproperties.GetCtime(path)
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
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "TestPrepareFileRecord1_sample_file_")
	if err != nil {
		err2 := os.RemoveAll(path)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", path, err2)
		}
		t.Fatalf("Could not create tmp sample file due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{path})

	// ########
	symlinkPath := testutils.GenerateTmpFilePath("backup_test_symlink_", "_to_plain_File")
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

	ctime, err := fileproperties.GetCtime(symlinkPath)
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
