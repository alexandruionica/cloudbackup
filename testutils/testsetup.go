package testutils

import (
	"testing"
	"io/ioutil"
)

var MockYaml = []byte(`---
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
    targets:
      - name: aws_1
        type: aws_s3
        user: BLABLA
        pass: zzzz
        bucket: myawesome-backup
        prefix: backup/backups-for-server-51
        storage_class: standard
    schedule:
      - 05 01 * * *
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    targets:
      - name: aws_2
        type: aws_s3
        user: JOHNDOE
        pass: qwqe
        bucket: some-stuff-goes-here
        prefix: backup/backups-for-server-51
        storage_class: infrequent-access
      - name: google_1
        type: google_cloud_storage
        user: JANEDOE
        pass: 34324fd
        bucket: my-google-bucket
        prefix: backup/backups-for-server-51
        storage_class: standard
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - 00 08 01 * *
      - 00 08 06 * *
    versioning: true
    versions_max_num: 10
    versions_max_age: 6w`)

// create a file in the tmpdir and populate it with whatever content was provided. The user must delete the file
// afterwards. Returns a string with is the full path of the file
func SetupFakeFile(content []byte, prefix string, t *testing.T) string {
	tmpfile, err := ioutil.TempFile("", prefix)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	return tmpfile.Name()
}
