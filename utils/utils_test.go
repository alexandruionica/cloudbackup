package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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

	err = os.Symlink(path, path+"_symlink")
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(path + "_symlink")
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path+"_symlink", true)
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

	err = os.Symlink(path, path+"_symlink")
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(path + "_symlink")
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(path+"_symlink", false)
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

// absolute path does not exist - do dereference
func TestDirExists3(t *testing.T) {
	var path string
	// absolute paths in Ms Windows start with the drive letter
	if runtime.GOOS == "windows" {
		path = "C:\\an\\inexisting\\dir"
	} else {
		path = "/an/inexisting/dir"
	}
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
	var path = "_an_inexisting_dir"
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

// plain file which exists
func TestGetFileMD5Sum1(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text goes here`), "unittest_utils_test_")
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
	expectedChecksum := "0711b8d9b25c7dee5f23575028a3c82f"
	checksum, err := GetFileMD5Sum(path)
	if err != nil {
		t.Fatalf("GetFileMD5Sum() returned an error despite none being expected: %s", err)
	}
	if checksum != expectedChecksum {
		t.Fatalf("GetFileMD5Sum() was supposed to return for %s the checksum '%s' but it returned '%s'",
			path, expectedChecksum, checksum)
	}
}

// targeted file does not exist
func TestGetFileMD5Sum2(t *testing.T) {
	var path = filepath.Join(os.TempDir(), "unittest_utils_test_broken_symlink")
	checksum, err := GetFileMD5Sum(path)
	if err == nil {
		t.Fatal("GetFileMD5Sum() did not return an error despite one being expected")
	}
	if checksum != "" {
		t.Fatalf("GetFileMD5Sum() was supposed to return for missing file %s the checksum '' (empty string) "+
			"but it returned '%s'", path, checksum)
	}
}

// plain file
func TestFileType1(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text goes here`), "unittest_utils_test_TestFileType1_")
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
	filestat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.stat() returned an error despite none being expected: %s", err)
	}
	filetype := FileType(filestat)
	if filetype != "file" {
		t.Fatalf("Was expecting FileType() to return value 'file' but it returned '%s'", filetype)
	}
}

// test with directory
func TestFileType2(t *testing.T) {
	var path = os.TempDir()
	stat, err := DirExists(path, true)
	if err != nil {
		t.Fatalf("While trying to get os.stat() of %s got error : %s", path, err)
	}

	filetype := FileType(stat)
	if filetype != "dir" {
		t.Fatalf("Was expecting FileType() to return value 'dir' but it returned '%s'", filetype)
	}
}

// test with symlink and check that both de-referencing and not de-referencing work as expected
func TestFileType3(t *testing.T) {
	path, err := SetupTmpFileWithContent([]byte(`some text goes here`), "unittest_utils_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	var symLinkPath = filepath.Join(os.TempDir(), "unittest_utils_test_symlink_to_dir7")
	err = os.Symlink(path, symLinkPath)
	if err != nil {
		t.Fatalf("Error setting up symlink: %s", err)
	}
	defer func() {
		err := os.Remove(symLinkPath)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = FileExists(symLinkPath, false)
	if err != nil {
		t.Fatal(err)
	}
	// do NOT dereference symlinks
	filestat, err := os.Lstat(symLinkPath)
	if err != nil {
		t.Fatalf("os.stat() returned an error despite none being expected: %s", err)
	}
	filetype := FileType(filestat)
	if filetype != "symlink" {
		t.Fatalf("Was expecting FileType() to return value 'symlink' (due to not de-referencing) but it returned '%s'", filetype)
	}

	// DO dereference symlinks
	filestat, err = os.Stat(symLinkPath)
	if err != nil {
		t.Fatalf("os.stat() returned an error despite none being expected: %s", err)
	}
	filetype = FileType(filestat)
	if filetype != "file" {
		t.Fatalf("Was expecting FileType() to return value 'file' (due to de-referencing) but it returned '%s'", filetype)
	}
}

// should not match exclusion
func TestIsExcluded1(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	//exclusions := []string{"**" + string(filepath.Separator) + "file{1,2}.txt"}
	exclusions := []string{"bla1234"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if excluded {
		t.Fatalf("Exclusion rule '%s' matched but it wasn't expected that it would match", exclusionRule)
	}
	if exclusionRule != "" {
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second "+
			"argument in reply) is empty but instead we got: '%s'", exclusionRule)
	}
}

// should match simple exclusion
func TestIsExcluded2(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	//exclusions := []string{"**" + string(filepath.Separator) + "file{1,2}.txt"}
	exclusions := []string{string(filepath.Separator) + "file1.txt"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if !excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion
func TestIsExcluded3(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	exclusions := []string{string(filepath.Separator) + "file?.txt"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if !excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion
func TestIsExcluded4(t *testing.T) {
	path := string(filepath.Separator) + "file1.txt"
	exclusions := []string{string(filepath.Separator) + "file*.txt"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if !excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should match shellglob exclusion with dir descend
func TestIsExcluded5(t *testing.T) {
	path := string(filepath.Separator) + "adir" + string(filepath.Separator) + "anotherDir" +
		string(filepath.Separator) + "file1.txt"
	exclusions := []string{"**" + string(filepath.Separator) + "*.txt"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if !excluded {
		t.Fatalf("Exclusion rule '%s' did not match but it was expected that it would match", exclusions[0])
	}
	if exclusionRule == "" {
		t.Fatal("When a match is found, it is expected that the matched exclusion pattern (second " +
			"argument in reply) is a non empty string representing the matche pattern, but instead we got an empty " +
			"string")
	}
}

// should NOT match shellglob exclusion due to lack of dir descend
func TestIsExcluded6(t *testing.T) {
	path := string(filepath.Separator) + "adir" + string(filepath.Separator) + "anotherDir" +
		string(filepath.Separator) + "file1.txt"
	exclusions := []string{"*.txt"}
	excluded, exclusionRule, err := IsPathExcluded(exclusions, path)
	if err != nil {
		t.Fatal(err)
	}
	if excluded {
		t.Fatalf("Exclusion rule '%s' matched but it wasn't expected that it would match", exclusionRule)
	}
	if exclusionRule != "" {
		t.Fatalf("When a match is NOT found, it is expected that the matched exclusion pattern (second "+
			"argument in reply) is empty but instead we got: '%s'", exclusionRule)
	}
}

func TestStripEndOfPathSeparators_Windows(t *testing.T) {
	path, separator, result := `c:\`, `\`, `c:\`
	res := StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `c:\\`, `\`, `c:\`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `c:\\\\\`, `\`, `c:\`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `c:\asdasd\a`, `\`, `c:\asdasd\a`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `c:\asdasd\a\`, `\`, `c:\asdasd\a`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `c:\asdasd\a\\`, `\`, `c:\asdasd\a`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}
}

func TestStripEndOfPathSeparators_Unixes(t *testing.T) {
	path, separator, result := `/`, `/`, `/`
	res := StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `/lala`, `/`, `/lala`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `/lala/`, `/`, `/lala`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `/lala//`, `/`, `/lala`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `/lala///`, `/`, `/lala`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}

	path, separator, result = `/lala/a`, `/`, `/lala/a`
	res = StripEndOfPathSeparators(path, separator)
	if res != result {
		t.Fatalf("Unexpected result of: '%s' while '%s' was expected for path '%s' and separator: '%s'", res,
			result, path, separator)
	}
}
