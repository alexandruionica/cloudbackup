package daemon

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/config"
	"cloudbackup/httpd"
	"cloudbackup/scheduler"
	"cloudbackup/shared"
	"os"
	"sync"
	"os/signal"
	"syscall"
	"fmt"
)

const loggingContext = "daemon"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start(configFile string, debug bool) {
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
	WaitForEvent(httpServer, rcvCfgChangeFromHttpd, sndCfgChangeToScheduler)
}

// sleeps until it receives on one of the many channels an event
func WaitForEvent(httpServer *httpd.SrvData, rcvCfgChangeFromHttpd <-chan bool, sndCfgChangeToScheduler chan<- bool) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	// infinite loop
	for {
		select {
		// received a SIGnal
		case s := <-signalChan:
			ProcessSignal(s, httpServer)
		// received an event
		case _ = <- rcvCfgChangeFromHttpd:
			logger.Debug("Notifying scheduler to reload configuration")
			sndCfgChangeToScheduler <- true
			if len(sndCfgChangeToScheduler) > 5 {
				logger.Warnf("%d messages pending processing by scheduler", len(sndCfgChangeToScheduler))
			}
			if len(rcvCfgChangeFromHttpd) > 5 {
				logger.Warnf("%d messages pending processing by event processor", len(rcvCfgChangeFromHttpd))
			}
		}
	}
}

// reacts to various system SIGNALS and takes care of exiting cleanly if such a signal is received
func ProcessSignal(s os.Signal, httpServer *httpd.SrvData) {
	switch s {
	case syscall.SIGINT:
		logger.Info("Received SIGINT")
		httpServer.Stop()
		// TODO - tell scheduler to stop (and also stop running backups / restores )
		logger.Info("Exiting")
		os.Exit(0)

	case syscall.SIGTERM:
		logger.Info("Received SIGTERM")
		httpServer.Stop()
		// TODO - tell scheduler to stop (and also stop running backups / restores )
		logger.Info("Exiting")
		os.Exit(0)

	default:
		logger.Warn(fmt.Sprintf("Received unknown signal: %s . Ignoring", s))
	}
}