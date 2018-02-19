package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
)

const loggingContext = "utils"
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

// check if file exists; parameters are path to file (String) and if to dereference symlinks (bool)
func FileExists(path string, dereference bool) (os.FileInfo, error) {
	var err error
	var stat os.FileInfo
	if dereference {
		stat, err = os.Stat(path)
	} else {
		stat, err = os.Lstat(path)
	}
	if os.IsNotExist(err) {
		msg := fmt.Sprintf("File %s does not exist", path)
		return stat, errors.New(msg)
	}
	if stat.Mode().IsRegular() != true{
		msg := fmt.Sprintf("%s is not a regular file", path)
		return stat, errors.New(msg)
	}
	return stat, nil

}
