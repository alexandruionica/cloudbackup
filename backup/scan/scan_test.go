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

func compareWithWatchMessages(t *testing.T, backupJobsState *shared.BackupJobsState) {
	// check that the Sequence is the sum of all examined_ stats
	sumExamined := backupJobsState.Running[0].StatsCounters["examined_directories"] + backupJobsState.Running[0].StatsCounters["examined_files"] +
		backupJobsState.Running[0].StatsCounters["examined_symlinks"] + backupJobsState.Running[0].StatsCounters["examined_unknown"]
	if sumExamined != backupJobsState.Running[0].Sequence {
		// dump all messages on the channel in order to use them for understanding what went wrong
	OUTERLOOP:
		for {
			select {
			case msg := <-backupJobsState.WatchMsgReceiver:
				{
					t.Logf("%+v", msg)
				}
			default:
				break OUTERLOOP
			}
		}

		t.Fatalf("Sequence has value %d and it doesn't match the sum of examined_directories + "+
			"examined_files + examined_symlinks +  examined_unknown; map containig those "+
			"is: %+v", backupJobsState.Running[0].Sequence, backupJobsState.Running[0].StatsCounters)
	}
	// loop over the Watcher channel and see if messages match expectations
	previousMsg := shared.WatchMessage{Sequence: 0}
	var examinedDirectories, examinedFiles, examinedSymlinks, examinedUnknown, failedToExamine, failedToEnumerate uint64 = 0, 0, 0, 0, 0, 0
	var uploadedDirectories, uploadedFiles, uploadedSymlinks uint64 = 0, 0, 0
OUTERLOOP2:
	for {
		select {
		case msg := <-backupJobsState.WatchMsgReceiver:
			{
				if msg.OperationType == "examine" && msg.Error != "" {
					failedToExamine += 1
				}
				if previousMsg.Sequence != 0 {
					if previousMsg.Path != msg.Path && previousMsg.PercentDone != 100 && previousMsg.Error == "" {
						t.Fatalf("For each file/dir/symlink we should get a 100%% done message (unless an error"+
							" is encountred) but for %+v we didn't get one", previousMsg)
					}
					if previousMsg.Path == msg.Path && previousMsg.PercentDone == 100 && msg.PercentDone == 100 {
						t.Fatalf("For each file/dir/symlink we should get ONLY a 100%% done message but for %s"+
							" we got at least two: %+v \nand\n %+v", previousMsg.Path, previousMsg, msg)
					}
				}
				switch msg.ObjectType {
				case "dir":
					{
						if msg.Error == "" {
							examinedDirectories += 1
						}
						if msg.PercentDone != 100 && msg.Error == "" {
							t.Fatalf("For watch() message %+v reported percent done is %d which != 100 . If no "+
								"error was encountered then we should always have 1 message for directories and it should"+
								" always report 100%% percent done", msg, msg.PercentDone)
						}
						if previousMsg.Sequence != 0 {
							if previousMsg.Path == msg.Path && msg.Error == "" {
								t.Fatalf("Only one message should be sent for type 'dir' (unless an error is "+
									"encountered when attempting to walk a dir) but we got two: %+v\n and \n%+v", previousMsg, msg)
							}
						}
						if msg.OperationType == "enumerate" && msg.Error != "" {
							failedToEnumerate += 1
						}
						if msg.PercentDone == 100 && msg.Error == "" {
							uploadedDirectories += 1
						}
					}
				case "symlink":
					{
						if msg.Error == "" {
							examinedSymlinks += 1
						}
						if msg.PercentDone != 100 && msg.Error == "" {
							t.Fatalf("For watch() message %+v reported percent done is %d which != 100 . If no "+
								"error was encountered then we should always have 1 message for symlink and it should"+
								" always report 100%% percent done", msg, msg.PercentDone)
						}
						if previousMsg.Sequence != 0 {
							if previousMsg.Path == msg.Path {
								t.Fatalf("Only one message should be sent for type 'symlink' but we got two: %+v\n "+
									"and \n%+v", previousMsg, msg)
							}

						}
						if msg.PercentDone == 100 && msg.Error == "" {
							uploadedSymlinks += 1
						}
					}
				case "file":
					{
						if previousMsg.Sequence != 0 && msg.Error == "" && previousMsg.Path != msg.Path {
							examinedFiles += 1
						} else {
							if msg.PercentDone == 100 {
								examinedFiles += 1
							}
						}
						if msg.PercentDone == 100 && msg.Error == "" {
							uploadedFiles += 1
						}
					}
				case "unknown":
					{
						if msg.OperationType == "examine" {
							examinedUnknown += 1
						}
					}
				default:
					{
						t.Fatalf("Unexpected type %s for watch() msg: %+v", msg.ObjectType, msg)

					}
				}
				previousMsg = msg
			}
		default:
			break OUTERLOOP2
		}
	}
	if backupJobsState.Running[0].StatsCounters["examined_directories"] != examinedDirectories {
		t.Fatalf("examined_directories reported by stats_counters is %d but examined_directories as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["examined_directories"], examinedDirectories)
	}
	if backupJobsState.Running[0].StatsCounters["examined_files"] != examinedFiles {
		t.Fatalf("examined_files reported by stats_counters is %d but examined_files as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["examined_files"], examinedFiles)
	}
	if backupJobsState.Running[0].StatsCounters["examined_symlinks"] != examinedSymlinks {
		t.Fatalf("examined_symlinks reported by stats_counters is %d but examined_symlinks as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["examined_symlinks"], examinedSymlinks)
	}
	if backupJobsState.Running[0].StatsCounters["examined_unknown"] != examinedUnknown {
		t.Fatalf("examined_unknown reported by stats_counters is %d but examined_unknown as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["examined_unknown"], examinedUnknown)
	}
	if backupJobsState.Running[0].StatsCounters["failed_to_examine"] != failedToExamine {
		t.Fatalf("failed_to_examine reported by stats_counters is %d but failed_to_examine as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["failed_to_examine"], failedToExamine)
	}
	if backupJobsState.Running[0].StatsCounters["failed_to_enumerate"] != failedToEnumerate {
		t.Fatalf("failed_to_enumerate reported by stats_counters is %d but failed_to_enumerate as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["failed_to_enumerate"], failedToEnumerate)
	}
	if backupJobsState.Running[0].StatsCounters["uploaded_directories"] != uploadedDirectories {
		t.Fatalf("uploaded_directories reported by stats_counters is %d but uploaded_directories as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["uploaded_directories"], uploadedDirectories)
	}
	if backupJobsState.Running[0].StatsCounters["uploaded_symlinks"] != uploadedSymlinks {
		t.Fatalf("uploaded_symlinks reported by stats_counters is %d but uploaded_symlinks as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["uploaded_symlinks"], uploadedSymlinks)
	}
	if backupJobsState.Running[0].StatsCounters["uploaded_files"] != uploadedFiles {
		t.Fatalf("uploaded_files reported by stats_counters is %d but uploaded_files as "+
			"counted from watch() messages is %d and it should be equal",
			backupJobsState.Running[0].StatsCounters["uploaded_files"], uploadedFiles)
	}
}

// test number of examined files as reported by Path() when  dereference=true
func TestPath1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      11, // including top level dir
		"examined_files":                            16,
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      7,  // including top level dir
		"examined_files":                            10, // 10 in total
		"examined_symlinks":                         2,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

		result, err := config.Load(path, false, &sync.RWMutex{})
		if err != nil {
			t.Fatalf("Could not load fake config file. Error was: %s", err)
		}

		// folder with some mock files and symlinks
		backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
		defer func() {
			_ = os.Chmod(backupDirPath+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
				string(filepath.Separator)+"dir3", 0700) // #nosec
			err = os.RemoveAll(backupDirPath) // #nosec
			if err != nil {
				t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
			}
		}()
		// make folder unreadable so it produces an error
		err = os.Chmod(backupDirPath+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
			string(filepath.Separator)+"dir3", 0000)
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

		objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
		if err != nil {
			t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
		}

		for _, backupPath := range backupConfig.Paths {
			_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
			if err != nil {
				t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
			}
		}

		expectedStats := map[string]uint64{
			"examined_directories":                      6, // including top level dir - 1 unreadable folder
			"examined_files":                            7, // 10 in total but there is 1 unreadable folder containing 3 files
			"examined_symlinks":                         2,
			"examined_unknown":                          0,
			"failed_to_examine":                         0,
			"failed_to_enumerate":                       1,
			"excluded":                                  0,
			"up_to_date_directories":                    0,
			"up_to_date_files":                          0,
			"up_to_date_symlinks":                       0,
			"uploaded_directories":                      0, // none due to dryrun=true
			"uploaded_files":                            0, // none due to dryrun=true
			"uploaded_symlinks":                         0, // none due to dryrun=true
			"failed_to_upload_directories":              0,
			"failed_to_upload_files":                    0,
			"failed_to_upload_symlinks":                 0,
			"failed_to_upload_unknown":                  0,
			"scripts_failed":                            0,
			"scripts_ran":                               0,
			"scripts_num":                               0,
			"updated_metadata_for_files":                0,
			"updated_metadata_for_directories":          0,
			"updated_metadata_for_symlinks":             0,
			"failed_to_update_metadata_for_directories": 0,
			"failed_to_update_metadata_for_files":       0,
			"failed_to_update_metadata_for_symlinks":    0,
		}
		if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
			utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      10, // including top level dir -1 excluded
		"examined_files":                            15, // 16 in total but there is 1 exclusion rule matching 1 file
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  2,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      10, // including top level dir - 1 excluded dir
		"examined_files":                            14, // 16 in total but 2 are in the excluded dir
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  1,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// folder to contain the 1 mock file
	backupDirPath := utils.SetupTmpDir("unittest_backup_scan_path", t)
	defer func() {
		err = os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	// create file to backup
	err = ioutil.WriteFile(backupDirPath+string(filepath.Separator)+"file1", []byte(`text for file1`), 0644)
	if err != nil {
		t.Fatalf("While trying to create a tmp file to test backing up of, got error: '%s'", err)
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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}
	expectedStats := map[string]uint64{
		"examined_directories":                      0,
		"examined_files":                            1,
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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
	err = ioutil.WriteFile(backupDirPath2+string(filepath.Separator)+"file1", []byte(`text for file1`), 0644)
	if err != nil {
		t.Fatalf("While trying to create a tmp file to test backing up of, got error: '%s'", err)
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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      11, // including top level dir
		"examined_files":                            17, // 16 + 1 from the 2nd path
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      22, // including top level dirs
		"examined_files":                            32,
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      7, // including top level dir
		"examined_files":                            8, // 10 in total but there is 1 exclusion rule matching 2 files
		"examined_symlinks":                         2,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  2,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      7, // including top level dir
		"examined_files":                            9, // 10 in total but there is 1 exclusion rule matching 1 file
		"examined_symlinks":                         2,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  1,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      7, // including top level dir
		"examined_files":                            9, // 10 in total but there is 1 exclusion rule matching 1 file
		"examined_symlinks":                         2,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  1,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

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

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, true, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      7, // including top level dir
		"examined_files":                            9, // 10 in total but there is 1 exclusion rule matching 1 file
		"examined_symlinks":                         2,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  1,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      0, // none due to dryrun=true
		"uploaded_files":                            0, // none due to dryrun=true
		"uploaded_symlinks":                         0, // none due to dryrun=true
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
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

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
	defer func() {
		//err = os.RemoveAll(backupDirPath) // #nosec
		//if err != nil {
		//	t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		//}
	}()
	backupConfig := result.Config.Backup[0]
	// overwrite whatever was in the mock config with the tmp path we want to test
	backupConfig.Paths = []string{backupDirPath}
	// set dereference to True
	backupConfig.Dereference = true
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{
		WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
		Lock:             &sync.RWMutex{},
	}
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
		Db:                 db,
		Connected:          true,
		PreparedStatements: preparedStatements,
	}

	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
	}

	for _, backupPath := range backupConfig.Paths {
		_, err = Path(ctx, backupPath, backupConfig, backupJobsState, false, dbData, objectStores)
		if err != nil {
			t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
		}
	}

	expectedStats := map[string]uint64{
		"examined_directories":                      11, // 11 (including top level) due to 2 symlinks pointing at dirs
		"examined_files":                            16, // 10 in total but 2 symlinks pointing at dirs make it 16 in total
		"examined_symlinks":                         0,
		"examined_unknown":                          0,
		"failed_to_examine":                         0,
		"failed_to_enumerate":                       0,
		"excluded":                                  0,
		"up_to_date_directories":                    0,
		"up_to_date_files":                          0,
		"up_to_date_symlinks":                       0,
		"uploaded_directories":                      11,
		"uploaded_files":                            16,
		"uploaded_symlinks":                         0,
		"failed_to_upload_directories":              0,
		"failed_to_upload_files":                    0,
		"failed_to_upload_symlinks":                 0,
		"failed_to_upload_unknown":                  0,
		"scripts_failed":                            0,
		"scripts_ran":                               0,
		"scripts_num":                               0,
		"updated_metadata_for_files":                0,
		"updated_metadata_for_directories":          0,
		"updated_metadata_for_symlinks":             0,
		"failed_to_update_metadata_for_directories": 0,
		"failed_to_update_metadata_for_files":       0,
		"failed_to_update_metadata_for_symlinks":    0,
	}
	if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
		utils.Pp(backupJobsState.Running[0].StatsCounters)
		t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
			backupJobsState.Running[0].StatsCounters, expectedStats)
	}
	// check how many bytes were read
	var expectedBytesRead uint64 = 260 // 168 bytes total + 46 * 2 for the 2 symlinks pointing at the same dir3 which contains a total of 46 bytes worth of file content
	if backupJobsState.Running[0].FileContentBytesRead != expectedBytesRead {
		t.Fatalf("The total file content was expected to be %d bytes but it is actually reported to be %d bytes", expectedBytesRead, backupJobsState.Running[0].FileContentBytesRead)
	}

	// a lot of testing goes in this function - compares messages on the Watch channel with the backupJobsState stats
	compareWithWatchMessages(t, backupJobsState)

	dbops.CloseStatementsAndDb(dbData)
}

// test number of examined files as reported by Path() when  dereference=false and when using an actual DB and a folder is unreadable.Similar to
// TestPath3 but  dryrun=false and a valid DB . Also test all sorts of Jobstate related stats
func TestPath14(t *testing.T) {
	// skip this test on Windows as  os.Chmod 0000 is not possible on Windows
	if runtime.GOOS != "windows" {
		path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_backup_scan_path_")
		// remove tmpfile which holds the yaml as the config has been parsed and loaded
		defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

		result, err := config.Load(path, false, &sync.RWMutex{})
		if err != nil {
			t.Fatalf("Could not load fake config file. Error was: %s", err)
		}

		// folder with some mock files and symlinks
		backupDirPath := testutils.SetupBackupDir("unittest_backup_scan_path", t)
		defer func() {
			_ = os.Chmod(backupDirPath+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
				string(filepath.Separator)+"dir3", 0700) // #nosec
			err = os.RemoveAll(backupDirPath) // #nosec
			if err != nil {
				t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
			}
		}()

		// make folder unreadable so it produces an error
		err = os.Chmod(backupDirPath+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
			string(filepath.Separator)+"dir3", 0000)
		if err != nil {
			t.Fatal(err)
		}

		backupConfig := result.Config.Backup[0]
		// overwrite whatever was in the mock config with the tmp path we want to test
		backupConfig.Paths = []string{backupDirPath}
		// set dereference to false
		backupConfig.Dereference = false
		// backupJobState contains the state of all running backup jobs plus it has some handy methods
		backupJobsState := &shared.BackupJobsState{
			WatchMsgReceiver: make(chan shared.WatchMessage, 1000),
			Lock:             &sync.RWMutex{},
		}
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
			Db:                 db,
			Connected:          true,
			PreparedStatements: preparedStatements,
		}

		objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
		if err != nil {
			t.Fatalf("Could not initialise backend object store(s) from the config due to error: %s", err)
		}

		for _, backupPath := range backupConfig.Paths {
			_, err = Path(ctx, backupPath, backupConfig, backupJobsState, false, dbData, objectStores)
			if err != nil {
				t.Fatalf("Failed to walk backup directory path %s. Error was: %s", backupPath, err)
			}
		}

		expectedStats := map[string]uint64{
			"examined_directories":                      6, // 7 in total but the 1 dir in "dir3" is unaccessible due to chmod 000 on dir3
			"examined_files":                            7, // 10 in total but the 3 dir in "dir3" are unaccessible due to chmod 000 on dir3
			"examined_symlinks":                         2,
			"examined_unknown":                          0,
			"failed_to_examine":                         0,
			"failed_to_enumerate":                       1,
			"excluded":                                  0,
			"up_to_date_directories":                    0,
			"up_to_date_files":                          0,
			"up_to_date_symlinks":                       0,
			"uploaded_directories":                      6,
			"uploaded_files":                            7,
			"uploaded_symlinks":                         2,
			"failed_to_upload_directories":              0,
			"failed_to_upload_files":                    0,
			"failed_to_upload_symlinks":                 0,
			"failed_to_upload_unknown":                  0,
			"scripts_failed":                            0,
			"scripts_ran":                               0,
			"scripts_num":                               0,
			"updated_metadata_for_files":                0,
			"updated_metadata_for_directories":          0,
			"updated_metadata_for_symlinks":             0,
			"failed_to_update_metadata_for_directories": 0,
			"failed_to_update_metadata_for_files":       0,
			"failed_to_update_metadata_for_symlinks":    0,
		}
		if !reflect.DeepEqual(expectedStats, backupJobsState.Running[0].StatsCounters) {
			utils.Pp(backupJobsState.Running[0].StatsCounters)
			t.Fatalf("Stats reported by Path() are %+v don't match expected %+v",
				backupJobsState.Running[0].StatsCounters, expectedStats)
		}
		// check how many bytes were read
		var expectedBytesRead uint64 = 122 // 168 bytes total but excluding the files in dir3 then it's 122 bytes
		if backupJobsState.Running[0].FileContentBytesRead != expectedBytesRead {
			t.Fatalf("The total file content was expected to be %d bytes but it is actually reported to be %d bytes", expectedBytesRead, backupJobsState.Running[0].FileContentBytesRead)
		}

		// a lot of testing goes in this function - compares messages on the Watch channel with the backupJobsState stats
		compareWithWatchMessages(t, backupJobsState)

		dbops.CloseStatementsAndDb(dbData)
	}
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
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second "+
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
	if !excluded {
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
	if !excluded {
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
	if !excluded {
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
	if !excluded {
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
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second "+
			"argument in reply) is empty but instead we got: '%s'", exclusionRule)
	}
}
