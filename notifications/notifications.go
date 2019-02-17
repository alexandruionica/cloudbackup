package notifications

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/jordan-wright/email"
	log "github.com/sirupsen/logrus"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const loggingContext = "client.notification"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// returns how many notifications we have defined in the config file
func GetNumNotificators (notificationDefs config.Notification) int {
	return len(notificationDefs.Email) + len(notificationDefs.Script)
}

// Executes all defined (in the config file) notifications
// $config is a copy of the server's config file; $JobId is the uuid of the job
// $JobType is one of "backup", "restore" or "purge"; $JobState is one if "started", "finished", "failed", "cancelled",
// "crashed" and "test" ("test" is supposed to be used only when it is tested that notifications deliver as expected)
// $JobName will be non empty only for backup jobs and it represents the name of the job corresponding to the "backup"
// entry in the config file; JobReport is a JSON encoded string specific to each job type
// Returns: the number of scripts which failed to run; an error object
func Execute(config config.CfgTemplate, JobId string, JobType string, JobState string, JobName string, JobReport string, JobError string) (int, error) {
	numErrors, numNotificators, failedScripts := 0, 0, 0
	totalErrors := ""
	for _, emailEntry := range config.Notifications.Email {
		numNotificators += 1
		err := sendEmail(emailEntry, JobId, JobType, JobState, JobName, JobReport, JobError)
		if err != nil {
			numErrors += 1
			totalErrors = totalErrors + err.Error() + "; "
		}
	}

	for _, scriptEntry := range config.Notifications.Script {
		numNotificators += 1
		err := runScript(scriptEntry, JobId, JobType, JobState, JobName, JobReport, JobError)
		if err != nil {
			numErrors += 1
			failedScripts += 1
			totalErrors = totalErrors + err.Error() + "; "
		}
	}
	if numErrors > 0 {
		return failedScripts, errors.New(fmt.Sprintf("%d notification definitions were run and %d encountered " +
			"errors. The errors were: %s", numNotificators, numErrors, totalErrors))
	}
	// if we got here then all was good
	return failedScripts, nil
}

// Sends one email ...
// see description of Execute() for most of parameters except $emailEntry which represents email configuration as
// described in the configuration file
func sendEmail (emailEntry config.NotificationEmail, JobId string, JobType string, JobState string, JobName string,
	JobReport string, JobError string) error {
	logger.Debug("Sending email notification")

	e := email.NewEmail()
	if emailEntry.From == "" {
		e.From = prepareFromAddress()
	} else {
		e.From = emailEntry.From
	}
	e.To = []string{emailEntry.To}
	e.Cc = emailEntry.CC
	e.Subject = fmt.Sprintf("%s job \"%s\" has %s", JobType, JobName, JobState)
	var emailText []string
	emailText = append(emailText, fmt.Sprintf("%s job \"%s\" having id %s has %s", JobType, JobName, JobId,
		JobState))
	emailText = append(emailText, "Check backup server logs for more details")
	switch JobState {
	case "test":
		e.Subject = "Notification test"
		e.Text = []byte(fmt.Sprintf("Receiving this email proves that the backup server's SMTP(email) settings" +
			" are correct.\nNotification test job having id '%s' has completed successfully.", JobId))
	case "cancelled":
		e.Subject = fmt.Sprintf("%s job \"%s\" has been %s", JobType, JobName, JobState)
		emailText = []string{fmt.Sprintf("%s job \"%s\" having id %s has been %s", JobType, JobName, JobId, JobState)}
		emailText = append(emailText, "Check backup server logs for more details")
		e.Text = []byte(strings.Join(emailText,"\n"))
	case "failed": {
		if JobError != "" {
			e.Text = []byte(fmt.Sprintf("Job failed with error: %s\nCheck backup server logs for more details",
				JobError))
		}
	}
	case "crashed":
		emailText = append(emailText,"When a job is marked as crashed it means that there is a record of the job" +
			" starting but not record of it finishing, failing or being cancelled and the job is no longer running. " +
			"A potential scenario for this is " +
			"that the server got restarted, it crashed itself or the backup software encountered an issue and crashed. " +
			"For the last scenario, check backup server logs for more details")
	default:
		e.Text = []byte(strings.Join(emailText,"\n"))
	}

	if JobType == "backup" && JobReport != "" {
		var decodedJson shared.BackupJobStatus
		err := json.Unmarshal([]byte(JobReport), &decodedJson)
		if err != nil {
			logger.Debugf("While trying to json decode the job report, encountered error: %s", err)
		} else {
			// add html body to the email
			e.HTML = []byte(prepareHtmlEmail(emailText, decodedJson))
		}
	}

	var err error
	if emailEntry.User == "" {
		err = e.Send(emailEntry.Server + ":" + emailEntry.Port, nil)
	} else {
		err = e.Send(emailEntry.Server + ":" + emailEntry.Port, smtp.PlainAuth("", emailEntry.User,
			emailEntry.Pass, emailEntry.Server))
	}
	if err != nil {
		logger.Errorf("While trying to send a notification via email to '%s', the following error was " +
			"encountered: %s", emailEntry.To, err)
		return err
	}
	// if we got here, all was good
	if len(emailEntry.CC) > 0 {
		logger.Infof("Email to '%s' having cc: '%v' was sent successfully", emailEntry.To, emailEntry.CC)
	} else {
		logger.Infof("Email to '%s' sent successfully", emailEntry.To)
	}
	return nil
}

// runs a Notification script
func runScript(scriptEntry config.NotificationScript, JobId string, JobType string, JobState string, JobName string, JobReport string, JobError string) error {
	logger.Debugf("Running notification script '%s'", scriptEntry.Path)
	reportFile, err := utils.SetupTmpFileWithContent([]byte(JobReport),"cloudbackup_job_report_notification_")
	if err != nil {
		reportFile = ""
		logger.Warningf("While trying to setup a temporary file to hold the job report which would be passed " +
			"to notification script '%s', the following error was encountered: %s", scriptEntry.Path, err)
	}
	logger.Debugf("Running (without the single quotes): '%s' '%s' '%s' '%s' '%s' '%s' '%s'", scriptEntry.Path,
		JobType, JobName, JobId, JobState, JobError, reportFile)
	var cmd *exec.Cmd
	// on Windows, to run Powershell scripts, you need to call powershell.exe itself
	if strings.ToLower(filepath.Ext(scriptEntry.Path)) == ".ps1" && runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-File", scriptEntry.Path, JobType, JobName, JobId, JobState,
			JobError, reportFile) // #nosec
	} else {
		cmd = exec.Command(scriptEntry.Path, JobType, JobName, JobId, JobState, JobError, reportFile) // #nosec
	}

	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		msg := fmt.Sprintf("While executing notification script '%s', encountered error: %s\nScript " +
			"output was: %s", scriptEntry.Path, err, stdoutStderr)
		logger.Error(msg)
		return errors.New(msg)
	}
	// if we got here, all was good
	return nil
}

// figure out a default From address - this is used if the config doesn't have anything specified so we try to come up
// with something reasonable
func prepareFromAddress () string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return "cloudbackup@" + hostname
}


// produces a HTML body which will be the Email body (if clients can read html)
// $emailTextBody represents the plaintext variant of the email; decodedJson represents the last state of the job
func prepareHtmlEmail (emailTextBody []string, decodedJson shared.BackupJobStatus) (result string) {
	tdStyle := `style="border:1px solid #ddd;padding:3px;"`
	td := "<td " + tdStyle + " >"
	tr := "<tr>" + td
	header := `<html>
<head>
<style>
#report {
  border-collapse: collapse;
}
#report td, #report th {
  border: 1px solid #ddd;
  padding: 3px;
}
#report tr:nth-child(even){background-color: #f2f2f2;}
#report tr:hover {background-color: #ddd;}
#report th {
  padding-top: 7px;
  padding-bottom: 7px;
  text-align: left;
  background-color: #4CAF50;
  color: white;
}
</style></head><body>` + fmt.Sprintf("\n")
	result = header + "<b>" + strings.Join(emailTextBody,"<br>") +
		fmt.Sprintf("</b>\n<hr>\n<table id='report' style='border-collapse:collapse;'\n")

	result += tr + "Start time" + td + fmt.Sprintf("%s\n", decodedJson.StartTime)
	result += tr + "Duration" + td + fmt.Sprintf("%s\n", decodedJson.EndTime.Sub(decodedJson.StartTime).Round(time.Second))

	if len(decodedJson.ObjectStoreRates) < 2 {
		result += tr + "1 minute rate:" + td  + humanize.Bytes(uint64(decodedJson.Rate1Min)) + "/s" + fmt.Sprintf("\n")
		result += tr + "5 minute rate:" + td + humanize.Bytes(uint64(decodedJson.Rate5Min)) + "/s" + fmt.Sprintf("\n")
		result += tr + "15 minute rate:" + td + humanize.Bytes(uint64(decodedJson.Rate15Min)) + "/s" + fmt.Sprintf("\n")
	} else {
		result += tr + "Global 1 minute rate:" + td + humanize.Bytes(uint64(decodedJson.Rate1Min)) + "/s"
		for _, objectStoreRate := range decodedJson.ObjectStoreRates {
			result += td + fmt.Sprintf("target %s 1 minute rate:", objectStoreRate.Name) + td + humanize.Bytes(uint64(objectStoreRate.Rate1Min)) + "/s" + fmt.Sprintf("\n")
		}

		result += tr + "Global 5 minute rate:" + td + humanize.Bytes(uint64(decodedJson.Rate5Min)) + "/s"
		for _, objectStoreRate := range decodedJson.ObjectStoreRates {
			result += td + fmt.Sprintf("target %s 5 minute rate:", objectStoreRate.Name) + td + humanize.Bytes(uint64(objectStoreRate.Rate5Min)) + "/s" + fmt.Sprintf("\n")
		}

		result += tr + "Global 15 minute rate:" + td + humanize.Bytes(uint64(decodedJson.Rate15Min)) + "/s"
		for _, objectStoreRate := range decodedJson.ObjectStoreRates {
			result += td + fmt.Sprintf("target %s 15 minute rate:", objectStoreRate.Name) + td + humanize.Bytes(uint64(objectStoreRate.Rate15Min)) + "/s" + fmt.Sprintf("\n")
		}
	}
	var tdTmp string
	// counters
	if tdTmp = td; decodedJson.StatsCounters["scripts_failed"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "How many user supplied scripts failed" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["scripts_failed"])

	result += tr + "Examined directories" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["examined_directories"])
	if tdTmp = td; decodedJson.StatsCounters["examined_files"] == 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Examined files" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["examined_files"])
	result += tr + "Examined symlinks" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["examined_symlinks"])
	if tdTmp = td; decodedJson.StatsCounters["examined_unknown"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Examined unordinary files" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["examined_unknown"])

	result += tr + "Files and directories excluded from examination" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["excluded"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_examine"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Files and directories which could not be examined" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["failed_to_examine"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_enumerate"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Directories for which a full listing of contents could not be done" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["failed_to_enumerate"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_upload_files"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Files which got marked for upload and failed to upload" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["failed_to_upload_files"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_upload_directories"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Directories which got marked for upload and failed to upload" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["failed_to_upload_directories"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_upload_symlinks"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Symlinks which got marked for upload and failed to upload" + tdTmp + fmt.Sprintf("%d\n", decodedJson.StatsCounters["failed_to_upload_symlinks"])
	if tdTmp = td; decodedJson.StatsCounters["failed_to_upload_unknown"] > 0 {
		tdTmp = "<td " + tdStyle + " bgcolor='orange'>"
	}
	result += tr + "Unordinary files which got marked for upload and failed to upload" + tdTmp + fmt.Sprintf(" %d\n", decodedJson.StatsCounters["failed_to_upload_unknown"])
	result += tr + "Files successfully uploaded" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["uploaded_files"])
	result += tr + "Directories for which properties where successfully uploaded" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["uploaded_directories"])
	result += tr + "Symlinks for which properties where successfully uploaded" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["uploaded_symlinks"])
	result += tr + "Files for which metadata only updates took place" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["updated_metadata_for_files"])
	result += tr + "Directories for which metadata only updates took place" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["updated_metadata_for_directories"])
	result += tr + "Symlinks for which metadata only updates took place" + td + fmt.Sprintf("%d\n", decodedJson.StatsCounters["updated_metadata_for_symlinks"])
	// how many bytes (file content only) were read from disk
	result += tr + "File content read in order to upload" + td + fmt.Sprintf("%s\n", humanize.Bytes(decodedJson.FileContentBytesRead))
	// text stats
	if decodedJson.StatsText["current_directory"] != "" {
		result += tr + "Last directory processed" + td + fmt.Sprintf("%s\n", decodedJson.StatsText["current_directory"])
	}
	if decodedJson.StatsText["current_file"] != "" {
		result += tr + "Last file processed" + td + fmt.Sprintf("%s\n", decodedJson.StatsText["current_file"])
	}

	result += "</table></body></html>"
	return result
}