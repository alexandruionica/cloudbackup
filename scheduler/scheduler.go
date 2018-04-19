package scheduler

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/config"
	"cloudbackup/shared"
	"github.com/satori/go.uuid"
	"fmt"
	"time"
)

const loggingContext = "scheduler"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})


func Start (cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
	backupJobsState *shared.BackupJobsState, configuration *config.RuntimeConfig) {
	go eventProcessor(cfgChange, SchedulerCommBackup, backupJobsState, configuration)
	return
}

func eventProcessor(cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
	backupJobsState *shared.BackupJobsState, configuration *config.RuntimeConfig) {
	logger.Info("Starting scheduling component")
	//const SleepSec = 1
	var receivedBackupCommand = shared.ReceiveBackupCommand{}
	// while a copy, some of the data is pointers so locking is still needed
	serverConfigCopy := configuration.GetWithLock(loggingContext + ".eventProcessor")
	// infinite loop
	for {
		select {
		case _ = <-cfgChange:
			{
				logger.Debug("Scheduler reloading configuration")
				// while a copy, some of the data is pointers so locking is still needed
				serverConfigCopy = configuration.GetWithLock(loggingContext + ".eventProcessor")
				// TODO - notify cron scheduler to reload too
			}
		case receivedBackupCommand = <-SchedulerCommBackup.ReceivedCommand:
			{
				logger.Debugf("Scheduler received command: %+v", receivedBackupCommand)
				select {
				case SchedulerCommBackup.SendResponse <- processBackupCommand(receivedBackupCommand, backupJobsState,
					serverConfigCopy):
					logger.Debugf("Scheduler response for '%s' request for backup job '%s' having request " +
						"id '%s'", receivedBackupCommand.Command, receivedBackupCommand.Name, receivedBackupCommand.Id)
				case <-time.After(5 * time.Second):
					logger.Warnf("Scheduler response for '%s' request for backup job '%s' having request " +
						"id '%s' has timed out after 5 seconds as no receiver was ready", receivedBackupCommand.Command,
						receivedBackupCommand.Name, receivedBackupCommand.Id)
				}
			}
		//default:
		//	{
		//		// TODO - add code to launch and scheduled backups or restores; actually add a separate routine
		// which communicates with this one
		//		//logger.Debugf("Sleeping for %d seconds", SleepSec)
		//		time.Sleep(SleepSec * time.Second)
		//	}
		}

	}
}

func processBackupCommand (receivedBackupCommand shared.ReceiveBackupCommand, backupJobsState *shared.BackupJobsState,
	serverConfigCopy config.CfgTemplate) shared.ResponseBackupCommand {
	// TODO - implement validation in order to check if another job hasn't started for the same name
	// (there is an up to 60 sec delay between the httpd request and the scheduler). A lock for starting jobs may be useful
	switch receivedBackupCommand.Command {
	case "start": {
		startJobUuid := uuid.NewV4().String()
		err := backupJobsState.MarkRunning(receivedBackupCommand.Name, loggingContext + ".processBackupCommand",
			startJobUuid)
		if err != nil {
			return shared.ResponseBackupCommand{
				Name: receivedBackupCommand.Name,
				Id: receivedBackupCommand.Id,
				Message: err.Error(),
				Err: true,
			}
		}
		startBackup(receivedBackupCommand.Name, startJobUuid, serverConfigCopy)
		return shared.ResponseBackupCommand{
			Name: receivedBackupCommand.Name,
			Id: receivedBackupCommand.Id,
			BackupJobId: startJobUuid,
			Err: false,
		}
	}
	case "stop": {
		if backupJobsState.IsRunning(receivedBackupCommand.Name, receivedBackupCommand.BackupJobId ,
			loggingContext + ".processBackupCommand"){
			stopBackup(receivedBackupCommand.Name, receivedBackupCommand.BackupJobId, serverConfigCopy)
			return shared.ResponseBackupCommand{
				Name: receivedBackupCommand.Name,
				Id: receivedBackupCommand.Id,
				BackupJobId: receivedBackupCommand.BackupJobId,
				Err: false,
			}
		} else {
			return shared.ResponseBackupCommand{
				Name: receivedBackupCommand.Name,
				Id: receivedBackupCommand.Id,
				Message: shared.ErrJobAlreadyStopped,
				Err: true,
			}
		}

	}
	default:
		return shared.ResponseBackupCommand{
			Name: receivedBackupCommand.Name,
			Id: receivedBackupCommand.Id,
			Err: true,
			Message: fmt.Sprintf("Scheduler received command %s which is not one of 'start' or 'stop'. This is" +
				" a bug", receivedBackupCommand.Name),
		}
	}
}

// TODO - add actual implementation; also figure out how to deal with the SQL connection sharing
func startBackup (name string, jobUuid string, serverConfigCopy config.CfgTemplate){
	logger.Infof("Starting backup job having name '%s' with allocated job id '%s'", name, jobUuid)
}

// TODO - add actual implementation; also figure out how to deal with the SQL connection sharing
func stopBackup (name string, jobUuid string, serverConfigCopy config.CfgTemplate){
	// TODO - if $jobUuid is empty string then stop whatever is the current running backup for the given $name (and
	// figure out the UUID in order to correctly log it in the below logger call
	logger.Infof("Stopping backup job having name '%s' with allocated job id '%s'", name, jobUuid)
}