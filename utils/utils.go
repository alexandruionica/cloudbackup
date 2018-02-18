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

// check if file exists
func FileExists(path string) error {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		msg := fmt.Sprintf("File %s does not exist", path)
		return errors.New(msg)
	}
	if stat.Mode().IsRegular() != true{
		msg := fmt.Sprintf("%s is not a regular file", path)
		return errors.New(msg)
	}
	return nil

}
