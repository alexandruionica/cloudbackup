package daemon

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"context"
	"github.com/gofrs/uuid"
	"sync"
	"testing"
)

// test struct.MarkRunning() struct.MarkStopping() struct.IsRunning() and struct.IsStopping()
func TestMarkRunningStoppingAndIsRunningIsStopping(t *testing.T) {
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob1"
	logContext := "TestMarkRunningStoppingAndIsRunningIsStopping"

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}
	if backupJobsState.Running[0].Name != jobName {
		t.Fatalf("Expected job having name '%s' to be marked as running but instead '%s' is reported as "+
			"running", jobName, backupJobsState.Running[0].Name)
	}

	if backupJobsState.Running[0].State != "running" {
		t.Fatalf("Expected job having name '%s' to be marked as running but instead is reported as having "+
			"state '%s'", jobName, backupJobsState.Running[0].State)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if !isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	isRunning = backupJobsState.IsRunning(jobName, "", logContext)
	if !isRunning {
		t.Fatal("Was expecting that isRunning() with blank jobid reports true for the job but instead we " +
			"got 'false'")
	}

	isRunning = backupJobsState.IsRunning("jobWhichDoesNotExist", JobUuid, logContext)
	if isRunning {
		t.Fatal("Was expecting that isRunning() reports false for inexisting job but instead we got 'true'")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}

	isRunning = backupJobsState.IsRunning(jobName, u.String(), logContext)
	if isRunning {
		t.Fatal("Was expecting that isRunning() with incorrect jobid reports false for the job but instead we " +
			"got 'true'")
	}

	isStopping := backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for the job but instead we got 'true'")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// mark "stopping" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, u.String(), false)
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
		t.Fatalf("backupJobsState.MarkStopped(stopped:=false) should have succeeded when using correct "+
			"job name and job id but we got error: '%s'", err)
	}
	isStopping = backupJobsState.IsStopping(jobName, JobUuid, logContext)
	if !isStopping {
		t.Fatal("Was expecting that isStopping() reports true for the job but instead we got false")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	isStopping = backupJobsState.IsStopping(jobName, u.String(), logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for the job when using incorrect job it but" +
			" instead we got true")
	}

	isStopping = backupJobsState.IsStopping("an_incorrect_name2", "", logContext)
	if isStopping {
		t.Fatal("Was expecting that isStopping() reports false for an incorrect job name but instead we got true")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// mark "stopped" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, u.String(), true)
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
		t.Fatalf("backupJobsState.MarkStopped(stopped:=true) should have succeeded when using correct "+
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
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob2"
	logContext := "TestMarkRunningStoppingAndIsRunningIsStopping2"

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if !isRunning {
		t.Fatal("Was expecting that isRunning() reports true for the job but instead we got 'false'")
	}

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	// mark "stopped" should fail when using incorrect job id
	err = backupJobsState.MarkStopped(jobName, logContext, u.String(), true)
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
		t.Fatalf("backupJobsState.MarkStopped(stopped:=true) should have succeeded when using correct "+
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

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	serverConfigCopy := configuration.GetCopyWithLock(logContext)

	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := serverConfigCopy.Backup[0].Name
	counterName := "examined_files"

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if !isRunning {
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
	if !found {
		t.Fatalf("Could not find a job with name '%s' in the output of backupJobsState.Get()", jobName)
	}
}

// test struct.UpdateStatsText() and also struct.Get()
func TestUpdateStatsTextAndGet(t *testing.T) {
	logContext := "TestIncrementCounter"
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_structs_scheduler_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	serverConfigCopy := configuration.GetCopyWithLock(logContext)

	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := serverConfigCopy.Backup[0].Name
	statName := "current_file"
	statValue := "dsf9023kjldsfji2894234"

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	err = backupJobsState.MarkRunning(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}

	isRunning := backupJobsState.IsRunning(jobName, JobUuid, logContext)
	if !isRunning {
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
	if !found {
		t.Fatalf("Could not find a job with name '%s' in the output of backupJobsState.Get()", jobName)
	}
}

// try to add consumer to a multiplexer which is either not yet running or shutting down
func TestAddConsumer1(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background()) //nolint:gosec // cancelMultiplexer is invoked later in the test
	multiplexer := &shared.WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ClientIdentifier := "192.168.0.43:3423"
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	ClientUuid := u.String()

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	err = multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		if err.Error() != shared.MultiplexerNotReady {
			t.Fatalf("multiplexer.AddConsumer() was expected to return error: %s   but it returned error:"+
				" %s", shared.MultiplexerNotReady, err)
		}
	} else {
		t.Fatalf("multiplexer.AddConsumer() was expected to return an error but it didn't")
	}

	multiplexer.Running = true
	err = multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}
}

// add consumer to running multiplexer and then remove it
func TestAddConsumerAndRemoveConsumer(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background()) //nolint:gosec // cancelMultiplexer is invoked later in the test
	multiplexer := &shared.WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ClientIdentifier := "192.168.0.43:3423"
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	ClientUuid := u.String()

	u, err = uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	JobUuid := u.String()
	multiplexer.Running = true
	err = multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	if len(multiplexer.Consumers) != 1 {
		t.Fatalf("1. Was expecting to have 1 watch consumer but insted found: %d", len(multiplexer.Consumers))
	}

	if multiplexer.Consumers[0].Uuid != ClientUuid {
		t.Fatalf("Was expecting the only watch consumer to have uuid %s but found %s", ClientUuid,
			multiplexer.Consumers[0].Uuid)
	}

	multiplexer.RemoveConsumer("abcd", "invaliduuid")
	// function doesn't return an error so we need to be sneaky
	if len(multiplexer.Consumers) != 1 {
		t.Fatalf("2. Was expecting to have 1 watch consumer but insted found: %d", len(multiplexer.Consumers))
	}

	multiplexer.RemoveConsumer(ClientIdentifier, ClientUuid)
	// function doesn't return an error so we need to be sneaky
	if len(multiplexer.Consumers) != 0 {
		t.Fatalf("Was expecting to have 0 watch consumers but insted found: %d", len(multiplexer.Consumers))
	}
}
