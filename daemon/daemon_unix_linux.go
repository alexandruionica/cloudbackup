// +build !windows

package daemon

import (
	"cloudbackup/httpd"
	"os"
	"os/signal"
	"syscall"
	"cloudbackup/daemon/globals"
	"fmt"
)

// sleeps until it receives on one of the many channels an event
func WaitForEvent(httpServer *httpd.SrvData, rcvCfgChangeFromHttpd <-chan bool, sndCfgChangeToScheduler chan<- bool,
	shutdownScheduler chan bool) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGUSR1,)
	// infinite loop
	for {
		select {
		// received a SIGnal
		case s := <-signalChan:
			ProcessSignal(s, httpServer, shutdownScheduler)
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
func ProcessSignal(s os.Signal, httpServer *httpd.SrvData, shutdownScheduler chan bool) {
	switch s {
	case syscall.SIGINT:
		logger.Info("Received SIGINT")
		httpServer.Stop()
		// tell scheduler to stop (and also stop running backups / restores )
		shutdownScheduler <- true
		// scheduler will reply back on the same channel when it has exited
		_ = <- shutdownScheduler
		logger.Info("Exiting")
		os.Exit(0)

	case syscall.SIGTERM:
		logger.Info("Received SIGTERM")
		httpServer.Stop()
		// tell scheduler to stop (and also stop running backups / restores )
		shutdownScheduler <- true
		logger.Info("Exiting")
		os.Exit(0)

	case syscall.SIGUSR1:
		logger.Info("Received SIGUSR1")
		globals.Stats.Log()

	default:
		logger.Warn(fmt.Sprintf("Received unknown signal: %s . Ignoring", s))
	}
}
