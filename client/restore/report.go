package restore

import (
	"bytes"
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
	"unicode/utf8"
)

// ReportListResponse is the JSON envelope returned by POST /report/restore/list.
type ReportListResponse struct {
	httpd.HttpStatusReply
	Next   string                            `json:"next"`
	Result []httpd.ReportBackupListDbResults `json:"result"`
}

// ReportShowResponse is the JSON envelope returned by POST /report/restore/show.
type ReportShowResponse struct {
	httpd.HttpStatusReply
	Next   string                 `json:"next"`
	Result shared.BackupJobStatus `json:"result"`
}

// doReportList performs the HTTP call and returns the decoded response. It is a pure function
// suitable for unit testing (no os.Exit, no stdout).
func doReportList(config clientConfig.Client, jobName string, startTime, endTime time.Time, nextToken string) (ReportListResponse, []byte, error) {
	payload := httpd.ReportRestoreList{
		Name:           jobName,
		FromStartTime:  startTime.Format(time.RFC3339Nano),
		UntilStartTime: endTime.Format(time.RFC3339Nano),
	}
	if nextToken != "" {
		payload = httpd.ReportRestoreList{
			Name: jobName,
			Next: nextToken,
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return ReportListResponse{}, nil, fmt.Errorf("could not JSON encode request payload: %w", err)
	}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/report/restore/list", bytes.NewBuffer(encoded))
	if err != nil {
		return ReportListResponse{}, nil, fmt.Errorf("error building http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Username, config.Password)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return ReportListResponse{}, nil, err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return ReportListResponse{}, nil, err
	}
	var decoded ReportListResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ReportListResponse{}, body, fmt.Errorf("could not decode server response: %w", err)
	}
	return decoded, body, nil
}

// doReportShow performs the HTTP call and returns the decoded response.
func doReportShow(config clientConfig.Client, jobName, jobId string) (ReportShowResponse, []byte, error) {
	payload := httpd.ReportRestoreJob{
		Name:  jobName,
		JobId: jobId,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ReportShowResponse{}, nil, fmt.Errorf("could not JSON encode request payload: %w", err)
	}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/report/restore/show", bytes.NewBuffer(encoded))
	if err != nil {
		return ReportShowResponse{}, nil, fmt.Errorf("error building http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Username, config.Password)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return ReportShowResponse{}, nil, err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return ReportShowResponse{}, nil, err
	}
	var decoded ReportShowResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ReportShowResponse{}, body, fmt.Errorf("could not decode server response: %w", err)
	}
	return decoded, body, nil
}

// ReportList is the CLI entry point for "cloudbackup client restore report list".
func ReportList(config clientConfig.Client, jsonOutput bool, jobName string, startTime, endTime time.Time) {
	firstPage := true
	var nextToken string

	for {
		decoded, body, err := doReportList(config, jobName, startTime, endTime, nextToken)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if jsonOutput {
			if err := utils.PpJson(body); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if decoded.Next == "" {
				os.Exit(0)
			}
			nextToken = decoded.Next
		} else {
			printRestoreReportList(decoded, firstPage)
			if decoded.Next == "" {
				os.Exit(0)
			}
			firstPage = false
			nextToken = decoded.Next
		}
	}
}

// ReportShow is the CLI entry point for "cloudbackup client restore report show".
func ReportShow(config clientConfig.Client, jsonOutput bool, jobName, jobId string) {
	decoded, body, err := doReportShow(config, jobName, jobId)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if jsonOutput {
		if err := utils.PpJson(body); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	clientCommon.PrintRestoreStatus(decoded.Result, true)
}

func printRestoreReportList(decoded ReportListResponse, showHeader bool) {
	logger.Debugf("%+v", decoded)
	JobIdLength, StateLength, DurationLength, StartTimeLength, EndTimeLength := 5, 5, 8, 10, 8
	for _, job := range decoded.Result {
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
	tableTemplate := "%" + strconv.Itoa(JobIdLength) + "s | %" + strconv.Itoa(StateLength) + "s | %" +
		strconv.Itoa(DurationLength) + "s | %" + strconv.Itoa(StartTimeLength) + "s | %" + strconv.Itoa(EndTimeLength) +
		"s\n"
	if showHeader {
		fmt.Printf(tableTemplate, "JobId", "State", "Duration", "Start Time", "End Time")
	}
	for _, job := range decoded.Result {
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
