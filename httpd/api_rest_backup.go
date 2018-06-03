package httpd

import (
	"net/http"
	"github.com/julienschmidt/httprouter"
	"encoding/json"
	"cloudbackup/shared"
	"fmt"
	"github.com/satori/go.uuid"
	"time"
)

type BackupJob struct {
	Name string `json:"name"`
	JobId string `json:"job_id"`
}

func (srvSrc SrvData) handlerPostBackupStart(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
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
	srvCopy := srvSrc.GetWithLock(loggingContext + ".handlerPostBackupStart")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetWithLock(loggingContext + ".handlerPostBackupStart")
	found := false
	for _, backup := range configCopy.Backup {
		if backup.Name == decodedJson.Name {
			found = true
		}
	}

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
	srvCopy := srvSrc.GetWithLock(loggingContext + ".handlerPostBackupStop")

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
	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetWithLock(loggingContext + ".handlerGetBackupList")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetWithLock(loggingContext + ".handlerGetBackupList")
	JSONSuccessWithResult(w, "success", "success",
		srvCopy.backupJobsState.Get(configCopy, loggingContext + ".handlerGetBackupList"))
}