package scheduler

import (
	"cloudbackup/backup"
	"cloudbackup/backup/scan"
	"cloudbackup/config"
	"cloudbackup/daemon/globals"
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/notifications"
	"cloudbackup/objectstore"
	"cloudbackup/restore"
	"cloudbackup/shared"
	"cloudbackup/watcher"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"strings"
	"time"
)

const loggingContext = "scheduler"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start(cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
	SchedulerCommRestore *shared.CommWithSchedulerForRestore,
	backupJobsState *shared.BackupJobsState, configuration *shared.RuntimeConfig) {
	// start component which listens for messages from http handlers and starts / stops backups & restores according to requests
	go eventProcessor(cfgChange, SchedulerCommBackup, SchedulerCommRestore, backupJobsState, configuration)
	// components which relays to clients real time info about the file/dir/symlink currently being backed up or restores
	go watcher.Start(backupJobsState.Watcher)
}

func eventProcessor(cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
	SchedulerCommRestore *shared.CommWithSchedulerForRestore,
	backupJobsState *shared.BackupJobsState, configuration *shared.RuntimeConfig) {
	globals.Stats.IncrementRoutines("other")
	defer globals.Stats.DecrementRoutines("other")

	logger.Debug("Starting scheduling component")
	//const SleepSec = 1
	var receivedBackupCommand = shared.ReceiveBackupCommand{}
	// infinite loop
	for {
		select {
		case <-SchedulerCommBackup.Shutdown:
			{
				logger.Debug("Scheduler requested to stop any running backups or restores and then exit")
				// TODO - add code to stop restores too (right now only backups are stopped)
				// while a copy, some of the data is pointers so locking is still needed
				serverConfigCopy := configuration.GetCopyWithLock(loggingContext + ".eventProcessor")
				stopAllBackups(backupJobsState, serverConfigCopy)
				// stop watcher (real time message multiplexer about file/dir/symlink currently being backedup/restored)
				backupJobsState.Watcher.Stop()
				// give the multiplexer a bit of time to get the signal and clean up
				time.Sleep(100 * time.Millisecond)
				// Signal back on the same channel that scheduler is done cleaning up
				SchedulerCommBackup.Shutdown <- true
				logger.Debug("Scheduler completed cleanup and is exiting.")
				return
			}
		case <-cfgChange:
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
					logger.Debugf("Scheduler response for '%s' request for backup job '%s' having request "+
						"id '%s'", receivedBackupCommand.Command, receivedBackupCommand.Name, receivedBackupCommand.Id)
				case <-time.After(5 * time.Second):
					logger.Warnf("Scheduler response for '%s' request for backup job '%s' having request "+
						"id '%s' has timed out after 5 seconds as no receiver was ready", receivedBackupCommand.Command,
						receivedBackupCommand.Name, receivedBackupCommand.Id)
				}
			}
		case receivedRestoreCommand := <-SchedulerCommRestore.ReceivedCommand:
			{
				logger.Debugf("Scheduler received restore command: %+v", receivedRestoreCommand)
				serverConfigCopy := configuration.GetCopyWithLock(loggingContext + ".eventProcessor")
				select {
				case SchedulerCommRestore.SendResponse <- processRestoreCommand(receivedRestoreCommand, backupJobsState,
					serverConfigCopy):
					logger.Debugf("Scheduler response for '%s' restore request for job '%s' having request id '%s'",
						receivedRestoreCommand.Command, receivedRestoreCommand.Name, receivedRestoreCommand.Id)
				case <-time.After(5 * time.Second):
					logger.Warnf("Scheduler response for '%s' restore request for job '%s' having request id '%s' "+
						"has timed out after 5 seconds as no receiver was ready", receivedRestoreCommand.Command,
						receivedRestoreCommand.Name, receivedRestoreCommand.Id)
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

func processBackupCommand(receivedBackupCommand shared.ReceiveBackupCommand, backupJobsState *shared.BackupJobsState,
	serverConfigCopy shared.CfgTemplate) shared.ResponseBackupCommand {
	switch receivedBackupCommand.Command {
	case "start":
		{
			startJobUuid, err := GenerateJobUuid(receivedBackupCommand.Name, backupJobsState, serverConfigCopy, "backup")
			if err != nil {
				return shared.ResponseBackupCommand{
					Name:    receivedBackupCommand.Name,
					Id:      receivedBackupCommand.Id,
					Message: err.Error(),
					Err:     true,
				}
			}
			// TODO - check also that a restore isn't running for the same backup name (to implement when restores are implemented)
			err = backupJobsState.MarkRunning(receivedBackupCommand.Name, loggingContext+".processBackupCommand",
				startJobUuid)
			if err != nil {
				return shared.ResponseBackupCommand{
					Name:    receivedBackupCommand.Name,
					Id:      receivedBackupCommand.Id,
					Message: err.Error(),
					Err:     true,
				}
			}
			go runBackup(receivedBackupCommand.Name, startJobUuid, serverConfigCopy, backupJobsState)
			return shared.ResponseBackupCommand{
				Name:        receivedBackupCommand.Name,
				Id:          receivedBackupCommand.Id,
				BackupJobId: startJobUuid,
				Err:         false,
			}
		}
	case "stop":
		{
			if backupJobsState.IsRunning(receivedBackupCommand.Name, receivedBackupCommand.BackupJobId,
				loggingContext+".processBackupCommand") {
				if backupJobsState.IsStopping(receivedBackupCommand.Name, receivedBackupCommand.BackupJobId,
					loggingContext+".processBackupCommand") {
					return shared.ResponseBackupCommand{
						Name:    receivedBackupCommand.Name,
						Id:      receivedBackupCommand.Id,
						Message: shared.ErrJobAlreadyStopping,
						Err:     true,
					}
				}

				cancelFunc, err := backupJobsState.GetCancelFunctionForJob(receivedBackupCommand.Name,
					receivedBackupCommand.BackupJobId)
				// if we got an error than something else has already marked the backup job as != "running"
				if err != nil {
					msg := fmt.Sprintf("It seems that while trying to get the signalling channel for backup job "+
						"'%s' having id '%s' that another process changed the job state to something else than 'running'",
						receivedBackupCommand.Name, receivedBackupCommand.BackupJobId)
					logger.Warn(msg)
					// it is assumed that whatever process requested the backup to be stopped will also take care of fully
					// stopping it
					return shared.ResponseBackupCommand{
						Name:    receivedBackupCommand.Name,
						Id:      receivedBackupCommand.Id,
						Message: shared.ErrJobNotFoundInRunningState,
						Err:     true,
					}
				}

				// set job state to "stopping"
				err = backupJobsState.MarkStopped(receivedBackupCommand.Name, loggingContext+".processBackupCommand",
					receivedBackupCommand.BackupJobId, false)
				if err != nil {
					return shared.ResponseBackupCommand{
						Name:    receivedBackupCommand.Name,
						Id:      receivedBackupCommand.Id,
						Message: err.Error(),
						Err:     true,
					}
				}

				// request job to stop (this is a non blocking call, the backup job may finish considerably later)
				signalBackupToStop(cancelFunc, receivedBackupCommand.Name, receivedBackupCommand.BackupJobId)

				return shared.ResponseBackupCommand{
					Name:        receivedBackupCommand.Name,
					Id:          receivedBackupCommand.Id,
					BackupJobId: receivedBackupCommand.BackupJobId,
					Err:         false,
				}
			} else {
				return shared.ResponseBackupCommand{
					Name:    receivedBackupCommand.Name,
					Id:      receivedBackupCommand.Id,
					Message: shared.ErrJobAlreadyStopped,
					Err:     true,
				}
			}

		}
	default:
		return shared.ResponseBackupCommand{
			Name: receivedBackupCommand.Name,
			Id:   receivedBackupCommand.Id,
			Err:  true,
			Message: fmt.Sprintf("Scheduler received command %s which is not one of 'start' or 'stop'. This is"+
				" a bug", receivedBackupCommand.Name),
		}
	}
}

// this function starts the backup for a given $jobName and $jobUuid
// $jobName MUST match the name of a backup job, as defined in the configuration file
func runBackup(jobName string, jobUuid string, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) {
	globals.Stats.IncrementRoutines("runBackup")
	defer globals.Stats.DecrementRoutines("runBackup")
	logger.Infof("Starting backup job having name '%s' with allocated job id '%s'", jobName, jobUuid)

	// extract config for this backup job only
	backupConfig, err := shared.MakeCopyOfBackupJobDefinition(jobName, serverConfigCopy)
	if err != nil {
		logger.Error(err)
		// needed as we haven't yet obtained the true list of object stores
		emptyStores := make([]objectstore.ObjectStore, 0)
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, shared.DbData{}, false, err, emptyStores)
		return
	}

	ctx, err := backupJobsState.GetContextForJob(jobName, jobUuid)
	// if we got an error than something else has already marked the backup job as != "running"
	if err != nil {
		logger.Errorf("It seems that while starting up backup job '%s' having id '%s' another process stopped "+
			"this backup job run", jobName, jobUuid)
		// it is assumed that whatever process requested the backup to be stopped will also take care of fully
		// stopping it
		return
	}

	dbData, err := dbops.PrepareDb(jobName, jobUuid, serverConfigCopy, backupJobsState, backupConfig, true, nil)
	if err != nil {
		// needed as we haven't yet obtained the true list of object stores
		emptyStores := make([]objectstore.ObjectStore, 0)
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err, emptyStores)
		return
	}

	// get object stores used for backing up files for this job
	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		// needed as we couldn't obtain the true list of object stores
		emptyStores := make([]objectstore.ObjectStore, 0)
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err, emptyStores)
		return
	}

	for _, objStor := range objectStores {
		// check that the object store settings (on the remote side) satisfy requirements and that the credentials we
		// have provided still grant sufficient access for the backup to be done
		_, err := objStor.Validate()
		if err != nil {
			StoreName, StoreType := objStor.GetStoreDetails()
			newErr := fmt.Errorf("while validating that backup target '%s' of type '%s' has the required "+
				"privileges, encountered error: %s", StoreName, StoreType, err)
			cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, newErr, objectStores)
			return
		}
	}

	// give "watch" clients a chance to connect before the backup starts emitting events
	time.Sleep(1 * time.Second)

	if strings.TrimSpace(backupConfig.PostRunScript) != "" {
		backupJobsState.IncrementCounter(backupConfig.Name, "scripts_num", "", "", "", "")
	}
	// run pre-backup script
	if strings.TrimSpace(backupConfig.PreRunScript) != "" {
		backupJobsState.IncrementCounter(backupConfig.Name, "scripts_num", "", "", "", "")
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation",
			fmt.Sprintf("Running pre_run_script %s", backupConfig.PreRunScript), "", "")
		err := backup.RunPrePostScript(backupConfig.PreRunScript, "pre", backupConfig.Name, jobUuid)
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation", "", "", "")
		backupJobsState.IncrementCounter(backupConfig.Name, "scripts_ran", "", "", "", "")
		if err != nil {
			backupJobsState.IncrementCounter(backupConfig.Name, "scripts_failed", "", "", "", "")
			cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err, objectStores)
			return
		}
	}
	backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation", "Examining and backing up", "", "")
	// examine each path listed and backup contained files/directories if needed
	for _, path := range backupConfig.Paths {
		select {
		case <-ctx.Done():
			{
				logger.Infof("Cancelling running backup job '%s' having id '%s'", jobName, jobUuid)
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil, objectStores)
				return
			}
		default:
			// backupJobsState MUST be a pointer
			cancelled, err := scan.Path(ctx, filepath.Clean(path), backupConfig, backupJobsState, false, dbData, objectStores, jobUuid)
			// Examine FIRST $exit and then $err
			if cancelled {
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil, objectStores)
				return
			}
			if err != nil {
				// some Major event happened which determined the whole backup to be considered failed
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err, objectStores)
				return
			}
		}
	}
	// if we got here then now we need to figure out what entries in the "files" table no longer represent on DISK status
	cancelled := backup.FindAndMarkDeleted(ctx, backupConfig, dbData, objectStores, backupJobsState, jobUuid, 5000)
	if cancelled {
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil, objectStores)
	}
	// if we got here than all was probably good (or "mostly" good) and the job did not get cancelled
	cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, nil, objectStores)
}

// does all of the housekeeping needed after a backup job has finished running (or has encountered an error and can't
// keep running or it was cancelled)
// $jobName MUST match the name of a backup job, as defined in the configuration file
func cleanupAfterBackup(jobName string, jobUuid string, backupConfig shared.ConfigBackup, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState, dbData shared.DbData, cancelled bool, backupError error, objectStores []objectstore.ObjectStore) {
	if backupError != nil {
		logger.Errorf("Backup '%s' having id '%s' did not finish as it encountered a non recoverable error. The error was: %s", jobName, jobUuid, backupError)
	}
	if cancelled {
		logger.Infof("Backup '%s' having id '%s' was cancelled while running", jobName, jobUuid)
	} else {
		// there are some edge cases where a cancelled job would not make it to a call to cleanupAfterBackup() receive cancelled=true so this is an extra check for that
		if backupJobsState.IsCancelled(jobName, jobUuid, loggingContext) {
			cancelled = true
			logger.Debugf("Job %s having id %s was cancelled but cleanupAfterBackup() didn't receive the call with cancelled=true", jobName, jobUuid)
			logger.Infof("Backup '%s' having id '%s' was cancelled while running", jobName, jobUuid)
		}
	}
	if !cancelled && backupError == nil {
		logger.Infof("Backup '%s' having id '%s' finished running", jobName, jobUuid)
	}
	// run post-backup script
	if strings.TrimSpace(backupConfig.PostRunScript) != "" {
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation",
			fmt.Sprintf("Running post_run_script %s", backupConfig.PostRunScript), "", "")
		backupJobsState.IncrementCounter(backupConfig.Name, "scripts_ran", "", "", "", "")
		err := backup.RunPrePostScript(backupConfig.PostRunScript, "post", backupConfig.Name, jobUuid) // #nosec
		if err != nil {
			backupJobsState.IncrementCounter(backupConfig.Name, "scripts_failed", "", "", "", "")
		}
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation", "", "", "")
	}

	backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation", "", "", "")

	// set state to "stopping"; ignore any errors (job may be set to state "stopping" already if stop was requested
	//  via the API but stop can also be triggered due to SIGTERM/SIGINT being received
	_ = backupJobsState.MarkStopped(jobName, loggingContext+".cleanupAfterBackup", jobUuid, false) // #nosec

	// sleep for 1 second - cheap way to get tests a chance to verify things or otherwise a new config parameter
	// would be needed for end to end tests
	time.Sleep(1 * time.Second)

	backupJobsStateCopy := backupJobsState.Get(serverConfigCopy, loggingContext+".cleanupAfterBackup")
	var jobStateCopy shared.BackupJobStatus
	for _, j := range backupJobsStateCopy {
		if jobName == j.Name {
			jobStateCopy = j
			break
		}
	}

	jobEndTime := time.Now()
	jobStateCopy.EndTime = jobEndTime

	jobStateCopy.State = "finished"
	if cancelled {
		jobStateCopy.State = "cancelled"
	}
	if backupError != nil {
		jobStateCopy.State = "failed"
	}
	jobReport := ""
	b, err := json.Marshal(jobStateCopy)
	if err != nil {
		logger.Warningf("Could not json encode the state of the just finished backup job. This means that "+
			"if any notifications are configured to be sent then they will not contain a detailed report. The "+
			"encountered error was: %s", err)
	} else {
		jobReport = string(b)
	}

	backupJobErrorMsg := ""
	if backupError != nil {
		backupJobErrorMsg = backupError.Error()
	}

	// Record in the DB the job status - if it errors out then UpdateJobDetails() will log. There is nothing we can do with the error here
	if dbData.Connected {
		_ = dbops.UpdateJobDetails(dbData.Db, jobUuid, jobName, "backup", jobEndTime, jobStateCopy.State, jobReport) // #nosec
	}

	// if the backup completed then upload the DB and the config file to the object store(s);
	// $backupError == true means that the backup could not start
	if !cancelled && backupError == nil {
		logger.Debugf("Attempting to upload a copy of the DB and of the config belonging to backup job '%s'", jobName)
		dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
		database.CloseDb(jobName, backupJobsState, false) // this closes down the DB but keeps a lock so no other client can open the DB

		err := backup.UploadBackupDatabase(jobName, jobUuid, backupConfig, serverConfigCopy.DataDir, backupJobsState, objectStores)
		if err != nil {
			logger.Warnf("While uploading a copy of the internal database, encountered error: %s", err)
		}

		database.UnlockDb(jobName, backupJobsState) // allow DB to be opened once again
		// reopen DB
		dbData, err = dbops.PrepareDb(jobName, jobUuid, serverConfigCopy, backupJobsState, backupConfig, false, nil)
		if err != nil {
			// there are cases where the DB was opened but another subsequent task produced the error so we should
			// attempt to close the DB. This operation is safe even if it did not succeed opening.
			dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
			logger.Errorf("After making a copy of the database, could not reopen the original due to error: %s", err)
		}

		sanitisedCfgFile, err := config.SaveSanitizedCfgToTmpFile(serverConfigCopy)
		if err != nil {
			logger.Errorf("Could not create a copy of the configuration file, in order to upload it to the remote object store, due to error: %s", err)
		}
		err = backup.UploadBackupConfigCopy(sanitisedCfgFile, jobUuid, backupConfig, backupJobsState, objectStores)
		if err != nil {
			logger.Errorf("Could not upload to the remote object store, a copy of the configuration file, due to error: %s", err)
		}
	} else {
		if cancelled {
			logger.Debugf("Not attempting to upload a copy of the DB and of the config as the backup was cancelled")
		} else {
			logger.Debugf("Not attempting to upload a copy of the DB and of the config as the backup could not start")
		}
	}

	// tell any connected Watch clients to exit
	backupFailed := backupError != nil
	watcher.TellClientsJobFinished("backup", jobName, jobUuid, backupJobsState.WatchMsgReceiver, cancelled, backupFailed)

	// in case the backup completed successfully then nothing called the cancel() function and if we don't do it then
	//  it will leak at least a channel. Calling it for an already cancelled backup will not cause any issue
	cancel, err := backupJobsState.GetCancelFunctionForJob(jobName, jobUuid)
	if err != nil {
		logger.Warnf("While trying cleanup the cancel context for backup '%s' having uuid '%s' the '%s' error "+
			"was encountered. This will leak some memory.", jobName, jobUuid, err)
	} else {
		cancel()
	}

	failedScripts, err := notifications.Execute(serverConfigCopy, jobUuid, "backup", jobStateCopy.State, jobName, jobReport, backupJobErrorMsg)
	if err != nil {
		if failedScripts > 0 {
			logger.Warningf("%d notification scripts exited with a non zero status code. Received errors "+
				"were: %s", failedScripts, err)
		}
		logger.Warningf("At least an error was encountered while trying to send notifications: %s", err)
	}

	// set state to "stopped"
	err = backupJobsState.MarkStopped(jobName, loggingContext+".cleanupAfterBackup",
		jobUuid, true)
	if err != nil {
		logger.Warnf("Encountered an error when trying to mark backup job '%s' having job id '%s' as 'stopped'. "+
			"The error was: %s", jobName, jobUuid, err)
	}

	// Record again in the DB the job status as the notification scripts or the DB copy may have caused issues (and
	// anyway when scripts were ok, they increment some counters because they ran). if it errors out then
	// UpdateJobDetails() will log. There is nothing we can do with the error here.
	if dbData.Connected {
		_ = dbops.UpdateJobDetails(dbData.Db, jobUuid, jobName, "backup", jobEndTime, jobStateCopy.State, jobReport) // #nosec
	}
	dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)

}

// non-blocking function which signals a given backup job that it should stop whatever is doing and exit
func signalBackupToStop(cancelFunction context.CancelFunc, name string, jobUuid string) {
	// TODO - if $jobUuid is empty string then stop whatever is the current running backup for the given $name (and
	// figure out the UUID in order to correctly log it in the below logger call
	logger.Infof("Signalling backup job having name '%s' with allocated job id '%s' to stop", name, jobUuid)
	// cancel running backup; this is a non blocking call and the running backup may keep going for a while before it
	// exits
	cancelFunction()
}

func stopAllBackups(backupJobsState *shared.BackupJobsState, serverConfigCopy shared.CfgTemplate) {
	if len(backupJobsState.Running) > 0 {
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
		for _, backupObject := range serverConfigCopy.Backup {
			if backupObject.Name == job.Name {
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
	logger.Debug("All running backup jobs have exited, as requested.")
}

// wait until the list of running backups has 0 length
func waitForAllBackupToBeStopped(backupJobsState *shared.BackupJobsState) {
	for {
		backupJobsState.Lock.RLock()
		if len(backupJobsState.Running) == 0 {
			backupJobsState.Lock.RUnlock()
			break
		}
		backupJobsState.Lock.RUnlock()
		time.Sleep(1 * time.Second)
	}
}

// generate uuid and validate that this is unique in both the running jobs state and also in the SQL DB
// returns: string with UUID if successful to generate, error object if an error is encountered (if this is the case
// then the value of the UUID string should be ignored)
func GenerateJobUuid(Name string, backupJobsState *shared.BackupJobsState, serverConfigCopy shared.CfgTemplate, jobType string) (string, error) {
	if jobType != "backup" && jobType != "restore" {
		logger.Warnf("generating UUIDs for job of type '%s' is not supported", jobType)
		return "", errors.New(shared.ErrUnknownJobType)
	}

	// get DataDir path as we need it for accessing the SQL DB
	serverConfigCopy.Mutex.RLock()
	dataDir := serverConfigCopy.DataDir
	serverConfigCopy.Mutex.RUnlock()

	// try at most 20 times to generate a unique UUID. If this is not sufficient then the program has a serious issue
	for i := 0; i < 20; i++ {
		u, err := uuid.NewV4()
		if err != nil {
			logger.Errorf("Could not generate a UUID due to error: %s", err)
			continue
		}
		jobUuid := u.String()

		// first check the state of running jobs and see if we got a matching UUID
		log.WithFields(log.Fields{"context": loggingContext + ".GenerateJobUuid"}).Debug("Acquiring read lock before " +
			"reading running backup jobs struct")
		backupJobsState.Lock.RLock()
		// Both backup and restore entries live in the same Running[] slice, so we simply check all
		// of them for a uuid clash regardless of the requested jobType.
		for _, job := range backupJobsState.Running {
			if job.BackupJobId == jobUuid {
				logger.Debugf("Found uuid '%s' already in the list of running jobs", jobUuid)
				continue
			}
		}
		backupJobsState.Lock.RUnlock()
		log.WithFields(log.Fields{"context": loggingContext + ".GenerateJobUuid"}).Debug("Read lock released after" +
			" reading running backup jobs struct")

		// if we got here, then start a DB connection and check if we got a job (no matter the type) with this UUID
		// known in the database ; we use the UUID as a primary key in a table so it needs to be unique
		// get DB connection pointer
		db, err := database.Start(dataDir, Name, backupJobsState)
		// the backup can not run as we can't initialise/connect to the database
		if err != nil {
			logger.Errorf("Could not connect to the SQL database in order to validate uniqueness of UUID for"+
				" %s job '%s'", jobType, Name)
			return "", err
		}
		// check db
		foundUuidInDB, err := dbops.CheckJobUuidExists(db, jobUuid)
		if err != nil {
			database.DisconnectFromDb(Name, backupJobsState, db)
			return "", err
		}
		if foundUuidInDB {
			database.DisconnectFromDb(Name, backupJobsState, db)
			continue
		} else {
			database.DisconnectFromDb(Name, backupJobsState, db)
			return jobUuid, nil
		}
	}
	// if we got here then we could not generate an unique UUID after 20 attempts so we'll give up
	logger.Warnf("Tried 20 times to generate a unique job id for '%s' job '%s'", jobType, Name)
	return "", errors.New(shared.ErrCouldNotGenerateJobId)
}

// processRestoreCommand handles start/stop/resume commands targeted at restore jobs.
func processRestoreCommand(cmd shared.ReceiveRestoreCommand, backupJobsState *shared.BackupJobsState,
	serverConfigCopy shared.CfgTemplate) shared.ResponseRestoreCommand {
	switch cmd.Command {
	case "start":
		{
			restoreJobUuid, err := GenerateJobUuid(cmd.Name, backupJobsState, serverConfigCopy, "restore")
			if err != nil {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: err.Error()}
			}
			err = backupJobsState.MarkRestoreRunning(cmd.Name, loggingContext+".processRestoreCommand", restoreJobUuid)
			if err != nil {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: err.Error()}
			}
			go runRestore(cmd, restoreJobUuid, serverConfigCopy, backupJobsState)
			return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, RestoreJobId: restoreJobUuid}
		}
	case "stop":
		{
			if !backupJobsState.IsRunning(cmd.Name, cmd.RestoreJobId, loggingContext+".processRestoreCommand") {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: shared.ErrJobAlreadyStopped}
			}
			if backupJobsState.IsStopping(cmd.Name, cmd.RestoreJobId, loggingContext+".processRestoreCommand") {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: shared.ErrJobAlreadyStopping}
			}
			cancelFunc, err := backupJobsState.GetCancelFunctionForJob(cmd.Name, cmd.RestoreJobId)
			if err != nil {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: shared.ErrJobNotFoundInRunningState}
			}
			err = backupJobsState.MarkStopped(cmd.Name, loggingContext+".processRestoreCommand", cmd.RestoreJobId, false)
			if err != nil {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: err.Error()}
			}
			logger.Infof("Signalling restore job having name '%s' with id '%s' to stop", cmd.Name, cmd.RestoreJobId)
			cancelFunc()
			return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, RestoreJobId: cmd.RestoreJobId}
		}
	case "resume":
		{
			if cmd.RestoreJobId == "" {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true,
					Message: "restore_job_id is required for 'resume' commands"}
			}
			if cmd.TargetName == "" {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true,
					Message: "target_name is required for 'resume' commands so the correct restore database can be located"}
			}
			if backupJobsState.IsRunning(cmd.Name, "", loggingContext+".processRestoreCommand") {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true,
					Message: shared.ErrJobAlreadyRunning}
			}
			if err := backupJobsState.MarkRestoreRunning(cmd.Name, loggingContext+".processRestoreCommand", cmd.RestoreJobId); err != nil {
				return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true, Message: err.Error()}
			}
			go runResume(cmd, serverConfigCopy, backupJobsState)
			return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, RestoreJobId: cmd.RestoreJobId}
		}
	default:
		return shared.ResponseRestoreCommand{Name: cmd.Name, Id: cmd.Id, Command: cmd.Command, Err: true,
			Message: fmt.Sprintf("Scheduler received restore command '%s' which is not one of 'start', 'stop' or 'resume'", cmd.Command)}
	}
}

// runRestore orchestrates a single restore job end to end: it calls into the restore package to
// perform the work, records the final outcome in the database, clears the state entry and tells
// any attached watch clients that the job has finished.
func runRestore(cmd shared.ReceiveRestoreCommand, restoreJobUuid string, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) {
	globals.Stats.IncrementRoutines("runRestore")
	defer globals.Stats.DecrementRoutines("runRestore")
	logger.Infof("Starting restore job for backup '%s' with allocated job id '%s'", cmd.Name, restoreJobUuid)

	req := restore.Request{
		JobName:            cmd.Name,
		RestoreJobId:       restoreJobUuid,
		SourceBackupJobId:  cmd.SourceBackupJobId,
		TargetName:         cmd.TargetName,
		Files:              cmd.Files,
		AllFiles:           cmd.AllFiles,
		RestoreDirOverride: cmd.RestoreDirOverride,
		Exclusions:         cmd.Exclusions,
	}
	result := restore.Do(cmd.Name, req, serverConfigCopy, backupJobsState)
	cleanupAfterRestore(cmd.Name, restoreJobUuid, result, serverConfigCopy, backupJobsState)
}

// runResume orchestrates a resume of a previously-crashed or stopped restore. It delegates to
// restore.Resume which reads the stored jobs-table row, seeds in-memory counters from the stored
// restore_counters row, and then re-invokes the main restore loop skipping already-completed
// files.
func runResume(cmd shared.ReceiveRestoreCommand, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState) {
	globals.Stats.IncrementRoutines("runResume")
	defer globals.Stats.DecrementRoutines("runResume")
	logger.Infof("Resuming restore job for backup '%s' target '%s' with restore job id '%s'",
		cmd.Name, cmd.TargetName, cmd.RestoreJobId)

	result := restore.Resume(cmd.Name, cmd.RestoreJobId, cmd.TargetName, serverConfigCopy, backupJobsState)
	cleanupAfterRestore(cmd.Name, cmd.RestoreJobId, result, serverConfigCopy, backupJobsState)
}

// cleanupAfterRestore persists the final job state, releases the cancel context, removes the
// entry from the running jobs slice and notifies watch clients.
func cleanupAfterRestore(jobName string, restoreJobUuid string, result restore.Result,
	serverConfigCopy shared.CfgTemplate, backupJobsState *shared.BackupJobsState) {

	if result.Err != nil {
		logger.Errorf("Restore '%s' having id '%s' finished with error: %s", jobName, restoreJobUuid, result.Err)
	} else {
		logger.Infof("Restore '%s' having id '%s' finished with state '%s' at '%s'", jobName, restoreJobUuid,
			result.State, result.RestoredDirectory)
	}

	// build a minimal report from the current BackupJobsState entry so later API calls can
	// see basic counters/end time.
	jobStateCopy := shared.BackupJobStatus{}
	for _, j := range backupJobsState.GetRestoresRunning(loggingContext + ".cleanupAfterRestore") {
		if j.Name == jobName && j.BackupJobId == restoreJobUuid {
			jobStateCopy = j
			break
		}
	}
	jobStateCopy.EndTime = time.Now()
	jobStateCopy.State = result.State

	jobReport := ""
	b, err := json.Marshal(jobStateCopy)
	if err != nil {
		logger.Warnf("Could not json encode final restore state: %s", err)
	} else {
		jobReport = string(b)
	}

	if result.TargetName == "" {
		logger.Warnf("Cannot update the restore DB jobs table entry for restore '%s' id '%s' because the target "+
			"name was not determined (restore likely failed before target resolution)", jobName, restoreJobUuid)
	} else if err := restore.FinalizeJobRecord(serverConfigCopy, jobName, result.TargetName, restoreJobUuid, result.State, jobReport, backupJobsState); err != nil {
		logger.Warnf("Could not update the jobs table entry for restore '%s' id '%s': %s", jobName, restoreJobUuid, err)
	}

	watcher.TellClientsJobFinished("restore", jobName, restoreJobUuid, backupJobsState.WatchMsgReceiver,
		result.State == "cancelled", result.State == "failed")

	cancelFunc, err := backupJobsState.GetCancelFunctionForJob(jobName, restoreJobUuid)
	if err != nil {
		logger.Warnf("While cleaning up restore '%s' id '%s', could not fetch cancel func: %s", jobName, restoreJobUuid, err)
	} else {
		cancelFunc()
	}

	if err := backupJobsState.MarkStopped(jobName, loggingContext+".cleanupAfterRestore", restoreJobUuid, true); err != nil {
		logger.Warnf("Could not mark restore '%s' id '%s' as stopped: %s", jobName, restoreJobUuid, err)
	}
}
