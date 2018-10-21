package fileproperties

import (
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// function used by the GetCtime tests in order to prevent repeating most of the testing code
func testGetcTime(t *testing.T, path string, timestart time.Time) {
	fileCtime, err := GetCtime(path)
	if err != nil {
		t.Fatalf("1. While trying to get ctime for %s got error: %s", path, err)
	}
	if timestart.After(fileCtime) {
		t.Fatalf("File creation time for %s is reported to be before this test started. Ctime: %s; test " +
			"start: %s", path, fileCtime, timestart)
	}
	time.Sleep(10 * time.Millisecond)
	time1stTest := time.Now()
	if fileCtime.After(time1stTest) {
		t.Fatalf("Ctime for %s is reported to be %s which is in the future", path, fileCtime)
	}
	time.Sleep(10 * time.Millisecond)
	// update file ctime and see we can see a different result from GetCtime
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatalf("While trying to chmod() file %s got error: %s", path, err)
	}
	fileCtime2, err := GetCtime(path)
	if err != nil {
		t.Fatalf("2. While trying to get ctime for %s got error: %s", path, err)
	}
	if fileCtime.Equal(fileCtime2) {
		t.Fatal("File ctime should have changed after chmod but it's reported to be the same")
	}
	if fileCtime2.Before(time1stTest){
		t.Fatal("After chmod() the 2nd file ctime should be newer but it isn't")
	}
}

func TestGetCtime1(t *testing.T) {
	timestart := time.Now()
	// seems that if we don't sleep for a bit then we get an error when we do the first comparison below
	time.Sleep(10 * time.Millisecond)
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
	testGetcTime(t, path, timestart)
}

// repeat above test but with multiple files
func TestGetCtime2(t *testing.T) {
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
		panic(err)
	}
	for _, file := range files {
		testGetcTime(t, file, timestart)
	}
}
