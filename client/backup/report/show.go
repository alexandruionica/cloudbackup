package report

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
)

type ShowResponse struct {
	httpd.HttpStatusReply
	Next   string `json:"next"`
	Result shared.BackupJobStatus
}

func Show(config clientConfig.Client, jsonOutput bool, jobName string, JobId string) {
	payload := httpd.ReportBackupJob{
		Name:  jobName,
		JobId: JobId,
	}

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Could not JSON encode request payload. Received error was: %s", err)
		os.Exit(1)
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", config.Address+ApiPrefix+"/report/backup/show",
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

	var decodedJson ShowResponse
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
		clientCommon.PrintBackupStatus(decodedJson.Result, true)
	}

}
