// +build darwin freebsd netbsd openbsd solaris linux

package fileproperties

import (
	"bufio"
	"cloudbackup/testutils"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// obtain data about current user using OS supplied utilities instead on relying on Golang libraries
func getRunningUserDetails(t *testing.T) (uid, username, gid, groupname string) {
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
		t.Fatalf("Command returned error: %s", err)
	}
	return uid, username, gid, groupname
}

func getFileMode(t *testing.T, path string, dereference bool) string {
	/* Command output for  stat is :

	freebsd/linux/MacOS:  stat -L  (for lstat = dereference)
	Linux stat output:
	  File: config.yaml
	  Size: 3977      	Blocks: 8          IO Block: 4096   regular file
	Device: fd03h/64771d	Inode: 8130693     Links: 1
	Access: (0664/-rw-rw-r--)  Uid: ( 1000/ aionica)   Gid: ( 1000/ aionica)
	Access: 2018-10-21 22:05:53.088448425 +1100
	Modify: 2018-10-04 19:03:52.099925973 +1000
	Change: 2018-10-04 19:03:52.099925973 +1000
	 Birth: -


	FreeBSD output from   stat -x
	  File: "secrets.tdb"
	  Size: 430080       FileType: Regular File
	  Mode: (0600/-rw-------)         Uid: (    0/    root)  Gid: (    0/   wheel)
	Device: 205,3574202471   Inode: 115026    Links: 1
	Access: Sun Dec 11 00:10:52 2016
	Modify: Sun Dec 11 00:10:52 2016
	Change: Tue Sep 12 20:44:54 2017

	MacOS X output from  stat -x  test.py
	  File: "test.py"
	  Size: 62           FileType: Regular File
	  Mode: (0644/-rw-r--r--)         Uid: (  501/ cionica)  Gid: (   20/   staff)
	Device: 1,4   Inode: 8388253    Links: 1
	Access: Fri May 20 01:23:56 2016
	Modify: Fri May 20 01:23:56 2016
	Change: Fri May 20 01:23:56 2016
	*/
	var cmd *exec.Cmd
	switch os := runtime.GOOS; os {
	case "darwin", "freebsd":
		{
			if dereference {
				cmd = exec.Command("stat", "-x", "-L", path)
			} else {
				cmd = exec.Command("stat", "-x", path)
			}
		}
	case "linux":
		{
			if dereference {
				cmd = exec.Command("stat", "-L", path)
			} else {
				cmd = exec.Command("stat", path)
			}
		}
	default:
		t.Fatalf("1. This test doesn't support OS of type: %s . Please adjust the test as needed", runtime.GOOS)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Could not setup pipe due to error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Could not start command due to err: %s", err)
	}

	scanner := bufio.NewScanner(stdout)
	var lineOutput string
	foundMatch := false
	// read only the first line of output
	for scanner.Scan() {
		lineOutput = scanner.Text()
		switch os := runtime.GOOS; os {
		case "darwin", "freebsd":
			{
				if strings.HasPrefix(lineOutput, "  Mode: (") {
					foundMatch = true
				}
			}
		case "linux":
			{
				if strings.HasPrefix(lineOutput, "Access: (") {
					foundMatch = true
				}
			}
		default:
			t.Fatalf("2. This test doesn't support OS of type: %s . Please adjust the test as needed", runtime.GOOS)

		}
		if foundMatch {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("while reading standard input got error: %s", err)
	}
	if !foundMatch {
		t.Fatalf("Command output did not match expected patterns so no regex search was attempted.")
	}
	// Use raw strings to avoid having to quote the backslashes.
	cmdOutput := regexp.MustCompile(`^ *[a-zA-Z]+: \(([0-9]+)/`)
	regexResultArray := cmdOutput.FindStringSubmatch(lineOutput)
	if regexResultArray == nil {
		t.Fatalf("File mode regex didn't return a match. Input was: '%s'", lineOutput)
	}
	return regexResultArray[1]

}

// workhorse for the test in this file
func examineFile(t *testing.T, file string, filestat os.FileInfo, uid, username, gid, groupname, mode string) {
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

	if owner != expandedPerm.Owner.Name {
		t.Fatalf("owner is %s while expandedPerm.Owner.Name is %s and it is expected they match", owner, expandedPerm.Owner.Name)
	}

	uidNumeric, err := strconv.Atoi(uid)
	if err != nil {
		t.Fatal(err)
	}
	if uint32(uidNumeric) != expandedPerm.Owner.Id {
		t.Fatalf("Expected owner id of %s to be %d but instead got id %d", file, uidNumeric, expandedPerm.Owner.Id)
	}
	// on FreeBSD files under tmp seem to always get the groupname of "wheel"
	if runtime.GOOS == "freebsd" {
		groupname = "wheel"
	}
	if groupname != expandedPerm.Group.Name {
		t.Fatalf("Expected group name of %s to be %s but instead got group name %s . Full details are: %+v",
			file, groupname, expandedPerm.Group.Name, expandedPerm)
	}
	gidNumeric, err := strconv.Atoi(gid)
	if err != nil {
		t.Fatal(err)
	}
	// on FreeBSD files under tmp seem to always get the groupname of "wheel"
	if runtime.GOOS == "freebsd" {
		gidNumeric = 0
	}
	if uint32(gidNumeric) != expandedPerm.Group.Id {
		t.Fatalf("Expected group id of %s to be %d but instead got id %d", file, gidNumeric, expandedPerm.Group.Id)
	}

	modeNumeric, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		t.Fatalf("Could not convert %s to uint64", mode)
	}
	if uint32(modeNumeric) != uint32(expandedPerm.Mode.Perm()) {
		t.Fatalf("Expected file mode for %s to be %s but instead it's %o . BEWARE that output from the second "+
			"field may have the first '0' truncated but actual comparison is done on the numeric value instead of hex"+
			" representation", file, mode, expandedPerm.Mode.Perm())
	}
}

// compare file / dir / symlink properties returned by GetObjectPermissions() with data supplied by OS tools while
// DEREFERENCE links
func TestGetObjectPermissions1withStat(t *testing.T) {
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
		mode := getFileMode(t, file, true)
		examineFile(t, file, filestat, uid, username, gid, groupname, mode)
	}
}

//use Lstat instead of Stat which means to NOT dereference links
func TestGetObjectPermissions1withLstat(t *testing.T) {
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
		mode := getFileMode(t, file, false)
		examineFile(t, file, filestat, uid, username, gid, groupname, mode)
	}
}
