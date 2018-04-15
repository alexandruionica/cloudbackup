package httpd

import (
	"net/http"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"encoding/json"
	"cloudbackup/shared"
	"fmt"
	"github.com/satori/go.uuid"
	"time"
)

type BackupJob struct {
	Name string `json:"name"`
	Id string `json:"id"`
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
	srvCopy := srvSrc.GetWithLock(loggingContext + ".handlerPostBackupStart")
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
	// TODO -  check if a backup is already running
	//  if yes then reply with   HttpErrIncorrectClientData and a 400 code
	// else attempt to start backup

	command := shared.ReceiveBackupCommand{
		Name: decodedJson.Name,
		Command: "start",
		Id: uuid.NewV4().String(),
	}
	log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStart"}).Debug("Acquiring lock in" +
		" order to communicate with the scheduler routine")
	// despite srvCopy being a copy, commWithSchedulerForBackup is a pointer so the below lock should be safe and
	// prevent others from accessing the same channels
	srvCopy.commWithSchedulerForBackup.Mutex.Lock()
	defer func() {
		srvCopy.commWithSchedulerForBackup.Mutex.Unlock()
		log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStart"}).Debug("Lock released" +
			" after communicating with the scheduler routine")
	}()
	log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStart"}).Debug("Lock for scheduler " +
		"routine acquired")
	// send command to scheduling routine
	srvCopy.commWithSchedulerForBackup.ReceivedCommand <- command
	httpUser, _, _ := r.BasicAuth()
	logger.Infof("Backup job start for job having name '%s' has been requested by '%s' from '%s'",
		decodedJson.Name, httpUser, r.RemoteAddr)
	var result shared.ResponseBackupCommand
	// wait for max 60 seconds for a response from the scheduling thread
	select {
		case result = <-srvCopy.commWithSchedulerForBackup.SendResponse:
			{
			logger.Debugf("Received response %+v from scheduling component", result)
			if result.Err == false {
				if command.Id != result.Id {
					logger.Errorf("Request to start backup job '%s' had id '%s' but response id is '%s'. This " +
						"is a bug and should be reported.")
					JSONError(w, http.StatusInternalServerError, HttpErrInternalError, "Response id does not " +
						"match request id. This is a bug. None the less, the backup job may have started so please " +
						"check the status of backup jobs")
					return
				}
				requestResult := BackupJob{
					Name: decodedJson.Name,
					Id: result.BackupJobId,
				}
				JSONSuccessWithResult(w, "success", "successfully requested backup job to be started",
					requestResult)

			}
			}
		case <-time.After(60 * time.Second):
			{
				logger.Warnf("Didn't receive in 60 seconds a response from the scheduling component. The request "+
					"was to start a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
					decodedJson.Name, httpUser, r.RemoteAddr)
				JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Didn't receive in " +
					"60 seconds a response from the scheduling component. The request was to start a backup job for" +
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
	srvCopy := srvSrc.GetWithLock(loggingContext + ".handlerPostBackupStop")

	// TODO - Check running backups (from state) and see if we have a match for the $name and if specified also the $id

	command := shared.ReceiveBackupCommand{
		Name: decodedJson.Name,
		Command: "stop",
		Id: uuid.NewV4().String(),
		BackupJobId: decodedJson.Id,
	}
	log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStop"}).Debug("Acquiring lock in" +
		" order to communicate with the scheduler routine")
	// despite srvCopy being a copy, commWithSchedulerForBackup is a pointer so the below lock should be safe and
	// prevent others from accessing the same channels
	srvCopy.commWithSchedulerForBackup.Mutex.Lock()
	defer func() {
		srvCopy.commWithSchedulerForBackup.Mutex.Unlock()
		log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStop"}).Debug("Lock released" +
			" after communicating with the scheduler routine")
	}()
	log.WithFields(log.Fields{"context": loggingContext + ".handlerPostBackupStop"}).Debug("Lock for scheduler " +
		"routine acquired")
	// send command to scheduling routine
	srvCopy.commWithSchedulerForBackup.ReceivedCommand <- command
	httpUser, _, _ := r.BasicAuth()
	if decodedJson.Id == "" {
		logger.Infof("Backup job stop for job having name '%s' has been requested by '%s' from '%s'",
			decodedJson.Name, httpUser, r.RemoteAddr)
	} else {
		logger.Infof("Backup job stop for job having name '%s' and id '%s' has been requested by '%s' " +
			"from '%s'", decodedJson.Name, decodedJson.Id, httpUser, r.RemoteAddr)
	}
	var result shared.ResponseBackupCommand
	// wait for max 60 seconds for a response from the scheduling thread
	select {
	case result = <-srvCopy.commWithSchedulerForBackup.SendResponse:
		{
			logger.Debugf("Received response %+v from scheduling component", result)
			if result.Err == false {
				if command.Id != result.Id {
					logger.Errorf("Request to stop backup job '%s' had id '%s' but response id is '%s'. This " +
						"is a bug and should be reported.")
					JSONError(w, http.StatusInternalServerError, HttpErrInternalError, "Response id does not " +
						"match request id. This is a bug. None the less, the backup job may have stopped so please " +
						"check the status of backup jobs")
					return
				}
				requestResult := BackupJob{
					Name: decodedJson.Name,
					Id: result.BackupJobId,
				}
				JSONSuccessWithResult(w, "success", "successfully requested backup job to be stopped",
					requestResult)
			}
		}
	case <-time.After(60 * time.Second):
		{
			logger.Warnf("Didn't receive in 60 seconds a response from the scheduling component. The request "+
				"was to stop a backup job for job having name '%s' and it has been requested by '%s' from '%s'",
				decodedJson.Name, httpUser, r.RemoteAddr)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalError, fmt.Sprintf("Didn't receive in " +
				"60 seconds a response from the scheduling component. The request was to stop a backup job for" +
				" job having name '%s'. This is abnormal unless your system is starved of CPU resources",
				decodedJson.Name))
			return
		}
	}
}

// return a summary of backup jobs (running and stopped)
func (srvSrc SrvData) handlerGetBackupList(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

}