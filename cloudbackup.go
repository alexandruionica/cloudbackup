package main

//noinspection GoRedundantImportAlias
import (
	flags "github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"cloudbackup/httpd"
	"cloudbackup/config"
	"cloudbackup/misc"
	"cloudbackup/utils"
	"sync"
)
const loggingContext = "main"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

var args misc.Args


func main() {
	_, err := flags.Parse(&args)
	if err != nil {
		os.Exit(1)
	}
	misc.SetupLogging(&args)
	// we use this to notify the HTTP server that the global config has changed
	sndCfgChangeToHttpd := make(chan bool)
	// we use this to get notified by the HTTP server that it changed the global config
	rcvCfgChangeFromHttpd := make(chan bool)
	configMutex := &sync.Mutex{}
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	configuration, err := config.Load(args.ConfigFile, args.Debug, configMutex)
	if err != nil {
		os.Exit(1)
	}

	utils.Pp(configuration.GetWithLock(loggingContext))

	var httpServer *httpd.SrvData
	if configuration.GetWithLock(loggingContext).Https.Enabled{
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetWithLock(loggingContext).Https.BindAddress, true,
				configuration.GetWithLock(loggingContext).Https.SslCertPath,
					configuration.GetWithLock(loggingContext).Https.SslKeyPath)
	}else {
		httpServer = httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,
			configuration.GetWithLock(loggingContext).Http.BindAddress, false, "", "")
	}

	httpServer.Start()

	// sleep until a SIGnal is received
	misc.WaitForSignal(httpServer)

	logger.Info("Eng of program")
}
