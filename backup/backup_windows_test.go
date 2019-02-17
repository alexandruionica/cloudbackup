package backup

import (
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"fmt"
	"github.com/satori/go.uuid"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

const testScript = `@echo off
set found=0
set argC=0
for %%x in (%*) do Set /A argC+=1

IF /I "%argC%" NEQ "1" (
    echo expected 1 arguments but got %argC%
    echo #### Received arguments where - one per line:
    for %%x in (%*) do echo %%x
    echo #### End of received arguments
    exit 1
)
@echo on
`

const testScript2 = `if ( $args.Count -ne 1 ) {
    Write-Host "You passed $($args.Count) arguments but only 1 was expected. Arguments passed(one per line):"
    $args | Write-Host
    exit 1
}
`

// test PreRunScript with a Windows Batch file
func TestRunPrePostScript1(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath("unittest_backup_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo %%1 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	jobId := uuid.NewV4().String()
	// ensure the file extension is .bat or otherwise execution will fail on Windows
	err = os.Rename(scriptPath, scriptPath + ".bat")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.bat'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".bat"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	err = RunPrePostScript(scriptPath, "pre", "backup1", jobId)
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}

	_, err = os.Stat(resultsFile)
	if err != nil {
		t.Fatalf("Results file '%s' does not exist. Test shell script did not execute as expected", resultsFile)
	}
	result, err := ioutil.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("Could not read contents of results file '%s'", resultsFile)
	}
	// check that the file produced by the script has the expected jobid in its content
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}

// test PostRunScript with a Powershell script
func TestRunPrePostScript2(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath("unittest_backup_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript2 + fmt.Sprintf("$args[0] | Out-File -Encoding ASCII -FilePath %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	jobId := uuid.NewV4().String()
	// ensure the file extension is .ps1 or otherwise execution will fail on Windows
	err = os.Rename(scriptPath, scriptPath + ".ps1")
	if err != nil {
		t.Fatalf("Could not rename '%s' to '%s.ps1'", scriptPath, scriptPath)
	}
	scriptPath = scriptPath + ".ps1"
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	err = RunPrePostScript(scriptPath, "post", "backup1", jobId)
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}

	_, err = os.Stat(resultsFile)
	if err != nil {
		t.Fatalf("Results file '%s' does not exist. Test shell script did not execute as expected", resultsFile)
	}
	result, err := ioutil.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("Could not read contents of results file '%s'", resultsFile)
	}
	// check that the file produced by the script has the expected jobid in its content
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}