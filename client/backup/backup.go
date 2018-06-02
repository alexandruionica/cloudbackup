package backup

import (
	"net/http"
	"fmt"

	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"unicode/utf8"
	"strconv"
	"encoding/json"
	"cloudbackup/utils"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.backup"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type BackupListResponse struct {
	httpd.HttpStatusReply
	Result []shared.BackupJobStatus
}

func List(config clientConfig.Client, jsonOutput bool) {
	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", config.Address + ApiPrefix + "/backup/list", nil)
	if err != nil {
		fmt.Printf("Error starting the http client: %s\n", err)
		os.Exit(1)
	}
	req.SetBasicAuth(config.Username, config.Password)

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		fmt.Println(err)
		os.Exit(1)
	}
	defer func(){
		err := resp.Body.Close()
		if err != nil {
			logger.Debugf("Received error when trying to close response body. Error was: %s", err)
		}
	}()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		fmt.Printf("Cloud not process the response body received from the server. The error was: %s\n", err)
		os.Exit(1)
	}
	var decodedJson BackupListResponse
	err = json.Unmarshal(body, &decodedJson)
	if err != nil {
		fmt.Printf("Could not decode the JSON response received from server. Error was: %s\n", err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		fmt.Println(decodedJson.Message)
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
		printBackupList(decodedJson)
	}
}

// formats result and prints it in a nice way
func printBackupList(decodedJson BackupListResponse){
	logger.Debugf("%+v", decodedJson)
	NameLentgh, StateLength, JobIdLength, StartTimeLenght, NextRunLenght := 4, 5, 6, 5, 8
	for _, job := range decodedJson.Result {
		if utf8.RuneCountInString(job.Name) > NameLentgh {
			NameLentgh = utf8.RuneCountInString(job.Name)
		}
		if utf8.RuneCountInString(job.State) > StateLength {
			StateLength = utf8.RuneCountInString(job.State)
		}
		if utf8.RuneCountInString(job.BackupJobId) > JobIdLength {
			JobIdLength = utf8.RuneCountInString(job.BackupJobId)
		}
		if !job.StartTime.IsZero() {
			if utf8.RuneCountInString(job.StartTime.String()) > StartTimeLenght {
				StartTimeLenght = utf8.RuneCountInString(job.StartTime.String())
			}
		}
		if !job.NextRun.IsZero() {
			if utf8.RuneCountInString(job.NextRun.String()) > NextRunLenght {
				NextRunLenght = utf8.RuneCountInString(job.NextRun.String())
			}
		}
	}
	// table header
	tableTemplate := "%" + strconv.Itoa(NameLentgh) + "s | %" + strconv.Itoa(StateLength) + "s | %" +
		strconv.Itoa(JobIdLength) + "s | %" + strconv.Itoa(StartTimeLenght) +  "s | %" + strconv.Itoa(NextRunLenght) +
		"s\n"
	fmt.Printf(tableTemplate, "Name", "State", "Job Id", "Start", "Next Run")
	for _, job := range decodedJson.Result {
		var startTime, nextRun, jobId string
		if job.StartTime.IsZero() {
			startTime = "n/a"
		} else {
			startTime = job.StartTime.String()
		}
		if job.NextRun.IsZero() {
			nextRun = "n/a"
		} else {
			nextRun = job.NextRun.String()
		}
		if job.BackupJobId == "" {
			jobId = "n/a"
		} else {
			jobId = job.BackupJobId
		}
		fmt.Printf(tableTemplate, job.Name, job.State, jobId, startTime, nextRun)
	}
}