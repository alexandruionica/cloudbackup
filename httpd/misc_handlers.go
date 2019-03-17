package httpd

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net/http"
)

// serve / and logger.Info requester
func (srvSrc SrvData) handlerRoot(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	LogHttpRequest(r)
	srv := srvSrc.GetCopyWithLock(loggingContext + ".handlerRoot")
	if srv.httpsEnabled {
		_, err := w.Write([]byte("HTTPS server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	} else {
		_, err := w.Write([]byte("HTTP server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	}
	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", r.RequestURI, r.RemoteAddr))
}
