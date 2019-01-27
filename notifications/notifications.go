package notifications

import (
	"cloudbackup/config"
	"errors"
	"fmt"
	"github.com/jordan-wright/email"
	log "github.com/sirupsen/logrus"
	"net/smtp"
	"os"
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
func Execute(config config.CfgTemplate, JobId string, JobType string, JobState string, JobName string, JobReport string, JobError string) error {
	numErrors, numNotificators := 0, 0
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
			totalErrors = totalErrors + err.Error() + "; "
		}
	}
	if numErrors > 0 {
		return errors.New(fmt.Sprintf("%d notification definitions were run and %d encountered errors. The " +
			"errors were: %s", numNotificators, numErrors, totalErrors))
	}
	// if we got here then all was good
	return nil
}

func sendEmail (emailEntry config.NotificationEmail, JobId string, JobType string, JobState string, JobName string, JobReport string, JobError string) error {
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
	e.Text = []byte(fmt.Sprintf("Check backup server logs for more details"))
	switch JobState {
	case "test":
		e.Subject = "Notification test"
	case "cancelled":
		e.Subject = fmt.Sprintf("%s job \"%s\" has been %s", JobType, JobName, JobState)
	case "failed": {
		if JobError != "" {
			e.Text = []byte(fmt.Sprintf("Job failed with error: %s\nCheck backup server logs for more details",
				JobError))
		}
		e.Text = []byte(fmt.Sprintf("Check backup server logs for more details"))
	}
	case "crashed":
		e.Text = []byte(fmt.Sprintf("When a job is marked as crashed it means that there is a record of the job" +
			" starting but not record of it finishing, failing or being cancelled and the job is no longer running. " +
			"A potential scenario for this is " +
			"that the server got restarted, it crashed itself or the backup software encountered an issue and crashed. " +
			"For the last scenario, check backup server logs for more details"))
	}
	err := e.Send(emailEntry.Server + ":" + emailEntry.Port, smtp.PlainAuth("", emailEntry.User,
		emailEntry.Pass, emailEntry.Server))
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

func runScript (scriptEntry config.NotificationScript, JobId string, JobType string, JobState string, JobName string, JobReport string, JobError string) error {
	logger.Debugf("Running notification script '%s'", scriptEntry.Path)
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