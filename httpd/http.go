package httpd

import (
	"cloudbackup/config"
	"context"
	log "github.com/sirupsen/logrus"
	"fmt"
	"net/http"
	"time"
)
const loggingContext = "httpd"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
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
	SslCertPath string
	SslKeyPath string
	httpsEnabled bool
	// when true then the web server is already being shutdown and cleanup is in progress
	serverExiting bool
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	globalcfg *config.Configuration
}

// pseudo constructor to setup a new http server
func New(rcvCfgChange chan bool, sndCfgChange chan bool, globalcfg *config.Configuration, addr string,
	httpsEnabled bool, SslCertPath string, SslKeyPath string ) (*SrvData) {

	return &SrvData{rcvCfgChange: rcvCfgChange,
		sndCfgChange: sndCfgChange,
		serverExiting: false,
		globalcfg: globalcfg,
		httpsrv: &http.Server{
			Addr: addr,
			Handler: nil,
		},
		SslCertPath: SslCertPath,
		SslKeyPath: SslKeyPath,
		httpsEnabled: httpsEnabled,
	}
}

// server / and logger.Debug requester
func (srv SrvData) pageRoot(res http.ResponseWriter, req *http.Request){
	if srv.httpsEnabled{
		fmt.Fprint(res, "HTTPS server is running\n")
	} else {
		fmt.Fprint(res, "HTTP server is running\n")
	}
	logger.Debug(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", req.RequestURI, req.RemoteAddr))
}

// start http server
func (srv *SrvData) Start() {
	var protocol, msg string
	if srv.httpsEnabled {
		protocol = "https://"
		msg = fmt.Sprintf(" using ssl certificate %s and ssl key %s", srv.SslCertPath, srv.SslKeyPath)
	} else {
		msg = ""
		protocol = "http://"
	}
	logger.Infof("Starting web server to listen on %s%s%s", protocol, srv.httpsrv.Addr, msg)
	http.HandleFunc("/", srv.pageRoot)
	logger.Debug(fmt.Sprintf("%+v", srv))
	go func() {
		var err error
		var extraMsg string
		if srv.httpsEnabled {
			extraMsg = "HTTPS"
			err = srv.httpsrv.ListenAndServeTLS(srv.globalcfg.GetWithLock(loggingContext).Https.SslCertPath,
				srv.globalcfg.GetWithLock(loggingContext).Https.SslKeyPath)
		} else {
			extraMsg = "HTTP"
			err = srv.httpsrv.ListenAndServe()
		}
		if err != nil && srv.serverExiting == false {
			logger.Errorf("%s server could not be started or encountered an error during it's operation",
				extraMsg)
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