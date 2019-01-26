package notifications

import (
	"cloudbackup/config"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
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
func Execute(config config.CfgTemplate, JobId string, JobType string, JobState string, JobName string, JobReport string) error {
	numErrors, numNotificators := 0, 0
	totalErrors := ""
	for _, emailEntry := range config.Notifications.Email {
		numNotificators += 1
		err := sendEmail(emailEntry, JobId, JobType, JobState, JobName, JobReport)
		if err != nil {
			numErrors += 1
			totalErrors = totalErrors + err.Error() + "; "
		}
	}

	for _, scriptEntry := range config.Notifications.Script {
		numNotificators += 1
		err := runScript(scriptEntry, JobId, JobType, JobState, JobName, JobReport)
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

func sendEmail (emailEntry config.NotificationEmail, JobId string, JobType string, JobState string, JobName string, JobReport string) error {
	logger.Debug("Sending email notification")
	// if we got here, all was good
	return nil
}

func runScript (scriptEntry config.NotificationScript, JobId string, JobType string, JobState string, JobName string, JobReport string) error {
	logger.Debugf("Running notification script '%s'", scriptEntry.Path)
	// if we got here, all was good
	return nil
}