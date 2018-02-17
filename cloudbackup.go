package main

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
var logger = log.WithFields(log.Fields{
	"context": "main",
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
	utils.Pp(configuration.GetWithLock())

	httpServer := httpd.New(sndCfgChangeToHttpd, rcvCfgChangeFromHttpd, configuration,8080, "localhost")
	httpServer.Start()

	// sleep until a SIGnal is received
	misc.WaitForSignal(httpServer)

	logger.Info("Eng of program")
}
