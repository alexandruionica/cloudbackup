package daemon

import (
	"cloudbackup/config"
	"cloudbackup/daemon/globals"
	"cloudbackup/database"
	"cloudbackup/httpd"
	"cloudbackup/scheduler"
	"cloudbackup/shared"
	log "github.com/sirupsen/logrus"
	"os"
	"sync"
)

const loggingContext = "daemon"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start(configFile string, debug bool) {
	// backupJobState contains the state of all running backup/restore jobs plus it has some handy methods and also
	// contains state about opened databases
	backupJobsState := shared.NewJobsState()

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
	// create DB files, if needed
	err = config.ValidateAndCreateDB(configuration.Config, backupJobsState)
	if err != nil {
		os.Exit(1)
	}

	// find crashed jobs and update their database status to represent this
	err = database.CheckForCrashedJobs(configuration.Config, backupJobsState)
	if err != nil {
		os.Exit(1)
	}

	//  struct containing the channels needed to communicate with the scheduler in order to start/stop Backups
	commWithSchedulerForBackup := &shared.CommWithSchedulerForBackup{}
	commWithSchedulerForBackup.Init()

	var httpServer *httpd.SrvData
	if configuration.GetCopyWithLock(loggingContext).Https.Enabled {
		logger.Info("Because the HTTPS server has been enabled the HTTP server will not be started")
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetCopyWithLock(loggingContext).Https.BindAddress, true,
			configuration.GetCopyWithLock(loggingContext).Https.SslCertPath,
			configuration.GetCopyWithLock(loggingContext).Https.SslKeyPath, commWithSchedulerForBackup, backupJobsState)
	} else {
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetCopyWithLock(loggingContext).Http.BindAddress, false, "",
			"", commWithSchedulerForBackup, backupJobsState)
	}

	httpServer.Start()
	scheduler.Start(sndCfgChangeToScheduler, commWithSchedulerForBackup, backupJobsState, configuration)

	// sleep until a SIGnal or an event is received
	WaitForEvent(httpServer, rcvCfgChangeFromHttpd, sndCfgChangeToScheduler, commWithSchedulerForBackup.Shutdown)
}
