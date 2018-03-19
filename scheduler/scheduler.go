package scheduler

import (
	"time"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "scheduler"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func Start (cfgChange <-chan bool) {
	go daemon(cfgChange)
	return
}

func daemon (cfgChange <-chan bool) {
	logger.Info("Starting scheduling component")
	const SleepSec = 1
	// infinite loop
	for {
		select {
		case _ = <-cfgChange:
			{
				logger.Debug("Scheduler reloading configuration")
			}
		default:
			{
				// TODO - add code to launch and scheduled backups or restores
				//logger.Debugf("Sleeping for %d seconds", SleepSec)
				time.Sleep(SleepSec * time.Second)
			}
		}

	}
}