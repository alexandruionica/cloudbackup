package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/shared"
	"context"
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

// RestoreJobResumeRequest is the JSON body accepted by POST /restore/resume. It identifies a
// previously-crashed restore job by (name, target_name, restore_job_id), so the scheduler can
// locate the correct per-target restore DB and hand execution to restore.Resume.
type RestoreJobResumeRequest struct {
	Name         string `json:"name"`
	TargetName   string `json:"target_name"`
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

func (srvSrc SrvData) handlerPostRestoreResume(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		return
	}
	var decoded RestoreJobResumeRequest
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decoded.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'name' key is mandatory")
		return
	}
	if decoded.TargetName == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'target_name' key is mandatory")
		return
	}
	if decoded.RestoreJobId == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'restore_job_id' key is mandatory")
		return
	}

	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostRestoreResume")
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostRestoreResume")
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

	if srvCopy.backupJobsState.IsRunning(decoded.Name, "", loggingContext+".handlerPostRestoreResume") {
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
		Id:           u.String(),
		Command:      "resume",
		Name:         decoded.Name,
		TargetName:   decoded.TargetName,
		RestoreJobId: decoded.RestoreJobId,
	}
	httpUser, _, _ := r.BasicAuth()
	select {
	case srvCopy.commWithSchedulerForRestore.ReceivedCommand <- command:
	case <-time.After(5 * time.Second):
		JSONError(w, http.StatusInternalServerError, HttpErrInternalError,
			"Sending restore resume to scheduler timed out after 5 seconds")
		return
	}
	logger.Infof("Restore resume for '%s' (restore_job_id '%s') requested by '%s' from '%s'",
		decoded.Name, decoded.RestoreJobId, httpUser, r.RemoteAddr)

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
			fmt.Sprintf("Could not resume restore for '%s': %s", decoded.Name, result.Message))
		return
	}
	JSONSuccessWithResult(w, "success", "Successfully requested restore to be resumed",
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

// RestoreWatchRequest is the JSON body accepted by POST /restore/watch.
type RestoreWatchRequest struct {
	Name         string `json:"name"`
	RestoreJobId string `json:"restore_job_id"`
}

// handlerPostRestoreWatch streams Server-Sent Events with real-time progress of a running
// restore job. The implementation mirrors handlerPostBackupWatch so that clients can use the
// same SSE parsing logic for both backup and restore watching.
func (srvSrc SrvData) handlerPostRestoreWatch(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		return
	}
	var decoded RestoreWatchRequest
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decoded.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, "'name' key is mandatory. The name"+
			" is needed in order to know what restore job you're requesting to watch")
		return
	}

	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostRestoreWatch")

	if !srvCopy.backupJobsState.IsRunning(decoded.Name, decoded.RestoreJobId, loggingContext+".handlerPostRestoreWatch") {
		var errorMsg string
		if decoded.RestoreJobId != "" && srvCopy.backupJobsState.IsRunning(decoded.Name, "", loggingContext+".handlerPostRestoreWatch") {
			errorMsg = fmt.Sprintf("Restore for job having name '%s' and a restore job id of '%s' is not "+
				"running so it can't be watched. There is a running restore job for the same name but with a "+
				"different job id", decoded.Name, decoded.RestoreJobId)
		} else {
			errorMsg = fmt.Sprintf("Restore for job having name '%s' is not running.", decoded.Name)
		}
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, errorMsg)
		return
	}

	if srvCopy.backupJobsState.IsStopping(decoded.Name, decoded.RestoreJobId, loggingContext+".handlerPostRestoreWatch") {
		JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Restore for job having "+
			"name '%s' is stopping so it can't be watched any more.", decoded.Name))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		logger.Debugf("HTTP2 Streaming unsupported in handlerPostRestoreWatch()")
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		msg := fmt.Sprintf("Could not generate a UUID so the watch operation can't proceed. The encountered error is: %s", err)
		logger.Error(msg)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	jobid := ""
	if decoded.RestoreJobId != "" {
		jobid = decoded.RestoreJobId
	} else {
		jobid, err = srvCopy.backupJobsState.GetRunningBackupJobId(decoded.Name, loggingContext+".handlerPostRestoreWatch")
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Restore for job having "+
				"name '%s' has started stopping so it can't be watched any more.", decoded.Name))
			return
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	commChan := make(chan shared.WatchMessage, 500)
	clientUUID := u.String()
	err = srvCopy.backupJobsState.Watcher.AddConsumer("restore", decoded.Name, jobid, commChan, ctx, cancel,
		r.RemoteAddr, clientUUID)
	if err != nil {
		_, _ = fmt.Fprintf(w, "data: %s\n", err.Error()) // #nosec
		close(commChan)
		return
	}

	for {
		select {
		case <-ctx.Done():
			{
				srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
				_, _ = fmt.Fprintf(w, "data: %s\n", "Backup server has been requested to exit. All running "+
					"jobs are being stopped.") // #nosec
				close(commChan)
				return
			}
		case <-r.Context().Done():
			{
				srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
				close(commChan)
				return
			}
		case message := <-commChan:
			{
				if message.JobCompleted || message.JobAborted || message.JobFailed {
					srvCopy.backupJobsState.Watcher.RemoveConsumer(r.RemoteAddr, clientUUID)
					if message.JobAborted {
						_, _ = fmt.Fprintf(w, "data: %s\n", "Restore job was cancelled while it was running") // #nosec
					}
					if message.JobFailed {
						_, _ = fmt.Fprintf(w, "data: %s\n", "Restore job failed to start. Check server logs for details") // #nosec
					}
					if message.JobCompleted {
						_, _ = fmt.Fprintf(w, "data: %s\n", "Restore job has finished") // #nosec
					}
					close(commChan)
					return
				}
				if message.ObjectType == "dir" {
					message.ObjectType = "directory"
				}
				jsonMsg, err := json.Marshal(message)
				if err != nil {
					logger.Warnf("Could not json encode message received from restore job live status. Error was: '%s'", err)
				} else {
					_, _ = fmt.Fprintf(w, "data: %s\n", jsonMsg) // #nosec
					flusher.Flush()
				}
			}
		}
	}
}
