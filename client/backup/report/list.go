package report

import (
	"bytes"
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/utils"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"time"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.backup.report"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ListResponse struct {
	httpd.HttpStatusReply
	Next   string `json:"next"`
	Result []httpd.ReportBackupListDbResults
}

func List(config clientConfig.Client, jsonOutput bool, jobName string, StartTime time.Time, EndTime time.Time) {
	payload := httpd.ReportBackupList{
		Name:           jobName,
		FromStartTime:  StartTime.Format(time.RFC3339Nano),
		UntilStartTime: EndTime.Format(time.RFC3339Nano),
	}
	firstLoopRun := true

	for {
		encodedPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
			os.Exit(1)
		}

		httpClient := &http.Client{}
		req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/report/backup/list",
			bytes.NewBuffer(encodedPayload))
		if err != nil {
			fmt.Printf("Error starting the http client: %s\n", err)
			os.Exit(1)
		}
		req.Header.Set("Content-Type", "application/json")
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
		var decodedJson ListResponse
		err = json.Unmarshal(body, &decodedJson)
		if err != nil {
			fmt.Printf("Could not decode the JSON response received from server. Error was: %s\n", err)
			logger.Debugf("Server response was: %s", body)
			os.Exit(1)
		}

		// process result
		if jsonOutput {
			err = utils.PpJson(body)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			// stop the loop if we don't have a "next" marker; otherwise build a new payload
			if decodedJson.Next == "" {
				os.Exit(0)
			} else {
				payload = httpd.ReportBackupList{
					Name: jobName,
					Next: decodedJson.Next,
				}
			}
		} else {
			// TODO - ensure in all places (in the CLI and API) where we convert time.Time to string to use the same conversion format: Format(time.RFC3339Nano)

			// stop the loop if we don't have a "next" marker; otherwise build a new payload
			if decodedJson.Next == "" {
				if firstLoopRun {
					clientCommon.PrintJobListTable(decodedJson.Result, true)
				} else {
					clientCommon.PrintJobListTable(decodedJson.Result, false)
				}
				os.Exit(0)
			} else {
				if firstLoopRun {
					clientCommon.PrintJobListTable(decodedJson.Result, true)
					firstLoopRun = false
				} else {
					clientCommon.PrintJobListTable(decodedJson.Result, false)
				}
				payload = httpd.ReportBackupList{
					Name: jobName,
					Next: decodedJson.Next,
				}
			}
		}
	}

}
