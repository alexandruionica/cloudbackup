package scheduler

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"github.com/satori/go.uuid"
	"sync"
	"testing"
)

//normal function usage, when no error conditions exist (empty backupJobsState.Running slice)
func TestGenerateJobUuid1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_scheduler_GenerateJobUuid_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	backupJobsState := NewJobsState()
	serverConfigCopy := result.GetCopyWithLock(loggingContext + ".TestGenerateJobUuid1")

	startJobUuid, err := GenerateJobUuid(result.Config.Backup[0].Name, backupJobsState, serverConfigCopy, "backup")
	if err != nil {
		t.Fatalf("GenerateJobUuid() returned error: %s", err)
	}
	if startJobUuid == "" {
		t.Fatal("GenerateJobUuid() returned an empty string instead of an UUID despite no error being reported." +
			" Normally GenerateJobUuid() returns the first parameter as empty string only if an error is encountered")
	}
}

//normal function usage, when no error conditions exist (populated backupJobsState.Running slice)
func TestGenerateJobUuid2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_scheduler_GenerateJobUuid_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	backupJobsState := NewJobsState()
	serverConfigCopy := result.GetCopyWithLock(loggingContext + ".TestGenerateJobUuid1")

	// mark second job as running so we have backupJobsState.Running[] populated
	otherJobUuid := uuid.NewV4().String()
	err = backupJobsState.MarkRunning(result.Config.Backup[1].Name, "TestGenerateJobUuid2", otherJobUuid)
	if err != nil {
		t.Fatal(err)
	}

	startJobUuid, err := GenerateJobUuid(result.Config.Backup[0].Name, backupJobsState, serverConfigCopy, "backup")
	if err != nil {
		t.Fatalf("GenerateJobUuid() returned error: %s", err)
	}
	if startJobUuid == "" {
		t.Fatal("GenerateJobUuid() returned an empty string instead of an UUID despite no error being reported." +
			" Normally GenerateJobUuid() returns the first parameter as empty string only if an error is encountered")
	}
}

// pass in unknown job type, should error
func TestGenerateJobUuid3(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_scheduler_GenerateJobUuid_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	backupJobsState := NewJobsState()
	serverConfigCopy := result.GetCopyWithLock(loggingContext + ".TestGenerateJobUuid1")

	startJobUuid, err := GenerateJobUuid(result.Config.Backup[0].Name, backupJobsState, serverConfigCopy,
		"aWeirdJobType")
	if err == nil {
		t.Fatalf("GenerateJobUuid() did not return an error despite '%s' being expected", shared.ErrUnknownJobType)
	}
	if err.Error() != shared.ErrUnknownJobType {
		t.Fatalf("GenerateJobUuid() did not return an error '%s' but instead it returned: '%s'",
			shared.ErrUnknownJobType, err)
	}
	if startJobUuid != "" {
		t.Fatalf("GenerateJobUuid() should have returned an empty string instead of an UUID because an error was "+
			"reported but instead it returned: '%s'. Normally GenerateJobUuid() returns the first parameter as empty"+
			" string if an error is encountered", startJobUuid)
	}
}
