package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
)

const loggingContext = "utils"
var ErrNoSuchFile = errors.New("File does not exist")
var ErrNotRegularFile = errors.New("File is not a regular file")

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


// check if string is an element of slice
func StringInSlice(str string, list []string) bool {
	for _, val := range list {
		if val == str {
			return true
		}
	}
	return false
}
