package utils

import (
	"testing"
	"os"
	"path/filepath"
)

// plain file exists
func TestFileExists1(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err = FileExists(path, true)
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
	if err != ErrNoSuchFile {
		t.Fatalf("Expected error:\"%s\" but got:\"%s\"", ErrNoSuchFile, err)
	}
}

// plain file exists - this time don't derefence
func TestFileExists4(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
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

// directory instead of file
func TestFileExists5(t *testing.T) {
	var path = SetupTmpDir("unittest_utils_test_", t)
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
	if err != ErrNotRegularFile {
		t.Fatalf("Expected error:\"%s\" but got:\"%s\"", ErrNoSuchFile, err)
	}
}

// symlink to plain file which exists - do dereference
func TestFileExists6(t *testing.T) {
	//if runtime.GOOS == "windows" {
	//	// Skipping test on Windows as "symlinks" don't exist there
	//	return
	//}
	path, err := SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err = os.Symlink(path, path + "_symlink")
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
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

// symlink to plain file which exists - do NOT dereference - should not error
func TestFileExists7(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err = os.Symlink(path, path + "_symlink")
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
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
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
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
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
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
	if err != ErrNoSuchFile {
		t.Fatalf("Expected error:\"%s\" but got:\"%s\"", ErrNoSuchFile, err)
	}
}

// should find string in slice
func TestStringInSlice1(t *testing.T) {
	source := make([]string, 0)
	source = append(source, "abc", "def")
	if StringInSlice("abc", source) == false {
		t.Fatal("Failed to find expected string in slice")
	}
}

// should not find string in slice
func TestStringInSlice2(t *testing.T) {
	source := make([]string, 0)
	source = append(source, "abc", "def")
	if StringInSlice("zyx", source) {
		t.Fatal("Found string in slice but it should have failed as searched string doesn't exist in slice")
	}
}


// path is directory which exists - do dereference
func TestDirExists1(t *testing.T) {
	var path = SetupTmpDir("unittest_utils_test_", t)
	defer func() {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err := DirExists(path, true)
	if err != nil {
		t.Fatalf("Path %s should be a folder but error: '%s' was reported", path, err.Error())
	}
}

// path is a file which exists (instead of directory) - do dereference
func TestDirExists2(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err = DirExists(path, true)
	if err == nil {
		t.Fatalf("No error was reported despite one being expected")
	}
	if err != ErrNotADir {
		t.Fatalf("Expected error '%s' but got error '%s'", ErrNotADir.Error(), err.Error())
	}
}

//absolute path does not exist - do dereference
func TestDirExists3(t *testing.T) {
	var path = "/an/inexisting/file"
	_, err := DirExists(path, true)
	if err == nil {
		t.Fatalf("Path %s does not exist and should have raised an error but didn't", path)
	}
	if err != ErrNoSuchDir {
		t.Fatalf("Expected error '%s' but got error '%s'", ErrNoSuchDir.Error(), err.Error())
	}
}

// relative path does not exist - do dereference
func TestDirExists4(t *testing.T) {
	var path = "_an_inexisting_file"
	_, err := DirExists(path, true)
	if err == nil {
		t.Fatalf("Path %s does not exist and should have raised an error but didn't", path)
	}
	if err != ErrNoSuchRelativeDir {
		t.Fatalf("Expected error '%s' but got error '%s'", ErrNoSuchRelativeDir.Error(), err.Error())
	}
}

// path is symlink to a directory which exists - do NOT dereference
func TestDirExists5(t *testing.T) {
	var path = SetupTmpDir("unittest_utils_test_", t)
	defer func() {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	var symLinkPath = filepath.Join(os.TempDir(), "unittest_utils_test_symlink_to_dir5")
	err := os.Symlink(path, symLinkPath)
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(symLinkPath)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = DirExists(symLinkPath, false)
	if err == nil {
		t.Fatal("Path should be a symlink and generate an error but none was generated")
	}
}

// path is symlink to a directory which exists - do dereference
func TestDirExists6(t *testing.T) {
	var path = SetupTmpDir("unittest_utils_test_", t)
	defer func() {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	var symLinkPath = filepath.Join(os.TempDir(), "unittest_utils_test_symlink_to_dir6")
	err := os.Symlink(path, symLinkPath)
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(symLinkPath)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = DirExists(symLinkPath, true)
	if err != nil {
		t.Fatalf("Path %s should be a folder but error: '%s' was reported", path, err.Error())
	}
}

// path is symlink to a directory which DOESN'T exist - do dereference
func TestDirExists7(t *testing.T) {
	var symLinkPath = filepath.Join(os.TempDir(), "unittest_utils_test_symlink_to_dir7")
	err := os.Symlink("a_folder_which_should_not_exist", symLinkPath)
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(symLinkPath)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = DirExists(symLinkPath, true)
	if err == nil {
		t.Fatalf("Path %s does not exist and should have raised an error but didn't", symLinkPath)
	}
	if err != ErrNoSuchDir {
		t.Fatalf("Expected error '%s' but got error '%s'", ErrNoSuchDir.Error(), err.Error())
	}
}