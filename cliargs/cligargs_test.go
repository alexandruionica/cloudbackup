package cliargs

import (
	"cloudbackup/testutils"
	"os"
	"os/exec"
	"testing"
)

func TestExecute1OfArgsCommandConfigCommandValidate(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cliargs_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded - for some reason, this doesn't
	// work as expected only for this particular unittest; might have something to do with the below cmd.exec()
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	testobj := &ArgsCommandServerConfigValidate{
		Debug:      false,
		ConfigFile: path,
	}

	// weird way of testing where we launch a subprocess doing the actual test and check it's exit code
	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecute1OfArgsCommandConfigCommandValidate") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("process ran with err %v, want exit status 0", err)
	}

}

// test with missing config file
func TestExecute2OfArgsCommandConfigCommandValidate(t *testing.T) {
	testobj := &ArgsCommandServerConfigValidate{
		Debug:      true,
		ConfigFile: "a/file/which/does/not/exist",
	}

	// weird way of testing where we launch a subprocess doing the actual test and check it's exit code
	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExecute2OfArgsCommandConfigCommandValidate") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1")
	}
}
