package misc

import (
	log "github.com/sirupsen/logrus"
	"os"
	"cloudbackup/httpd"
	"os/signal"
	"syscall"
	"fmt"
)


var logger = log.WithFields(log.Fields{
	"context": "misc",
})

type Args struct {
	Verbose bool `short:"v" long:"verbose" description:"Set logging to verbose"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug"`
	TextLog bool `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
}

func SetupLogging(args *Args){
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