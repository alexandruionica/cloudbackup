package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/notifications"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
	"net/http"
)

// runs all Notification definitions from the config file, wait for them to complete(or fail) and reply to the client
func (srvSrc SrvData) handlerPostNotificationTest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostNotificationTest")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostNotificationTest")

	if notifications.GetNumNotificators(configCopy.Notifications) == 0 {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "Notification test can not be run as there "+
			"are no notification entries in the server's configuration file")
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		msg := fmt.Sprintf("Could not generate a UUID so the notification test operation can't be started. Encountered error was: %s", err)
		logger.Error(msg)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		return
	}
	jobId := u.String()
	_, err = notifications.Execute(configCopy, jobId, "backup", "test", "notifications_test", "", "")
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	JSONSuccess(w, "success", fmt.Sprintf("Test completed successfully for job id '%s'", jobId))
}
