package daemon

import (
	log "github.com/sirupsen/logrus"
	"cloudbackup/config"
	"cloudbackup/httpd"
	"cloudbackup/misc"
	"os"
	"sync"
)

const loggingContext = "daemon"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})


//type ArgsCommandStart struct {
//	Verbose bool `short:"v" long:"verbose" description:"Set logging to verbose"`
//	Debug bool `short:"d" long:"debug" description:"Set logging to debug"`
//	TextLog bool `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
//	ConfigFile string `short:"c" long:"configfile" description:"Configuration file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
//}


func Start(configFile string, debug bool) {
	// we use this to notify the HTTP server that the global config has changed
	sndCfgChangeToHttpd := make(chan bool)
	// we use this to get notified by the HTTP server that it changed the global config
	rcvCfgChangeFromHttpd := make(chan bool)
	configMutex := &sync.Mutex{}
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	configuration, err := config.Load(configFile, debug, configMutex)
	if err != nil {
		os.Exit(1)
	}

	var httpServer *httpd.SrvData
	if configuration.GetWithLock(loggingContext).Https.Enabled{
		logger.Info("Because the HTTPS server has been enabled the HTTP server will not be started")
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

	// return to Main (cloudbackup.go top level file)
}