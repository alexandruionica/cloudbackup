package utils

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
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