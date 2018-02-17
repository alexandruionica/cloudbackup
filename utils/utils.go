package utils

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{
	"context": "utils",
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