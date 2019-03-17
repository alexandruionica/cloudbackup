package notification

import (
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/utils"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.notification"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type HttpStatusReply struct {
	HTTPCode int    `json:"-"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func Test(config clientConfig.Client, jsonOutput bool) {
	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/report/notification/test", nil)
	if err != nil {
		fmt.Printf("Error starting the notification test: %s\n", err)
		os.Exit(1)
	}
	req.SetBasicAuth(config.Username, config.Password)

	// make request
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		fmt.Println(err)
		os.Exit(1)
	}

	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	var decodedJson HttpStatusReply
	err = json.Unmarshal(body, &decodedJson)
	if err != nil {
		fmt.Printf("Could not decode the JSON response received from server. Error was: %s\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		err = utils.PpJson(body)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	} else {
		fmt.Println(decodedJson.Message)
	}

}
