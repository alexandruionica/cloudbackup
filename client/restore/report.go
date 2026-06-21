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
	"time"
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
			clientCommon.PrintJobListTable(decoded.Result, firstPage)
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
