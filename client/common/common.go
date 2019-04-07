package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"cloudbackup/httpd"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "client.common"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// some basic validation of responses received from the Cloudbackup API server
func ValidateServerResponse(resp *http.Response) ([]byte, error) {
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Debugf("Received error when trying to close response body. Error was: %s", err)
		}
	}()
	// check we can read the body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		return body, fmt.Errorf("cloud not process the response body received from the server. "+
			"The error was: %s", err)
	}

	// check we actually got JSON
	var decodedJson httpd.HttpStatusReply
	err = json.Unmarshal(body, &decodedJson)
	if err != nil {
		return body, fmt.Errorf("could not decode the JSON response received from server. Error "+
			"was: %s", err)
	}

	// http status code should be 200
	if resp.StatusCode != 200 {
		return body, errors.New(decodedJson.Message)
	}

	// 	// Code, Message fields should never be empty
	if decodedJson.Code == "" {
		return body, errors.New("invalid response from server: mandatory top level field 'Code' is empty")
	}
	if decodedJson.Message == "" {
		return body, errors.New("invalid response from server: mandatory top level field 'Message' is empty")
	}
	return body, nil
}
