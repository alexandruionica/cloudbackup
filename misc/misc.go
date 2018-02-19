package misc

import (
	log "github.com/sirupsen/logrus"
	"os"
	"cloudbackup/httpd"
	"os/signal"
	"syscall"
	"fmt"
)

const loggingContext = "misc"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type LoggingArgs struct {
	Verbose bool
	Debug bool
	TextLog bool
}

func SetupLogging(args LoggingArgs){
	log.SetOutput(os.Stdout)
	if args.TextLog {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}
	if args.Debug {
		log.SetLevel(log.DebugLevel)
		logger.Debug("Debug level messages enabled")
	} else {
		if args.Verbose {
			log.SetLevel(log.InfoLevel)
			logger.Info("Verbose level messages enabled")
		} else {
			log.SetLevel(log.WarnLevel)
		}
	}

}


// reacts to various system SIGNALS and takes care of exiting cleanly if such a signal is received
func WaitForSignal(httpServer *httpd.SrvData) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	for {
		s := <-signalChan
		switch s {
		case syscall.SIGINT:
			logger.Info("Received SIGINT")
			httpServer.Stop()
			logger.Info("Exiting")
			os.Exit(0)

		case syscall.SIGTERM:
			logger.Info("Received SIGTERM")
			httpServer.Stop()
			logger.Info("Exiting")
			os.Exit(0)

		default:
			logger.Warn(fmt.Sprintf("Received unknown signal: %s . Ignoring", s))
		}
	}


}