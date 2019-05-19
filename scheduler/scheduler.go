package scheduler

import (
	"cloudbackup/backup"
	"cloudbackup/backup/scan"
	"cloudbackup/daemon/globals"
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/notifications"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
	"cloudbackup/watcher"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"
)

const loggingContext = "scheduler"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start(cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
	backupJobsState *shared.BackupJobsState, configuration *shared.RuntimeConfig) {
	// start component which listens for messages from http handlers and starts / stops backups & restores according to requests
	go eventProcessor(cfgChange, SchedulerCommBackup, backupJobsState, configuration)
	// components which relays to clients real time info about the file/dir/symlink currently being backed up or restores
	go watcher.Start(backupJobsState.Watcher)
}

func eventProcessor(cfgChange <-chan bool, SchedulerCommBackup *shared.CommWithSchedulerForBackup,
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
	var backupConfig shared.ConfigBackup
	serverConfigCopy.Mutex.RLock()
	for _, backupObject := range serverConfigCopy.Backup {
		if backupObject.Name == jobName {
			// deep copy
			backupConfig = shared.CopyConfigBackupStruct(backupObject)
			break
		}
	}
	serverConfigCopy.Mutex.RUnlock()

	ctx, err := backupJobsState.GetContextForJob(jobName, jobUuid)
	// if we got an error than something else has already marked the backup job as != "running"
	if err != nil {
		logger.Errorf("It seems that while starting up backup job '%s' having id '%s' another process stopped "+
			"this backup job run", jobName, jobUuid)
		// it is assumed that whatever process requested the backup to be stopped will also take care of fully
		// stopping it
		return
	}

	dbData, err := dbops.PrepareDb(jobName, jobUuid, serverConfigCopy, backupJobsState, backupConfig, true)
	if err != nil {
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err)
		return
	}

	// get object stores used for backing up files for this job
	objectStores, err := objectstore.GetObjectStores(ctx, backupConfig, backupJobsState)
	if err != nil {
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err)
		return
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
			cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err)
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
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil)
				return
			}
		default:
			// backupJobsState MUST be a pointer
			cancelled, err := scan.Path(ctx, path, backupConfig, backupJobsState, false, dbData, objectStores, jobUuid)
			// Examine FIRST $exit and then $err
			if cancelled {
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil)
				return
			}
			if err != nil {
				// some Major event happened which determined the whole backup to be considered failed
				cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, err)
				return
			}
		}
	}
	// if we got here then now we need to figure out what entries in the "files" table no longer represent on DISK status
	// TODO - test with 3 as $MaxResults
	cancelled := backup.FindAndMarkDeleted(ctx, backupConfig, dbData, objectStores, backupJobsState, jobUuid, 5000)
	if cancelled {
		cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, true, nil)
	}
	// if we got here than all was probably good (or "mostly" good) and the job did not get cancelled
	cleanupAfterBackup(jobName, jobUuid, backupConfig, serverConfigCopy, backupJobsState, dbData, false, nil)
}

// does all of the housekeeping needed after a backup job has finished running (or has encountered an error and can't
// keep running or it was cancelled)
// $jobName MUST match the name of a backup job, as defined in the configuration file
func cleanupAfterBackup(jobName string, jobUuid string, backupConfig shared.ConfigBackup, serverConfigCopy shared.CfgTemplate,
	backupJobsState *shared.BackupJobsState, dbData shared.DbData, cancelled bool, backupError error) {
	// in case the backup completed successfully then nothing called the cancel() function and if we don't do it then
	//  it will leak at least a channel. Calling it for an already cancelled backup will not cause any issue
	cancel, err := backupJobsState.GetCancelFunctionForJob(jobName, jobUuid)
	if err != nil {
		logger.Warnf("While trying cleanup the cancel context for backup '%s' having uuid '%s' the '%s' error "+
			"was encountered. This will leak some memory.", jobName, jobUuid, err)
	} else {
		cancel()
	}

	// run post-backup script
	if strings.TrimSpace(backupConfig.PostRunScript) != "" {
		backupJobsState.UpdateStatsText(backupConfig.Name, "current_operation",
			fmt.Sprintf("Running post_run_script %s", backupConfig.PostRunScript), "", "")
		backupJobsState.IncrementCounter(backupConfig.Name, "scripts_ran", "", "", "", "")
		err = backup.RunPrePostScript(backupConfig.PostRunScript, "post", backupConfig.Name, jobUuid) // #nosec
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

	// tell any connected Watch clients to exit
	backupFailed := false
	if backupError != nil {
		backupFailed = true
	}
	watcher.TellClientsJobFinished("backup", jobName, jobUuid, backupJobsState.WatchMsgReceiver, cancelled, backupFailed)

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
	// TODO - figure out when a job is "CRASHED" and set $jobStatus accordingly - probably doesn't make sense to do it
	//  from this function but rather a function which runs at server startup and examines the state of the DB
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

	// Record in the DB the job statatus - if it errors out then UpdateJobDetails() will log. There is nothing we can do with the error here
	_ = dbops.UpdateJobDetails(dbData.Db, jobUuid, jobName, "backup", jobEndTime, jobStateCopy.State, jobReport) // #nosec
	// TODO - close the database, put a lock on it, copy & gzip it and then upload the resulting file to the remote object store
	dbops.CloseStatementsAndDb(dbData, backupJobsState)
	// do the copy
	dbCopyPath, err := dbops.MakeDbCopy(jobName, jobUuid, serverConfigCopy.DataDir, backupJobsState)
	// TODO - upload $dbCopyPath

	err = os.Remove(dbCopyPath)
	if err != nil {
		logger.Warnf("Could not delete database copy held in file '%s' due to error: %s", dbCopyPath, err)
	}

	dbData, err = dbops.PrepareDb(jobName, jobUuid, serverConfigCopy, backupJobsState, backupConfig, false)
	if err != nil {
		logger.Errorf("After making a copy of the database, could not reopen the original due to error: %s", err)
		// TODO - add to job status a new counter and increment it in order to signal that a problem was encountered
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
	_ = dbops.UpdateJobDetails(dbData.Db, jobUuid, jobName, "backup", jobEndTime, jobStateCopy.State, jobReport) // #nosec

	// close SQL connection and opened statements
	dbops.CloseStatementsAndDb(dbData, backupJobsState)

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
	// TODO - when implementing restore or purge jobs , add support in this function too
	if jobType != "backup" {
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
		// TODO - add separate if block for "restore" and "purge" type jobs
		if jobType == "backup" {
			for _, job := range backupJobsState.Running {
				if job.BackupJobId == jobUuid {
					logger.Debugf("Found uuid '%s' already in the list of running backup jobs", jobUuid)
					continue
				}
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
			database.CloseDb(db, Name, backupJobsState)
			return "", err
		}
		if foundUuidInDB {
			database.CloseDb(db, Name, backupJobsState)
			continue
		} else {
			database.CloseDb(db, Name, backupJobsState)
			return jobUuid, nil
		}
	}
	// if we got here then we could not generate an unique UUID after 20 attempts so we'll give up
	logger.Warnf("Tried 20 times to generate a unique job id for '%s' job '%s'", jobType, Name)
	return "", errors.New(shared.ErrCouldNotGenerateJobId)
}
