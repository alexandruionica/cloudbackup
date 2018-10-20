package database

import (
	"testing"
	"cloudbackup/utils"
	"os"
	"path/filepath"
	"cloudbackup/testutils"
)

// test GetDbFilePath() with valid, absolute data_dir path
func TestGetDbFilePath1(t *testing.T) {
	backupName := "backup1"
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	resultPath, err := GetDbFilePath(dbDataDirPath, backupName)
	if err != nil {
		t.Fatalf("GetDbFilePath() returned unexpected error: '%s'", err)
	}
	expectedPath := dbDataDirPath + string(filepath.Separator) + backupName + ".sqlite"
	if expectedPath != resultPath {
		t.Fatalf("Expected GetDbFilePath() to return: '%s' but it returned: '%s'", expectedPath, resultPath)
	}
}

// test GetDbFilePath() with relative data_dir path
func TestGetDbFilePath2(t *testing.T) {
	backupName := "backup1"
	dbDataDirPath := "a_relative_path"
	resultPath, err := GetDbFilePath(dbDataDirPath, backupName)
	if err != nil {
		t.Fatalf("GetDbFilePath() returned unexpected error: '%s'", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("While trying to get the current working directory, encountered error: '%s'", err)
	}
	expectedPath := workingDir + string(filepath.Separator) + dbDataDirPath + string(filepath.Separator) + backupName + ".sqlite"
	if expectedPath != resultPath {
		t.Fatalf("Expected GetDbFilePath() to return: '%s' but it returned: '%s'", expectedPath, resultPath)
	}
}

// test DbFileExists with valid, absolute path returns true
func TestDbFileExists1(t *testing.T) {
	// we will test with a yaml file (not an .sqlite) but it doesn't matter the extension or content as long as the
	// file exits
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_database_DbFileExists_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	exists := DbFileExists(path)
	if ! exists {
		t.Fatalf("DbFileExists() was supposed to return true for '%s' but it didn't", path)
	}

}

// test DbFileExists with invalid, absolute path returns false as expected
func TestDbFileExists2(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	path := dbDataDirPath + string(filepath.Separator) + "a_file_which_does_not_exist.sqlite"
	exists := DbFileExists(path)
	if exists {
		t.Fatalf("DbFileExists() was supposed to return false for '%s' but it didn't", path)
	}
}

// test OpenDb() with invalid, absolute path to the .sqlite database file
func TestOpenDb1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	_, err := OpenDb(dbDataDirPath + string(filepath.Separator) + "folder_which_does_not_exist", backupName, true)
	if err == nil {
		t.Fatal("OpenDb() was supposed to return an error but didn't")
	}
}

// test CreateDb() with valid, absolute path to the .sqlite database file
func TestCreateDb1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	db, err := OpenDb(dbDataDirPath, backupName, false)
	defer func() {
		CloseDb(db, backupName)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	if err != nil {
		t.Fatalf("CreateDb() returned error: '%s'", err)
	}
}

// test CreateDb() with invalid, absolute path to the .sqlite database file
func TestCreateDb2_1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := OpenDb(dbDataDirPath + string(filepath.Separator) + "folder_which_does_not_exist", backupName,
		false)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	expectedErr := "unable to open database file"
	if err == nil {
		t.Fatal("Expected CreateDb() to produce an error but it didn't")
	}
	if err.Error() != expectedErr {
		t.Fatalf("CreateDb() was expected to return error: '%s' but it returned: '%s'", expectedErr, err)
	}
}

// test CreateDb() with valid, absolute path to the .sqlite database file and then call it again with same path ; 2nd
//  call should return an error
func TestCreateDb3(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := OpenDb(dbDataDirPath, backupName, false)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	if err != nil {
		t.Fatalf("CreateDb() returned error: '%s'", err)
	}

	db2, err := OpenDb(dbDataDirPath, backupName, true)
	defer CloseDb(db2, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db2, backupName)
	expectedErr := "table files already exists"
	if err.Error() != expectedErr {
		t.Fatalf("2nd call to CreateDb() was expected to return error: '%s' but it returned: '%s'",
			expectedErr, err)
	}
}

// ValidateAndCreate() with valid parameters and configInit=false - db file does not exist
func TestValidateAndCreate1(t *testing.T) {
	backupName := "backup1"
	configInit := false
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	err := ValidateAndCreate(dbDataDirPath, backupName, configInit)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
}

// ValidateAndCreate() with valid parameters and configInit=true - db file does not exist
func TestValidateAndCreate2(t *testing.T) {
	backupName := "backup1"
	configInit := true
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	err := ValidateAndCreate(dbDataDirPath, backupName, configInit)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
}

// ValidateAndCreate() with valid parameters and configInit=false - db file already exists
func TestValidateAndCreate3(t *testing.T) {
	backupName := "backup1"
	configInit := false
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := OpenDb(dbDataDirPath, backupName, false)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	if err != nil {
		t.Fatalf("CreateDb() returned error: '%s'", err)
	}

	err = ValidateAndCreate(dbDataDirPath, backupName, configInit)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
}

// ValidateAndCreate() with valid parameters and configInit=true - db file already exists
func TestValidateAndCreate4(t *testing.T) {
	backupName := "backup1"
	configInit := true
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := OpenDb(dbDataDirPath, backupName, false)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	if err != nil {
		t.Fatalf("CreateDb() returned error: '%s'", err)
	}

	err = ValidateAndCreate(dbDataDirPath, backupName, configInit)
	if err != nil {
		t.Fatalf("ValidateAndCreate() returned error: '%s'", err)
	}
}

// Start() with valid parameters - db file does not exist
func TestStart1(t *testing.T) {
	backupName := "backup1"
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db , err := Start(dbDataDirPath, backupName)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}
}

// Start() with valid parameters - db file already exists
func TestStart2(t *testing.T) {
	backupName := "backup1"
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := OpenDb(dbDataDirPath, backupName, false)
	defer CloseDb(db, backupName)
	if err != nil {
		t.Fatalf("OpenDb() returned error: '%s'", err)
	}
	err = CreateDb(db, backupName)
	if err != nil {
		t.Fatalf("CreateDb() returned error: '%s'", err)
	}
	CloseDb(db, backupName)

	db2, err := Start(dbDataDirPath, backupName)
	defer CloseDb(db2, backupName)
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}
}