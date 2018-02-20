package utils

import (
	"cloudbackup/testutils"
	"testing"
	"os"
	"path/filepath"
)

// plain file exists
func TestFileExists1(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err := FileExists(path, true)
	if err != nil {
		t.Fatal(err)
	}
}

// plain file which doesn't exist
func TestFileExists2(t *testing.T) {
	var path = "a/file/which/does/not/exist"
	_, err := FileExists(path, true)
	if err == nil {
		t.Fatalf("File %s should not exist but it is reported to exist", path)
	}
}

// plain file which doesn't exist - this time don't derefence
func TestFileExists3(t *testing.T) {
	var path = "a/file/which/does/not/exist"
	_, err := FileExists(path, false)
	if err == nil {
		t.Fatalf("File %s should not exist but it is reported to exist", path)
	}
}

// plain file exists - this time don't derefence
func TestFileExists4(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err := FileExists(path, false)
	if err != nil {
		t.Fatal(err)
	}
}

// directory instead of file
func TestFileExists5(t *testing.T) {
	var path = testutils.SetupTmpDir("unittest_utils_test_", t)
	defer func() {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err := FileExists(path, true)
	if err == nil {
		t.Fatalf("Path %s should be a folder but it is reported to exist as a regular file", path)
	}
}

// symlink to plain file which exists - do dereference
func TestFileExists6(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err := os.Symlink(path, path + "_symlink")
	defer func() {
		err := os.Remove(path + "_symlink")
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path + "_symlink", true)
	if err != nil {
		t.Fatal(err)
	}
}

// symlink to plain file which exists - do NOT dereference
func TestFileExists7(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err := os.Symlink(path, path + "_symlink")
	defer func() {
		err := os.Remove(path + "_symlink")
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path + "_symlink", false)
	if err != nil {
		t.Fatal(err)
	}
}

// symlink to plain file which DOES NOT exists - do NOT dereference - should not cause error
func TestFileExists8(t *testing.T) {

	var path = filepath.Join(os.TempDir(), "unittest_utils_test_broken_symlink")
	err := os.Symlink("a_file_which_should_not_exist", path)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path, false)
	if err != nil {
		t.Fatal(err)
	}
}

// symlink to plain file which DOES NOT exists - do dereference - should cause error
func TestFileExists9(t *testing.T) {

	var path = filepath.Join(os.TempDir(), "unittest_utils_test_broken_symlink")
	err := os.Symlink("a_file_which_should_not_exist", path)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path, true)
	if err == nil {
		t.Fatal("Should have errored out because symlink should be pointing to inexistent file")
	}
}