package scan

import (
	"cloudbackup/config"
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"github.com/satori/go.uuid"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

// test number of examined files as reported by Path() when  dereference=true
func TestPath1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	database.CloseDb(db, backupConfig.Name)
}

// test number of examined files as reported by Path() when  dereference=false
func TestPath2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	database.CloseDb(db, backupConfig.Name)
}

// test number of examined files as reported by Path() when  dereference=false and top level folder is unreadable
func TestPath3(t *testing.T) {
	// skip this test on Windows as  os.Chmod 0000 is not possible on Windows
	if runtime.GOOS != "windows" {
		path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
		// remove tmpfile which holds the yaml as the config has been parsed and loaded
		defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
		ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
		if err != nil {
			t.Fatalf("Failed to get signalling channel. Error was: %s", err)
		}

		err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
		if err != nil {
			t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
		}
		db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
		if err != nil {
			t.Fatalf("database.OpenDb() returned error: '%s'", err)
		}
		dbData := shared.DbData{Db: db, Connected: true}

		objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
		if err != nil {
			t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
		}

		for _, backupPath := range backupConfig.Paths {
			_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
		database.CloseDb(db, backupConfig.Name)
	}
}

// test number of examined files as reported by Path() when  dereference=true and we have two simple exclusion rules
func TestPath4(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	database.CloseDb(db, backupConfig.Name)
}

// test number of examined files as reported by Path() when  dereference=true and we have an exclusion rule
// matching a unicode dir name
func TestPath5(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	database.CloseDb(db, backupConfig.Name)
}

// test number of examined files as reported by Path() when  dereference=true and the top level path is a file
func TestPath6(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=true with two top level paths in the config file:
//  one a folder (containing stuff) and the other one a file
func TestPath7(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=true with two top level paths in the config file:
//  one a folder (containing stuff) and the other one also a folder (having a copy of the files/folders from 1st path)
func TestPath8(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with .txt within any folder
func TestPath9(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	backupConfig.Exclusions = []string{"**" + string(filepath.Separator) + "*.txt"}
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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file[2-4]*.txt within any folder
func TestPath10(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	backupConfig.Exclusions = []string{"**" + string(filepath.Separator) + "file[2-4]*.txt"}
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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file?.txt within any folder
func TestPath11(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	backupConfig.Exclusions = []string{"**" + string(filepath.Separator) + "file?.txt"}
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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true, Name: backupConfig.Name}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=false and we have an exclusion rule
// matching any file ending with file{1,2}.txt within any folder
func TestPath12(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	backupConfig.Exclusions = []string{"**" + string(filepath.Separator) + "file{1,2}.txt"}
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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling channel. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.OpenDb(result.Config.DataDir, backupConfig.Name, true)
	if err != nil {
		t.Fatalf("database.OpenDb() returned error: '%s'", err)
	}
	dbData := shared.DbData{Db: db, Connected: true}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
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
	database.CloseDb(db, backupConfig.Name)
}

// test number of examined files as reported by Path() when  dereference=true and when using an actual DB
func TestPath13(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

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
	ctx, err := backupJobsState.GetContextForJob(backupConfig.Name, jobId)
	if err != nil {
		t.Fatalf("Failed to get signalling context. Error was: %s", err)
	}

	err = database.ValidateAndCreate(result.Config.DataDir, backupConfig.Name, false)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
	db, err := database.Start(result.Config.DataDir, backupConfig.Name)
	if err != nil {
		t.Fatalf("database.Start() returned error: '%s'", err)
	}

	preparedStatements, err := dbops.Prepare(db)
	if err != nil {
		t.Fatalf("dbops.Prepare() returned error: '%s'", err)
		database.CloseDb(db, backupConfig.Name)
	}

	dbData := shared.DbData{
		Db: db,
		Connected: true,
		PreparedStatements: preparedStatements,
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, false, dbData, objectStores)
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
	dbops.CloseStatementsAndDb(dbData)
}

// should not match exclusion
func TestIsExcluded1(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	//exclusions := []string{"**" + string(filepath.Separator) + "file{1,2}.txt"}
	exclusions := []string{"bla1234"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if excluded {
		t.Fatalf("Exclusion rule '%s' matched but it wasn't expected that it would match", exclusionRule)
	}
	if exclusionRule != "" {
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is empty but instead we got: '%s'", exclusionRule)
	}
}

// should match simple exclusion
func TestIsExcluded2(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	//exclusions := []string{"**" + string(filepath.Separator) + "file{1,2}.txt"}
	exclusions := []string{string(filepath.Separator) + "file1.txt"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if ! excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion
func TestIsExcluded3(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	exclusions := []string{string(filepath.Separator) + "file?.txt"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if ! excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion
func TestIsExcluded4(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	exclusions := []string{string(filepath.Separator) + "file*.txt"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if ! excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion with dir descend
func TestIsExcluded5(t *testing.T) {
	path := string(filepath.Separator) + "adir" + string(filepath.Separator) + "anotherDir" +
		string(filepath.Separator) + "file1.txt"
	exclusions := []string{"**" + string(filepath.Separator) + "*.txt"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if ! excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should NOT match shellglob exclusion due to lack of dir descend
func TestIsExcluded6(t *testing.T) {
	path := string(filepath.Separator) + "adir" + string(filepath.Separator) + "anotherDir" +
		string(filepath.Separator) + "file1.txt"
	exclusions := []string{"*.txt"}
	excluded, exclusionRule, err := isExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if excluded {
		t.Fatalf("Exclusion rule '%s' matched but it wasn't expected that it would match", exclusionRule)
	}
	if exclusionRule != "" {
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is empty but instead we got: '%s'", exclusionRule)
	}
}
