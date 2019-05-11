package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/shared"
	"context"
	"fmt"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"net/http"
	"sync"
	"time"
)

const (
	loggingContext = "httpd"
	ApiPrefix      = "/api/v1"
)

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// used to check if a PATH and method are accessible for users which have read-only type access . Path will be
// prefixed with $ApiPrefix in the calling function
var ReadAccess = map[string][]string{
	//"POST": []string{"aaa", "bbb"},
	"GET":  {"/config", "/backup/list"},
	"POST": {"/backup/dryrun", "/backup/watch"},
}

// pseudo constructor to setup a new http server
func New(rcvCfgChange chan bool, sndCfgChange chan bool, globalcfg *shared.RuntimeConfig, addr string,
	httpsEnabled bool, SslCertPath string, SslKeyPath string,
	commWithSchedulerForBackup *shared.CommWithSchedulerForBackup,
	backupJobsState *shared.BackupJobsState) *SrvData {

	return &SrvData{rcvCfgChange: rcvCfgChange,
		sndCfgChange:  sndCfgChange,
		serverExiting: false,
		globalcfg:     globalcfg,
		httpsrv: &http.Server{
			Addr:    addr,
			Handler: nil,
		},
		SslCertPath:                SslCertPath,
		SslKeyPath:                 SslKeyPath,
		httpsEnabled:               httpsEnabled,
		Mutex:                      &sync.RWMutex{},
		commWithSchedulerForBackup: commWithSchedulerForBackup,
		backupJobsState:            backupJobsState,
	}
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
	staticHtmlDir := srv.globalcfg.GetCopyWithLock(loggingContext).HtmlDir
	logger.Infof("Starting web server to listen on %s%s%s", protocol, srv.httpsrv.Addr, msg)
	router := httprouter.New()
	router.GET("/", srv.handlerRoot)
	// serve documentation - static files - NO AUTHENTICATION needed; NO REQUEST LOGGING done
	router.ServeFiles("/docs/*filepath", http.Dir(staticHtmlDir+"/docs"))
	router.ServeFiles("/docs_api/*filepath", http.Dir(staticHtmlDir+"/docs_api"))
	// redirect /swgger.json to /docs/api/swgger.json - NO AUTHENTICATION needed; NO REQUEST LOGGING done
	router.GET("/swagger.json", handlerGETtlSwaggerJson)
	// redirect /swgger.yaml to /docs/api/swgger.yaml - NO AUTHENTICATION needed; NO REQUEST LOGGING done
	router.GET("/swagger.yaml", handlerGETtlSwaggerYaml)
	// API endpoints - MUST wrap around srv.BasicAuth(srv.CheckAccess($HANDLER_NAME))
	router.GET(ApiPrefix+"/config", srv.BasicAuth(srv.CheckAccess(srv.handlerGetConfig)))
	router.POST(ApiPrefix+"/config", srv.BasicAuth(srv.CheckAccess(srv.handlerPutConfig)))
	router.POST(ApiPrefix+"/config/backup", srv.BasicAuth(srv.CheckAccess(srv.handlerPutConfigBackup)))
	router.POST(ApiPrefix+"/backup/start", srv.BasicAuth(srv.CheckAccess(srv.handlerPostBackupStart)))
	router.POST(ApiPrefix+"/backup/stop", srv.BasicAuth(srv.CheckAccess(srv.handlerPostBackupStop)))
	router.GET(ApiPrefix+"/backup/list", srv.BasicAuth(srv.CheckAccess(srv.handlerGetBackupList)))
	router.POST(ApiPrefix+"/backup/dryrun", srv.BasicAuth(srv.CheckAccess(srv.handlerPostBackupDryRun)))
	router.POST(ApiPrefix+"/backup/watch", srv.BasicAuth(srv.CheckAccess(srv.handlerPostBackupWatch)))
	router.POST(ApiPrefix+"/backup/target/test", srv.BasicAuth(srv.CheckAccess(srv.handlerPostBackupTargetTest)))
	router.POST(ApiPrefix+"/report/notification/test", srv.BasicAuth(srv.CheckAccess(srv.handlerPostNotificationTest)))

	// put a write lock and update the router - by this point all routes should have been added
	srv.Mutex.Lock()
	srv.httpsrv.Handler = router
	srv.Mutex.Unlock()
	logger.Debug(fmt.Sprintf("%+v", srv))
	// start http or https server in a separate routine
	go func() {
		globals.Stats.IncrementRoutines("other")
		defer globals.Stats.DecrementRoutines("other")
		var err error
		var extraMsg string
		if srv.httpsEnabled {
			extraMsg = "HTTPS"
			err = srv.httpsrv.ListenAndServeTLS(srv.SslCertPath, srv.SslKeyPath)
		} else {
			extraMsg = "HTTP"
			err = srv.httpsrv.ListenAndServe()
		}
		srvCopy := srv.GetCopyWithLock(loggingContext)
		if err != nil && !srvCopy.serverExiting {
			logger.Errorf("%s server could not be started or encountered an error during it's operation",
				extraMsg)
			logger.Error(err)
		}
	}()
}

// shutdown gracefully the http server using 30 sec timeout
func (srv *SrvData) Stop() {
	logger.Debug("Shutting down the http server...")
	srv.Mutex.Lock()
	srv.serverExiting = true
	srv.Mutex.Unlock()

	// preparation to exit with grace period of 10 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := srv.httpsrv.Shutdown(ctx)
	if err != nil {
		logger.Error(err)
	}

}
