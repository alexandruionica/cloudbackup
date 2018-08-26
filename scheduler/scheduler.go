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
	"cloudbackup/database"
	"context"
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
	// infinite loop
	for {
		select {
		case _ = <-SchedulerCommBackup.Shutdown:
			{
				logger.Info("Scheduler requested to stop any running backups or restores and then exit")
				// TODO - add code to stop restores too (right now only backups are stopped)
				// while a copy, some of the data is pointers so locking is still needed
				serverConfigCopy := configuration.GetCopyWithLock(loggingContext + ".eventProcessor")
				stopAllBackups(backupJobsState, serverConfigCopy)
				// Signal back on the same channel that scheduler is done cleaning up
				SchedulerCommBackup.Shutdown <- true
				return
			}
		case _ = <-cfgChange:
			{
				logger.Info("Scheduler reloading configuration")

				// TODO - notify cron scheduler to reload
			}
		case receivedBackupCommand = <-SchedulerCommBackup.ReceivedCommand:
			{
				logger.Debugf("Scheduler received command: %+v", receivedBackupCommand)
				// while a copy, some of the data is pointers so locking is still needed
				serverConfigCopy := configuration.GetCopyWithLock(loggingContext + ".eventProcessor")
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
		// TODO - check also that a restore isn't running for the same backup name (to implement when restores are implemented)
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

			cancelFunc, err := backupJobsState.GetCancelFunctionForJob(receivedBackupCommand.Name,
				receivedBackupCommand.BackupJobId)
			// if we got an error than something else has already marked the backup job as != "running"
			if err != nil {
				msg := fmt.Sprintf("It seems that while trying to get the signalling channel for backup job " +
					"'%s' having id '%s' that another process changed the job state to something else than 'running'",
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

			// request job to stop (this is a non blocking call, the backup job may finish considerably later)
			signalBackupToStop(cancelFunc, receivedBackupCommand.Name, receivedBackupCommand.BackupJobId)

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
	serverConfigCopy.Mutex.RLock()
	for _, backup := range serverConfigCopy.Backup {
		if backup.Name == name{
			backupConfig = backup
			break
		}
	}
	serverConfigCopy.Mutex.RUnlock()

	// TODO - update some status report object and pass it to scan.Path()

	ctx, err := backupJobsState.GetContextForJob(name, jobUuid)
	// if we got an error than something else has already marked the backup job as != "running"
	if err != nil {
		logger.Errorf("It seems that while starting up backup job '%s' having id '%s' another process stopped " +
			"this backup job run", name, jobUuid)
		// it is assumed that whatever process requested the backup to be stopped will also take care of fully
		// stopping it
		return
	}

	// get DB connection pointer
	db, err := database.Start(serverConfigCopy.DataDir, name)
	// the backup can not run as we can't initialise/connect to the database
	if err != nil {
		// TODO - mark the backup as failed (failed to start)
		cleanupAfterBackup(name, jobUuid, backupConfig, backupJobsState)
		return
	}

	// examine each path listed and backup contained files/directories if needed
	for _, path := range backupConfig.Paths {
		select {
		case <-ctx.Done():
			{
				logger.Infof("Cancelling running backup job '%s' having id '%s'", name, jobUuid)
				break
			}
		default:
			// backupJobsState MUST be a pointer
			exiting, err := scan.Path(ctx, path, backupConfig, backupJobsState, false, db)
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
	database.CloseDb(db, name)
	cleanupAfterBackup(name, jobUuid, backupConfig, backupJobsState)
}

// TODO - add actual implementation; also figure out how to deal with the SQL connection sharing
func cleanupAfterBackup(name string, jobUuid string, backupConfig config.Backup,
	backupJobsState *shared.BackupJobsState){
	// in case the backup completed successfully then nothing called the cancel() function and if we don't do it then
	//  it will leak at least a channel. Calling it for an already cancelled backup will not cause any issue
	cancel, err := backupJobsState.GetCancelFunctionForJob(name, jobUuid)
	if err != nil {
		logger.Warnf("While trying cleanup the cancel context for backup '%s' having uuid '%s' the '%s' error " +
			"was encountered. This will leak some memory.", name, jobUuid, err )
	} else {
		cancel()
	}

	// TODO - implement stop; in the mean time sleep for 20 seconds and then mark job as stopped
	time.Sleep(20 * time.Second)

	// set state to "stopping"; ignore any errors (job may be set to state "stopping" already if stop was requested
	//  via the API but stop can also be triggered due to SIGTERM/SIGINT being received
	_ = backupJobsState.MarkStopped(name, loggingContext + ".cleanupAfterBackup", jobUuid, false) // #nosec


	// TODO - before MarkStopped(stopped=true) copy report (state) somewhere

	// set state to "stopped"
	err = backupJobsState.MarkStopped(name, loggingContext + ".cleanupAfterBackup",
		jobUuid, true)
	if err != nil {
		logger.Warnf("Encountered an error when trying to mark backup job '%s' having job id '%s' as 'stopped'. " +
			"The error was: %s", name, jobUuid, err)
	}

	// TODO - close SQL connection
}

// non-blocking function which signals a given backup job that it should stop whatever is doing and exit
func signalBackupToStop(cancelFunction context.CancelFunc, name string, jobUuid string){
	// TODO - if $jobUuid is empty string then stop whatever is the current running backup for the given $name (and
	// figure out the UUID in order to correctly log it in the below logger call
	logger.Infof("Signalling backup job having name '%s' with allocated job id '%s' to stop", name, jobUuid)
	// cancel running backup; this is a non blocking call and the running backup may keep going for a while before it
	// exits
	cancelFunction()
	}

func stopAllBackups (backupJobsState *shared.BackupJobsState, serverConfigCopy config.CfgTemplate){
	if len(backupJobsState.Running) >0 {
		logger.Info("Stopping all running backup jobs")
	}

	log.WithFields(log.Fields{"context": loggingContext + ".stopAllBackups"}).Debug("Acquiring read lock before" +
		" reading the backup jobs struct")
	backupJobsState.Lock.RLock()
	for _, job := range backupJobsState.Running {
		// find the config for this backup job only; lock config copy before walking the "Backup" slice
		serverConfigCopy.Mutex.RLock()
		log.WithFields(log.Fields{"context": loggingContext + ".stopAllBackups"}).Debug("Acquiring read lock " +
			"before reading the copy of the server config struct")
		for _, backup := range serverConfigCopy.Backup {
			if backup.Name == job.Name{
				// this is a non blocking call
				signalBackupToStop(job.Cancel, job.Name, job.BackupJobId)
				break
			}
		}
		serverConfigCopy.Mutex.RUnlock()
		log.WithFields(log.Fields{"context": loggingContext + ".stopAllBackups"}).Debug("Read lock released after" +
			" reading the copy of the server config struct")
	}
	backupJobsState.Lock.RUnlock()
	log.WithFields(log.Fields{"context": loggingContext + ".stopAllBackups"}).Debug("Read lock released after" +
		" reading the backup jobs struct")

	waitForAllBackupToBeStopped(backupJobsState)
	logger.Info("All running backup jobs have exited, as requested.")
}

// wait until the list of running backups has 0 length
func waitForAllBackupToBeStopped(backupJobsState *shared.BackupJobsState) {
	for {
		backupJobsState.Lock.RLock()
		if len(backupJobsState.Running) == 0 {
			break
		}
		backupJobsState.Lock.RUnlock()
		time.Sleep(1 * time.Second)
	}
}