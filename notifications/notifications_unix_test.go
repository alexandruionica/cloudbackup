// +build darwin freebsd netbsd openbsd solaris linux

package notifications

import (
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"fmt"
	"github.com/gofrs/uuid"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

const testScript = `#!/bin/sh
# arguments are supposed to be JobType, JobName, JobId, JobState, JobError, reportFile
if [ $# -ne 6 ]; then
    echo "expected 6 arguments but got $#"
    echo -n "received arguments were: "
    for var in "$@"; do
        echo -n "\"$var\" "
    done
    echo ""
    exit 1
fi
FOUND=0
case $1 in
    backup | restore | purge)
        FOUND=1
        ;;
esac
if [ $FOUND -ne 1 ]; then
    echo "First argument must be one of  backup | restore | purge  but it was: $1"
    exit 2
fi

if [ ! -f "$6" ]; then
    echo "The sixth argument is supposed to be a regular file but in this case it isn't. The argument is: $6"
    exit 3
fi
`

// test with script which has not the execute bit set (should fail)
func TestRunScript1(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
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
		t.Fatal("Running the notification script did not return an error despite the script not being executable")
	}
}

// test with script having execute bit set (should work) ; leave some fields unpopulated when calling the script
func TestRunScript2(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath(t, "unittest_notifications_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo $3 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}
	// leave some fields unpopulated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
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
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}

// test with script having execute bit set (should work) ; ensure all fields are populated when calling the script
func TestRunScript3(t *testing.T) {
	// adjust test shell script to create a file if it was successful
	resultsFile := testutils.GenerateTmpFilePath(t, "unittest_notifications_", "")
	defer testutils.DeleteTestFilesAndDirs([]string{resultsFile})
	testScript2 := testScript + fmt.Sprintf("echo $3 > %s", resultsFile)
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript2), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}

	//  make sure all fields are populated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "some report", "bla bla asdasd")
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
	if strings.TrimSpace(string(result)) != jobId {
		t.Fatalf("Was expecting to find in the results file '%s' uuid '%s' but instead found '%s'",
			resultsFile, jobId, strings.TrimSpace(string(result)))
	}
}

// test we properly pass the JobType parameter to the script (and that it fails when we send it garbage instead of a JobType)
func TestRunScript4(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	u, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("Could not generate UUID due to error: %s", err)
	}
	jobId := u.String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}

	//  test script should fail because the JobType value is not one of "backup", "restore", "purge"
	err = runScript(scriptEntry, jobId, "somethingElse", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err == nil {
		t.Fatal("Script should have failed but it didn't")
	}
}
