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

const SampleYamlConfig = `# host and port for the HTTP server; if HTTPS server is enabled then http server is automatically disabled. 
# By default HTTP server is enabled and HTTPS is disabled
#http:
#  bind_address: "127.0.0.1:8080"
https:
  enabled: true
  bind_address: "127.0.0.1:8443"
  ssl_cert_path: /etc/ssl/cert.crt
  ssl_key_path: /etc/ssl/cert.key
backup:
  - name: generic
    paths:
      - /something
      - /var/lib
    exclusions:
      - /something/else
      - /var/lib/mysql
    targets:
      - name: aws_1
        type: s3
        user: AWS_ACCESS_KEY_ID
        pass: AWS_SECRET_ACCESS_KEY
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        storage_class: standard
    schedule:
      - '05 01 * * *'
  - name: http_logs
    paths:
      - /var/log
      - /var/www/html/data/log/
    targets:
      - name: aws_2
        type: aws_s3
        user: JOHNDOE
        pass: qwqe
        bucket: 'some-stuff-goes-here'
        prefix: 'backup/backups-for-server-51'
        storage_class: 'infrequent-access'
      - name: google_1
        type: google_cloud_storage
        user: JANEDOE
        pass: 34324fd
        bucket: 'my-google-bucket'
        prefix: 'backup/backups-for-server-51'
        storage_class: standard
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - '00 08 01 * *'
      - '00 08 06 * *'
    versioning: true
    versions_max_num: 10
    versions_max_age: 6w`

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