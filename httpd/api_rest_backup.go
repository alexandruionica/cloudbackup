package httpd

import (
	"context"
	"net/http"
	"github.com/julienschmidt/httprouter"
	"encoding/json"
	"cloudbackup/shared"
	"fmt"
	"github.com/satori/go.uuid"
	"time"
	"sync"
	"cloudbackup/config"
	"cloudbackup/daemon/globals"
)

type BackupJob struct {
	Name string `json:"name"`
	JobId string `json:"job_id"`
}

func (srvSrc SrvData) handlerPostBackupStart(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson BackupJob
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name" +
			" is needed in order to know what backup job you're requesting to be started"))
		return
	}
	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostBackupStart")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostBackupStart")
	found := false
	// while "runtimeCfg" is a copy, some of the data is pointers so locking is still needed as it may be
	// shared with other functions (running in other routines)
	configCopy.Mutex.RLock()
	for _, backup := range configCopy.Backup {
		if backup.Name == decodedJson.Name {
			found = true
		}
	}
	configCopy.Mutex.RUnlock()

	if found == false {
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job was found matching name:" +
			" %s", decodedJson.Name))
		return
	}

	if srvCopy.backupJobsState.IsRunning(decodedJson.Name, "" , loggingContext + ".handlerPostBackupStart"){
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Backup for job having " +
			"name '%s' is already running.", decodedJson.Name))
		return
	}

	command := shared.ReceiveBackupCommand{
		Name: decodedJson.Name,
		Command: "start",
		Id: uuid.NewV4().String(),
	}
	httpUser, _, _ := r.BasicAuth()
	// send command to scheduling routine - blocks until the other end reads it
	select {
	case srvCopy.commWithSchedulerForBackup.ReceivedCommand <- command:
	case <-time.After(5 * time.Second): {
		logger.Warnf("Sending a request to the scheduling component timed out after 5 seconds. The request "+
			"was to start a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
			decodedJson.Name, httpUser, r.RemoteAddr)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Sending a request to " +
			"the scheduling component timed out after 5 seconds. The request was to start a backup job for" +
			" job having name '%s'. This is abnormal unless your system is starved of CPU resources. It is possible" +
			" that the request may have succeeded", decodedJson.Name))
		return
	}
	}

	logger.Infof("Backup job start for job having name '%s' has been requested by '%s' from '%s'",
		decodedJson.Name, httpUser, r.RemoteAddr)
	var result shared.ResponseBackupCommand
	// wait for max 20 seconds for a response from the scheduling thread
	select {
		case result = <-srvCopy.commWithSchedulerForBackup.SendResponse:
			{
			logger.Debugf("Received response %+v from scheduling component", result)
				if result.Err == false {
					requestResult := BackupJob{
						Name: decodedJson.Name,
						JobId: result.BackupJobId,
					}
					JSONSuccessWithResult(w, "success", "Successfully requested backup job to be started",
						requestResult)
					return
				} else {
					JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Could not start " +
						"backup for job having name '%s'. The error is: %s", decodedJson.Name, result.Message ))
					return
				}
			}
		case <-time.After(20 * time.Second):
			{
				logger.Warnf("Didn't receive in 20 seconds a response from the scheduling component. The request "+
					"was to start a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
					decodedJson.Name, httpUser, r.RemoteAddr)
				JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Didn't receive in " +
					"20 seconds a response from the scheduling component. The request was to start a backup job for" +
					" job having name '%s'. This is abnormal unless your system is starved of CPU resources",
					decodedJson.Name))
				return
			}

	}
}

func (srvSrc SrvData) handlerPostBackupStop(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson BackupJob
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name" +
			" is needed in order to know what backup job you're requesting to be stopped"))
		return
	}
	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostBackupStop")

	if srvCopy.backupJobsState.IsRunning(decodedJson.Name, decodedJson.JobId , loggingContext + ".handlerPostBackupStart") == false {
		var errorMsg string
		if decodedJson.JobId != "" && srvCopy.backupJobsState.IsRunning(decodedJson.Name, "", loggingContext + ".handlerPostBackupStart") {
			errorMsg = fmt.Sprintf("Backup for job having name '%s' and a backup job id of '%s' is not " +
				"running so it can't be stopped. There is a running backup job for the same name but with a " +
				"different job id", decodedJson.Name, decodedJson.JobId)
		} else {
			errorMsg = fmt.Sprintf("Backup for job having " + "name '%s' is not running.", decodedJson.Name)
		}
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, errorMsg)
		return
	}

	// check if job already stopping
	if srvCopy.backupJobsState.IsStopping(decodedJson.Name, decodedJson.JobId , loggingContext + ".handlerPostBackupStart") {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Backup for job having " +
			"name '%s' is already stopping.", decodedJson.Name))
		return
	}

	command := shared.ReceiveBackupCommand{
		Name: decodedJson.Name,
		Command: "stop",
		Id: uuid.NewV4().String(),
		BackupJobId: decodedJson.JobId,
	}
	httpUser, _, _ := r.BasicAuth()
	// send command to scheduling routine
	select {
		case srvCopy.commWithSchedulerForBackup.ReceivedCommand <- command:
		case <-time.After(5 * time.Second): {
			logger.Warnf("Sending a request to the scheduling component timed out after 5 seconds. The request "+
				"was to stop a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
				decodedJson.Name, httpUser, r.RemoteAddr)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Sending a request to " +
				"the scheduling component timed out after 5 seconds. The request was to stop a backup job for" +
				" job having name '%s'. This is abnormal unless your system is starved of CPU resources. It is possible" +
				" that the request may have succeeded", decodedJson.Name))
			return
		}
	}

	if decodedJson.JobId == "" {
		logger.Infof("Backup job stop for job having name '%s' has been requested by '%s' from '%s'",
			decodedJson.Name, httpUser, r.RemoteAddr)
	} else {
		logger.Infof("Backup job stop for job having name '%s' and id '%s' has been requested by '%s' " +
			"from '%s'", decodedJson.Name, decodedJson.JobId, httpUser, r.RemoteAddr)
	}
	var result shared.ResponseBackupCommand
	// wait for max 20 seconds for a response from the scheduling thread
	select {
	case result = <-srvCopy.commWithSchedulerForBackup.SendResponse:
		{
			logger.Debugf("Received response %+v from scheduling component", result)
			if result.Err == false {
				requestResult := BackupJob{
					Name: decodedJson.Name,
					JobId: result.BackupJobId,
				}
				JSONSuccessWithResult(w, "success", "Successfully requested backup job to be stopped",
					requestResult)
				return
			} else {
				JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Could not stop " +
					"backup for job having name '%s'. The error is: %s", decodedJson.Name, result.Message ))
				return
			}
		}
	case <-time.After(20 * time.Second):
		{
			logger.Warnf("Didn't receive in 20 seconds a response from the scheduling component. The request "+
				"was to stop a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
				decodedJson.Name, httpUser, r.RemoteAddr)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Didn't receive in " +
				"20 seconds a response from the scheduling component. The request was to stop a backup job for" +
				" job having name '%s'. This is abnormal unless your system is starved of CPU resources",
				decodedJson.Name))
			return
		}
	}
}

// return a summary of backup jobs (running and stopped)
func (srvSrc SrvData) handlerGetBackupList(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerGetBackupList")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerGetBackupList")
	JSONSuccessWithResult(w, "success", "success",
		srvCopy.backupJobsState.Get(configCopy, loggingContext + ".handlerGetBackupList"))
}

// for a given backup job name return the list of files that would be examined and optionally any excluded files
func (srvSrc SrvData) handlerPostBackupDryRun(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson BackupJob
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name" +
			" is needed in order to know what backup job you're requesting to be started"))
		return
	}
	// get notified if the client closes the connection
	notify := w.(http.CloseNotifier).CloseNotify()

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostBackupDryRun")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostBackupDryRun")
	found := false
	var backupConfig config.Backup
	// while "runtimeCfg" is a copy, some of the data is pointers so locking is still needed as it may be
	// shared with other functions (running in other routines)
	configCopy.Mutex.RLock()
	for _, backup := range configCopy.Backup {
		if backup.Name == decodedJson.Name {
			found = true
			backupConfig = backup
		}
	}
	configCopy.Mutex.RUnlock()

	if found == false {
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job was found matching name:" +
			" %s", decodedJson.Name))
		return
	}
	flusher, ok := w.(http.Flusher)

	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		logger.Debugf("HTTP2 Streaming unsupported in handlerPostBackupDryRun()")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// backupJobState contains the state of the evaluated job
	backupJobsState := &shared.DryRunBackupJobsState{Lock: &sync.RWMutex{}}
	reportChan := make(chan shared.ScanEvalItemReport)
	backupJobsState.ReportChan = reportChan
	evaljobId := uuid.NewV4().String()
	err = backupJobsState.MarkEvaluating(decodedJson.Name, loggingContext + ".handlerPostBackupDryRun",
		evaljobId)
	if err != nil {
		logger.Debugf("While trying to start an evaluate backup job, received error: '%s'", err)
	}
	cancel, err := backupJobsState.GetCancelFunctionForJob(decodedJson.Name, evaljobId)

	// this channel is used to tell this http handler that scanPathExit has completed it's run and exited (either exit
	// was cause by a cancel request or scan.Path just finished it's run)
	scanPathExit := make(chan bool)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Received error '%s' " +
			"while trying to setup the state object for the evaluate job run", err))
		return
	}

	ctx, err := backupJobsState.GetContextForJob(decodedJson.Name, evaljobId)
	// if we got an error than something else has already marked the backup job as != "running"
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Received error '%s' " +
			"while trying to setup the state object for the evaluate job run", err))
		return
	}

	// launch GO routine which collects and reports
	go dryRunBackupPaths(ctx, backupConfig, backupJobsState, scanPathExit)

	//counter := 0
	for {
		select {
		// if the client closed the connection then exit
		case _ = <-notify:
			logger.Debug("Client closed connection so we're exiting. Sending signal to scan.Path() so " +
				"dryRunBackupPaths() exits")
			// signal scan.Path() to exit; this is a non blocking call, it may take a lot longer for it to exit
			cancel()
			logger.Debug("Successfully sent signal to scan.Path(), waiting for reply from dryRunBackupPaths() " +
				"that it is ready to exit")
			// when dryRunBackupPaths() is done it will put a message on the "scanPathExit" channel
		case message := <- reportChan:
			{
				// Write to the ResponseWriter
				// Server Sent Events compatible
				jsonMsg, err := json.Marshal(message)
				if err != nil {
					logger.Warnf("Could not json encode message received from evaluate job. Error was: '%s'", err)
				} else {
					_, _ = fmt.Fprintf(w, "data: %s\n", jsonMsg) // #nosec
					// Flush the data immediately instead of buffering it for later.
					flusher.Flush()
				}
		}
		// scan.Path completed it's run (a cancel may have been requested)
		case _ = <- scanPathExit:
			{
				logger.Debug("scan.Path() triggered by handlerPostBackupDryRun() has completed its run so the " +
				"http handler will exit now")
				finalMsg := "Completed run"
				result, err := backupJobsState.GetStats(decodedJson.Name)
				if err != nil {
					logger.Warnf("Received error while trying to get stats at the final of Dry Run job '%s'. " +
						"Error was: '%s'", decodedJson.Name, err)
				} else {
					finalMsg += fmt.Sprintf(": %d examined files, %d examined directories, %d excluded files " +
						"or directories, %d errors encountered", result.StatsCounters["examined_files"],
						result.StatsCounters["examined_directories"], result.StatsCounters["excluded"],
						result.StatsCounters["failed_to_examine"])
				}

				_, _ = fmt.Fprintf(w, "data: %s\n", finalMsg) // #nosec
				// close channels to avoid memory leaks
				close(reportChan)
				close(scanPathExit)
				return
			}
		}

	}

	//JSONSuccessWithResult(w, "success", "success",
	//	srvCopy.backupJobsState.Get(configCopy, loggingContext + ".handlerGetBackupList"))
}



// for a given backup job name (and optional job id) show real time progress
func (srvSrc SrvData) handlerPostBackupWatch(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson BackupJob
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name"+
			" is needed in order to know what backup job you're requesting to watch"))
		return
	}

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostBackupDryRun")

	if srvCopy.backupJobsState.IsRunning(decodedJson.Name, decodedJson.JobId , loggingContext + ".handlerPostBackupWatch") == false {
		var errorMsg string
		if decodedJson.JobId != "" && srvCopy.backupJobsState.IsRunning(decodedJson.Name, "", loggingContext + ".handlerPostBackupWatch") {
			errorMsg = fmt.Sprintf("Backup for job having name '%s' and a backup job id of '%s' is not " +
				"running so it can't be watched. There is a running backup job for the same name but with a " +
				"different job id", decodedJson.Name, decodedJson.JobId)
		} else {
			errorMsg = fmt.Sprintf("Backup for job having " + "name '%s' is not running.", decodedJson.Name)
		}
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, errorMsg)
		return
	}

	// check if job already stopping
	if srvCopy.backupJobsState.IsStopping(decodedJson.Name, decodedJson.JobId , loggingContext + ".handlerPostBackupStart") {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Backup for job having " +
			"name '%s' is stopping so it can't be watched any more.", decodedJson.Name))
		return
	}

	flusher, ok := w.(http.Flusher)

	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		logger.Debugf("HTTP2 Streaming unsupported in handlerPostBackupDryRun()")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//
	// currentPath := ""
	// Sequence number of the object (file) for which the last message was received. A jump of more than 1 means
	// messages were lost about files. Rate limiting makes it likely messages will be skipped for the current file but
	// lost messages means that not even 1 message was received for some file(s) and rate limiting will not prevent that
	// var currentSequence uint64 = 0

	// prepare all data need to register the new client
	jobid := ""
	if decodedJson.JobId != "" {
		jobid = decodedJson.JobId
	} else {
		jobid, err = srvCopy.backupJobsState.GetRunningBackupJobId(decodedJson.Name, loggingContext + ".handlerPostBackupStart")
		// the GetRunningBackupJobId() function returns an error only if the job name can't be found in the list of running jobs
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Backup for job having " +
				"name '%s' has started stopping so it can't be watched any more.", decodedJson.Name))
			return
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	// buffer up to 100 messages before discarding if the http client is too slow to receive
    commChan := make(chan shared.WatchMessage, 100)
	clientUUID := uuid.NewV4().String()
	srvCopy.backupJobsState.Watcher.AddConsumer("backup", decodedJson.Name, jobid, commChan, ctx, cancel,
		r.RemoteAddr, clientUUID)

	for {
		select {
		// backup server is shutting down
		case <-ctx.Done(): {
			srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
			_, _ = fmt.Fprintf(w, "data: %s\n", "Backup server has been requested to exit. All running " +
				"backups are being stopped.") // #nosec
			// close channel to avoid memory leaks
			close(commChan)
			return
		}
		// client disconnected
		case <- r.Context().Done(): {
			srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
			// close channel to avoid memory leaks
			close(commChan)
			return
		}
		case message := <- commChan: {
			// Write to the ResponseWriter
			// Server Sent Events compatible
			jsonMsg, err := json.Marshal(message)
			if err != nil {
				logger.Warnf("Could not json encode message received from backup job live status. Error was: '%s'", err)
			} else {
				_, _ = fmt.Fprintf(w, "data: %s\n", jsonMsg) // #nosec
				// Flush the data immediately instead of buffering it for later.
				flusher.Flush()
			}
			// if this is the last message then remove the consumer from the consumer list and close this http connection
			if message.Completed {
				srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
				_, _ = fmt.Fprintf(w, "data: %s\n", "Backup job has finished.") // #nosec
				// close channel to avoid memory leaks
				close(commChan)
				return
			}
		}
		}
	}

}