package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/notifications"
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
		JSONError(w, 500, HttpErrInternalServerError, "Notification test can not be run as there " +
			"are no notification entries in the server's configuration file")
		return
	}

	err := notifications.Execute(configCopy, "", "backup", "test", "notifications test", "", "")
	if err != nil {
		JSONError(w, 500, HttpErrInternalServerError, err.Error())
		return
	}

	JSONSuccess(w, "success", "Test completed successfully")
	return
}
