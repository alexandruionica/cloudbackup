package backup

import (
	"net/http"
	"fmt"
	"os"
	"unicode/utf8"
	"strconv"
	"encoding/json"
	"bytes"

	log "github.com/sirupsen/logrus"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"cloudbackup/utils"
	clientConfig "cloudbackup/client/config"
	clientCommon "cloudbackup/client/common"
	"bufio"
	"io"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.backup"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ListResponse struct {
	httpd.HttpStatusReply
	Result []shared.BackupJobStatus
}

type StartStopResponse struct {
	httpd.HttpStatusReply
	Result httpd.BackupJob
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
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	var decodedJson ListResponse
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
		printBackupList(decodedJson)
	}
}

func Start(config clientConfig.Client, jsonOutput bool, jobName string) {
	payload := struct{Name string `json:"name"`}{jobName,}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address + ApiPrefix + "/backup/start",
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
	var decodedJson StartStopResponse
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
		os.Exit(0)
	} else {
		fmt.Printf("%s\nJob id '%s' has been allocated for this run of backup job '%s'\n", decodedJson.Message,
			decodedJson.Result.JobId, decodedJson.Result.Name)
	}
}

func Stop(config clientConfig.Client, jsonOutput bool, jobName string, JobId string) {
	payload := httpd.BackupJob{
		Name: jobName,
		JobId: JobId,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address + ApiPrefix + "/backup/stop",
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
	var decodedJson StartStopResponse
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
		os.Exit(0)
	} else {
		fmt.Printf("%s\n", decodedJson.Message)
	}
}

func DryRun(config clientConfig.Client, jsonOutput bool, jobName string) {
	payload := struct{Name string `json:"name"`}{jobName,}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address + ApiPrefix + "/backup/dryrun",
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
	// clientCommon.ValidateServerResponse() reads the whole response body and then closes it and this won't work with
	//  http2 SSE (or will buffer all responses) so we want to trigger this only if something went wrong
	if resp.StatusCode != 200 {
		_, err := clientCommon.ValidateServerResponse(resp)
		if err != nil {
			fmt.Printf("%s\n", err)
			os.Exit(1)
		}
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("While parsing response from server, the following error was encountered: '%s'\n", err)
			os.Exit(1)
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			if jsonOutput {
				fmt.Printf(string(line)[6:])
			} else {
				var decodedJsonMessage shared.ScanEvalItemReport
				err := json.Unmarshal(line[6:], &decodedJsonMessage)
				if err != nil {
					fmt.Printf(string(line)[6:])
				} else {
					incl := "include"
					inclAppend := ""
					fType := " " + decodedJsonMessage.Type
					if decodedJsonMessage.Excluded {
						incl = "exclude"
						inclAppend = fmt.Sprintf( " matching exclusion rule: '%s'", decodedJsonMessage.ExclusionExpr)
						// type is unknown so we'll skip printing this field
						fType = ""
					}
					fmt.Printf("%s%s %s%s\n", incl, fType, decodedJsonMessage.Name, inclAppend)
				}
			}
		}
	}
}

// for a "list" command this formats the result and prints it in a nice way
func printBackupList(decodedJson ListResponse){
	logger.Debugf("%+v", decodedJson)
	NameLength, StateLength, JobIdLength, StartTimeLenght, NextRunLenght := 4, 5, 6, 5, 8
	for _, job := range decodedJson.Result {
		if utf8.RuneCountInString(job.Name) > NameLength {
			NameLength = utf8.RuneCountInString(job.Name)
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
	tableTemplate := "%" + strconv.Itoa(NameLength) + "s | %" + strconv.Itoa(StateLength) + "s | %" +
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
