package testutils

import (
	"bytes"
	"cloudbackup/utils"
	"fmt"
	"github.com/gofrs/uuid"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// BEWARE that if you change "data_dir: /tmp" or "html_dir: /tmp" then the function SetupMockConfigAndTmpPaths
// needs to also be adjusted
var MockYaml = []byte(`---
data_dir: /tmp
html_dir: /tmp
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup misc hash-password to hash passwords
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    access: write
  - name: testuser2
    # bcrypt hash of password  "Oonaawai8Eep]eethe8eefa$"
    pass: $2a$05$Pgdwe14mHjOQ33C5LahmmugCY85Yfqlkj2rGvbDMGCDXKKwmhbwVC
    access: read
# global settings affect all backups and can't be specified per backup with different values
# section specific settings are repetitive and can't be overridden by globals
# clarity and safety are paramount to the design so repeating a particular key - value over and over is acceptable
backup:
  - name: first_backup
    paths:
      - /something
      - /var/lib
    exclusions:
      - /something/else
      - /var/lib/mysql
    target:
      - name: aws_1
        type: test_null
        bucket: myawesome-backup
        prefix: backup/backups-for-server-51
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: STANDARD
    schedule:
      - 05 01 * * *
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    target:
      - name: aws_2
        type: test_null
        bucket: some-stuff-goes-here
        prefix: backup/backups-for-server-51
        parameters:
          - name: storage_class
            value: STANDARD
      - name: google_1
        type: gcp_storage
        bucket: my-google-bucket
        prefix: backup/backups-for-server-51
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - 00 08 01 * *
      - 00 08 06 * *
    checksum: true
    versions_max_num: 10
    versions_max_age: 6w`)

// missing encryption password
var MockYamlInvalidConfig1 = []byte(`---
# global settings affect all backups and can't be specified per backup with different values
# section specific settings are repetitive and can't be overridden by globals
# clarity and safety are paramount to the design so repeating a particular key - value over and over is acceptable
backup:
  - name: first_backup
    paths:
      - /something
      - /var/lib
    exclusions:
      - /something/else
      - /var/lib/mysql
    target:
      - name: aws_1
        type: aws_s3
        bucket: myawesome-backup
        prefix: backup/backups-for-server-51
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: STANDARD
    schedule:
      - 05 01 * * *
    encrypt: true
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    target:
      - name: aws_2
        type: aws_s3
        bucket: some-stuff-goes-here
        prefix: backup/backups-for-server-51
      - name: google_1
        type: gcp_storage
        bucket: my-google-bucket
        prefix: backup/backups-for-server-51
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - 00 08 01 * *
      - 00 08 06 * *
    versioning: true
    versions_max_num: 10
    versions_max_age: 6w`)

// invalid password hash for testuser1
var MockYamlInvalidConfig2 = []byte(`---
data_dir: /tmp
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup hash-password to hash passwords
    pass: blabla
# global settings affect all backups and can't be specified per backup with different values
# section specific settings are repetitive and can't be overridden by globals
# clarity and safety are paramount to the design so repeating a particular key - value over and over is acceptable
backup:
  - name: first_backup
    paths:
      - /something
      - /var/lib
    exclusions:
      - /something/else
      - /var/lib/mysql
    target:
      - name: aws_1
        type: aws_s3
        bucket: myawesome-backup
        prefix: backup/backups-for-server-51
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: STANDARD
    schedule:
      - 05 01 * * *
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    target:
      - name: aws_2
        type: aws_s3
        bucket: some-stuff-goes-here
        prefix: backup/backups-for-server-51
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: STANDARD
      - name: google_1
        type: gcp_storage
        bucket: my-google-bucket
        prefix: backup/backups-for-server-51
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - 00 08 01 * *
      - 00 08 06 * *
    versioning: true
    versions_max_num: 10
    versions_max_age: 6w`)

// valid until Feb 16 03:28:17 2028 GMT
var SelfSignedSslCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDhTCCAm2gAwIBAgIJAIIc4rd6tIASMA0GCSqGSIb3DQEBCwUAMFkxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxEjAQBgNVBAMMCTEyNy4wLjAuMTAeFw0xODAyMTgwMzI4
MTdaFw0yODAyMTYwMzI4MTdaMFkxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21l
LVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxEjAQBgNV
BAMMCTEyNy4wLjAuMTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKI4
La8Iskibkejz9t2lF45kAgEcjksaPDAPm8U0bEocbBcExK7gUK7p84abSEwggAed
WMIZh3ivxhukcxnVlvHqLqosgPwM6lVTJrVBrmp9PX/TNA9N14C4Q/EI5u1LaBHc
b5h6HfjSUIMKEPOoF1tIH7p00OSEhhU4uBLOC72251GaW6MfsFGbLaOADKbjto1i
s3/icOwxR6Ud9jQ5Op/MDOAPc5SheFb35RrduQ/SakhK6jAZNMn7zJUNi2A4oKeq
1mpavvstbYF2gaDsUeSTRSGY+tosKASXfPYFGS7IilMGPHemIb2zYiXEr6j0hzew
fe+PU6JlarOoVLxteEUCAwEAAaNQME4wHQYDVR0OBBYEFFoH2exmJ67S9uRQsN35
TgU8F/v9MB8GA1UdIwQYMBaAFFoH2exmJ67S9uRQsN35TgU8F/v9MAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQELBQADggEBABGRnGVva2iljgfZkjNvwHJRs0LSSNtt
UaTuyMM8zKImMpEf+NZIQdmnhsn/rDsO4RoLODP9Qcn7f0QtNpUmLmmJQlT6vGM0
xmRoSZNEWF+7UiDA3TtJCYxGkrqEVMeMpTubqmMtPzc3c8/tnBTHc2nAqtz/Czzy
Ne/+pecF1wUEbgdRoNxhJh1qOsJ+17qs4CiLOVOebG0e5Z8A2ilkZ9Tq+zbsM/Cf
e6UHmrHTh0yoIkJOOyF8Ngv7CySr+q2f7NuujpPIjYRGb0xJOebuTg2d1LAopAw/
bS3Yyp95a+jmmKp9bD1UmMTYiMNUsoOxH6X9lcZivDI+9YWrRpz+/DY=
-----END CERTIFICATE-----`)

// valid until Feb 16 03:28:17 2028 GMT
var SelfSignedSslKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCiOC2vCLJIm5Ho
8/bdpReOZAIBHI5LGjwwD5vFNGxKHGwXBMSu4FCu6fOGm0hMIIAHnVjCGYd4r8Yb
pHMZ1Zbx6i6qLID8DOpVUya1Qa5qfT1/0zQPTdeAuEPxCObtS2gR3G+Yeh340lCD
ChDzqBdbSB+6dNDkhIYVOLgSzgu9tudRmlujH7BRmy2jgAym47aNYrN/4nDsMUel
HfY0OTqfzAzgD3OUoXhW9+Ua3bkP0mpISuowGTTJ+8yVDYtgOKCnqtZqWr77LW2B
doGg7FHkk0UhmPraLCgEl3z2BRkuyIpTBjx3piG9s2IlxK+o9Ic3sH3vj1OiZWqz
qFS8bXhFAgMBAAECggEAIIgxqTOORX9lcJlUfbi5E6Y8vKpUYv1c6qqGq7LKsMYo
aylapFN5+soSO4Fyq0mtQ1mrzik+gNaHXU3Kg3jRL6yuNRR9vY59hCUL0zfb2aFK
LxNVEmii+j556aHGZfpEYaiafLKoxhivasgfBC5GmNjK/CKnLdzh4umgCK1nr2Do
Fxh9PQ9gVqFb7GWtG+iDZWDo3Y75vCRzrkQ6XY/JAedvojcrnWNl5BNvTK/24SMU
aDpNv2vkl9UTPUMKGr/7lly5eYE0KIFI26kFPAM4yQ0sJlwJazhQXJE+l8jmT/+P
BsqofFSKcCApkyAGEPXHSI0R9AlWYVZ6OLkOaBzfdQKBgQDR9a2npPZNY0qMffa3
oNjx7qCTPRMCwaJ0/qKdWbNkevTnd4DTnEDDvavSjXEXEY39udPcmRvuGbVMuJ3Z
ccq50nUOXvn2MndUHIqMAOBjK0+38T6YrQlVaFWRNhE+TV2JqyCIuMUjMHsulKNs
8Gh12MqfpFHyQqFam4Zj8pxCdwKBgQDFyox0nroKyujl6KzjoWgF5Ki4zc8I+bkJ
BN/NdAu8rDRHmWR1TWst4SA4VmTKpFaCMMz9n4caPrmODdPEIMJGJtYlurnJPNmt
s3+f5tIqIFysnbf61xtP9DVktTrjub5jOjj3I9BJZI6hobuozj+rkm4RieAhfg6Y
LvmUXJkuIwKBgQCgEMd6FlY7+2V7JBDyP2sFTmIWvin/IPYkcXgxs5ADG4YX7NBH
A0mQsMoMdA5ygsyYUZJGDGfxpqHEQr78ZjciYWMiOKAh5Kl6c2Pghk6K7BsTZZTO
OqTx+t+5G9obgEm+Sbs84HhScoSGp4TL6aAJr+QRvulGYyu18vmKuwwL0wKBgQCW
FFr/InGIPu75hNOq5Y5I6ngbwg6WgOYmMcyf2K4PO5tvuLTBTT1GUsxf8y4HlSsP
Hnhs+d9Jys6BO3y0FSdUk6NqfYT7bXC+nLT6X+qYjHXFhOdVLmNLB8J76AgHQ6lz
IXqYDFS/W83eVxpNvDITvchHBpdK0pvAXeSC7sBMgQKBgBm8U9XOeI5suj7+s114
qW9MEP5/jMKOXqg/9n3iSjfs5TmceU//OD9kSPibO6avxXX9eDxAtACwbfDkDODK
gY+aeR8l9EsQPSwpE1BfPhdBwxMEmTKymOtQaDLXAiJjaGEaFrP3kMtRgQ/klvfz
029tv/IKycdHt3Grv4rUs4IA
-----END PRIVATE KEY-----`)

// sets up a self signed ssl certificate and key
func SetupSslCertAndKey(prefix string, t *testing.T) (string, string) {
	sslCert, err := utils.SetupTmpFileWithContent(SelfSignedSslCert, prefix)
	if err != nil {
		t.Fatal(err)
	}
	sslKey, err := utils.SetupTmpFileWithContent(SelfSignedSslKey, prefix)
	if err != nil {
		t.Fatal(err)
	}
	return sslCert, sslKey
}

func WaitForServerToStart(host string, port string, t *testing.T) error {
	// check several times is port is being listened on
	counter := 0
	for {
		conn, _ := net.DialTimeout("tcp", net.JoinHostPort(host, port), 100*time.Millisecond) // #nosec
		if conn != nil {
			err := conn.Close()
			if err != nil {
				return err
			} else {
				return nil
			}
		} else {
			if counter > 200 {
				return fmt.Errorf("ffter 20 seconds of waiting, nothing is listening on %s:%s", host, port)
			}
		}
		counter += 1
	}
}

// client config
var MockClientYaml = []byte(`---
username: testuser1
password: 'HV}H/y?<9$]Z5N4N'
address: http://127.0.0.1:8080
`)

// folder with some files to test backing up; user must delete folder afterwards
func SetupBackupDir(testName string, t *testing.T) string {
	var path = utils.SetupTmpDir("unittest_backup_scan_test_"+testName, t)
	// something like /tmp/$RANDOM/dir1/dir2/dir3/dir4
	err := os.MkdirAll(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3"+string(filepath.Separator)+"dir4", 0755) // #nosec
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	// something like /tmp/$RANDOM/absolute_symlink_to_dir3 -> /tmp/$RANDOM/dir1/dir2/dir3
	err = os.Symlink(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3", path+string(filepath.Separator)+"absolute_symlink_to_dir3")
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// something like /tmp/$RANDOM/relative_symlink_to_dir3 -> dir1/dir2/dir3
	err = os.Symlink("dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3", path+string(filepath.Separator)+"relative_symlink_to_dir3")
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/file1
	err = ioutil.WriteFile(path+string(filepath.Separator)+"file1", []byte(`text for file1`), 0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/dir2/dir3/file2.txt
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3"+string(filepath.Separator)+"file2.txt", []byte(`text for file2.txt`),
		0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}
	// /tmp/$RANDOM/dir1/dir2/dir3/file3
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3"+string(filepath.Separator)+"file3", []byte(`text for file3`), 0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/dir2/dir3/file4
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"dir3"+string(filepath.Separator)+"file4", []byte(`text for file4`), 0644)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/dir2/file5
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"file5", []byte(`text for file5`), 0640)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/dir2/file6
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+"dir2"+
		string(filepath.Separator)+"file6", []byte(`text for file6`), 0600)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/dir5/
	err = os.MkdirAll(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"dir5", 0755) // #nosec
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// /tmp/$RANDOM/dir1/file7
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"file7", []byte(`text for file7`), 0600)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// unicode in filename /tmp/$RANDOM/dir1/file8世界⌘ä
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"file8世界⌘ä", []byte(`text for file8世界⌘ä`), 0600)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// dir with unicode in name /tmp/$RANDOM/dir1/dir6öüÂș/
	err = os.MkdirAll(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"dir6öüÂș", 0755) // #nosec
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// plain file name in unicode dirname /tmp/$RANDOM/dir1/dir6öüÂș/file9
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"dir6öüÂș"+string(filepath.Separator)+"file9", []byte(`text for file9`), 0600)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	// unicode file name in unicode dirname /tmp/$RANDOM/dir1/dir6öüÂș/file10ŹżÇù.txt
	err = ioutil.WriteFile(path+string(filepath.Separator)+"dir1"+string(filepath.Separator)+
		"dir6öüÂș"+string(filepath.Separator)+"file10ŹżÇù.txt", []byte(`text for file10ŹżÇù.txt`), 0600)
	if err != nil {
		_ = os.RemoveAll(path) // #nosec
		t.Fatal(err)
	}

	return path
}

// sets up a server config file with whatever additional tmp dirs are needed. It's up to the user to delete the
// created items which are returned as an slice of strings
// returns: path to config file; slice of paths to delete
func SetupMockConfigAndTmpPaths(t *testing.T, prefix string) (string, []string) {
	dbDataDirPath := utils.SetupTmpDir(prefix+"_datadir_", t)
	HtmlDirPath := utils.SetupTmpDir(prefix+"_htmldir_", t)
	newMockYaml := bytes.Replace(MockYaml, []byte("data_dir: /tmp"), []byte("data_dir: "+dbDataDirPath), 1)
	newMockYaml = bytes.Replace(newMockYaml, []byte("html_dir: /tmp"), []byte("html_dir: "+HtmlDirPath), 1)
	if runtime.GOOS == "windows" {
		newMockYaml = bytes.Replace(newMockYaml, []byte("- /something"), []byte(`- c:\something`), 1)
		newMockYaml = bytes.Replace(newMockYaml, []byte("- /var/lib"), []byte(`- c:\Windows\system`), 1)

		newMockYaml = bytes.Replace(newMockYaml, []byte("- /var/log"), []byte(`- c:\Program Files\`), 1)
		newMockYaml = bytes.Replace(newMockYaml, []byte("- /var/www/html/data/"), []byte(`- C:\Windows\system32`), 1)
	}
	path, err := utils.SetupTmpFileWithContent(newMockYaml, prefix+"_config_")
	if err != nil {
		err2 := os.RemoveAll(dbDataDirPath)
		if err2 != nil {
			fmt.Printf("Failed to delete %s due to error: %s", dbDataDirPath, err2)
		}
		t.Fatalf("Could not create tmp config file due to error: %s", err)
	}

	pathsToDelete := make([]string, 0)
	pathsToDelete = append(pathsToDelete, path, dbDataDirPath, HtmlDirPath)
	return path, pathsToDelete
}

// given a slice containing paths, it tries delete each one of them from disk
func DeleteTestFilesAndDirs(toDelete []string) {
	for _, item := range toDelete {
		err := os.RemoveAll(item)
		if err != nil {
			fmt.Printf("Failed to delete %s due to error: %s", item, err)
		}

	}
}

// produce a tmp file path, not guaranteed to be unique but given an uuid is used, collision chances should be low
func GenerateTmpFilePath(t *testing.T, prefix string, suffix string) string {
	var u uuid.UUID
	var err error
	found := false
	// try 20 times to get a UUID
	for i := 0; i < 20; i++ {
		u, err = uuid.NewV4()
		if err != nil {
			continue
		} else {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Could not generate a UUID so a tmp file path can't be generated. Encountered error was: %s", err)
	}

	namePart := u.String()
	return filepath.Join(os.TempDir(), prefix+namePart+suffix)
}
