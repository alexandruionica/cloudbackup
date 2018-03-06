package cliargs

import (
	"testing"
	"cloudbackup/testutils"
	"os"
	"os/exec"
)


func TestExecute1OfArgsCommandConfigCommandValidate(t *testing.T){
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	testobj := &ArgsCommandConfigCommandValidate {
		Debug: false,
		ConfigFile: path,
	}

	// weird way of testing where we launch a subprocess doing the actual test and check it's exit code
	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecute1OfArgsCommandConfigCommandValidate")
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("process ran with err %v, want exit status 0", err)
	}


}

// test with missing config file
func TestExecute2OfArgsCommandConfigCommandValidate(t *testing.T){
	testobj := &ArgsCommandConfigCommandValidate {
		Debug: true,
		ConfigFile: "a/file/which/does/not/exist",
	}

	// weird way of testing where we launch a subprocess doing the actual test and check it's exit code
	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExecute2OfArgsCommandConfigCommandValidate")
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1")
	}
}
