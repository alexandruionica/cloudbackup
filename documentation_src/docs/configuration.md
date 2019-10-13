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
  - name: daily
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
      - name: aws
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
      - name: gcp
        type: gcp_storage
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: type
            value: service_account
          - name: project_id
            value: emerald-city-321300
          - name: private_key_id
            value: oa9ohhuo4quiefo8iiw9yah5Ohkeigi5Zeilei1j
          - name: private_key
            # MUST BE SURROUNDED BY DOUBLE QUOTES (NOT SINGLE QUOTES)
            value: "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCo2+YMGAghHx1Y\nxtSU48nJtZbEstByqsXvseBM491U3cZfd17kTMPfHAHhBHtS52z0wQVBVIRlTz7v\nEc5GZMP8Lq8aclVcTYlot/bZ/yES6eiP70jWW4zYDtnZBzby4gnta6DkwgCCMWWJ\nd8xz8kNynz+2lnxAJoeDhJgQlIQdpP/lbmJeUU8naMbbPQClA3H5LsJHb5ct0GcY\n5NFOSQWwtTZLah8uN4dF3fR5Ae3zpETzWN1ykOIHDmHTxXfZQSPmNHom6iTUd6jA\n8z5ldXbhmEUlmQPRhDvSGNRGMxo9iBUuqLYfPqsglOVVFBEq+v2IIN8y2U5xT0K5\np8blgKuPAgMBAAECggEAAsCYKVVqDkkKb9w6QJk/rlDjoVbreRodRLY/9s8ZygOr\nZTNnyo27/V1u1faMhAzBL3JVFu319gxVSn7zlKt8sBYMffIVzho9cD5qDveV1/qi\nSHSKryri2Qq07W1XkQ5ds4DFYXxHJWZWf1mQwuW4OmOdq8xxeLBvMesmsgrvApLi\nb62HMn5rxPyeLoXc7QSCiF7wOp/0S4rT1ScnCDyGWqKJ0TFtzgZG/Rm4Mp4G7cXw\nWmaWZ87bq4MI0BFkAIt9t9Ph5BYsdgC6SIGzn0MIsLQ5EQyElkMbk9XHPcPIioG8\nwZ2JS1QezpEXi7GLFrEI749uOqYI/QoBvswJn6dDoQKBgQDpsbTchzCSliAtekoX\nMvyrZtB9y47ah4A1+3Mf1JUpzANvPkOfrh/cp7WJIeeB8N0OT21gZZU9f17L5yWN\n5mSLJMf7Db9NL9H5A37PJsTJpPzUtfkffV+mtwQtYfwgguXYIogeOIFYWjcfSQSE\nAy+idvL/WcUryk2KWly2UTlQyQKBgQC4+fNqP6dbaU1g3gklJaIfq6G0iDB0BeeI\n1PCp5lgC3WXHpfY46GaPxCwn1n16z3+AQ8fu+e8lbTVyI3uGANRitF2LujJu5nPg\ni8VaMOZDbWIw6rEFX489HU/hbCTZOg+kdxng0GfsCi9caaLh1DTkKhF8gdX3auip\nbYLh5RFdlwKBgQCBT7vsa0INWtTjVU+6FpSJo5KqiQC7G09uj3zcmB0Ry7n6zFFP\nAmLPDl39S6120XkAeiLjvFIgfWJPIdA9/MaV1/xwhuLcKyHc0HpS1fj+OzVL3oXD\nTvSmo47ELfv9YXEdb74yOsIXyZPG0/iTs8+f7oH3mgzodkEB1Y6Hs9orQQKBgAfK\n9/dM8TcHq6veDtKS0E63Q1vAtRHeQc/g8LanrqOIQkZz9niVSeTapeWTwruOzFdS\nA7VMsEeKX0sMtaKCnHAAG0TMtl03tkAKg2j2UG0cyZs39/c6/GTdvETJ8o94Q7px\nDhULkqU+FJq3FJahAw1tvEjbi3Ed/ulMZMwxg1bHAoGAKK2VwKIFYN64oTnxUCd1\nti8+/CN+U73sEETxcXs2xN2eu1cK5WoxbLjBwstUirr7Z88TZZ3zaprVNqJATuhd\nDXCTNc9ciV7bX4zra48MaPKjB6a2kVa0vik2+I4cKnqLScSbr+bGpNLMRqK/jr+Q\nqsAPucgXdv3IKfgXNQ1pF1E=\n-----END PRIVATE KEY-----\n"
          - name: client_email
            value: backup-client@emerald-city-321300.iam.gserviceaccount.com
          - name: client_id
            value: 121343554521236787787
          - name: auth_uri
            value: 'https://accounts.google.com/o/oauth2/auth'
          - name: token_uri
            value: 'https://oauth2.googleapis.com/token'
          - name: auth_provider_x509_cert_url
            value: 'https://www.googleapis.com/oauth2/v1/certs'
          - name: client_x509_cert_url
            value: 'https://www.googleapis.com/robot/v1/metadata/x509/backup-client%40emerald-city-321300.iam.gserviceaccount.com'
          - name: storage_class
            value: regional
          - name: disable_crc32c_hash
            value: no
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

The value of the `name` key is case insensitive while the value of the `value` key is case sensitive (unless otherwise specified).

Once a target is configured, it is recommended to start the server and then run: `cloudbackup client backup target test` in order to validate that the configuration of the target is valid and that the credentials supplied (or available when using something like the EC2 metadata service) grant sufficient access. 

### aws_s3
 
- `AWS_ACCESS_KEY_ID` - optional parameter. AWS access key id. If not specified then the AWS library will use the [standard resolution method](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials). If specified then also the `AWS_SECRET_ACCESS_KEY` needs to be specified.
- `AWS_SECRET_ACCESS_KEY` - optional parameter. AWS secret key. If not specified then the AWS library will use the [standard resolution method](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials). If specified then also the `AWS_ACCESS_KEY_ID` needs to be specified.
- `storage_class` - optional parameter. If specified, it must be one of "STANDARD", "REDUCED_REDUNDANCY", "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING". Values correspond to AWS storage tiers for S3.
- `region` - optional parameter. Must be a valid AWS region, lower cased. For example: "us-east-1" or "ap-southeast-2". If specified, it will be used only in case the region of the S3 bucket can not be programatically determined using the S3 API.

It is required to:
- enable [S3 bucket versioning](https://docs.aws.amazon.com/AmazonS3/latest/dev/Versioning.html#how-to-enable-disable-versioning-intro)

It is highly advisable to: 

- setup strict access controls on the S3 buckets in order to ensure only backup software has access to read or write contents.
- dedicate S3 buckets only for backup only purposes. Mixing use of S3 buckets may lead to backups being corrupted if any other software or persons manage the contents of the S3 buckets
- make use of the `prefix` backup configuration setting so a given system is the only one making use of any S3 bucket key beginning with said prefix. Failing to do so may lead to corrupted backups. 
- put a lifecycle rule on the S3 bucket so multipart uploads parts older than several days are automatically purged

Example configuration:
```yaml
data_dir: /var/lib/cloudbackup
user:
  - name: testuser1
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    access: write
backup:
  - name: daily
    paths:
      - /something
      - /var/lib
    target:
      - name: aws
        type: aws_s3
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: standard
``` 

### gcp_storage

Entries below marked as "credential" parameter are to be extracted from the [service account key](https://cloud.google.com/iam/docs/creating-managing-service-account-keys) json file provided by Google Compute. These parameters are optional as long no other credential parameter is mentioned. 
If one of them is mentioned then all of them are required. If the GCP credential parameters are not used then "Application Default Credentials" method will be used to [find automatically the credentials](https://cloud.google.com/docs/authentication/production#finding_credentials_automatically).

- `type` - optional credential parameter extracted from the service account key json file.
- `project_id` - optional credential parameter extracted from the service account key json file.
- `private_key_id` - optional credential parameter extracted from the service account key json file.
- `private_key` **must be surrounded by double quotes** (not single quotes) - optional credential parameter extracted from the service account key json file.
- `client_email` - optional credential parameter extracted from the service account key json file.
- `client_id` - optional credential parameter extracted from the service account key json file.
- `auth_uri` - optional credential parameter extracted from the service account key json file.
- `token_uri` - optional credential parameter extracted from the service account key json file.
- `auth_provider_x509_cert_url` - optional credential parameter extracted from the service account key json file.
- `client_x509_cert_url` - optional credential parameter extracted from the service account key json file.
- `storage_class` - optional parameter. If specified, it must be one of "multi_regional", "regional", "nearline" or "coldline". Values correspond to GCP storage tiers.
- `disable_crc32c_hash` - optional parameter. If not specified then it defaults to "false" and then a CRC32c hash will be calculated for each uploaded file and then sent to GCP storage together with the file. The hash will then be used by GCP storage to validate that the file did not get corrupted during the upload. The only downside to this is that in order to compute the hash, the file will be read one extra time from the local disk. Setting this parameter to a value of "yes" or "true" means the CRC32c hash will not be calculated and sent.

It is required to:
- enable [GCP bucket versioning](https://cloud.google.com/storage/docs/using-object-versioning)

It is highly advisable to:

- setup strict access controls on the GCP storage buckets in order to ensure only backup software has access to read or write contents.
- dedicate GCP storage buckets only for backup only purposes. Mixing use of GCP storage buckets may lead to backups being corrupted if any other software or persons manage the contents of the buckets.
- make use of the `prefix` backup configuration setting so a given system is the only one making use of any GCP storage bucket key beginning with said prefix. Failing to do so may lead to corrupted backups.
- not disable the computation of CRC32c hashes. 

Example configuration:
```yaml
data_dir: /var/lib/cloudbackup
user:
  - name: testuser1
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    access: write
backup:
  - name: daily
    paths:
      - /something
      - /var/lib
    target:
      - name: gcp
        type: gcp_storage
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: type
            value: service_account
          - name: project_id
            value: emerald-city-321300
          - name: private_key_id
            value: oa9ohhuo4quiefo8iiw9yah5Ohkeigi5Zeilei1j
          - name: private_key
            # MUST BE SURROUNDED BY DOUBLE QUOTES (NOT SINGLE QUOTES)
            value: "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCo2+YMGAghHx1Y\nxtSU48nJtZbEstByqsXvseBM491U3cZfd17kTMPfHAHhBHtS52z0wQVBVIRlTz7v\nEc5GZMP8Lq8aclVcTYlot/bZ/yES6eiP70jWW4zYDtnZBzby4gnta6DkwgCCMWWJ\nd8xz8kNynz+2lnxAJoeDhJgQlIQdpP/lbmJeUU8naMbbPQClA3H5LsJHb5ct0GcY\n5NFOSQWwtTZLah8uN4dF3fR5Ae3zpETzWN1ykOIHDmHTxXfZQSPmNHom6iTUd6jA\n8z5ldXbhmEUlmQPRhDvSGNRGMxo9iBUuqLYfPqsglOVVFBEq+v2IIN8y2U5xT0K5\np8blgKuPAgMBAAECggEAAsCYKVVqDkkKb9w6QJk/rlDjoVbreRodRLY/9s8ZygOr\nZTNnyo27/V1u1faMhAzBL3JVFu319gxVSn7zlKt8sBYMffIVzho9cD5qDveV1/qi\nSHSKryri2Qq07W1XkQ5ds4DFYXxHJWZWf1mQwuW4OmOdq8xxeLBvMesmsgrvApLi\nb62HMn5rxPyeLoXc7QSCiF7wOp/0S4rT1ScnCDyGWqKJ0TFtzgZG/Rm4Mp4G7cXw\nWmaWZ87bq4MI0BFkAIt9t9Ph5BYsdgC6SIGzn0MIsLQ5EQyElkMbk9XHPcPIioG8\nwZ2JS1QezpEXi7GLFrEI749uOqYI/QoBvswJn6dDoQKBgQDpsbTchzCSliAtekoX\nMvyrZtB9y47ah4A1+3Mf1JUpzANvPkOfrh/cp7WJIeeB8N0OT21gZZU9f17L5yWN\n5mSLJMf7Db9NL9H5A37PJsTJpPzUtfkffV+mtwQtYfwgguXYIogeOIFYWjcfSQSE\nAy+idvL/WcUryk2KWly2UTlQyQKBgQC4+fNqP6dbaU1g3gklJaIfq6G0iDB0BeeI\n1PCp5lgC3WXHpfY46GaPxCwn1n16z3+AQ8fu+e8lbTVyI3uGANRitF2LujJu5nPg\ni8VaMOZDbWIw6rEFX489HU/hbCTZOg+kdxng0GfsCi9caaLh1DTkKhF8gdX3auip\nbYLh5RFdlwKBgQCBT7vsa0INWtTjVU+6FpSJo5KqiQC7G09uj3zcmB0Ry7n6zFFP\nAmLPDl39S6120XkAeiLjvFIgfWJPIdA9/MaV1/xwhuLcKyHc0HpS1fj+OzVL3oXD\nTvSmo47ELfv9YXEdb74yOsIXyZPG0/iTs8+f7oH3mgzodkEB1Y6Hs9orQQKBgAfK\n9/dM8TcHq6veDtKS0E63Q1vAtRHeQc/g8LanrqOIQkZz9niVSeTapeWTwruOzFdS\nA7VMsEeKX0sMtaKCnHAAG0TMtl03tkAKg2j2UG0cyZs39/c6/GTdvETJ8o94Q7px\nDhULkqU+FJq3FJahAw1tvEjbi3Ed/ulMZMwxg1bHAoGAKK2VwKIFYN64oTnxUCd1\nti8+/CN+U73sEETxcXs2xN2eu1cK5WoxbLjBwstUirr7Z88TZZ3zaprVNqJATuhd\nDXCTNc9ciV7bX4zra48MaPKjB6a2kVa0vik2+I4cKnqLScSbr+bGpNLMRqK/jr+Q\nqsAPucgXdv3IKfgXNQ1pF1E=\n-----END PRIVATE KEY-----\n"
          - name: client_email
            value: backup-client@emerald-city-321300.iam.gserviceaccount.com
          - name: client_id
            value: 121343554521236787787
          - name: auth_uri
            value: 'https://accounts.google.com/o/oauth2/auth'
          - name: token_uri
            value: 'https://oauth2.googleapis.com/token'
          - name: auth_provider_x509_cert_url
            value: 'https://www.googleapis.com/oauth2/v1/certs'
          - name: client_x509_cert_url
            value: 'https://www.googleapis.com/robot/v1/metadata/x509/backup-client%40emerald-city-321300.iam.gserviceaccount.com'
          - name: storage_class
            value: regional
          - name: disable_crc32c_hash
            value: no
``` 

### azure_blob
 
- `storage_account` - required parameter. Azure Blob Storage account to use (must be of type Blob Storage).
- `storage_access_key` - required parameter. Azure Blob Storage account key.
- `primary_blob_service_endpoint` - optional parameter. If specified, it must be URL show in the Microsoft Azure portal
 when navigating to `Home > Storage accounts > ${STORAGE_ACCOUNT_NAME} > Properties` and looking at the 
 `Primary Blob Service Endpoint` property (`${STORAGE_ACCOUNT_NAME}` represents the name of your storage account as you've 
 mentioned it in the `storage_account` setting above. Only HTTPS urls will be accepted for this parameter. If not specified 
 then `https://${STORAGE_ACCOUNT_NAME}.blob.core.windows.net/` will be used and `${STORAGE_ACCOUNT_NAME}` will be automatically replaced with the value of `storage_account` as you 
 have specified it.


It is highly advisable to: 

- setup strict access controls on the Azure Blob Storage containers (buckets) in order to ensure only backup software has access to read or write contents.
- dedicate Azure Blob Storage containers (buckets) only for backup only purposes. Mixing use of containers (buckets) may lead to backups being corrupted if any other software or persons manage the contents of the Azure Blob Storage containers (buckets)
- make use of the `prefix` backup configuration setting so a given system is the only one making use of any Azure Blob Storage containers (buckets) key beginning with said prefix. Failing to do so may lead to corrupted backups. 

Example configuration:
```yaml
data_dir: /var/lib/cloudbackup
user:
  - name: testuser1
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    access: write
backup:
  - name: daily
    paths:
      - /something
      - /var/lib
    target:
      - name: azure
        type: azure_blob
        bucket: 'example-com-us-servers'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: storage_account
            value: backups
          - name: storage_access_key
            value: 'YmHQE9dil9wI48sv41HhAek/jr2VWSfY4QiCMLs6qi0XVYWEb9MxInOSH1039IdvxiqJZDhzniZCotPIRXQzwA=='
``` 

# Notification


