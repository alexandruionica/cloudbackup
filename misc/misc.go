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
	LogFile string
}

const SampleServerYamlConfig = `---
# where are the internal SQL databases to be kept
data_dir: /var/lib/cloudbackup
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup misc hash-password to hash passwords
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    # can be either 'read' or 'write' . 'write' basically gives access to all the API while 'read' only to read-only
    #  operations so for example it excludes things starting/stopping backups or adjusting the configuration
    access: write
  - name: testuser2
    # bcrypt hash of password  "Oonaawai8Eep]eethe8eefa$"
    pass: $2a$05$Pgdwe14mHjOQ33C5LahmmugCY85Yfqlkj2rGvbDMGCDXKKwmhbwVC
    access: read
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
    # Bourne-Again shell like globing and globstar is supported in the below "exclusions" section.  
    # Term	     Meaning
    # *	         matches any sequence of non-path-separators
    # **	     matches any sequence of characters, including path separators
    # ?	         matches any single non-path-separator character
    # [class]	 matches any single non-path-separator character against a class of characters (see below)
    # {alt1,...} matches a sequence of characters if one of the comma-separated alternatives matches
    #
    # Character classes support the following:
    # Class	 Meaning
    # [abc]    matches any single character within the set
    # [a-z]	 matches any single character in the range
    # [^class] matches any single character which does not match the class
    # Example:  
    #     - "**/*.txt"  would exclude all .txt files across all directories
    #     - "/var/lib/*.txt" would exclude .txt files only if they are located in "/var/lib/". If .txt files are 
    #         located in "/var/lib/something/" then they would not be matched by the exclusion rule
    # If using exclusion rules then please run "cloudbackup client backup dryrun" in order to check that they work as 
    #  expected
    exclusions:
      - /something/else
      - /var/lib/*.db
    target:
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
    # do not follow symbolic links (defaults to true)
    dereference: false
    # use the file's checksum in order to establish if a backup is needed (defaults to false)
    checksum: true
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
    # defaults to 0 which means unlimited number of versions
    versions_max_num: 10
    # defaults to 0 which means unlimited age
    versions_max_age: 6w`

const SampleClientYamlConfig = `---
username: testuser1
password: 'HV}H/y?<9$]Z5N4N'
address: http://127.0.0.1:8080`

func SetupLogging(args LoggingArgs){
	log.SetOutput(os.Stdout)

	if args.TextLog {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if args.LogFile != "" {
		logFile, err := os.OpenFile(args.LogFile, os.O_CREATE|os.O_WRONLY, 0640) // #nosec
		if err == nil {
			log.SetOutput(logFile)
		} else {
			log.Errorf("Failed to log to file %s as the following error was received: '%s'. Reverting to using " +
				"stdout for log output.", args.LogFile, err)
		}
	}

	if args.Debug {
		log.SetLevel(log.DebugLevel)
		logger.Debug("Debug level messages enabled")
	} else {
		if args.Quiet {
			log.SetLevel(log.PanicLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}
	}

}
