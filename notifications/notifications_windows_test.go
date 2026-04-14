//go:build windows
// +build windows

package notifications

import (
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"fmt"
	"github.com/gofrs/uuid"
	"os"
	"strings"
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
@echo on
`

const testScriptPowershell = `if ( $args.Count -ne 6 ) {
    Write-Host "You passed $($args.Count) arguments but only 6 were expected. Arguments passed(one per line):"
    $args | Write-Host
    exit 1
}
`

func TestRunScript1(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := shared.ConfigNotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err == nil {
		t.Fatal("Running the notification script did not return an error despite the script not having the '.bat' file extension")
	}
}

func TestRunScript2(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath(t, "unittest_notifications_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo %%3 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := shared.ConfigNotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// ensure the file extension is .bat or otherwise execution will fail on windows
	err = os.Rename(scriptPath, scriptPath+".bat")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.bat'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".bat"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	// leave some fields unpopulated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}

	_, err = os.Stat(resultsFile)
	if err != nil {
		t.Fatalf("Results file '%s' does not exist. Test shell script did not execute as expected", resultsFile)
	}
	result, err := os.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("Could not read contents of results file '%s'", resultsFile)
	}
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}

func TestRunScript3(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath(t, "unittest_notifications_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo %%3 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := shared.ConfigNotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// ensure the file extension is .bat or otherwise execution will fail on windows
	err = os.Rename(scriptPath, scriptPath+".bat")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.bat'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".bat"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	//  make sure all fields are populated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}

	_, err = os.Stat(resultsFile)
	if err != nil {
		t.Fatalf("Results file '%s' does not exist. Test shell script did not execute as expected", resultsFile)
	}
	result, err := os.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("Could not read contents of results file '%s'", resultsFile)
	}
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}

func TestRunScript4(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := shared.ConfigNotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// ensure the file extension is .bat or otherwise execution will fail on windows
	err = os.Rename(scriptPath, scriptPath+".bat")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.bat'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".bat"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	//  test script should fail because the JobId value is not one of "backup", "restore", "purge"
	err = runScript(scriptEntry, jobId, "somethingElse", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err == nil {
		t.Fatal("Script should have failed but it didn't")
	}
}

func TestRunScript5(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath(t, "unittest_notifications_powershell_script_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScriptPowershell + fmt.Sprintf("$args[2] | Out-File -Encoding ASCII -FilePath %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := shared.ConfigNotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()

	// ensure the file extension is .bat or otherwise execution will fail on windows
	err = os.Rename(scriptPath, scriptPath+".ps1")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.ps1'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".ps1"
	scriptEntry.Path = scriptPath
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	// leave some fields unpopulated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}

	_, err = os.Stat(resultsFile)
	if err != nil {
		t.Fatalf("Results file '%s' does not exist. Test shell script did not execute as expected", resultsFile)
	}
	result, err := os.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("Could not read contents of results file '%s'", resultsFile)
	}
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}
