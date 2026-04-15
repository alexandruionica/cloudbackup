package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dustin/go-humanize"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"bufio"
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"cloudbackup/utils"
	log "github.com/sirupsen/logrus"
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
	req, err := http.NewRequest("GET", config.Address+ApiPrefix+"/backup/list", nil)
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

func Status(config clientConfig.Client, jsonOutput bool, jobName string, jobId string) {
	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", config.Address+ApiPrefix+"/backup/list", nil)
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

	found := false
	var decodedJob shared.BackupJobStatus
	for _, jobStatus := range decodedJson.Result {
		if jobStatus.Name == jobName {
			if jobId != "" && jobStatus.BackupJobId == jobId {
				found = true
				decodedJob = jobStatus
			}
			if jobId == "" {
				found = true
				decodedJob = jobStatus
			}
			break
		}
	}
	if !found {
		if jobId != "" {
			fmt.Printf("No job having name %s and id %s was found\n", jobName, jobId)
		} else {
			fmt.Printf("No job having name %s was found\n", jobName)
		}
		os.Exit(1)
	}
	if jsonOutput {
		utils.Pp(decodedJob)
		os.Exit(0)
	} else {
		clientCommon.PrintBackupStatus(decodedJob, false)
	}
}

func Start(config clientConfig.Client, jsonOutput bool, jobName string, watch bool) {
	payload := struct {
		Name string `json:"name"`
	}{jobName}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/backup/start",
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
		if watch {
			Watch(config, jsonOutput, jobName, decodedJson.Result.JobId)
		}
		os.Exit(0)
	} else {
		fmt.Printf("%s\nJob id '%s' has been allocated for this run of backup job '%s'\n", decodedJson.Message,
			decodedJson.Result.JobId, decodedJson.Result.Name)
		if watch {
			Watch(config, jsonOutput, jobName, decodedJson.Result.JobId)
		}
	}
}

func Stop(config clientConfig.Client, jsonOutput bool, jobName string, JobId string) {
	payload := httpd.BackupJob{
		Name:  jobName,
		JobId: JobId,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/backup/stop",
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

func Watch(config clientConfig.Client, jsonOutput bool, jobName string, JobId string) {
	payload := httpd.BackupJob{
		Name:  jobName,
		JobId: JobId,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/backup/watch",
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

	var seq uint64 = 0
	reader := bufio.NewReader(resp.Body)
	//
	maxLenghtObjectStoreType := 5
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
				fmt.Print(string(line)[6:])
			} else {
				var decodedJsonMessage shared.WatchMessage
				err := json.Unmarshal(line[6:], &decodedJsonMessage)
				if err != nil {
					fmt.Print("\n" + string(line)[6:])
				} else {
					if seq == 0 {
						if seq+1 < decodedJsonMessage.Sequence {
							fmt.Printf("Before attaching %d files, directories and symlinks were processed\n", decodedJsonMessage.Sequence-(seq+1))
						}
						fmt.Printf("Percent     Rate   Store    Type   OpType Path Error\n")
						fmt.Printf("------- -------- ------- ------- -------- ---- -----\n")
						seq = decodedJsonMessage.Sequence
					}
					if utf8.RuneCountInString(decodedJsonMessage.ObjectStoreType) > maxLenghtObjectStoreType {
						maxLenghtObjectStoreType = utf8.RuneCountInString(decodedJsonMessage.ObjectStoreType)
					}
					fmtPrefix := "\n"
					if seq == decodedJsonMessage.Sequence {
						// redraw over the same line when the file name hasn't changed (meaning sequence number is unchanged)
						fmtPrefix = "\033[2K\r"
					} else {
						if seq+1 < decodedJsonMessage.Sequence {
							fmt.Printf("\nSKIPPED MESSAGES ABOUT %d files, directories and symlinks", decodedJsonMessage.Sequence-(seq+1))
						}
						seq = decodedJsonMessage.Sequence
					}
					errorField := ""
					if decodedJsonMessage.Error != "" {
						errorField = "#ERROR =====> " + decodedJsonMessage.Error
						// if we got an error then decodedJsonMessage.ObjectStoreType is empty so we'll add "ERROR" to it to make it clear there was an errror
						decodedJsonMessage.ObjectStoreType = "ERROR"
					}
					//
					// pad with spaces the ObjectStoreType (when empty) to align things nicely
					if decodedJsonMessage.ObjectStoreType == "" {
						decodedJsonMessage.ObjectStoreType = strings.Repeat(" ", maxLenghtObjectStoreType)
					}
					fmt.Printf(fmtPrefix+"%3d%% %7s/sec %"+strconv.Itoa(maxLenghtObjectStoreType)+"s %9s %9s %s %s", decodedJsonMessage.PercentDone,
						humanize.Bytes(uint64(decodedJsonMessage.Rate)), decodedJsonMessage.ObjectStoreType,
						decodedJsonMessage.ObjectType, decodedJsonMessage.OperationType, decodedJsonMessage.Path,
						errorField)
				}
			}
		}
	}
}

func DryRun(config clientConfig.Client, jsonOutput bool, jobName string) {
	payload := struct {
		Name string `json:"name"`
	}{jobName}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/backup/dryrun",
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
				fmt.Print(string(line)[6:])
			} else {
				var decodedJsonMessage shared.ScanEvalItemReport
				err := json.Unmarshal(line[6:], &decodedJsonMessage)
				if err != nil {
					fmt.Print(string(line)[6:])
				} else {
					marker := " "
					incl := "include"
					inclAppend := ""
					fType := " " + decodedJsonMessage.Type
					if decodedJsonMessage.Excluded {
						marker = "X"
						incl = "exclude"
						inclAppend = fmt.Sprintf(" matching exclusion rule: '%s'",
							decodedJsonMessage.ExclusionExpr)
						// type is unknown so we'll skip printing this field
						fType = ""
					}
					errAppend := ""
					if decodedJsonMessage.Error != "" {
						errAppend = fmt.Sprintf(" not possible due to error: '%s'", decodedJsonMessage.Error)
						marker = "X"
					}
					fmt.Printf("%s| %s%s %s%s%s\n", marker, incl, fType, decodedJsonMessage.Name, inclAppend,
						errAppend)
				}
			}
		}
	}
}

// for a "list" command this formats the result and prints it in a nice way
func printBackupList(decodedJson ListResponse) {
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
		strconv.Itoa(JobIdLength) + "s | %" + strconv.Itoa(StartTimeLenght) + "s | %" + strconv.Itoa(NextRunLenght) +
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
