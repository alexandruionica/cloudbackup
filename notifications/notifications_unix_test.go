// +build darwin freebsd netbsd openbsd solaris linux

package notifications

import (
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"github.com/satori/go.uuid"
	"os"
	"testing"
)

const testScript = `#!/bin/bash
# arguments are supposed to be JobType, JobName, JobId, JobState, JobError, reportFile
if [[ $# -ne 6 ]]; then
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
if [[ FOUND -ne 1 ]]; then
    echo "First argument must be one of  backup | restore | purge | test  but it was: $1"
    exit 2
fi

if [[ ! -f $6 ]]; then
    echo "The sixth argument is supposed to be a regular file but in this case it isn't. The argument is: $6"
    exit 3
fi`

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
	jobId := uuid.NewV4().String()
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err == nil {
		t.Fatal("Running the notification script did not return an error despite the script not being executable")
	}
}

func TestRunScript2(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	jobId := uuid.NewV4().String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}
	// leave some fields unpopulated
	err = runScript(scriptEntry, jobId, "backup", "finished", "a_test_job", "", "")
	if err != nil {
		t.Fatalf("Running the notification script returned error: %s", err)
	}
}

func TestRunScript3(t *testing.T) {
	scriptPath, err := utils.SetupTmpFileWithContent([]byte(testScript), "unittest_notifications_")
	if err != nil {
		t.Fatalf("Could not setup tmp shell script for testing due to error: %s", err)
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})
	scriptEntry := config.NotificationScript{
		Path: scriptPath,
		Type: []string{"finished"},
	}
	jobId := uuid.NewV4().String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
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
	jobId := uuid.NewV4().String()
	err = os.Chmod(scriptPath, 0700)
	if err != nil {
		t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
	}

	//  test script should fail because the JobId value is not one of "backup", "restore", "purge"
	err = runScript(scriptEntry, jobId, "somethingElse", "finished", "a_test_job", "some report", "bla bla asdasd")
	if err == nil {
		t.Fatal("Script should have failed but it didn't")
	}
}