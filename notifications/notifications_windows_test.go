// +build windows

package notifications

import (
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"github.com/satori/go.uuid"
	"os"
	"testing"
)

const testScript = `@echo off
set found=0
set argC=0
for %%x in (%*) do Set /A argC+=1

IF /I "%argC%" NEQ "6" (
    echo expected 6 arguments but got %argC%
    echo #### Received arguments where - one per line:
    for %%x in (%*) do echo %%x
    echo #### End of received arguments
    exit 1
)

REM test input matches expectation
IF "%1" == "backup"  GOTO :cond
IF "%1" == "restore" GOTO :cond
IF "%1" == "purge"   GOTO :cond
GOTO :skip
:cond
set found=1
:skip

IF /I "%found%" NEQ "1" (
    echo First argument must be one of  backup, restore, purge  but it was: %1
    exit 2
)

IF NOT EXIST "%6" (
    echo The sixth argument is supposed to be a regular file but in this case it isn't. The argument is: %6
    exit 3
)

echo end
@echo on`

func TestRunScript1(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_scheduler_GenerateJobUuid_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
		}
	jobId := uuid.NewV4().String()
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err == nil {
		t.Fatal("Running the notification script did not return an error despite the script not having the '.bat' file extension")
	}

	// ensure the file extension is .bat or otherwise execution will fail on windows
	err = os.Rename(scriptPath, scriptPath + ".bat")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.bat'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".bat"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	// leave some fields unpopulated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err != nil {
		t.Fatalf("1. Running the notification script returned error: %s", err)
	}

	//  make sure all fields are populated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err != nil {
		t.Fatalf("2. Running the notification script returned error: %s", err)
	}

	//  test script should fail because the JobId value is not one of "backup", "restore", "purge"
	err = runScript(scriptEntry, jobId, "somethingElse", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err == nil {
		t.Fatal("Script should have failed but it didn't")
	}
}