package client

import (
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"cloudbackup/misc"
	"cloudbackup/utils"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
)

const ApiPrefix = "/api/v1"
const loggingContext = "client.server-version"

type VersionResponse struct {
	httpd.HttpStatusReply
	Result misc.Version
}

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ListResponse struct {
	httpd.HttpStatusReply
	Next   string `json:"next"`
	Result []httpd.ReportBackupListDbResults
}

func RetrieveServerVersion(config clientConfig.Client, jsonOutput bool) {

	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", config.Address+ApiPrefix+"/report/version", nil)
	if err != nil {
		fmt.Printf("Error starting the http client: %s\n", err)
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
	var decodedJson VersionResponse
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
	} else {
		v := decodedJson.Result
		fmt.Printf("Server version: %s\nBuild date: %s\nOS: %s\nArch: %s\nRuntime: %s\nAWS SDK: %s\nAzure Blob "+
			"Storage SDK: %s\nGoogle Cloud Platform SDK: %s\n", v.CloudBackup, v.BuildDate, v.OS, v.Arch, v.Runtime,
			v.AwsSdk, v.AzureBlobStorageSdk, v.GcpStorageSdk)
	}
	os.Exit(0)
}
