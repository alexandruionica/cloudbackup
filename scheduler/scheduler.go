package scheduler

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/config"
	"cloudbackup/shared"
	"github.com/satori/go.uuid"
	"fmt"
	"time"
	"cloudbackup/backup/scan"
	"cloudbackup/daemon/globals"
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
	globals.Stats.IncrementRoutines("other")
	defer globals.Stats.DecrementRoutines("other")

	logger.Info("Starting scheduling component")
	//const SleepSec = 1
	var receivedBackupCommand = shared.ReceiveBackupCommand{}
	// while a copy, some of the data is pointers so locking is still needed
	serverConfigCopy := configuration.GetWithLock(loggingContext + ".eventProcessor")
	// infinite loop
	for {
		select {
		case _ = <-SchedulerCommBackup.Shutdown:
			{
				logger.Info("Scheduler requested to stop any running backups or restores and then exit")
				// TODO - add code to stop restores too (right now only backups are stopped)
				stopAllBackups(backupJobsState, serverConfigCopy)
				// Signal back on the same channel that scheduler is done cleaning up
				SchedulerCommBackup.Shutdown <- true
				return
			}
		case _ = <-cfgChange:
			{
				logger.Info("Scheduler reloading configuration")
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
		//		// TODO - add code to launch any scheduled backups or restores; actually add a separate routine
		// which communicates with this one
		//		//logger.Debugf("Sleeping for %d seconds", SleepSec)
		//		time.Sleep(SleepSec * time.Second)
		//	}
		}

	}
}

func processBackupCommand (receivedBackupCommand shared.ReceiveBackupCommand, backupJobsState *shared.BackupJobsState,
	serverConfigCopy config.CfgTemplate) shared.ResponseBackupCommand {
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
		go runBackup(receivedBackupCommand.Name, startJobUuid, serverConfigCopy, backupJobsState)
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
			if backupJobsState.IsStopping(receivedBackupCommand.Name, receivedBackupCommand.BackupJobId ,
				loggingContext + ".processBackupCommand") {
				return shared.ResponseBackupCommand{
					Name: receivedBackupCommand.Name,
					Id: receivedBackupCommand.Id,
					Message: shared.ErrJobAlreadyStopping,
					Err: true,
				}
			}

			closeChan, err := backupJobsState.GetSignalChanForJob(receivedBackupCommand.Name,
				receivedBackupCommand.BackupJobId)
			// if we got an error than something else has already marked the backup job as != "running"
			if err != nil {
				msg := fmt.Sprintf("It seems that while trying to get the signalling channel for backup job " +
					"'%s' having id '%s' that another process change the job state to something else than 'running'",
					receivedBackupCommand.Name, receivedBackupCommand.BackupJobId)
				logger.Warn(msg)
				// it is assumed that whatever process requested the backup to be stopped will also take care of fully
				// stopping it
				return shared.ResponseBackupCommand{
					Name: receivedBackupCommand.Name,
					Id: receivedBackupCommand.Id,
					Message: shared.ErrJobNotFoundInRunningState,
					Err: true,
				}
			}

			// set job state to "stopping"
			err = backupJobsState.MarkStopped(receivedBackupCommand.Name, loggingContext + ".processBackupCommand",
				receivedBackupCommand.BackupJobId, false)
			if err != nil {
				return shared.ResponseBackupCommand{
					Name: receivedBackupCommand.Name,
					Id: receivedBackupCommand.Id,
					Message: err.Error(),
					Err: true,
				}
			}

			// this will block until something reads the channel so we'll launch it in a GO routine
			// this signals the runBackup() go routine that the current running backup should stop.
			go func(){
				closeChan <- true
			}()
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
func runBackup(name string, jobUuid string, serverConfigCopy config.CfgTemplate,
	backupJobsState *shared.BackupJobsState){
	globals.Stats.IncrementRoutines("runBackup")
	defer globals.Stats.DecrementRoutines("runBackup")
	logger.Infof("Starting backup job having name '%s' with allocated job id '%s'", name, jobUuid)

	// extract config for this backup job only
	var backupConfig config.Backup
	for _, backup := range serverConfigCopy.Backup {
		if backup.Name == name{
			backupConfig = backup
			break
		}
	}

	// TODO - set lock for SQL object and then pass it to scan.Path()
	// TODO - update some status report object and pass it to scan.Path()

	closeChan, err := backupJobsState.GetSignalChanForJob(name, jobUuid)
	// if we got an error than something else has already marked the backup job as != "running"
	if err != nil {
		logger.Errorf("It seems that while starting up backup job '%s' having id '%s' another process stopped " +
			"this backup job run", name, jobUuid)
		// it is assumed that whatever process requested the backup to be stopped will also take care of fully
		// stopping it
		return
	}

	// examine each path listed and backup contained files/directories if needed
	for _, path := range backupConfig.Paths {
		select {
		case <-closeChan:
			{
				logger.Infof("cancelling running backup job '%s' having id '%s'", name, jobUuid)
				break
			}
		default:
			// backupJobsState MUST be a pointer
			exiting, err := scan.Path(path, backupConfig, backupJobsState, closeChan, false)
			// Examine FIRST $exit and then $err
			if exiting {
				// TODO - mark backup as interrupted
				break
			}
			if err != nil {
				// TODO - mark backup as failed as some Major error was encountered

				break
			}
		}
	}
	stopBackup(name, jobUuid, backupConfig, backupJobsState, closeChan)
}

// TODO - add actual implementation; also figure out how to deal with the SQL connection sharing
func stopBackup (name string, jobUuid string, backupConfig config.Backup,
	backupJobsState *shared.BackupJobsState, closeChan chan bool ){
	// TODO - if $jobUuid is empty string then stop whatever is the current running backup for the given $name (and
	// figure out the UUID in order to correctly log it in the below logger call
	logger.Infof("Stopping backup job having name '%s' with allocated job id '%s'", name, jobUuid)
	// TODO - implement stop; in the mean time sleep for 20 seconds and then mark job as stopped
	time.Sleep(20 * time.Second)

	// set state to "stopping"; ignore any errors (job may be set to state "stopping" already if stop was requested
	//  via the API but stop can also be triggered due to SIGTERM/SIGINT being received
	_ = backupJobsState.MarkStopped(name, loggingContext + ".stopBackup", jobUuid, false) // #nosec

	// before exiting loop several times over the communication channel; nothing should be there but this ensures we
	// don't end up with a memory leak or a panic in case we got a bug
	for i := 0; i < 100; i++ {
		select {
		case <- closeChan:
			{
				logger.Warnf("signalling channel for backup job '%s' having id '%s' should have been empty " +
					"but there was a message on it", name, jobUuid)
			}
		default:
		}
	}
	close(closeChan)

	// TODO - before MarkStopped(stopped=true) copy report (state) somewhere

	// set state to "stopped"
	err := backupJobsState.MarkStopped(name, loggingContext + ".stopBackup",
		jobUuid, true)
	if err != nil {
		logger.Warnf("Encountered an error when trying to mark backup job '%s' having job id '%s' as 'stopped'. " +
			"The error was: %s", name, jobUuid, err)
	}

	// TODO - close SQL connection
}

func stopAllBackups (backupJobsState *shared.BackupJobsState, serverConfigCopy config.CfgTemplate){
	if len(backupJobsState.Running) >0 {
		logger.Info("Stopping all running backup jobs")
	}
	for _, job := range backupJobsState.Running {
		// find the config for this backup job only
		for _, backup := range serverConfigCopy.Backup {
			if backup.Name == job.Name{
				stopBackup(job.Name, job.BackupJobId, backup, backupJobsState, job.SignalClose)
				break
			}
		}
	}
}