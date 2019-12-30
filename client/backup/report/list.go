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
	"strconv"
	"time"
	"unicode/utf8"
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
					printBackupList(decodedJson, true)
				} else {
					printBackupList(decodedJson, false)
				}
				os.Exit(0)
			} else {
				if firstLoopRun {
					printBackupList(decodedJson, true)
					firstLoopRun = false
				} else {
					printBackupList(decodedJson, false)
				}
				payload = httpd.ReportBackupList{
					Name: jobName,
					Next: decodedJson.Next,
				}
			}
		}
	}

}

// for a "list" command this formats the result and prints it in a nice way
func printBackupList(decodedJson ListResponse, showHeader bool) {
	logger.Debugf("%+v", decodedJson)
	JobIdLength, StateLength, DurationLength, StartTimeLength, EndTimeLength := 5, 5, 8, 10, 8 // minimum is the lenght of the column headers
	for _, job := range decodedJson.Result {
		if utf8.RuneCountInString(job.JobId) > JobIdLength {
			JobIdLength = utf8.RuneCountInString(job.JobId)
		}
		if utf8.RuneCountInString(job.State) > StateLength {
			StateLength = utf8.RuneCountInString(job.State)
		}
		if utf8.RuneCountInString(job.StartTime) > StartTimeLength {
			StartTimeLength = utf8.RuneCountInString(job.StartTime)
		}
		if utf8.RuneCountInString(job.EndTime) > EndTimeLength {
			EndTimeLength = utf8.RuneCountInString(job.EndTime)
		}
		conversionErr := false
		tmpStartTime, err := time.Parse(time.RFC3339Nano, job.StartTime)
		if err != nil {
			conversionErr = true
		}
		tmpEndTime, err := time.Parse(time.RFC3339Nano, job.EndTime)
		if err != nil {
			conversionErr = true
		}
		if !conversionErr {
			if utf8.RuneCountInString(tmpEndTime.Sub(tmpStartTime).Round(time.Second).String()) > DurationLength {
				DurationLength = utf8.RuneCountInString(tmpEndTime.Sub(tmpStartTime).Round(time.Second).String())
			}
		}
	}
	// table header
	tableTemplate := "%" + strconv.Itoa(JobIdLength) + "s | %" + strconv.Itoa(StateLength) + "s | %" +
		strconv.Itoa(DurationLength) + "s | %" + strconv.Itoa(StartTimeLength) + "s | %" + strconv.Itoa(EndTimeLength) +
		"s\n"
	if showHeader {
		fmt.Printf(tableTemplate, "JobId", "State", "Duration", "Start Time", "End Time")
	}
	for _, job := range decodedJson.Result {
		conversionErr := false
		tmpStartTime, err := time.Parse(time.RFC3339Nano, job.StartTime)
		if err != nil {
			conversionErr = true
		}
		tmpEndTime, err := time.Parse(time.RFC3339Nano, job.EndTime)
		if err != nil {
			conversionErr = true
		}
		var endTime string
		if !conversionErr {
			if tmpEndTime.IsZero() {
				endTime = "n/a"
			} else {
				endTime = job.EndTime
			}
		} else {
			endTime = "n/a"
		}
		var duration string
		if conversionErr {
			duration = "n/a"
		} else {
			duration = tmpEndTime.Sub(tmpStartTime).Round(time.Second).String()
		}
		fmt.Printf(tableTemplate, job.JobId, job.State, duration, job.StartTime, endTime)
	}
}
