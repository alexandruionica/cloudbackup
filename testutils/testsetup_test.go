package testutils

import (
	"bufio"
	"cloudbackup/utils"
	"os"
	"testing"
)

// test that we can create a fake file and that it's contents match what we wrote to it
func TestSetupFakeFile(t *testing.T) {
	var fileContent = "some sample text"
	path, err := utils.SetupTmpFileWithContent([]byte(fileContent), "unittest_testutils_test_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(file)
	result, err := reader.ReadString('\n')
	if result != fileContent {
		t.Fatal("Sample input for the fake file did not match what we actually read from the file")
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}
