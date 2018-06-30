package scan

import (
	"testing"
	"cloudbackup/config"
	"cloudbackup/utils"
	"cloudbackup/testutils"
	"os"
	"sync"
	"cloudbackup/shared"
	"github.com/satori/go.uuid"
	"reflect"
	"runtime"
	"path/filepath"
	"io/ioutil"
)

// test number of examined files as reported by Path() when  dereference=true
func TestPath1(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 11,
		"examined_files": 16,
		"excluded": 0,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false
func TestPath2(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	// set dereference to False
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 7,
		"examined_files": 12,
		"excluded": 0,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false and top level folder is unreadable
func TestPath3(t *testing.T) {
	// skip this test on Windows as  os.Chmod 0000 is not possible on Windows
	if runtime.GOOS != "windows" {
		path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
		if err != nil {
			t.Fatal(err)
		}
		// remove tmpfile which holds the yaml as the config has been parsed and loaded
		defer func() {
			err := os.Remove(path)
			if err != nil {
				t.Fatal(err)
			}
		}()

		result , err := config.Load(path, false, &sync.RWMutex{})
		if err != nil {
			t.Fatalf("Could not load fake config file. Error was: %s", err)
		}
		utils.Pp(result)

		// folder with some mock files and symlinks
		backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
		defer func() {
			_ = os.Chmod(backupDirPath + string(filepath.Separator) + "dir1" + string(filepath.Separator) + "dir2" +
				string(filepath.Separator) + "dir3", 0700) // #nosec
			err = os.RemoveAll(backupDirPath) // #nosec
			if err != nil {
				t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
			}
		}()
		// make folder unreadable so it produces an error
		err = os.Chmod(backupDirPath + string(filepath.Separator) + "dir1" + string(filepath.Separator) + "dir2" +
			string(filepath.Separator) + "dir3", 0000)
		if err != nil {
			t.Fatal(err)
		}

		backupConfig := result.Config.Backup[0]
		// overwrite whatever was in the mock config with the tmp path we want to test
		backupConfig.Paths = []string{backupDirPath}
		// set dereference to True
		backupConfig.Dereference = false
		// backupJobState contains the state of all running backup jobs plus it has some handy methods
		backupJobsState := &shared.BackupJobsState{}
		backupJobsState.Lock = &sync.RWMutex{}
		// populate state object with default values
		jobId := uuid.NewV4().String()
		err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
		if err != nil {
			t.Fatal(err)
		}
		closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
		if err != nil {
			t.Fatalf("Failed to get signalling channel. Error was: %s", err)
		}

		for _, backupPath := range backupConfig.Paths {
			_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
			if err != nil {
				t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
			}
		}

		utils.Pp(backupJobsState.Running[0].StatsCounters)
		expectedStats := map[string]uint64{
			"examine_produced_errors": 1,
			"examined_directories": 6,
			"examined_files": 9,
			"excluded": 0,
			"upload_produced_errors": 0,
			"uploaded_directories_metadata": 0,
			"uploaded_files": 0,
		}
		if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
			t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
				backupJobsState.Running[0].StatsCounters, expectedStats)
		}
	}
}

// test number of examined files as reported by Path() when  dereference=true and we have two simple exclusion rules
func TestPath4(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{
		backupDirPath + string(filepath.Separator) + "dir1" + string(filepath.Separator) + "dir5",
		backupDirPath + string(filepath.Separator) + "dir1" + string(filepath.Separator) + "file7",
	}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 10,
		"examined_files": 15,
		"excluded": 2,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=true and we have an exclusion rule
// matching a unicode dir name
func TestPath5(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{
		backupDirPath + string(filepath.Separator) + "dir1" + string(filepath.Separator) + "dir6*",
	}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 10,
		"examined_files": 14,
		"excluded": 1,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=true and the top level path is a file
func TestPath6(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder to contain the 1 mock file
	backupDirPath := utils.SetupTmpDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	// create file to backup
	err = ioutil.WriteFile(backupDirPath + string(filepath.Separator) + "file1", []byte(`text for file1`), 0644)
	if err != nil {
		t.Fatalf("While trying to create a tmp file to test backing up of, got error: '%s'",err)
	}

	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath + string(filepath.Separator) + "file1"}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}
	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}
	utils.Pp(backupConfig)
	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 0,
		"examined_files": 1,
		"excluded": 0,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=true with two top level paths in the config file:
//  one a folder (containing stuff) and the other one a file
func TestPath7(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	// folder to contain the 1 mock file
	backupDirPath2 := utils.SetupTmpDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath2) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	// create file to backup
	err = ioutil.WriteFile(backupDirPath2 + string(filepath.Separator) + "file1", []byte(`text for file1`), 0644)
	if err != nil {
		t.Fatalf("While trying to create a tmp file to test backing up of, got error: '%s'",err)
	}

	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath, backupDirPath2 + string(filepath.Separator) + "file1"}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 11,
		"examined_files": 17,
		"excluded": 0,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=true with two top level paths in the config file:
//  one a folder (containing stuff) and the other one also a folder (having a copy of the files/folders from 1st path)
func TestPath8(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	// folder with some mock files and symlinks
	backupDirPath2 := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath2) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath, backupDirPath2}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 22,
		"examined_files": 32,
		"excluded": 0,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with .txt within any folder
func TestPath9(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{"**/*.txt"}
	// set dereference to False
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 7,
		"examined_files": 10,
		"excluded": 2,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file[2-4]*.txt within any folder
func TestPath10(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{"**/file[2-4]*.txt"}
	// set dereference to False
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 7,
		"examined_files": 11,
		"excluded": 1,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file?.txt within any folder
func TestPath11(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{"**/file?.txt"}
	// set dereference to False
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 7,
		"examined_files": 11,
		"excluded": 1,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file{1,2}.txt within any folder
func TestPath12(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_backup_scan_path_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	utils.Pp(result)

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	backupConfig.Exclusions = []string{"**/file{1,2}.txt"}
	// set dereference to False
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	jobId := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", jobId)
	if err != nil {
		t.Fatal(err)
	}
	closeChan, err := backupJobsState.GetSignalChanForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(backupPath, backupConfig, backupJobsState, closeChan, false)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 7,
		"examined_files": 11,
		"excluded": 1,
		"upload_produced_errors": 0,
		"uploaded_directories_metadata": 0,
		"uploaded_files": 0,
	}
	if ! reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
}