package misc

import (
	log "github.com/sirupsen/logrus"
	"os"
)

const loggingContext = "misc"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type LoggingArgs struct {
	Debug bool
	Quiet bool
	TextLog bool
}

const SampleYamlConfig = `# where are the internal SQL databases to be kept
data_dir: /var/lib/cloudbackup
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup hash-password to hash passwords
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
	# can be either 'read' or 'write' . 'write' basically gives access to all the API while 'read' only to read-only
    #  operations so for example it excludes things starting/stopping backups or adjusting the configuration
    access: write
# host and port for the HTTP server; if HTTPS server is enabled then http server is automatically disabled. 
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
    target:
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
		if args.Quiet {
			log.SetLevel(log.WarnLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}
	}

}