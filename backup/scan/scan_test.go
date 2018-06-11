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
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", uuid.NewV4().String())
	if err != nil {
		t.Fatal(err)
	}
	err = Path(backupDirPath, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Failed to walk mock backup directory path. Error was: %s", err)
	}
	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 9,
		"examined_files": 12,
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
	// set dereference to True
	backupConfig.Dereference = false
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// populate state object with default values
	err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", uuid.NewV4().String())
	if err != nil {
		t.Fatal(err)
	}
	err = Path(backupDirPath, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Failed to walk mock backup directory path. Error was: %s", err)
	}
	utils.Pp(backupJobsState.Running[0].StatsCounters)
	expectedStats := map[string]uint64{
		"examine_produced_errors": 0,
		"examined_directories": 5,
		"examined_files": 8,
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
		err = backupJobsState.MarkRunning(backupConfig.Name, "unittest_backup_scan", uuid.NewV4().String())
		if err != nil {
			t.Fatal(err)
		}
		err = Path(backupDirPath, backupConfig, backupJobsState)
		if err != nil {
			t.Fatalf("Failed to walk mock backup directory path. Error was: %s", err)
		}
		utils.Pp(backupJobsState.Running[0].StatsCounters)
		expectedStats := map[string]uint64{
			"examine_produced_errors": 1,
			"examined_directories": 4,
			"examined_files": 5,
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