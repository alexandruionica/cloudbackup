package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/url"
	"strings"
)

const loggingContext = "utils"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// pretty print by converting received input to JSON and then doing a fmt.println()
func Pp(input interface{}) {
	output, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		logger.Errorf("Could not Pretty Print due to: %s", err)
	} else {
		fmt.Println(string(output))
	}
}

// for a given []byte array containing a JSON encoded message, indent said message and print it
func PpJson(input []byte) error {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, input, "", "  ")
	if err != nil {
		logger.Debugf("provided message is not valid JSON. Received error was: '%s' and provided message was:"+
			" %s ", err, string(input))
		return fmt.Errorf("provided message is not valid JSON. Received error was: '%s'", err)
	}
	fmt.Println(prettyJSON.String())
	return nil
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

// squashes forward slashes in a given string. For example "aa/#bb//#cc///#dd////#eeee" gets returned as "aa/#bb/#cc/#dd/#eeee"
func SquashForwardSlashes(in string) string {
	for strings.Contains(in, "//") {
		in = strings.ReplaceAll(in, "//", "/")
	}
	return in
}

func IsValidUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	} else {
		return true
	}
}
