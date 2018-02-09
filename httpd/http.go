package httpd

import (
	"cloudbackup/config"
	"context"
	log "github.com/sirupsen/logrus"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var logger = log.WithFields(log.Fields{
	"context": "httpd",
})

//type SrvData interface {
//	New(chan bool, int, string)
//}

type SrvData struct {
	// if we receive something over the channel then a configuration change happened and we are being notified
	rcvCfgChange chan bool
	// we send something over the channel in order to notify when we adjusted the global config
	sndCfgChange chan bool
	httpsrv *http.Server
	// when true then the web server is already being shutdown and cleanup is in progress
	serverExiting bool
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	globalcfg *config.Configuration
}

// pseudo constructor to setup a new http server
func New(rcvCfgChange chan bool, sndCfgChange chan bool, globalcfg *config.Configuration, port int, host string) (*SrvData) {
	return &SrvData{rcvCfgChange: rcvCfgChange,
					sndCfgChange: sndCfgChange,
					serverExiting: false,
					globalcfg: globalcfg,
					httpsrv: &http.Server{Addr:  host + ":" + strconv.Itoa(port),
										  Handler: nil,
										  },
	}
}

// server / and logger.Debug requester
func pageRoot(res http.ResponseWriter, req *http.Request){
	fmt.Fprint(res, "Http server is running\n")
	logger.Debug(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", req.RequestURI, req.RemoteAddr))
}

// start http server
func (srv *SrvData) Start() {
	logger.Info("Starting web server")
	http.HandleFunc("/", pageRoot)
	logger.Debug(fmt.Sprintf("%+v", srv))
	go func() {
		err := srv.httpsrv.ListenAndServe()
		if err != nil && srv.serverExiting == false {
			logger.Error("Http server could not be started or encountered an error during it's operation")
			logger.Error(err)
		}
	}()
}

// shutdown gracefully the http server using 30 sec timeout
func (srv *SrvData) Stop(){
	logger.Info("Shutting down the http server...")
	srv.serverExiting = true

	// preparation to exit with grace period of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := srv.httpsrv.Shutdown(ctx)
	if err != nil {
		logger.Error(err)
	}

}