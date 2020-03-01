package dbops

import (
	"cloudbackup/config"
	"cloudbackup/database"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"github.com/gofrs/uuid"
	"os"
	"sync"
	"testing"
	"time"
)

// test EnsureTargetsInDb() with empty db
func TestEnsureTargetsInDb1(t *testing.T) {
	// setup config file in a tmpdir
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_database_dbops_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database - while this is normally setup automatically, here we want to be sure the DB
	// is closed before we attempt to delete the file
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}
	numDbClients += 1

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
		t.Fatalf("While trying to get from the database the list of targets, the following error was "+
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
				t.Fatalf("Target '%s' has type '%s' in the config file but the result from the DB shows type "+
					"'%s", targetInConfig.Name, targetInConfig.Type, dbFoundTargetNames[targetInConfig.Name].targetType)
			}
			if backupConfig.Name != dbFoundTargetNames[targetInConfig.Name].backupName {
				t.Fatalf("Target '%s' has backup job name '%s' in the config file but the result from the DB "+
					"shows name '%s'", targetInConfig.Name, backupConfig.Name,
					dbFoundTargetNames[targetInConfig.Name].backupName)
			}
			continue
		} else {
			t.Fatalf("Target '%s' exists in the config file but it wasn't found in the DB", targetInConfig.Name)
		}
	}
}

// test EnsureTargetsInDb() with empty db and then with non empty db where it doesn't need to do anything
func TestEnsureTargetsInDb2(t *testing.T) {
	// setup config file in a tmpdir
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_database_dbops_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database - while this is normally setup automatically, here we want to be sure the DB
	// is closed before we attempt to delete the file
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}
	numDbClients += 1

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
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_database_dbops_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	srvConfig, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// setup tmp dir to hold the database - while this is normally setup automatically, here we want to be sure the DB
	// is closed before we attempt to delete the file
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("Start() returned error: '%s'", err)
	}
	numDbClients += 1

	// get second back job config
	backupConfig := srvConfig.GetCopyWithLock("dbops_test.go").Backup[1]
	// populate DB
	err = EnsureTargetsInDb(db, backupConfig)
	if err != nil {
		t.Fatalf("EnsureTargetsInDb() returned error: '%s'", err)
	}

	result, err := db.Exec(`DELETE FROM targets WHERE name="aws_2"`)
	if err != nil {
		t.Fatalf("Attempting to delete 1 record from 'targets' returned error: '%s'", err)
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
		t.Fatalf("While trying to get from the database the list of targets, the following error was "+
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
				t.Fatalf("Target '%s' has type '%s' in the config file but the result from the DB shows type "+
					"'%s", targetInConfig.Name, targetInConfig.Type, dbFoundTargetNames[targetInConfig.Name].targetType)
			}
			if backupConfig.Name != dbFoundTargetNames[targetInConfig.Name].backupName {
				t.Fatalf("Target '%s' has backup job name '%s' in the config file but the result from the DB "+
					"shows name '%s'", targetInConfig.Name, backupConfig.Name,
					dbFoundTargetNames[targetInConfig.Name].backupName)
			}
			continue
		} else {
			t.Fatalf("Target '%s' exists in the config file but it wasn't found in the DB", targetInConfig.Name)
		}
	}
}

// test that Prepare() produces usable prepared statements
func TestPrepare1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
	path := "an_imaginary_file_name"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}
	numDbClients += 1

	preparedStatements, err := Prepare(db)
	if err != nil {
		t.Fatalf("Prepare() wasn't supposed to return an error but did return: '%s'", err)
	}

	// test Prepared Query
	rows, err := preparedStatements.FilesQueryStmt.Query(path)
	if err != nil {
		t.Fatalf("While querying the database in order to check if '%s' has been previously backed"+
			" up, the following error was encountered: %s", path, err)
	}

	err = rows.Close()
	if err != nil {
		t.Fatalf("While trying to Close() a prepared statement for checking if '%s' has been"+
			" previously backed up, the following error was encountered: %s", path, err)
	}

	// add entry to "jobs" DB table
	err = AddJobDetails(db, jobId, "my_backup", "backup", time.Now())
	if err != nil {
		t.Fatalf("Could not add job details to 'jobs' table")
	}

	// test Prepared Insert
	_, err = preparedStatements.FilesInsertStmt.Exec(path, "file", "", 1234, time.Now(), time.Now(), "testuser1", "", "",
		"none", 0, jobId)
	if err != nil {
		t.Fatalf("While trying to use prepared statement with preparedStatements.FilesInsertStmt.Exec() for checking "+
			"if '%s' has been previously backed up, the following error was encountered: %s", path, err)
	}

	// test Prepared Update
	_, err = preparedStatements.FilesUpdateStmt.Exec("file", "", 1234, time.Now(), time.Now(), "testuser2", "", "",
		"none", 0, jobId, path)
	if err != nil {
		t.Fatalf("While trying to use prepared statement with preparedStatements.FilesUpdateStmt.Exec() for checking "+
			"if '%s' has been previously backed up, the following error was encountered: %s", path, err)
	}

	ClosePreparedStatements(preparedStatements)
}

func TestClosePreparedStatements1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	defer func() {
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}
	numDbClients += 1

	preparedStatements, err := Prepare(db)
	if err != nil {
		t.Fatalf("Prepare() wasn't supposed to return an error but did return: '%s'", err)
	}

	ClosePreparedStatements(preparedStatements)
}

// test with empty shared.DbData struct
func TestCloseStatementsAndDb1(t *testing.T) {
	dbData := shared.DbData{}
	backupJobsState := shared.NewJobsState()
	CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
}

// test with populated shared.DbData struct but without populated shared.DbData.PreparedStatements
func TestCloseStatementsAndDb2(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()

	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}
	// numDbClients += 1  <--- not needed as we use the below CloseStatementsAndDisconnectFromDb()

	dbData := shared.DbData{
		Connected: true,
		Db:        db,
		Name:      backupName,
	}
	CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
}

// test with populated shared.DbData struct and with populated shared.DbData.PreparedStatements
func TestCloseStatementsAndDb3(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}

	preparedStatements, err := Prepare(db)
	if err != nil {
		numDbClients += 1 // needed as below CloseStatementsAndDisconnectFromDb() doesn't get called any more
		t.Fatalf("Prepare() wasn't supposed to return an error but did return: '%s'", err)
	}

	dbData := shared.DbData{
		Connected:          true,
		Db:                 db,
		Name:               backupName,
		PreparedStatements: preparedStatements,
	}
	CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
}

// should succeed
func TestAddJobDetails1_and_CheckJobUuidExists1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobid := u.String()
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}
	numDbClients += 1

	err = AddJobDetails(db, jobid, "first_backup", "backup", time.Now())
	if err != nil {
		t.Fatalf("AddJobDetails() wasn't supposed to return an error but did return: '%s'", err)
	}

	// check data actually made it to the DB
	rows, err := db.Query("SELECT id FROM jobs WHERE id = ?", jobid)
	if err != nil {
		t.Fatalf("While trying to get from the database any job id with uuid '%s', the following error was "+
			"encountered: '%s'", jobid, err)
	}

	foundRecord := false
	var jobIdInDb string
	for rows.Next() {
		err := rows.Scan(&jobIdInDb)
		if err != nil {
			logger.Errorf("While enumerating from the database the list of jobs with a given uuid, the "+
				"following error was encountered: '%s'", err)
		}
		// any result row means we had a match
		foundRecord = true
	}
	err = rows.Err()
	if err != nil {
		t.Fatalf("Could not enumerate the list of all targets from the database due to the following "+
			"error: '%s'", err)
	}
	_ = rows.Close() // #nosec

	if !foundRecord {
		t.Fatalf("Did not find in the DB a match for the job details which just got added")
	}

	// test above also using CheckJobUuidExists()

	foundRecordUsingFunction, err := CheckJobUuidExists(db, jobid)
	if err != nil {
		t.Fatalf("CheckJobUuidExists() wasn't supposed to return an error but did return: '%s'", err)
	}

	if !foundRecordUsingFunction {
		t.Fatalf("CheckJobUuidExists() did not find in the DB a match for the job details which just got added")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// search for record which doesn't exist
	foundRecordUsingFunction, err = CheckJobUuidExists(db, u.String())
	if err != nil {
		t.Fatalf("CheckJobUuidExists() wasn't supposed to return an error but did return: '%s'", err)
	}

	if foundRecordUsingFunction {
		t.Fatalf("CheckJobUuidExists() found a record in the DB but it should have not")
	}
}

// should return false as we're using an empty db
func TestCheckJobUuidExists1(t *testing.T) {
	dbDataDirPath := utils.SetupTmpDir("unittest_database_GetDbFilePath_", t)
	backupName := "backup1"
	backupJobsState := shared.NewJobsState()
	numDbClients := 0
	defer func() {
		for i := 0; i < numDbClients; i++ {
			database.DisconnectFromDb(backupName, backupJobsState, nil)
		}
		database.CloseDb(backupName, backupJobsState, true)
		err := os.RemoveAll(dbDataDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	db, err := database.Start(dbDataDirPath, backupName, backupJobsState)
	if err != nil {
		t.Fatalf("database.Start() wasn't supposed to return an error but did return: '%s'", err)
	}
	numDbClients += 1

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// search for record which doesn't exist
	foundRecordUsingFunction, err := CheckJobUuidExists(db, u.String())
	if err != nil {
		t.Fatalf("2. CheckJobUuidExists() wasn't supposed to return an error but did return: '%s'", err)
	}

	if foundRecordUsingFunction {
		t.Fatalf("CheckJobUuidExists() found a record in the DB but it should have not as we're using an empty DB")
	}

}
