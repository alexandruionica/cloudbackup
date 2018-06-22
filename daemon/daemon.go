package daemon

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/config"
	"cloudbackup/httpd"
	"cloudbackup/scheduler"
	"cloudbackup/shared"
	"os"
	"sync"
	"cloudbackup/daemon/globals"
)

const loggingContext = "daemon"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start(configFile string, debug bool) {
	globals.Stats.IncrementRoutines("other")
	// we use this to notify the HTTP server that the global config has changed
	sndCfgChangeToHttpd := make(chan bool, 50)
	// we use this to notify the Backup Scheduler that the global config has changed
	sndCfgChangeToScheduler := make(chan bool, 50)
	// we use this to get notified by the HTTP server that it changed the global config
	rcvCfgChangeFromHttpd := make(chan bool, 50)
	configMutex := &sync.RWMutex{}
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	configuration, err := config.Load(configFile, debug, configMutex)
	if err != nil {
		os.Exit(1)
	}
	//  struct containing the channels needed to communicate with the scheduler in order to start/stop Backups
	commWithSchedulerForBackup := &shared.CommWithSchedulerForBackup{}
	commWithSchedulerForBackup.Init()
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}

	var httpServer *httpd.SrvData
	if configuration.GetWithLock(loggingContext).Https.Enabled{
		logger.Info("Because the HTTPS server has been enabled the HTTP server will not be started")
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetWithLock(loggingContext).Https.BindAddress, true,
			configuration.GetWithLock(loggingContext).Https.SslCertPath,
			configuration.GetWithLock(loggingContext).Https.SslKeyPath, commWithSchedulerForBackup, backupJobsState)
	}else {
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetWithLock(loggingContext).Http.BindAddress, false, "",
			"", commWithSchedulerForBackup, backupJobsState)
	}

	httpServer.Start()
	scheduler.Start(sndCfgChangeToScheduler, commWithSchedulerForBackup, backupJobsState, configuration)

	// sleep until a SIGnal or an event is received
	WaitForEvent(httpServer, rcvCfgChangeFromHttpd, sndCfgChangeToScheduler, commWithSchedulerForBackup.Shutdown)
}
