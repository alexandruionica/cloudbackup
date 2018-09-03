package dbops

import (
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"cloudbackup/database"
	"os"
	"sync"
	"testing"
)

// test EnsureTargetsInDb() with empty db
func TestEnsureTargetsInDb1(t *testing.T) {
	// setup config file in a tmpdir
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	db , err := database.Start(dbDataDirPath, backupName)
	defer func() {
		database.CloseDb(db, backupName)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}

	// get second back job config
	backupConfig := srvConfig.GetCopyWithLock("dbops_test.go").Backup[1]
	// actual test
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}

	// validate data in DB
	var (
		targetName string
		bkpName    string
		targetType string
	)
	type foundData struct {
		targetName string
		backupName string
		targetType string
	}
	dbFoundTargetNames := make(map[string]foundData)
	rows, err := db.Query("SELECT name, backup_name, type from targets")
	if err != nil {
		t.Fatalf("While trying to get from the database the list of targets, the following error was " +
			"encountered: '%s'", err)
	}
	for rows.Next() {
		err := rows.Scan(&targetName, &bkpName, &targetType)
		if err != nil {
			t.Fatalf("While enumerating from the database the list of targets, the following error was "+
				"encountered: '%s'", err)
		}
		dbFoundTargetNames[targetName] = foundData{
			targetName: targetName,
			backupName: bkpName,
			targetType: targetType,
		}
	}
	// target names in the config(for selected back job) should also exist in the DB
	for _, targetInConfig := range backupConfig.Target {
		if _, ok := dbFoundTargetNames[targetInConfig.Name]; ok {
			if targetInConfig.Type != dbFoundTargetNames[targetInConfig.Name].targetType {
				t.Fatalf("Target '%s' has type '%s' in the config file but the result from the DB shows type " +
					"'%s", targetInConfig.Name, targetInConfig.Type, dbFoundTargetNames[targetInConfig.Name].targetType)
			}
			if backupConfig.Name != dbFoundTargetNames[targetInConfig.Name].backupName {
				t.Fatalf("Target '%s' has backup job name '%s' in the config file but the result from the DB " +
					"shows name '%s'", targetInConfig.Name, backupConfig.Name,
					dbFoundTargetNames[targetInConfig.Name].backupName)
			}
			continue
		} else {
			t.Fatalf("Target '%s' exists in the config file but it wasn't found in the DB", backupConfig.Target)
		}
	}
}

// test EnsureTargetsInDb() with empty db and then with non empty db where it doesn't need to do anything
func TestEnsureTargetsInDb2(t *testing.T) {
	// setup config file in a tmpdir
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	db , err := database.Start(dbDataDirPath, backupName)
	defer func() {
		database.CloseDb(db, backupName)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}

	// get second back job config
	backupConfig := srvConfig.GetCopyWithLock("dbops_test.go").Backup[1]
	// populate DB
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}

	// actual test
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}
}

// test EnsureTargetsInDb() with empty db and then with non empty db where it needs to add 1 entry
func TestEnsureTargetsInDb3(t *testing.T) {
	// setup config file in a tmpdir
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	db , err := database.Start(dbDataDirPath, backupName)
	defer func() {
		database.CloseDb(db, backupName)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}

	// get second back job config
	backupConfig := srvConfig.GetCopyWithLock("dbops_test.go").Backup[1]
	// populate DB
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}

	result , err := db.Exec(`DELETE FROM targets WHERE name="aws_2"`)
	if err != nil {
		if err != nil {
			t.Fatalf("Attempting to delete 1 record from 'targets' returned error: '%s'", err)
		}
	}
	rowCount, _ := result.RowsAffected()
	if rowCount == 0 {
		t.Fatalf("Attempted to delete 1 record with name='first_backup' but no match was found")
	}
	// actual test
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}

	// validate data in DB
	var (
		targetName string
		bkpName    string
		targetType string
	)
	type foundData struct {
		targetName string
		backupName string
		targetType string
	}
	dbFoundTargetNames := make(map[string]foundData)

	rows, err := db.Query("SELECT name, backup_name, type from targets")
	if err != nil {
		t.Fatalf("While trying to get from the database the list of targets, the following error was " +
			"encountered: '%s'", err)
	}
	for rows.Next() {
		err := rows.Scan(&targetName, &bkpName, &targetType)
		if err != nil {
			t.Fatalf("While enumerating from the database the list of targets, the following error was "+
				"encountered: '%s'", err)
		}
		dbFoundTargetNames[targetName] = foundData{
			targetName: targetName,
			backupName: bkpName,
			targetType: targetType,
		}
	}
	// target names in the config(for selected back job) should also exist in the DB
	for _, targetInConfig := range backupConfig.Target {
		if _, ok := dbFoundTargetNames[targetInConfig.Name]; ok {
			if targetInConfig.Type != dbFoundTargetNames[targetInConfig.Name].targetType {
				t.Fatalf("Target '%s' has type '%s' in the config file but the result from the DB shows type " +
					"'%s", targetInConfig.Name, targetInConfig.Type, dbFoundTargetNames[targetInConfig.Name].targetType)
			}
			if backupConfig.Name != dbFoundTargetNames[targetInConfig.Name].backupName {
				t.Fatalf("Target '%s' has backup job name '%s' in the config file but the result from the DB " +
					"shows name '%s'", targetInConfig.Name, backupConfig.Name,
					dbFoundTargetNames[targetInConfig.Name].backupName)
			}
			continue
		} else {
			t.Fatalf("Target '%s' exists in the config file but it wasn't found in the DB", backupConfig.Target)
		}
	}
}