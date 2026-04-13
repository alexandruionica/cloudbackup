package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/shared"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
)

// RestoreJobRequest is the JSON body accepted by POST /restore/start.
type RestoreJobRequest struct {
	Name              string   `json:"name"`
	SourceBackupJobId string   `json:"source_backup_job_id"`
	TargetName        string   `json:"target_name,omitempty"`
	Files             []string `json:"files,omitempty"`
	AllFiles          bool     `json:"all_files,omitempty"`
	RestoreDir        string   `json:"restore_dir,omitempty"`
	Exclusions        []string `json:"exclusions,omitempty"`
}

// RestoreJobStopRequest is the JSON body accepted by POST /restore/stop.
type RestoreJobStopRequest struct {
	Name         string `json:"name"`
	RestoreJobId string `json:"restore_job_id"`
}

// RestoreJobResponse is what successful /restore/start responses return in the "result" field.
type RestoreJobResponse struct {
	Name         string `json:"name"`
	RestoreJobId string `json:"restore_job_id"`
}

func (srvSrc SrvData) handlerPostRestoreStart(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		return
	}
	var decoded RestoreJobRequest
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decoded.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'name' key is mandatory")
		return
	}
	if decoded.SourceBackupJobId == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'source_backup_job_id' key is mandatory")
		return
	}
	if decoded.AllFiles && len(decoded.Files) > 0 {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'files' and 'all_files' are mutually exclusive")
		return
	}
	if !decoded.AllFiles && len(decoded.Files) == 0 {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "either 'files' or 'all_files' must be specified")
		return
	}

	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostRestoreStart")
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostRestoreStart")
	found := false
	configCopy.Mutex.RLock()
	for _, b := range configCopy.Backup {
		if b.Name == decoded.Name {
			found = true
			break
		}
	}
	configCopy.Mutex.RUnlock()
	if !found {
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job found matching name: %s", decoded.Name))
		return
	}

	if srvCopy.backupJobsState.IsRunning(decoded.Name, "", loggingContext+".handlerPostRestoreStart") {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData,
			fmt.Sprintf("A job (backup or restore) for '%s' is already running.", decoded.Name))
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}
	command := shared.ReceiveRestoreCommand{
		Id:                 u.String(),
		Command:            "start",
		Name:               decoded.Name,
		SourceBackupJobId:  decoded.SourceBackupJobId,
		TargetName:         decoded.TargetName,
		Files:              decoded.Files,
		AllFiles:           decoded.AllFiles,
		RestoreDirOverride: decoded.RestoreDir,
		Exclusions:         decoded.Exclusions,
	}
	httpUser, _, _ := r.BasicAuth()
	select {
	case srvCopy.commWithSchedulerForRestore.ReceivedCommand <- command:
	case <-time.After(5 * time.Second):
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError,
			"Sending restore start to scheduler timed out after 5 seconds")
		return
	}
	logger.Infof("Restore start for '%s' requested by '%s' from '%s'", decoded.Name, httpUser, r.RemoteAddr)

	var result shared.ResponseRestoreCommand
	select {
	case result = <-srvCopy.commWithSchedulerForRestore.SendResponse:
	case <-time.After(20 * time.Second):
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError,
			"Did not receive a response from the scheduler within 20 seconds")
		return
	}
	if result.Err {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData,
			fmt.Sprintf("Could not start restore for '%s': %s", decoded.Name, result.Message))
		return
	}
	JSONSuccessWithResult(w, "success", "Successfully requested restore to be started",
		RestoreJobResponse{Name: decoded.Name, RestoreJobId: result.RestoreJobId})
}

func (srvSrc SrvData) handlerPostRestoreStop(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		return
	}
	var decoded RestoreJobStopRequest
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decoded.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'name' key is mandatory")
		return
	}

	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostRestoreStop")
	if !srvCopy.backupJobsState.IsRunning(decoded.Name, decoded.RestoreJobId, loggingContext+".handlerPostRestoreStop") {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData,
			fmt.Sprintf("Restore for '%s' is not running.", decoded.Name))
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}
	command := shared.ReceiveRestoreCommand{
		Id:           u.String(),
		Command:      "stop",
		Name:         decoded.Name,
		RestoreJobId: decoded.RestoreJobId,
	}
	select {
	case srvCopy.commWithSchedulerForRestore.ReceivedCommand <- command:
	case <-time.After(5 * time.Second):
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError,
			"Sending restore stop to scheduler timed out after 5 seconds")
		return
	}
	var result shared.ResponseRestoreCommand
	select {
	case result = <-srvCopy.commWithSchedulerForRestore.SendResponse:
	case <-time.After(20 * time.Second):
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError,
			"Did not receive a response from the scheduler within 20 seconds")
		return
	}
	if result.Err {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData,
			fmt.Sprintf("Could not stop restore for '%s': %s", decoded.Name, result.Message))
		return
	}
	JSONSuccessWithResult(w, "success", "Successfully requested restore to be stopped",
		RestoreJobResponse{Name: decoded.Name, RestoreJobId: result.RestoreJobId})
}

// handlerGetRestoreList returns the list of currently running restore jobs. Unlike
// /backup/list, it does not emit placeholder "stopped" entries, since restores are ephemeral.
func (srvSrc SrvData) handlerGetRestoreList(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerGetRestoreList")
	JSONSuccessWithResult(w, "success", "success",
		srvCopy.backupJobsState.GetRestoresRunning(loggingContext+".handlerGetRestoreList"))
}
