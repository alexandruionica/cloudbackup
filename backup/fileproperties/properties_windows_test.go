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
func getRunningUserDetails(t *testing.T) (sid, username, domain string) {
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

type propertiesFromPowerShell struct {
	Name              string
	Domain            string
	FileSystemRights  string
	AccessControlType string
	IsInherited       string
}

func getFilePropertiesUsingPowershell(t *testing.T, file string) []propertiesFromPowerShell {
	/* The command will return output similar to the below example
	PS C:\Users\vagrant> (get-acl <folder or file name>).access | ft IdentityReference,FileSystemRights,AccessControlType,IsInherited,InheritanceFlags -auto

	IdentityReference       FileSystemRights AccessControlType IsInherited                InheritanceFlags
	-----------------       ---------------- ----------------- -----------                ----------------
	NT AUTHORITY\SYSTEM          FullControl             Allow        True ContainerInherit, ObjectInherit
	BUILTIN\Administrators       FullControl             Allow        True ContainerInherit, ObjectInherit
	VAGRANT-RS57QRT\vagrant      FullControl             Allow        True ContainerInherit, ObjectInherit
	*/
	cmd := exec.Command("powershell", "-NonInteractive", `(get-acl `+file+`).access | ft IdentityReference,FileSystemRights,AccessControlType,IsInherited,InheritanceFlags -auto`) // #nosec
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Could not setup pipe for Powershell command due to error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Could not start for Powershell command due to err: %s", err)
	}

	var lineOutput, allOutput string
	var retrievedProperties []propertiesFromPowerShell
	const marker = "----------"
	foundMarker := false
	scanner := bufio.NewScanner(stdout)
	// read only the first line of output
	for scanner.Scan() {
		lineOutput = scanner.Text()
		allOutput = allOutput + lineOutput + "\n"
		// start doing the regexes only after the marker is found
		if strings.HasPrefix(lineOutput, marker) {
			foundMarker = true
			continue
		}
		if foundMarker && lineOutput != "" {
			// matching for stuff like:   NT AUTHORITY\SYSTEM          FullControl             Allow        True ContainerInherit, ObjectInherit
			cmdOutput := regexp.MustCompile(`^([a-zA-Z0-9 -]+)\\([a-zA-Z0-9-]*) +([a-zA-Z0-9-]+) +([a-zA-Z0-9-]+) +([a-zA-Z0-9-]+)`)
			regexResultArray := cmdOutput.FindStringSubmatch(lineOutput)
			if regexResultArray == nil {
				t.Fatalf("Could not regex file properties from output returned by the Powershell commandlet. "+
					"Line which failed to regex is: '%s'", lineOutput)
			}
			fileProp := propertiesFromPowerShell{
				Name:              regexResultArray[2],
				Domain:            regexResultArray[1],
				FileSystemRights:  regexResultArray[3],
				AccessControlType: regexResultArray[4],
				IsInherited:       regexResultArray[5],
			}
			retrievedProperties = append(retrievedProperties, fileProp)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("while reading standard input from Powershell command, got error: %s", err)
	}
	if len(retrievedProperties) == 0 {
		t.Fatalf("Either the file %s has no access controls on it or more likely parsing the file properties "+
			"output from Powershell did not find any lines containing ACLs. The output of the powershell was: %s"+
			"\n", file, allOutput)
	}
	return retrievedProperties
}

// workhorse for the test in this file
func examineFile(t *testing.T, file string, filestat os.FileInfo, sid, username, domain string) {
	owner, permissions, err := GetObjectPermissions(file, filestat)
	if err != nil {
		t.Fatalf("While trying to get permissions of %s got error: %s", file, err)
	}

	// JSON decode permissions object
	var expandedPerm FilePermissions
	err = json.Unmarshal([]byte(permissions), &expandedPerm)
	if err != nil {
		t.Fatalf("Could not json decode the permissions string due to error: %s", err)
	}

	// variable depicts if the creator  of the file is shown as the "Onwer" in the top level entry or in one of the ACE
	// entries
	creatorInACES := false
	if strings.EqualFold(username, owner) {
		// check the json structure to see if the ACLs contain the user we run under . Windows permissions seem to work in misterious ways
		for _, AceEntry := range expandedPerm.ACEs {
			if strings.EqualFold(username, AceEntry.Account.Name) {
				creatorInACES = true
				if strings.EqualFold(domain, AceEntry.Account.Domain) {
					t.Fatalf("Expected domain user %s who created %s to be %s but instead got domain %s", username, file, strings.ToLower(domain),
						strings.ToLower(AceEntry.Account.Domain))
				}
			}
		}
		if !creatorInACES {
			t.Fatalf("1. Expected owner of %s to be %s but instead got owner %s . The expected owner wasn't "+
				"found in any ACE entry either", file, username, owner)
		}
	}

	// variable depicts if the creator  of the file is shown as the "Onwer" in the top level entry or in one of the ACE
	// entries
	creatorInACES = false
	// check permissions object has expected content
	if strings.EqualFold(username, expandedPerm.Owner.Name) {
		// check the json structure to see if the ACLs contain the user we run under . Windows permissions seem to work in mysterious ways
		for _, AceEntry := range expandedPerm.ACEs {
			if strings.EqualFold(username, AceEntry.Account.Name) {
				creatorInACES = true
			}
		}
		if !creatorInACES {
			t.Fatalf("2. Expected owner of %s to be %s but instead got owner %s . The expected owner wasn't "+
				"found in any ACE entry either", file, username, owner)
		}
	}

	if strings.EqualFold(owner, expandedPerm.Owner.Name) {
		t.Fatalf("owner is %s while expandedPerm.Owner.Name is %s and it is expected they match", owner, expandedPerm.Owner.Name)
	}

	// if we know that the creator of the file doesn't have an entry in the top level response of the API call then
	// we need to go through each ACE
	if creatorInACES {
		foundOwnerSidMatch := false
		for _, AceEntry := range expandedPerm.ACEs {
			if sid == AceEntry.Account.SID {
				foundOwnerSidMatch = true
			}
		}

		if !foundOwnerSidMatch {
			t.Fatalf("Creator sid %s of %s was not found in any of the ACEs", sid, file)
		}

		// walk file properties and ensure that the creator has access:

	} else {
		if sid != expandedPerm.Owner.SID {
			t.Fatalf("Expected owner sid of %s to be %s but instead got sid %s", file, sid, expandedPerm.Owner.SID)
		}
		if strings.EqualFold(domain, expandedPerm.Owner.Domain) {
			t.Fatalf("Expected owner domain of %s to be %s but instead got domain %s", file, strings.ToLower(domain),
				strings.ToLower(expandedPerm.Owner.Domain))
		}
	}

	// TODO - get file ACLs and compare - right now we basically just compare output from "whoami" command with
	// Powershell ACL output but this is wrong because it doesn't actaully compare against the data the GO code gets
	// when using the native Windows System calls
	foundMatchInProperties := false
	filePropertiesFromPowerShell := getFilePropertiesUsingPowershell(t, file)
	for _, fileProperties := range filePropertiesFromPowerShell {
		if strings.EqualFold(username, fileProperties.Name) && strings.EqualFold(domain, fileProperties.Domain) {
			// FileSystemRights AccessControlType
			// FullControl             Allow
			if fileProperties.FileSystemRights == "FullControl" && fileProperties.AccessControlType == "Allow" {
				foundMatchInProperties = true
			}
		}
	}
	if !foundMatchInProperties {
		t.Fatalf("The Powershell retrieved file properties don't have the creator of the files listed as " +
			"having 'FullControl' on the file")
	}

}

// compare file / dir / symlink properties returned by GetObjectPermissions() with data supplied by OS tools
func TestGetObjectPermissions1withStat(t *testing.T) {
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
//func TestGetObjectPermissions2withLstat(t *testing.T){
//	sid, username, domain := getRunningUserDetails(t)
//
//	// folder with some mock files and symlinks
//	backupDirPath := testutils.SetupBackupDir("unittest_backup_fileproperties_TestGetCtime2_", t)
//	defer func() {
//		err := os.RemoveAll(backupDirPath) // #nosec
//		if err != nil {
//			t.Fatalf("Could not remove mock folder used to test backup. Error was: %s", err)
//		}
//	}()
//	var files []string
//	err := filepath.Walk(backupDirPath, func(path string, info os.FileInfo, err error) error {
//		files = append(files, path)
//		return nil
//	})
//	if err != nil {
//		t.Fatalf("While walking path %s got error: %s", backupDirPath, err)
//	}
//	for _, file := range files {
//		filestat, err := os.Lstat(file)
//		if err != nil {
//			t.Fatalf("while running run os.Stat(%s) got error: %s", file, err)
//		}
//		examineFile(t, file, filestat, sid, username, domain)
//	}
//}
