package httpd

import (
	"cloudbackup/daemon/globals"
	"github.com/julienschmidt/httprouter"
	"net/http"
)

// runs all Notification definitions from the config file
func (srvSrc SrvData) handlerPostNotificationTest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostBackupDryRun")

	// TODO - delete following line
	logger.Debug(srvCopy)
}
