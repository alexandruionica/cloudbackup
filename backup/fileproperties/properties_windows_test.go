// +build windows

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
	"strings"
	"testing"
)

// obtain data about current user using OS supplied utilities instead on relying on Golang libraries
func getRunningUserDetails(t *testing.T)(sid, username, domain string){
	//cmd := exec.Command(`C:\WINDOWS\System32\whoami.exe /user /NH`)
	cmd := exec.Command("whoami", "/user", "/NH")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Could not setup pipe due to error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Could not start command due to err: %s", err)
	}

	/* expected command output is like
	PS C:\Users\aionica> whoami /USER /NH
	desktop-a2b\aionica S-1-5-21-1166836205-3126379902-2704385944-1001

	PS C:\Users\aionica>
	 */

	scanner := bufio.NewScanner(stdout)
	// read only the first line of output
	scanner.Scan()
	lineOutput := scanner.Text()
	// Use raw strings to avoid having to quote the backslashes.
	cmdOutput := regexp.MustCompile(`^([a-zA-Z0-9-]+)\\([a-zA-Z0-9-]*) (S[a-zA-Z0-9-]+)`)
	regexResultArray := cmdOutput.FindStringSubmatch(lineOutput)
	if regexResultArray == nil {
		t.Fatal("Could not regex sid, username & machine name")
	}
	domain = regexResultArray[1]
	username = regexResultArray[2]
	sid = regexResultArray[3]

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
	return sid, username, domain
}

// workhorse for the test in this file
func examineFile(t *testing.T, file string, filestat os.FileInfo, sid, username, domain string){
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

	if sid != expandedPerm.Owner.SID {
		t.Fatalf("Expected owner sid of %s to be %s but instead got sid %s", file, sid, expandedPerm.Owner.SID)
	}
	if strings.ToLower(domain) != strings.ToLower(expandedPerm.Owner.Domain) {
		t.Fatalf("Expected owner domain of %s to be %s but instead got domain %s", file, strings.ToLower(domain),
			strings.ToLower(expandedPerm.Owner.Domain))
	}

	// TODO - get file ACLs and compare
}

// compare file / dir / symlink properties returned by GetObjectPermissions() with data supplied by OS tools
func TestGetObjectPermissions1withStat(t *testing.T){
	sid, username, domain := getRunningUserDetails(t)

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
		examineFile(t, file, filestat, sid, username, domain)
	}
}

// use Lstat instead of Stat
func TestGetObjectPermissions2withLstat(t *testing.T){
	sid, username, domain := getRunningUserDetails(t)

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
		examineFile(t, file, filestat, sid, username, domain)
	}
}