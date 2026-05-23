package cliargs

import (
	"cloudbackup/testutils"
	"os"
	"os/exec"
	"testing"
)

// ArgsCommandServerResetKeystore.Execute calls os.Exit so we drive the test through
// a subprocess re-exec, matching the pattern used by TestExecute*OfArgsCommandConfigCommandValidate.

// Sanity check the happy path: a valid config that lists the requested backup job and
// the -y flag set so the interactive confirmation prompt is skipped. The DB file does
// not exist yet — database.Start will create it on demand.
func TestExecuteResetKeystore_ValidConfigValidJob(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cliargs_resetkeystore_ok_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	testobj := &ArgsCommandServerResetKeystore{
		ConfigFile: path,
		Yes:        true,
	}
	testobj.Job.Name = "first_backup"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteResetKeystore_ValidConfigValidJob") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("process ran with err %v, want exit status 0", err)
	}
}

// A non-existent config file should cause the command to fail.
func TestExecuteResetKeystore_MissingConfig(t *testing.T) {
	testobj := &ArgsCommandServerResetKeystore{
		ConfigFile: "/no/such/path/exists/here.yaml",
		Yes:        true,
	}
	testobj.Job.Name = "first_backup"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteResetKeystore_MissingConfig") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("process ran without error, want non-zero exit status")
	}
}

// Job name that does not exist in the config should cause the command to fail before
// the DB is touched.
func TestExecuteResetKeystore_UnknownJob(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cliargs_resetkeystore_unknown_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	testobj := &ArgsCommandServerResetKeystore{
		ConfigFile: path,
		Yes:        true,
	}
	testobj.Job.Name = "no_such_backup_job"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteResetKeystore_UnknownJob") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("process ran without error, want non-zero exit status")
	}
}

// Interactive confirmation: when -y is not set and stdin doesn't echo the job name back,
// the command should abort with non-zero status.
func TestExecuteResetKeystore_ConfirmationMismatchAborts(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cliargs_resetkeystore_confirm_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	testobj := &ArgsCommandServerResetKeystore{
		ConfigFile: path,
		Yes:        false, // forces the interactive prompt
	}
	testobj.Job.Name = "first_backup"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteResetKeystore_ConfirmationMismatchAborts") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	// Provide stdin with the wrong confirmation text — fmt.Scanln will read this and the command should refuse.
	cmd.Stdin = stringReader("not_the_job_name\n")
	if err := cmd.Run(); err == nil {
		t.Fatal("process ran without error, want non-zero exit status because confirmation did not match")
	}
}

// stringReader is a tiny io.Reader over a string, used to drive the interactive
// confirmation prompt in TestExecuteResetKeystore_ConfirmationMismatchAborts.
type stringReader string

func (s stringReader) Read(p []byte) (int, error) {
	n := copy(p, s)
	if n == 0 {
		return 0, os.ErrClosed
	}
	return n, nil
}
