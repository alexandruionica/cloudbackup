package common

import (
	"cloudbackup/shared"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"cloudbackup/httpd"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "client.common"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// PrintJobListTable prints a job report list as an aligned "JobId | State | Duration | Start Time | End Time"
// table. Column widths are sized to the content and the header is printed only when showHeader is true (so
// paginated callers print it once). Shared by the backup-report and restore-report "list" commands, whose
// rows are both []httpd.ReportBackupListDbResults.
func PrintJobListTable(jobs []httpd.ReportBackupListDbResults, showHeader bool) {
	logger.Debugf("%+v", jobs)
	JobIdLength, StateLength, DurationLength, StartTimeLength, EndTimeLength := 5, 5, 8, 10, 8 // minimum is the lenght of the column headers
	for _, job := range jobs {
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
	for _, job := range jobs {
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
		if !decodedJson.EndTime.IsZero() && decodedJson.State != "running" {
			fmt.Printf("End time: %s\n", decodedJson.EndTime.String())
			fmt.Printf("Duration: %s\n", decodedJson.EndTime.Sub(decodedJson.StartTime).Round(time.Second))
		} else {
			fmt.Printf("Duration so far: %s\n", time.Since(decodedJson.StartTime).Round(time.Second))
		}
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
		// client-side-encryption counters
		fmt.Printf("Files skipped because path collides with the .cbcrypt reserved namespace: %d\n", decodedJson.StatsCounters["skipped_reserved_path"])
		fmt.Printf("Files skipped because their encrypted size exceeds the target's MaxObjectSize: %d\n", decodedJson.StatsCounters["skipped_too_large_for_target"])
		fmt.Printf("Keystore inconsistency events (sidecar missing but local DB has encrypted files): %d\n", decodedJson.StatsCounters["keystore_inconsistent"])
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

// PrintRestoreStatus is the restore-side counterpart of PrintBackupStatus. Restore jobs track a
// different, much smaller set of counters (populated in restore/restore.go and seeded in
// shared.BackupJobsState.MarkRestoreRunning), so reusing PrintBackupStatus would render every
// value as 0. alwaysExpand mirrors the backup printer: when false, only running jobs show the
// expanded body.
func PrintRestoreStatus(decodedJson shared.BackupJobStatus, alwaysExpand bool) {
	fmt.Printf("Name: %s\n", decodedJson.Name)
	fmt.Printf("State: %s\n", decodedJson.State)
	if decodedJson.State == "running" || alwaysExpand {
		fmt.Printf("Current operation: %s\n", decodedJson.StatsText["current_operation"])
		fmt.Printf("Job id: %s\n", decodedJson.BackupJobId)
		fmt.Printf("Start time: %s\n", decodedJson.StartTime.String())
		if !decodedJson.EndTime.IsZero() && decodedJson.State != "running" {
			fmt.Printf("End time: %s\n", decodedJson.EndTime.String())
			fmt.Printf("Duration: %s\n", decodedJson.EndTime.Sub(decodedJson.StartTime).Round(time.Second))
		} else {
			fmt.Printf("Duration so far: %s\n", time.Since(decodedJson.StartTime).Round(time.Second))
		}
		fmt.Printf(" 1 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate1Min)))
		fmt.Printf(" 5 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate5Min)))
		fmt.Printf("15 minute rate: %s/s\n", humanize.Bytes(uint64(decodedJson.Rate15Min)))
		fmt.Printf("Files restored: %d\n", decodedJson.StatsCounters["restored_files"])
		fmt.Printf("Directories restored: %d\n", decodedJson.StatsCounters["restored_directories"])
		fmt.Printf("Symlinks restored: %d\n", decodedJson.StatsCounters["restored_symlinks"])
		fmt.Printf("Files that failed to restore: %d\n", decodedJson.StatsCounters["failed_to_restore_files"])
		fmt.Printf("Items skipped because they were marked deleted: %d\n", decodedJson.StatsCounters["skipped_delete_markers"])
		fmt.Printf("Files where the keystore UUID in the header doesn't match the current sidecar: %d\n", decodedJson.StatsCounters["decrypt_keystore_mismatch"])
		fmt.Printf("Bytes written to disk for restored files: %s\n", humanize.Bytes(decodedJson.StatsCounters["bytes_restored"]))
		fmt.Printf("Current file being processed: %s\n", decodedJson.StatsText["current_file"])
	}
}
