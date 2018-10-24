// +build darwin freebsd netbsd openbsd solaris linux

package fileproperties

import (
	"bufio"
	"cloudbackup/testutils"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// obtain data about current user using OS supplied utilities instead on relying on Golang libraries
func getRunningUserDetails(t *testing.T)(uid, username, gid, groupname string){
	cmd := exec.Command("id")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Could not setup pipe due to error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Could not start command due to err: %s", err)
	}

	scanner := bufio.NewScanner(stdout)
	// read only the first line of output
	scanner.Scan()
	lineOutput := scanner.Text()
	// Use raw strings to avoid having to quote the backslashes.
	cmdOutput := regexp.MustCompile(`^uid=([0-9]+)\(([a-zA-Z0-9-]*)\) gid=([0-9]+)\(([a-zA-Z0-9-]*)\)`)
	regexResultArray := cmdOutput.FindStringSubmatch(lineOutput)
	if regexResultArray == nil {
		t.Fatal("Could not regex uid, username, gid & groupname")
	}
	uid = regexResultArray[1]
	username = regexResultArray[2]
	gid = regexResultArray[3]
	groupname = regexResultArray[4]

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
	return uid, username, gid, groupname
}

// workhorse for the test in this file
func examineFile(t *testing.T, file string, filestat os.FileInfo, uid, username, gid, groupname string){
	owner, permissions, err := GetObjectPermissions(file, filestat)
	if err != nil {
		t.Fatalf("While trying to get permissions of %s got error: %s", file, err)
	}
	if username != owner {
		t.Fatalf("1. Expected owner of %s to be %s but instead got owner %s", file, username, owner)
	}

	// JSON decode permissions object
	var expandedPerm FilePermissions
	err = json.Unmarshal([]byte(permissions), &expandedPerm)
	if err != nil {
		t.Fatalf("Could not json decode the permissions string due to error: %s", err)
	}
	// check permissions object has expected content
	if username != expandedPerm.Owner.Name {
			t.Fatalf("2. Expected owner of %s to be %s but instead got owner %s", file, username, owner)
	}
	uidNumeric, err := strconv.Atoi(uid)
	if err != nil {
		t.Fatal(err)
	}
	if uint32(uidNumeric) != expandedPerm.Owner.Id {
		t.Fatalf("Expected owner id of %s to be %d but instead got id %d", file, uidNumeric, expandedPerm.Owner.Id)
	}
	//
	if groupname != expandedPerm.Group.Name {
		t.Fatalf("Expected group of %s to be %s but instead got owner %s", file, groupname, expandedPerm.Group.Name)
	}
	gidNumeric, err := strconv.Atoi(gid)
	if err != nil {
		t.Fatal(err)
	}
	if uint32(gidNumeric) != expandedPerm.Group.Id {
		t.Fatalf("Expected group id of %s to be %d but instead got id %d", file, gidNumeric, expandedPerm.Group.Id)
	}
	fileMode := filestat.Mode()
	if fileMode != expandedPerm.Mode {
		t.Fatalf("Expected file mode for %s to be %s but instead it's %s", file, fileMode, expandedPerm.Mode)
	}
}

// compare file / dir / symlink properties returned by GetObjectPermissions() with data supplied by OS tools
func TestGetObjectPermissions1withStat(t *testing.T){
	uid, username, gid, groupname := getRunningUserDetails(t)

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
		filestat, err := os.Stat(file)
		if err != nil {
			t.Fatalf("while running run os.Stat(%s) got error: %s", file, err)
		}
		examineFile(t, file, filestat, uid, username, gid, groupname)
	}
}

// use Lstat instead of Stat
func TestGetObjectPermissions1withLstat(t *testing.T){
	uid, username, gid, groupname := getRunningUserDetails(t)

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
		filestat, err := os.Lstat(file)
		if err != nil {
			t.Fatalf("while running run os.Stat(%s) got error: %s", file, err)
		}
		examineFile(t, file, filestat, uid, username, gid, groupname)
	}
}