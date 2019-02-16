// +build darwin freebsd netbsd openbsd solaris linux

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

const testScript = `#!/bin/sh
# arguments are supposed to be JobId
if [ $# -ne 1 ]; then
    echo "expected 1 arguments but got $#"
    echo -n "received arguments were: "
    for var in "$@"; do
        echo -n "\"$var\" "
    done
    echo ""
    exit 1
fi
`

// test PreRunScript having execute bit set (should work)
func TestRunPrePostScript1(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath("unittest_backup_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo $1 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	jobId := uuid.NewV4().String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}

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

// test PostRunScript having execute bit set (should work)
func TestRunPrePostScript2(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath("unittest_backup_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo $1 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	jobId := uuid.NewV4().String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}

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