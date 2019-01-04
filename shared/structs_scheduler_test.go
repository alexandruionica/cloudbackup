package shared

import (
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"github.com/satori/go.uuid"
	"sync"
	"testing"
)

// test struct.MarkRunning() struct.MarkStopping() struct.IsRunning() and struct.IsStopping()
func TestMarkRunningStoppingAndIsRunningIsStopping(t *testing.T) {
	backupJobsState := &BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob1"
	logContext := "TestMarkRunningStoppingAndIsRunningIsStopping"

	JobUuid := uuid.NewV4().String()
	err := backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}
	if backupJobsState.Running[0].Name != jobName {
		t.Fatalf("Expected job having name '%s' to be marked as running but instead '%s' is reported as " +
			"running", jobName, backupJobsState.Running[0].Name)
	}

	if backupJobsState.Running[0].State != "running" {
		t.Fatalf("Expected job having name '%s' to be marked as running but instead is reported as having " +
			"state '%s'", jobName, backupJobsState.Running[0].State)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if ! isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	isRunning = backupJobsState.IsRunning(jobName, "", logContext)
	if ! isRunning {
		t.Fatal("Was expecting that isRunning() with blank jobid reports true for the job but instead we " +
			"got 'false'")
	}

	isRunning = backupJobsState.IsRunning("jobWhichDoesNotExist", JobUuid, logContext)
	if isRunning {
		t.Fatal("Was expecting that isRunning() reports false for inexisting job but instead we got 'true'")
	}

	isRunning = backupJobsState.IsRunning(jobName, uuid.NewV4().String(), logContext)
	if isRunning {
		t.Fatal("Was expecting that isRunning() with incorrect jobid reports false for the job but instead we " +
			"got 'true'")
	}

	isStopping := backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for the job but instead we got 'true'")
	}

	// mark "stopping" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, uuid.NewV4().String(), false)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=false) should have failed due to incorrect JobId being " +
			"passed")
	}

	// mark "stopping" should fail when using incorrect job name
	err = backupJobsState.MarkStopped("an_incorrect_name", logContext, JobUuid, false)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=false) should have failed due to incorrect Job Name being " +
			"passed")
	}

	// mark "stopping" should succeed when using correct job name and correct Job ID
	err = backupJobsState.MarkStopped(jobName, logContext, JobUuid, false)
	if err != nil {
		t.Fatalf("backupJobsState.MarkStopped(stopped:=false) should have succeeded when using correct " +
			"job name and job id but we got error: '%s'", err)
	}
	isStopping = backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if ! isStopping {
		t.Fatal("Was expecting that isStopping() reports true for the job but instead we got false")
	}

	isStopping = backupJobsState.IsStopping(jobName, uuid.NewV4().String(), logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for the job when using incorrect job it but" +
			" instead we got true")
	}

	isStopping = backupJobsState.IsStopping("an_incorrect_name2", "", logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for an incorrect job name but instead we got true")
	}

	// mark "stopped" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, uuid.NewV4().String(), true)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=true) should have failed due to incorrect JobId being " +
			"passed")
	}

	// mark "stopped" should fail when using incorrect job name
	err = backupJobsState.MarkStopped("an_incorrect_name", logContext, JobUuid, true)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=true) should have failed due to incorrect Job Name being " +
			"passed")
	}

	// mark "stopped" should succeed when using correct job name and correct Job ID
	err = backupJobsState.MarkStopped(jobName, logContext, JobUuid, true)
	if err != nil {
		t.Fatalf("backupJobsState.MarkStopped(stopped:=true) should have succeeded when using correct " +
			"job name and job id but we got error: '%s'", err)
	}
	// given that job is now stopped isStopping() should return false
	isStopping = backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for a stopped job but instead we got 'true'")
	}
}

// this one tests that MarkStopped(stopped=true) works without first calling MarkStopped(stopped=false)
func TestMarkRunningStoppingAndIsRunningIsStopping2(t *testing.T) {
	backupJobsState := &BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob2"
	logContext := "TestMarkRunningStoppingAndIsRunningIsStopping2"

	JobUuid := uuid.NewV4().String()
	err := backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if ! isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	// mark "stopped" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, uuid.NewV4().String(), true)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=true) should have failed due to incorrect JobId being " +
			"passed")
	}

	// mark "stopped" should fail when using incorrect job name
	err = backupJobsState.MarkStopped("an_incorrect_name", logContext, JobUuid, true)
	if err == nil {
		t.Fatal("backupJobsState.MarkStopped(stopped:=true) should have failed due to incorrect Job Name being " +
			"passed")
	}

	// mark "stopped" should succeed when using correct job name and correct Job ID
	err = backupJobsState.MarkStopped(jobName, logContext, JobUuid, true)
	if err != nil {
		t.Fatalf("backupJobsState.MarkStopped(stopped:=true) should have succeeded when using correct " +
			"job name and job id but we got error: '%s'", err)
	}
	// given that job is now stopped isStopping() should return false
	isStopping := backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for a stopped job but instead we got 'true'")
	}
}

// test struct.IncrementCounter() and also struct.Get()
func TestIncrementCounterAndGet(t *testing.T) {
	logContext := "TestIncrementCounter"
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_structs_scheduler_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	serverConfigCopy := configuration.GetCopyWithLock(logContext)

	backupJobsState := &BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := serverConfigCopy.Backup[0].Name
	counterName := "examined_files"

	JobUuid := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if ! isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	backupJobsState.IncrementCounter(jobName, counterName, "/another/path", "file", "examine", "")
	AllJobsStatus := backupJobsState.Get(serverConfigCopy, logContext)

	found := false
	for _, jobStatus := range AllJobsStatus {
		if jobStatus.Name == jobName {
			found = true
			if jobStatus.StatsCounters[counterName] != 1 {
				t.Fatalf("%s counter was expected to have value 1 but instead it has: %d", counterName,
					jobStatus.StatsCounters[counterName])
			}
		}
	}
	if ! found {
		t.Fatalf("Could not find a job with name '%s' in the output of backupJobsState.Get()", jobName)
	}
}

// test struct.UpdateStatsText() and also struct.Get()
func TestUpdateStatsTextAndGet(t *testing.T) {
	logContext := "TestIncrementCounter"
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_structs_scheduler_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration , err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	serverConfigCopy := configuration.GetCopyWithLock(logContext)

	backupJobsState := &BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := serverConfigCopy.Backup[0].Name
	statName := "current_file"
	statValue := "dsf9023kjldsfji2894234"

	JobUuid := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if ! isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	backupJobsState.UpdateStatsText(jobName, statName, statValue, "", "")
	AllJobsStatus := backupJobsState.Get(serverConfigCopy, logContext)

	utils.Pp(AllJobsStatus)
	found := false
	for _, jobStatus := range AllJobsStatus {
		if jobStatus.Name == jobName {
			found = true
			if jobStatus.StatsText[statName] != statValue {
				t.Fatalf("%s stat was expected to have value %s but instead it has: %s", statName, statValue,
					jobStatus.StatsText[statName])
			}
		}
	}
	if ! found {
		t.Fatalf("Could not find a job with name '%s' in the output of backupJobsState.Get()", jobName)
	}
}