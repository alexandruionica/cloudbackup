package httpd

import (
	"cloudbackup/misc"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"runtime"
)

// serve / and logger.Info requester. Redirects to the web UI at /ui/
func (srvSrc SrvData) handlerRoot(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	LogHttpRequest(r)
	http.Redirect(w, r, "/ui/", http.StatusFound)
	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", r.RequestURI, r.RemoteAddr))
}

func (srvSrc SrvData) handlerVersion(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	v := misc.CloudBackupVersion()
	v.OS = runtime.GOOS
	v.Arch = runtime.GOARCH
	v.Runtime = runtime.Version()
	JSONSuccessWithResult(w, "success", "success", v)
	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", r.RequestURI, r.RemoteAddr))
}
