package httpd

import (
	"cloudbackup/config"
	"cloudbackup/password"
	"context"
	log "github.com/sirupsen/logrus"
	"github.com/julienschmidt/httprouter"
	"fmt"
	"net/http"
	"time"
	"sync"
	"encoding/json"
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
	globalcfg *config.RuntimeConfig
	// lock this before reading or writing the loaded config variables
	Mutex *sync.RWMutex
}

func (srv *SrvData) GetWithLock(logContext string) SrvData {
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying HTTPD config " +
		"struct")
	srv.Mutex.RLock()
	defer func() {
		srv.Mutex.RUnlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying HTTPD " +
			"config struct")
	}()
	cfgCopy := *srv
	return cfgCopy
}

// pseudo constructor to setup a new http server
func New(rcvCfgChange chan bool, sndCfgChange chan bool, globalcfg *config.RuntimeConfig, addr string,
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
		Mutex: &sync.RWMutex{},
	}
}

// serve / and logger.Info requester
func (srvSrc SrvData) handlerRoot(res http.ResponseWriter, req *http.Request, _ httprouter.Params){
	srv := srvSrc.GetWithLock(loggingContext + ".handlerRoot")
	if srv.httpsEnabled{
		_, err := res.Write([]byte("HTTPS server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	} else {
		_, err := res.Write([]byte("HTTP server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	}
	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", req.RequestURI, req.RemoteAddr))
}

// serve $api_prefix/config and logger.Info requester
func (srvSrc SrvData) handlerGetConfig(res http.ResponseWriter, req *http.Request, _ httprouter.Params){
	srv := srvSrc.GetWithLock(loggingContext + "_pageRoot")
	runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".handlerGetConfig")

	// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
	//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
	js, err := json.Marshal(config.SanitizeCfgTemplate(runtimeCfg))
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	res.Header().Set("Content-Type", "application/json")
	_, err = res.Write(js)
	if err != nil {
		logger.Debug("handlerGetConfig() - could not write response back to client ")
	}

	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", req.RequestURI, req.RemoteAddr))
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
	apiPrefix := "/api/v1"
	router := httprouter.New()
	router.GET("/", srv.handlerRoot)
	router.GET(apiPrefix + "/config", srv.BasicAuth(srv.handlerGetConfig))
	// put a write lock and update the router - by this point all routes should have been added
	srv.Mutex.Lock()
	srv.httpsrv.Handler = router
	srv.Mutex.Unlock()
	logger.Debug(fmt.Sprintf("%+v", srv))
	go func() {
		var err error
		var extraMsg string
		if srv.httpsEnabled {
			extraMsg = "HTTPS"
			err = srv.httpsrv.ListenAndServeTLS(srv.SslCertPath, srv.SslKeyPath)
		} else {
			extraMsg = "HTTP"
			err = srv.httpsrv.ListenAndServe()
		}
		srvCopy := srv.GetWithLock(loggingContext)
		if err != nil && srvCopy.serverExiting == false {
			logger.Errorf("%s server could not be started or encountered an error during it's operation",
				extraMsg)
			logger.Error(err)
		}
	}()
}

// shutdown gracefully the http server using 30 sec timeout
func (srv *SrvData) Stop(){
	logger.Info("Shutting down the http server...")
	srv.Mutex.Lock()
	srv.serverExiting = true
	srv.Mutex.Unlock()

	// preparation to exit with grace period of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := srv.httpsrv.Shutdown(ctx)
	if err != nil {
		logger.Error(err)
	}

}

// provides basic Authentication agains username + password hashes stored in the config
// returns a httprouter.Handle function
func (srvSrc *SrvData) BasicAuth(handle httprouter.Handle) httprouter.Handle {
	return func(res http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		// Get the Basic Authentication credentials
		httpUser, httpPassword, hasAuth := req.BasicAuth()
		srv := srvSrc.GetWithLock(loggingContext + ".BasicAuth")
		runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".BasicAuth")
		isAuthenticated := false

		if hasAuth {
			logger.Debugf("Checking if user: '%s' provided via HTTP(S) matches any username + password hash " +
				"from the config", httpUser)
			if len(runtimeCfg.User) == 0 {
				logger.Debug("The configuration doesn't have a 'User' section defined so http(s) authentication " +
					"will fail ")
			} else {
				// check if a matching username + pass exists
				for _, user := range runtimeCfg.User {
					if user.Name == httpUser {
						logger.Debugf("Username '%s' matches an entry from the config, checking if password" +
							" matches the stored hash", httpUser)
						if password.CheckPasswordHash(httpPassword, user.Pass) {
							logger.Debugf("Password provided for username '%s' matches stored password hash",
								httpUser)
							isAuthenticated = true
							break
						}
					}
				}
			}

			if isAuthenticated == false {
				logger.Debug("Could not find any matching username + password(hash) in the config")
			}
		}

		if isAuthenticated {
			// Delegate request to the given handle
			handle(res, req, ps)
		} else {
			// Request Basic Authentication otherwise
			res.Header().Set("WWW-Authenticate", "Basic realm=Restricted")
			http.Error(res, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
	}
}