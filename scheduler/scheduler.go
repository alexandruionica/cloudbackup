package scheduler

import (
	"time"
	log "github.com/sirupsen/logrus"
	"cloudbackup/shared"
)

const loggingContext = "scheduler"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})


func Start (cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup) {
	go daemon(cfgChange, SchedulerCommBackup)
	return
}

func daemon (cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup) {
	logger.Info("Starting scheduling component")
	const SleepSec = 1
	var receivedBackupCommand = shared.ReceiveBackupCommand{}
	// infinite loop
	for {
		select {
		case _ = <-cfgChange:
			{
				// TODO - actually implement reload
				logger.Debug("Scheduler reloading configuration")
			}
		case receivedBackupCommand = <-SchedulerCommBackup.ReceivedCommand:
			{
				// TODO - implement action on command
				logger.Debugf("Scheduler received command: %+v", receivedBackupCommand)
			}
		default:
			{
				// TODO - add code to launch and scheduled backups or restores
				//logger.Debugf("Sleeping for %d seconds", SleepSec)
				time.Sleep(SleepSec * time.Second)
			}
		}

	}
}