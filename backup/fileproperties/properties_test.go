package fileproperties

import (
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// function used by the GetCtime tests in order to prevent repeating most of the testing code
func testGetcTime(t *testing.T, path string, timestart time.Time, dereference bool) {
	fileCtime, err := GetCtime(path, dereference)
	if err != nil {
		t.Fatalf("1. While trying to get ctime for %s got error: %s", path, err)
	}
	if timestart.After(fileCtime) {
		t.Fatalf("File creation time for %s is reported to be before this test started. Ctime: %s; test "+
			"start: %s", path, fileCtime, timestart)
	}
	time.Sleep(20 * time.Millisecond)
	time1stTest := time.Now()
	if fileCtime.After(time1stTest) {
		t.Fatalf("Ctime for %s is reported to be %s which is in the future", path, fileCtime)
	}
	// Symlinks on windows don't seem to have their ctime updated so we'll skip the test on this platform only
	if !(strings.Contains(path, "symlink") && runtime.GOOS == "windows") {
		time.Sleep(20 * time.Millisecond)
		target := path
		// for Symlinks, if we dereference then change the property of the link target, not one of the symlink itself
		if strings.Contains(path, "symlink") {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				t.Fatalf("While trying to read the symlink target for %s got error %s", path, err)
			}
			if dereference {
				// if dereference then changes should be done on the target itself, not the symlink
				target = linkTarget
			} else {
				// remove and add back the symlink so this ends up with a newer Ctime for the link itself
				err := os.RemoveAll(path)
				if err != nil {
					t.Fatalf("While trying to delete %s, got error: %s", path, err)
				}
				err = os.Symlink(linkTarget, path)
				if err != nil {
					t.Fatalf("While creating back the Symlink %s got error: %s", path, err)
				}
			}

		}
		// update file ctime and see we can see a different result from GetCtime
		if err := os.Chmod(target, 0444); err != nil {
			// if the symlink target is relative chmod() fails and its fine to skip this test
			if strings.Contains(err.Error(), "no such file or directory") && strings.Contains(path, "symlink") && dereference {
				return
			}
			t.Fatalf("While trying to chmod() file %s got error: %s", target, err)
		}
		if err := os.Chmod(target, 0700); err != nil {
			t.Fatalf("While trying to chmod() file %s got error: %s", target, err)
		}
		fileCtime2, err := GetCtime(path, dereference)
		if err != nil {
			t.Fatalf("2. While trying to get ctime for %s got error: %s", path, err)
		}
		if fileCtime.Equal(fileCtime2) {
			t.Fatalf("File %s ctime should have changed after chmod but it's reported to be the same: %s vs %s", path, fileCtime, fileCtime2)
		}
		if fileCtime2.Before(time1stTest) {
			t.Fatal("After chmod() the 2nd file ctime should be newer but it isn't")
		}
	}
}

// getCtime with dereference on a regular file
func TestGetCtime1(t *testing.T) {
	timestart := time.Now()
	// seems that if we don't sleep for a bit then we get an error when we do the first comparison below
	time.Sleep(20 * time.Millisecond)
	path, err := utils.SetupTmpFileWithContent([]byte("blablabla"), "unittest_backup_fileproperties_TestGetCtime1_")
	if err != nil {
		t.Fatalf("While trying to setup tmpfile used for test got error: %s", err)
	}
	defer func() {
		err = os.RemoveAll(path)
		if err != nil {
			t.Fatalf("Could not remove mock file %s used to test GetCtime(). Error was: %s", path, err)
		}
	}()
	testGetcTime(t, path, timestart, true)
}

// getCtime without dereference on a regular file
func TestGetCtime2(t *testing.T) {
	timestart := time.Now()
	// seems that if we don't sleep for a bit then we get an error when we do the first comparison below
	time.Sleep(20 * time.Millisecond)
	path, err := utils.SetupTmpFileWithContent([]byte("blablabla"), "unittest_backup_fileproperties_TestGetCtime1_")
	if err != nil {
		t.Fatalf("While trying to setup tmpfile used for test got error: %s", err)
	}
	defer func() {
		err = os.RemoveAll(path)
		if err != nil {
			t.Fatalf("Could not remove mock file %s used to test GetCtime(). Error was: %s", path, err)
		}
	}()
	testGetcTime(t, path, timestart, false)
}

// repeat above test but with multiple files/folders/symlinks which also have unicode in their name
func TestGetCtime3(t *testing.T) {
	timestart := time.Now()
	// seems that if we don't sleep for a bit then we get an error when we do the first comparison below
	time.Sleep(10 * time.Millisecond)
	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_fileproperties_TestGetCtime2_", t)
	defer func() {
		err := os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	var files []string
	err := filepath.Walk(backupDirPath, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("While walking path %s got error: %s", backupDirPath, err)
	}
	for _, file := range files {
		testGetcTime(t, file, timestart, false)
	}
}

// repeat above test but with multiple files/folders/symlinks which also have unicode in their name
func TestGetCtime4(t *testing.T) {
	timestart := time.Now()
	// seems that if we don't sleep for a bit then we get an error when we do the first comparison below
	time.Sleep(10 * time.Millisecond)
	// folder with some mock files and symlinks
	backupDirPath := testutils.SetupBackupDir("unittest_backup_fileproperties_TestGetCtime2_", t)
	defer func() {
		err := os.RemoveAll(backupDirPath) // #nosec
		if err != nil {
			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
		}
	}()
	var files []string
	err := filepath.Walk(backupDirPath, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("While walking path %s got error: %s", backupDirPath, err)
	}
	for _, file := range files {
		testGetcTime(t, file, timestart, true)
	}
}
