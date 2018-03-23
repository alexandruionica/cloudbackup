package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"testing"
	"io/ioutil"
	"path/filepath"
)

const loggingContext = "utils"
var ErrNoSuchFile = errors.New("file does not exist")
var ErrNotRegularFile = errors.New("file is not a regular file")
var ErrNoSuchDir = errors.New("directory does not exist")
var ErrNoSuchRelativeDir = errors.New("relative directory path does not exist")
var ErrNotADir = errors.New("path is not a directory")
var ErrUnusableDirPath = errors.New("provided directory path is unusable")

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// pretty print
func Pp(input interface{}){
	output, err := json.MarshalIndent(input, "", "  ")
	if err != nil{
		logger.Errorf("Could not Pretty Print due to: %s", err)
	} else {
		fmt.Println(string(output))
	}
}

// check if file exists; parameters are path to file (String) and if to dereference symlinks (bool). Works only with
// regular files and symlinks
func FileExists(path string, dereference bool) (os.FileInfo, error) {
	var err error
	var stat os.FileInfo
	if dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if os.IsNotExist(err) {
		return stat, ErrNoSuchFile
	}

	if dereference {
		if stat.Mode().IsRegular() != true {
			return stat, ErrNotRegularFile
		}
	} else {
		if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
			// This is a symlink and we're ok with that
			return stat, nil
		} else {
			// Not a symlink
			if stat.Mode().IsRegular() != true {
				return stat, ErrNotRegularFile
			}
		}
	}
	return stat, nil
}

// check if directory exists; parameters are path to file (String) and if to dereference symlinks (bool). Works only with
// regular files and symlinks
func DirExists(path string, dereference bool) (os.FileInfo, error) {
	var err error
	var stat os.FileInfo
	if dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}

	// provided path does not exist
	if err != nil{
		if filepath.IsAbs(path){
			// for absolute path provided return error as Directory does not exist or is unaccesible
			return stat, ErrNoSuchDir
		} else {
			_, err := filepath.Abs(path)
			if err != nil{
				// provided path string is unusable
				return stat, ErrUnusableDirPath
			} else {
				// it's a relative path so then mark this in the error response. Directory does not exist or
				// is unaccesible
				return stat, ErrNoSuchRelativeDir
			}
		}
	}

	// path exists so let's see if it is a Directory
	if stat.IsDir() {
		return stat, nil
	} else {
		// path exists but it isn't a directory
		return stat, ErrNotADir
	}
}

// check if string is an element of slice
func StringInSlice(str string, list []string) bool {
	for _, val := range list {
		if val == str {
			return true
		}
	}
	return false
}

// create a file in the tmpdir and populate it with whatever content was provided. The user must delete the file
// afterwards. Returns a string with is the full path of the file
func SetupTmpFileWithContent(content []byte, prefix string) (string, error) {
	tmpfile, err := ioutil.TempFile("", prefix)
	if err != nil {
		return "", err
	}

	if _, err := tmpfile.Write(content); err != nil {
		return "", err
	}
	if err := tmpfile.Close(); err != nil {
		return "", err
	}
	logger.Debugf("Created tmp file %s and successfully wrote content to it.", tmpfile.Name())
	return tmpfile.Name(), nil
}


// create a directory in the tmpdir. The user must delete the file
// afterwards. Returns a string with is the full path of the directory
func SetupTmpDir(prefix string, t *testing.T) string {
	tmpdir, err := ioutil.TempDir("", prefix)
	if err != nil {
		t.Fatal(err)
	}
	return tmpdir
}