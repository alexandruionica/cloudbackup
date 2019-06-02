# Overview 

There is only one binary which can be started as a server or which can be used as a CLI client. In server mode this is the actual backup engine while the CLI's purpose is only to interact with the backup engine and give it instructions.

When running in server mode it requires a configuration file.

Once a configuration file is composed, it is recommended to run:

- `cloudbackup server config validate -c path_to_configuration_file.yaml` in order to validate that the produced configuration has a valid syntax 
- `cloudbackup server config dump -c path_to_configuration_file.yaml` and examine the output in order to check that the configuration file's parsing produced the expected structure. This is advised because the `yaml` format does sometimes lead to incorrectly 
indented blocks which may end up being parsed in a different logical section than expected. The output of this command represents the internal configuration structure after parsing the `.yaml` configuration file and also reading any specified environment 
variables. Further more, the output itself is presented as valid `json` in order to make clear how the indenting of the input `.yaml` file was processed.

## Example
A sample configuration file:
```
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
        type: aws_s3
        # rate limit uploads to the object store. Specified rate in bytes per second or using a unit like KB/MB/GB etc (Example: 231 KB).
        # Leave unset or set to 0 to have unlimited rate
        ratelimit: 100 KB
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: standard
    # Script to run before commencing to backup files. The script must exist or otherwise the backup server will refuse
    # to start. On Unix like operating systems the user executing the script must have execute rights on the script
    # (+x flag). On Windows the script must have .bat or .ps1 extension. Alternatively you can supply the path to an 
    # executable instead of a script. The script (or executable) will be passed only one argument, the job id 
    # (which is an uuid). If the script has an exit code different than 0 then it will be considered to have failed 
    # and the whole backup job will be cancelled and considered failed. Also an error will be logged together with the 
    # combined standard output and standard error of the said script. You should keep in mind that the standard output 
    # and standard error of the scripts are gathered by the backup server so if their output is large, it will increase
    # memory usage. If a pre run script is already started then cancelling a running backup job will still wait for the
    # script to complete its run and will not attempt to stop it or any of its children processes.
    #
    #pre_run_script: /usr/local/bin/take_db_snapshot.sh
    #
    # Similar to the above with the difference that it will be ran after a backup and it will be ran no matter if the 
    # backup completed, failed or was cancelled
    #
    #post_run_script: c:\\remove_volume_shadow_copy.ps1
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
      - name: google_1
        type: gcp_storage
        user: JANEDOE
        pass: 34324fd
        bucket: 'my-google-bucket'
        prefix: 'backup/backups-for-server-51'
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - '00 08 01 * *'
      - '00 08 06 * *'
    # defaults to 0 which means unlimited number of versions
    versions_max_num: 10
    # defaults to 0 which means unlimited age
    versions_max_age: 6w
# The notification section is optional. Notification will be sent for various events like a backup job has failed 
notification:
  # if the "email" block is specified then at least one entry needs to exist
  email:
      # SMTP server address. If using Gmail directly then you may be limited (by Gmail) to 99 emails per day per account
    - server: smtp.gmail.com
      # if unspecified, it defaults to "25"
      port: 587
      # email recipient, only one address is allowed
      to: someone@gmail.com
      # CC section is optional, multiple addresses can be specified
      cc:
        - someone@bar.com
        - soneome.else@bar.com
      # for what events to send notifications (for this email definition block; other blocks can have different settings)
      # If unspecified, it defaults to "failed" and "crashed"
      type:
        - started
        - finished
        - failed
        - cancelled
        - crashed
      # Username to use when authenticating to the SMTP server. If the SMTP server address is "127.0.0.1" or 
      #  "localhost" then the "user"" and "pass"" fields can be skipped as generally speaking local SMTP doesn't 
      #   require authentication 
      user: my.backup.email27@gmail.com
      pass: 'A_HARD_TO_GUESS_PASSWORD'
  # if the "script" block is specified then at least one entry needs to exist
  script:
      # absolute path to script. On Unix like operating systems the user executing the script must have execute rights 
      #  on the script (+x flag). On Windows the script must have .bat or .ps1 extension. Alternatively you can supply 
      #  the path to an executable instead of a script. The script (or executable) will be passed the following six 
      #  parameters: JobType, JobName, JobId, JobState, JobError, reportFile . The last parameter will be a path to a 
      #  plain text file containing a JSON encoded string. For a full description of the JSON structure please check 
      #  the documentation for the HTTP API and look at the "ResultBackupJobStatus" model.
    - path: /usr/local/bin/custom_hook.sh
      # for what events to call the script (for this script definition block; other blocks can have different settings)
      # If unspecified, it defaults to "failed" and "crashed"
      type:
        - started
        - finished
        - failed
        - cancelled
        - crashed

```

# Generic

# User

Definition of users which can access the backup server's API.

# Backup

What paths to backup, when to backup and how to store the backups.

## Target

A target belongs to a "backup" section and it defines one or more object stores where to save the backed up data.

Parameters specific to a given target type (object store type) are listed in the below sub section.

The value of the `name` key is case insensitive while the value of the `value` key is case sensitive

Once a target is configured, it is recommended to start the server and then run: `cloudbackup client backup target test` in order to validate that the configuration of the target is valid and that the credentials supplied (or available when using something like the EC2 metadata service) grant sufficient access. 

### aws_s3
 
- `AWS_ACCESS_KEY_ID` - optional parameter. AWS access key id. If not specified then the AWS library will use the standard resolution method. If specified then also the `AWS_SECRET_ACCESS_KEY` needs to be specified.
- `AWS_SECRET_ACCESS_KEY` - optional parameter. AWS secret key. If not specified then the AWS library will use the standard resolution method. If specified then also the `AWS_ACCESS_KEY_ID` needs to be specified.
- `storage_class` - optional parameter. If specified, it must be one of "STANDARD", "REDUCED_REDUNDANCY", "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING". Values correspond to AWS storage tiers for S3.

# Notification


