package restore

import (
	"bufio"
	"bytes"
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.restore"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ListResponse struct {
	httpd.HttpStatusReply
	Result []shared.BackupJobStatus
}

type StartStopResponse struct {
	httpd.HttpStatusReply
	Result httpd.RestoreJobResponse
}

// doStart performs POST /restore/start and returns the parsed response, raw body or error.
// It has no side effects on stdout/stderr and never exits the process, so it is unit-testable.
func doStart(config clientConfig.Client, jobName, sourceJobId, targetName, restoreDir string,
	files []string, allFiles bool, exclusions []string) (StartStopResponse, []byte, error) {
	payload := httpd.RestoreJobRequest{
		Name:              jobName,
		SourceBackupJobId: sourceJobId,
		TargetName:        targetName,
		Files:             files,
		AllFiles:          allFiles,
		RestoreDir:        restoreDir,
		Exclusions:        exclusions,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return StartStopResponse{}, nil, fmt.Errorf("could not JSON encode request payload: %w", err)
	}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/restore/start", bytes.NewBuffer(encodedPayload))
	if err != nil {
		return StartStopResponse{}, nil, fmt.Errorf("error creating http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Username, config.Password)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return StartStopResponse{}, nil, err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return StartStopResponse{}, body, err
	}
	var decoded StartStopResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return StartStopResponse{}, body, fmt.Errorf("could not decode server response: %w", err)
	}
	return decoded, body, nil
}

// doStop performs POST /restore/stop — testable twin of Stop.
func doStop(config clientConfig.Client, jobName, restoreJobId string) (StartStopResponse, []byte, error) {
	payload := httpd.RestoreJobStopRequest{Name: jobName, RestoreJobId: restoreJobId}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return StartStopResponse{}, nil, fmt.Errorf("could not JSON encode request payload: %w", err)
	}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/restore/stop", bytes.NewBuffer(encodedPayload))
	if err != nil {
		return StartStopResponse{}, nil, fmt.Errorf("error creating http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Username, config.Password)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return StartStopResponse{}, nil, err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return StartStopResponse{}, body, err
	}
	var decoded StartStopResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return StartStopResponse{}, body, fmt.Errorf("could not decode server response: %w", err)
	}
	return decoded, body, nil
}

// doList performs GET /restore/list — testable twin of List.
func doList(config clientConfig.Client) (ListResponse, []byte, error) {
	req, err := http.NewRequest("GET", config.Address+ApiPrefix+"/restore/list", nil)
	if err != nil {
		return ListResponse{}, nil, fmt.Errorf("error creating http request: %w", err)
	}
	req.SetBasicAuth(config.Username, config.Password)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return ListResponse{}, nil, err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return ListResponse{}, body, err
	}
	var decoded ListResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ListResponse{}, body, fmt.Errorf("could not decode server response: %w", err)
	}
	return decoded, body, nil
}

// Start submits a POST /restore/start with the given parameters. When watch is true and the
// restore is successfully started, it attaches to /restore/watch for live progress.
func Start(config clientConfig.Client, jsonOutput bool, jobName string, sourceJobId string, targetName string,
	restoreDir string, files []string, allFiles bool, exclusions []string, watch bool) {
	decoded, body, err := doStart(config, jobName, sourceJobId, targetName, restoreDir, files, allFiles, exclusions)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		if err := utils.PpJson(body); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if watch {
			Watch(config, jsonOutput, jobName, decoded.Result.RestoreJobId)
		}
		os.Exit(0)
	}
	fmt.Printf("%s\nRestore job id '%s' has been allocated for this run of backup definition '%s'\n",
		decoded.Message, decoded.Result.RestoreJobId, decoded.Result.Name)
	if watch {
		Watch(config, jsonOutput, jobName, decoded.Result.RestoreJobId)
	}
}

func Stop(config clientConfig.Client, jsonOutput bool, jobName string, restoreJobId string) {
	decoded, body, err := doStop(config, jobName, restoreJobId)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		if err := utils.PpJson(body); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	fmt.Printf("%s\n", decoded.Message)
}

func List(config clientConfig.Client, jsonOutput bool) {
	decoded, body, err := doList(config)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		if err := utils.PpJson(body); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	printRestoreList(decoded)
}

// Watch is a functional twin of client/backup.Watch — the server emits identical SSE events
// for restores and backups, so the rendering logic is the same.
func Watch(config clientConfig.Client, jsonOutput bool, jobName string, restoreJobId string) {
	payload := httpd.RestoreWatchRequest{
		Name:         jobName,
		RestoreJobId: restoreJobId,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/restore/watch",
		bytes.NewBuffer(encodedPayload))
	if err != nil {
		fmt.Printf("Error starting the http client: %s\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Username, config.Password)

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		fmt.Println(err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		_, err := clientCommon.ValidateServerResponse(resp)
		if err != nil {
			fmt.Printf("%s\n", err)
			os.Exit(1)
		}
	}

	var seq uint64 = 0
	reader := bufio.NewReader(resp.Body)
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
						decodedJsonMessage.ObjectStoreType = "ERROR"
					}
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

func printRestoreList(decodedJson ListResponse) {
	logger.Debugf("%+v", decodedJson)
	if len(decodedJson.Result) == 0 {
		fmt.Println("No restore jobs are currently running.")
		return
	}
	NameLength, StateLength, JobIdLength, StartTimeLength := 4, 5, 6, 5
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
			if utf8.RuneCountInString(job.StartTime.String()) > StartTimeLength {
				StartTimeLength = utf8.RuneCountInString(job.StartTime.String())
			}
		}
	}
	tableTemplate := "%" + strconv.Itoa(NameLength) + "s | %" + strconv.Itoa(StateLength) + "s | %" +
		strconv.Itoa(JobIdLength) + "s | %" + strconv.Itoa(StartTimeLength) + "s\n"
	fmt.Printf(tableTemplate, "Name", "State", "Job Id", "Start")
	for _, job := range decodedJson.Result {
		var startTime, jobId string
		if job.StartTime.IsZero() {
			startTime = "n/a"
		} else {
			startTime = job.StartTime.String()
		}
		if job.BackupJobId == "" {
			jobId = "n/a"
		} else {
			jobId = job.BackupJobId
		}
		fmt.Printf(tableTemplate, job.Name, job.State, jobId, startTime)
	}
}
