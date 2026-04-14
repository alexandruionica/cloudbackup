package common

import (
	"cloudbackup/shared"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"net/http"
	"time"

	"cloudbackup/httpd"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "client.common"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// some basic validation of responses received from the Cloudbackup API server
func ValidateServerResponse(resp *http.Response) ([]byte, error) {
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Debugf("Received error when trying to close response body. Error was: %s", err)
		}
	}()
	// check we can read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Debugf("%s %+v", err, resp)
		return body, fmt.Errorf("cloud not process the response body received from the server. "+
			"The error was: %s", err)
	}

	// check we actually got JSON
	var decodedJson httpd.HttpStatusReply
	err = json.Unmarshal(body, &decodedJson)
	if err != nil {
		return body, fmt.Errorf("could not decode the JSON response received from server. Error "+
			"was: %s", err)
	}

	// http status code should be 200
	if resp.StatusCode != 200 {
		return body, errors.New(decodedJson.Message)
	}

	// 	// Code, Message fields should never be empty
	if decodedJson.Code == "" {
		return body, errors.New("invalid response from server: mandatory top level field 'Code' is empty")
	}
	if decodedJson.Message == "" {
		return body, errors.New("invalid response from server: mandatory top level field 'Message' is empty")
	}
	return body, nil
}

// for a "backup status" or "backup report show" command this formats the result and prints it in a nice way
func PrintBackupStatus(decodedJson shared.BackupJobStatus, alwaysExpand bool) {
	fmt.Printf("Name: %s\n", decodedJson.Name)
	fmt.Printf("State: %s\n", decodedJson.State)
	if decodedJson.State == "running" || alwaysExpand {
		fmt.Printf("Current operation: %s\n", decodedJson.StatsText["current_operation"])
		fmt.Printf("Job id: %s\n", decodedJson.BackupJobId)
		fmt.Printf("Start time: %s\n", decodedJson.StartTime.String())
		fmt.Printf("Duration so far: %s\n", time.Since(decodedJson.StartTime).Round(time.Second))
		if len(decodedJson.ObjectStoreRates) < 2 {
			fmt.Printf(" 1 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate1Min)))
			fmt.Printf(" 5 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate5Min)))
			fmt.Printf("15 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate15Min)))
		} else {
			fmt.Printf("Global  1 minute rate: %7s/s ", humanize.Bytes(uint64(decodedJson.Rate1Min)))
			for _, objectStoreRate := range decodedJson.ObjectStoreRates {
				fmt.Printf("| target %s  1 minute rate: %7s/s ", objectStoreRate.Name, humanize.Bytes(uint64(objectStoreRate.Rate1Min)))
			}
			fmt.Println("")

			fmt.Printf("Global  5 minute rate: %7s/s ", humanize.Bytes(uint64(decodedJson.Rate5Min)))
			for _, objectStoreRate := range decodedJson.ObjectStoreRates {
				fmt.Printf("| target %s  5 minute rate: %7s/s ", objectStoreRate.Name, humanize.Bytes(uint64(objectStoreRate.Rate5Min)))
			}
			fmt.Println("")

			fmt.Printf("Global 15 minute rate: %7s/s ", humanize.Bytes(uint64(decodedJson.Rate15Min)))
			for _, objectStoreRate := range decodedJson.ObjectStoreRates {
				fmt.Printf("| target %s 15 minute rate: %7s/s ", objectStoreRate.Name, humanize.Bytes(uint64(objectStoreRate.Rate15Min)))
			}
			fmt.Println("")
		}
		// counters
		fmt.Printf("Examined directories: %d\n", decodedJson.StatsCounters["examined_directories"])
		fmt.Printf("Examined files: %d\n", decodedJson.StatsCounters["examined_files"])
		fmt.Printf("Examined symlinks: %d\n", decodedJson.StatsCounters["examined_symlinks"])
		fmt.Printf("Examined unordinary files: %d\n", decodedJson.StatsCounters["examined_unknown"])
		fmt.Printf("Files and directories excluded from examination: %d\n", decodedJson.StatsCounters["excluded"])
		fmt.Printf("Files and directories which could not be examined: %d\n", decodedJson.StatsCounters["failed_to_examine"])
		fmt.Printf("Directories for which a full listing of contents could not be done: %d\n", decodedJson.StatsCounters["failed_to_enumerate"])
		fmt.Printf("Files which got marked for upload and failed to upload: %d\n", decodedJson.StatsCounters["failed_to_upload_files"])
		fmt.Printf("Directories which got marked for upload and failed to upload: %d\n", decodedJson.StatsCounters["failed_to_upload_directories"])
		fmt.Printf("Symlinks which got marked for upload and failed to upload: %d\n", decodedJson.StatsCounters["failed_to_upload_symlinks"])
		fmt.Printf("Unordinary files which got marked for upload and failed to upload: %d\n", decodedJson.StatsCounters["failed_to_upload_unknown"])
		fmt.Printf("Files successfully uploaded: %d\n", decodedJson.StatsCounters["uploaded_files"])
		fmt.Printf("Directories for which properties where successfully uploaded: %d\n", decodedJson.StatsCounters["uploaded_directories"])
		fmt.Printf("Symlinks for which properties where successfully uploaded: %d\n", decodedJson.StatsCounters["uploaded_symlinks"])
		fmt.Printf("Files for which metadata only updates took place: %d\n", decodedJson.StatsCounters["updated_metadata_for_files"])
		fmt.Printf("Directories for which metadata only updates took place: %d\n", decodedJson.StatsCounters["updated_metadata_for_directories"])
		fmt.Printf("Symlinks for which metadata only updates took place: %d\n", decodedJson.StatsCounters["updated_metadata_for_symlinks"])
		// how many bytes (file content only) were read from disk
		fmt.Printf("File content read in order to upload: %s\n", humanize.Bytes(decodedJson.FileContentBytesRead))
		fmt.Printf("User provided scripts number/ran/failed: %d/%d/%d\n",
			decodedJson.StatsCounters["scripts_num"], decodedJson.StatsCounters["scripts_ran"],
			decodedJson.StatsCounters["scripts_failed"])
		//
		fmt.Printf("Errors encountered while building the list of deleted files/symlinks/dirs: %d\n", decodedJson.StatsCounters["failed_to_find_deleted"])
		fmt.Printf("Errors encountered while making a copy of the metadata database: %d\n", decodedJson.StatsCounters["database_copy_errors"])
		fmt.Printf("Deleted directories for which updating internal state failed: %d\n", decodedJson.StatsCounters["failed_to_mark_deleted_directories"])
		fmt.Printf("Deleted files for which updating internal state failed: %d\n", decodedJson.StatsCounters["failed_to_mark_deleted_files"])
		fmt.Printf("Deleted symlinks for which updating internal state failed: %d\n", decodedJson.StatsCounters["failed_to_mark_deleted_symlinks"])
		fmt.Printf("Directories detected to have been deleted and for which internal state was updated: %d\n", decodedJson.StatsCounters["marked_deleted_directories"])
		fmt.Printf("Files detected to have been deleted and for which internal state was updated: %d\n", decodedJson.StatsCounters["marked_deleted_files"])
		fmt.Printf("Symlinks detected to have been deleted and for which internal state was updated: %d\n", decodedJson.StatsCounters["marked_deleted_symlinks"])
		// text stats
		fmt.Printf("Current directory being processed: %s\n", decodedJson.StatsText["current_directory"])
		fmt.Printf("Current file being processed: %s\n", decodedJson.StatsText["current_file"])

	}
	var nextRun string
	if decodedJson.NextRun.IsZero() {
		nextRun = "n/a"
	} else {
		nextRun = decodedJson.NextRun.String()
	}
	fmt.Printf("Next scheduled run for this job: %s\n", nextRun)
}
