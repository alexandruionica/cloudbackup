package config

import (
	"cloudbackup/database"
	"cloudbackup/shared"
	"cloudbackup/utils"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/jinzhu/configor"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/utf8string"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const loggingContext = "config"
const SecretReplace = "****************"

// used for looking up environment variables holding configuration data
const EnvPrefix = "CLOUDBACKUP"

var NotificationTypes = []string{"started", "finished", "failed", "cancelled", "crashed"}

// allowed backup target types (this is used in a validation function below). If this is updated then please also
// update the Swagger file
var BackupTargetTypes = [...]string{"aws_s3", "gcp_storage", "azure_blob"}

// allowed backup target types used for testing purposes only
var HiddenBackupTargetTypes = [...]string{"test_null"}

// list of keys (of the $Parameters slice) which contain secrets in the .Value entry
var ParemtersWithSecrets = [...]string{"aws_secret_access_key", "private_key", "password", "pass"}

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// load configuration from yaml file at "path" and if boolean "debug" is set then also enable debugging in the yaml
// config parser library
func Load(path string, debug bool, mutex *sync.RWMutex) (*shared.RuntimeConfig, error) {
	logger.Info(fmt.Sprintf("Loading server config file %s", path))

	var Config = shared.CfgTemplate{}
	var err error

	if _, err := utils.FileExists(path, true); err != nil {
		logger.Error(err)
		return &shared.RuntimeConfig{}, err
	}

	//logger.Debug("Acquiring read lock before reading config file")
	mutex.RLock()
	defer func() {
		mutex.RUnlock()
		//logger.Debug("Read lock released after reading config file")
	}()
	//logger.Debug("Acquired read lock for reading config file")
	// if debug then also adjust logging level of configor library (set library to Verbose not Debug as
	// Verbose is actually what we expect when using  "debug")
	if debug {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix, Verbose: true}).Load(&Config, path)
	} else {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix}).Load(&Config, path)
	}

	if err != nil {
		msg := fmt.Sprintf("When parsing the server configuration file %s the following error was encountered:"+
			" %s", path, err)
		logger.Error(msg)
		return &shared.RuntimeConfig{}, errors.New(msg)
	}

	err = Validate(Config, false)
	if err != nil {
		return &shared.RuntimeConfig{}, err
	}

	// rarely used lock (this is to be used only by functions which get the Config struct but don't get also the
	// parent struct (which has a different lock)
	Config.Mutex = &sync.RWMutex{}

	return &shared.RuntimeConfig{Mutex: mutex,
		Path:   path,
		Config: Config,
	}, nil
}

// saves new configuration to file
func Save(runtimeCfg *shared.RuntimeConfig, newConfig shared.CfgTemplate) error {
	logger.Debug("Acquiring lock before writing config file")
	runtimeCfg.Mutex.Lock()
	defer func() {
		runtimeCfg.Mutex.Unlock()
		logger.Debug("Lock released after attempting to write config file")
	}()
	toWrite, err := yaml.Marshal(newConfig)
	if err != nil {
		return fmt.Errorf("could not marshall YAML when preparing for write the configuration "+
			"file. The error received was: %s", err.Error())
	}
	if err := os.WriteFile(runtimeCfg.Path, toWrite, 0600); err != nil {
		return fmt.Errorf("could not write to configuration file '%s' . Received error was: %s",
			runtimeCfg.Path, err)
	}
	logger.Debug("Updating in-memory configuration")
	runtimeCfg.Config = newConfig
	return nil
}

// validate several config options which depends on other options having certain values. Trying to do this with
// reflection ends up being harder to understand and still requires application logic in the validator
// params: config struct to validate; hiddenPass is if to allow obfuscated passwords (meaning strings with value *****)
func Validate(config shared.CfgTemplate, hiddenPass bool) error {
	// check if "data_dir" exists
	if err := ValidateDir(config.DataDir, "data_dir", true); err != nil {
		return err
	}
	// check if "html_dir" exists
	if err := ValidateDir(config.HtmlDir, "html_dir", true); err != nil {
		return err
	}
	// validate "restore_dir". If unset, this is a soft default resolved at restore time
	// to "<DataDir>/restores" and we skip the existence check here because the directory
	// may legitimately not exist yet until the first restore runs. If the user has set a
	// custom value it must exist and be usable.
	if config.RestoreDir != "" {
		if err := ValidateDir(config.RestoreDir, "restore_dir", true); err != nil {
			return err
		}
	}
	// validate HTTPS section of the config
	if err := ValidateHttps(config, true); err != nil {
		return err
	}
	// validate "Backup" section of the config
	if err := ValidateBackup(config.Backup, true); err != nil {
		return err
	}
	// validate "User" section of the config
	if err := ValidateUser(config, true, hiddenPass); err != nil {
		return err
	}
	// validate "Notification" section of the config
	if err := ValidateNotification(config.Notifications, true); err != nil {
		return err
	}

	// if we got here than all was fine
	return nil
}

// validate "Backup" section of the config
func ValidateBackup(backups []shared.ConfigBackup, logError bool) error {
	names := make([]string, 0)
	i := 0
	for _, backup := range backups {
		// have this as the first check as subsequent ones use the Backup name in error output in order to indicate
		// where did things go wrong
		if utils.StringInSlice(backup.Name, names) {
			msg := fmt.Sprintf("more than one Backups have the same 'name=%s' . Backup 'name' values must"+
				" be unique", backup.Name)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		} else {
			names = append(names, backup.Name)
		}
		// check backup "Name" is ASCII only. We use the backup name as part of the file name holding the SQL database
		// and there is potential that on some OSes the filesystem doesn't support ASCII
		nameTested := utf8string.NewString(backup.Name)
		if !nameTested.IsASCII() {
			msg := fmt.Sprintf("Backup having name '%s' contains non ASCII characters but only ASCII characters "+
				"are allowed for backup names", backup.Name)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}

		// The backup name is embedded in per-target restore DB filenames using "__" as a
		// separator (see database.RestoreDbSeparator), and it is also used directly as a
		// file name for the backup DB. Reject names that would make the filename ambiguous
		// or would escape the data directory.
		if err := validateBackupOrTargetName("Backup", backup.Name, logError); err != nil {
			return err
		}

		if backup.Encrypt && backup.EncryptPass == "" {
			msg := fmt.Sprintf("backup[%d] having 'name=%s' has setting 'encrypt=true' but 'encrypt_pass' is not"+
				" set. Set a password or disable encryption", i, backup.Name)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}

		// ensure only absolute Paths are specified to be backed up. Various logic in the database records (relared to
		// figuring out what is the parent directory) expects that paths are always absolute
		for _, path := range backup.Paths {
			if !filepath.IsAbs(path) {
				msg := fmt.Sprintf("backup[%d] having 'name=%s' has specified that path '%s' is to be backed "+
					"but this particular path is not absolute. Only absolute paths are permitted in the 'paths' section",
					i, backup.Name, path)
				if logError {
					logger.Error(msg)
				}
				return errors.New(msg)
			}
		}

		if backup.PreRunScript != "" {
			err := ValidatePrePostRunScript(backup.PreRunScript, "pre", backup.Name, logError)
			if err != nil {
				return err
			}
		}

		if backup.PostRunScript != "" {
			err := ValidatePrePostRunScript(backup.PostRunScript, "post", backup.Name, logError)
			if err != nil {
				return err
			}
		}

		err := ValidateBackupTarget(backup.Target, logError, backup.Name)
		if err != nil {
			return err
		}

		if err := ValidateBackupSchedule(backup.Schedule, backup.Name, logError); err != nil {
			return err
		}

		i += 1
	}
	return nil
}

// ValidateBackupSchedule verifies every entry in a backup's "schedule" list is a valid 5-field
// cron expression (or one of the supported descriptors like "@daily"). The parser used here is
// the same one the scheduler uses at runtime, so accepting it during config load guarantees the
// scheduler will be able to register the entry.
func ValidateBackupSchedule(schedules []string, backupName string, logError bool) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	for _, expr := range schedules {
		if strings.TrimSpace(expr) == "" {
			msg := fmt.Sprintf("backup '%s' has an empty schedule entry. Either remove the entry or set a "+
				"valid cron expression", backupName)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if _, err := parser.Parse(expr); err != nil {
			msg := fmt.Sprintf("backup '%s' has invalid schedule entry '%s': %s", backupName, expr, err)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
	}
	return nil
}

// checks if a mentioned script (or binary) file exists and on Unixes also that it's executable
// $scriptType is expected to be one of "pre" or "post" (for PreRun and PostRun script)
func ValidatePrePostRunScript(path string, scriptType string, backupName string, logError bool) error {
	if _, err := utils.FileExists(path, true); err != nil {
		msg := fmt.Sprintf("When validating the existence of %s run script '%s' belonging to backup job '%s' "+
			"the following error ocurred: %s", scriptType, path, backupName, err)
		if logError {
			logger.Error(msg)
		}
		return err
	}

	err := isExecutable(path)
	if err != nil {
		msg := fmt.Sprintf("Encountered the following error when checking if %s run script '%s', belonging to "+
			"backup job '%s' is executable: %s", scriptType, path, backupName, err)
		if logError {
			logger.Error(msg)
		}
		return errors.New(msg)
	}

	return nil
}

// validate HTTPS section of the config
func ValidateHttps(config shared.CfgTemplate, logError bool) error {
	if config.Https.Enabled {
		if config.Https.SslCertPath == "" {
			msg := "https: enabled=true  but https: ssl_cert_path  is not set"
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if config.Https.SslKeyPath == "" {
			msg := "https: enabled=true  but https: ssl_key_path  is not set"
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if _, err := utils.FileExists(config.Https.SslCertPath, true); err != nil {
			msg := fmt.Sprintf("https: enabled=true  and https: ssl_cert_path=%s but when evaluating "+
				"the latter the following error ocurred: %s", config.Https.SslCertPath, err)
			if logError {
				logger.Error(msg)
			}
			return err
		}
		if _, err := utils.FileExists(config.Https.SslKeyPath, true); err != nil {
			msg := fmt.Sprintf("https: enabled=true  and https: ssl_key_path=%s but when evaluating "+
				"the latter the following error ocurred: %s", config.Https.SslKeyPath, err)
			if logError {
				logger.Error(msg)
			}
			return err
		}
	}
	return nil
}

// validate "Backup/Target" section of the config
func ValidateBackupTarget(targets []shared.ConfigBackupTarget, logError bool, BackupName string) error {
	names := make([]string, 0)
	for _, target := range targets {
		// have this as the first check as subsequent ones use the Target name in error output in order to indicate
		// where did things go wrong

		// check uniqueness of backup Target name
		if utils.StringInSlice(target.Name, names) {
			msg := fmt.Sprintf("more than one 'target' of the same 'backup' (belonging to backup section having"+
				" 'name=%s') have the same 'name=%s' . Target 'name' values must be unique within a 'backup'"+
				" section", BackupName, target.Name)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		} else {
			names = append(names, target.Name)
		}

		// Target names are embedded in per-target restore DB filenames using "__" as a
		// separator. Reject names containing "__" or path separators so the filename
		// split is unambiguous and cannot escape the data directory.
		if err := validateBackupOrTargetName("Target", target.Name, logError); err != nil {
			return err
		}

		// check ratelimit is valid
		ratelimit, err := humanize.ParseBytes(target.RateLimit)
		if err != nil {
			msg := fmt.Sprintf("Target '%s' for backup '%s' has ratelimit defined as %s could not be "+
				"translated to a number. While attempting translation the following error was encountered: %s",
				target.Name, BackupName, target.RateLimit, err)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}

		// check ratelimit is not < 0
		if ratelimit < 0 { //nolint:staticcheck
			msg := fmt.Sprintf("Target '%s' for backup '%s' has ratelimit defined as %d which is a negative "+
				"number. Only 0 and positive numbers are allowed", target.Name, BackupName, ratelimit)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}

		matched := false
		// check target type is allowed
		for _, AllowedTargetType := range BackupTargetTypes {
			if strings.EqualFold(target.Type, AllowedTargetType) {
				matched = true
			}
		}
		for _, AllowedTargetType := range HiddenBackupTargetTypes {
			if strings.EqualFold(target.Type, AllowedTargetType) {
				matched = true
			}
		}
		if !matched {
			msg := fmt.Sprintf("Target '%s' is of type '%s' but this is not an allowed type. Allowed types "+
				"are one of: %s", target.Name, target.Type, strings.Trim(fmt.Sprintf("%s", BackupTargetTypes), "[]"))
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		// validate the per target type (object store type) specific parameters
		err = ValidateBackupTargetParameters(target.Parameters, BackupName, target.Name, target.Type)
		if err != nil {
			if logError {
				logger.Error(err)
				return err
			}
		}
	}
	return nil
}

// validate "Backup/Target/Parameters" section of the config. Different backends will have different rules and expectations
func ValidateBackupTargetParameters(parameters []shared.ConfigBackupTargetParams, BackupName string, TargetName string, TargetType string) error {
	if len(parameters) == 0 {
		return nil
	}
	// ensure we don't have duplicate parameters
	for _, i := range parameters {
		foundMatches := 0
		for _, j := range parameters {
			if strings.EqualFold(i.Name, j.Name) {
				foundMatches += 1
			}
			if foundMatches > 1 {
				return fmt.Errorf("backup target parameter '%s' belonging to backup '%s' and target '%s' of "+
					"type '%s' is present more than once", i.Name, BackupName, TargetName, TargetType)
			}
		}
	}

	switch TargetType {
	case "test_null":
		return ValidateBackupTargetParametersForTestNull(parameters, BackupName, TargetName, TargetType)
	case "aws_s3":
		return ValidateBackupTargetParametersForS3(parameters, BackupName, TargetName, TargetType)
	case "gcp_storage":
		return ValidateBackupTargetParametersForGCPStorage(parameters, BackupName, TargetName, TargetType)
	case "azure_blob":
		return ValidateBackupTargetParametersForAzureBlob(parameters, BackupName, TargetName, TargetType)
	default:
		return fmt.Errorf("can not validate parameters for unknown target type of %s for backup %s", TargetType, BackupName)
	}
}

// validate "Backup/Target/Parameters" section of the config for the test_null object store type
func ValidateBackupTargetParametersForTestNull(parameters []shared.ConfigBackupTargetParams, BackupName string, TargetName string, TargetType string) error {
	//foundBucket := false
	//for _, entry := range parameters {
	//	switch strings.ToLower(entry.Name) {
	//	case "bucket":
	//		if entry.Value != "" {
	//			foundBucket = true
	//		}
	//	}
	//}
	//if !foundBucket {
	//	return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' is missing required parameter 'bucket'", TargetName, TargetType, BackupName)
	//}
	return nil
}

// validate "Backup/Target/Parameters" section of the config for the aws_s3 object store type
func ValidateBackupTargetParametersForS3(parameters []shared.ConfigBackupTargetParams, BackupName string, TargetName string, TargetType string) error {
	allowedParameters := [...]string{"aws_access_key_id", "aws_secret_access_key", "storage_class", "region"}
	err := validateTargetParametersAreKnown(parameters, allowedParameters[:], BackupName, TargetName, TargetType)
	if err != nil {
		return err
	}
	foundKeyId, foundPrivateKey := false, false
	for _, entry := range parameters {
		switch strings.ToLower(entry.Name) {
		case "aws_access_key_id":
			if entry.Value != "" {
				foundKeyId = true
			}
		case "aws_secret_access_key":
			if entry.Value != "" {
				foundPrivateKey = true
			}
		case "storage_class":
			allowedClass := [...]string{"STANDARD", "REDUCED_REDUNDANCY", "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING"}
			found := false
			for _, val := range allowedClass {
				if entry.Value == val {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter "+
					"'%s' with value '%s'. This is not an accepted value; accepted values for this parameter are case "+
					"sensitive and one of: %+v", TargetName, TargetType, BackupName, entry.Name, entry.Value, allowedClass)
			}
		case "region":
			if entry.Value == "" {
				return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' is has a parameter "+
					"with name 'region' set with an empty value. Either specify a value for it or remove the parameter",
					TargetName, TargetType, BackupName)
			}
		}
	}
	if (foundKeyId && !foundPrivateKey) || (!foundKeyId && foundPrivateKey) {
		if foundKeyId {
			return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter "+
				"'AWS_ACCESS_KEY_ID' but is missing parameter 'AWS_SECRET_ACCESS_KEY'. You must specify a value for both"+
				" of them or otherwise not specify both of them", TargetName, TargetType, BackupName)
		}
		if foundPrivateKey {
			return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter "+
				"'AWS_SECRET_ACCESS_KEY' but is missing parameter 'AWS_ACCESS_KEY_ID'. You must specify a value for both"+
				" of them or otherwise not specify both of them", TargetName, TargetType, BackupName)
		}
	}
	return nil
}

// validate "Backup/Target/Parameters" section of the config for the gcp_storage object store type
func ValidateBackupTargetParametersForGCPStorage(parameters []shared.ConfigBackupTargetParams, BackupName string, TargetName string, TargetType string) error {
	allowedParameters := [...]string{"type", "project_id", "private_key_id", "private_key", "client_email", "client_id",
		"auth_uri", "token_uri", "auth_provider_x509_cert_url", "client_x509_cert_url", "storage_class", "disable_crc32c_hash"}
	// list of parameters which are used to compose the service account credentials file. If any of those is specified
	// then all of them must be specified
	credentialParameters := [...]string{"type", "project_id", "private_key_id", "private_key", "client_email", "client_id",
		"auth_uri", "token_uri", "auth_provider_x509_cert_url", "client_x509_cert_url"}
	err := validateTargetParametersAreKnown(parameters, allowedParameters[:], BackupName, TargetName, TargetType)
	if err != nil {
		return err
	}

	foundCredentials := false
	foundCredential := ""
Loop:
	for _, entry := range parameters {
		for _, param := range credentialParameters {
			if strings.ToLower(entry.Name) == param {
				foundCredentials = true
				foundCredential = entry.Name
				break Loop
			}
		}
	}
	// check that all required credential parameters are present in the user supplied parameters
	if foundCredentials {
		for _, param := range credentialParameters {
			found := false
			for _, entry := range parameters {
				if strings.ToLower(entry.Name) == param {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter '%s' "+
					"which implies all parameters for credentials need to be specified but at least parameter '%s' is "+
					"missing. Parameters for credentials are: '%s'", TargetName, TargetType, BackupName,
					foundCredential, param, credentialParameters)
			}
		}
	}
	// check individual parameters for validity
	for _, entry := range parameters {
		switch strings.ToLower(entry.Name) {
		case "storage_class":
			allowedClass := [...]string{"multi_regional", "regional", "nearline", "coldline"}
			found := false
			for _, val := range allowedClass {
				if entry.Value == val {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter "+
					"'%s' with value '%s'. This is not an accepted value; accepted values for this parameter are case "+
					"sensitive and one of: %+v", TargetName, TargetType, BackupName, entry.Name, entry.Value, allowedClass)
			}
		case "disable_crc32c_hash":
			{
				_, err := StringParameterToBoolean(entry.Value)
				if err != nil {
					return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter "+
						"'%s' with value '%s'. This is not an accepted value; accepted values for this parameter are "+
						"one of: %+v", TargetName, TargetType, BackupName, entry.Name, entry.Value, `"yes"(any case), "no"(any case), "1", "t", "T", "true", "TRUE", "True", "0", "f", "F", "false", "FALSE", "False"`)
				}
			}
		}
	}

	return nil
}

// validate "Backup/Target/Parameters" section of the config for the azure_blob object store type
func ValidateBackupTargetParametersForAzureBlob(parameters []shared.ConfigBackupTargetParams, BackupName string, TargetName string, TargetType string) error {
	allowedParameters := [...]string{"storage_account", "storage_access_key", "primary_blob_service_endpoint"}
	// list of required parameters
	// TODO - add support for Azure metadata service based tokens and stop requiring the "storage_access_key" parameter
	requiredParameters := [...]string{"storage_account", "storage_access_key"}

	err := validateTargetParametersAreKnown(parameters, allowedParameters[:], BackupName, TargetName, TargetType)
	if err != nil {
		return err
	}

	err = validateTargetRequiredParameters(parameters, requiredParameters[:], BackupName, TargetName, TargetType)
	if err != nil {
		return err
	}

	// check individual parameters for validity
	for _, entry := range parameters {
		switch strings.ToLower(entry.Name) {
		case "primary_blob_service_endpoint":
			{
				if !utils.IsValidUrl(entry.Value) {
					return fmt.Errorf("parameter 'primary_blob_service_endpoint' has value '%s' which is not a valid URL", entry.Value)
				}
				if strings.HasPrefix(entry.Value, "http://") {
					return fmt.Errorf("parameter 'primary_blob_service_endpoint' has value '%s' which is an HTTP url. Only HTTPS urls are allowed", entry.Value)
				}
			}
		}
	}

	return nil
}

// checks that supplied parameters for a given target are all known(allowed) parameters for said target
func validateTargetParametersAreKnown(parameters []shared.ConfigBackupTargetParams, allowedParameters []string, BackupName string, TargetName string, TargetType string) error {
	for _, entry := range parameters {
		foundMatch := false
		for _, param := range allowedParameters {
			if strings.ToLower(entry.Name) == param {
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' has specified parameter '%s' "+
				"which is not a known configuration option", TargetName, TargetType, BackupName, entry.Name)
		}
	}
	return nil
}

// checks that required parameters for a target, are specified
func validateTargetRequiredParameters(parameters []shared.ConfigBackupTargetParams, requiredParameters []string, BackupName string, TargetName string, TargetType string) error {
	for _, param := range requiredParameters {
		foundMatch := false
		for _, entry := range parameters {
			if strings.ToLower(entry.Name) == param {
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return fmt.Errorf("target '%s' of type '%s' belonging to backup '%s' requires parameter '%s' "+
				"but this parameter has not been specified", TargetName, TargetType, BackupName, param)
		}
	}
	return nil
}

// check Notification.Email and Notification.Script config sections
func ValidateNotification(notifications shared.ConfigNotification, logError bool) error {
	err := ValidateNotificationCommand(notifications.Script, logError)
	if err != nil {
		return err
	}

	err = ValidateNotificationEmail(notifications.Email, logError)
	if err != nil {
		return err
	}

	// if we got here than all was fine
	return nil
}

func ValidateNotificationCommand(commands []shared.ConfigNotificationScript, logError bool) error {
	for _, notificationCommand := range commands {
		// check supplied file path at least exists (if it's executable is a different story which also may be
		// platform dependent
		if _, err := utils.FileExists(notificationCommand.Path, true); err != nil {
			msg := fmt.Sprintf("When validating the existence of notification script '%s' "+
				"the following error ocurred: %s", notificationCommand.Path, err)
			if logError {
				logger.Error(msg)
			}
			return err
		}

		err := isExecutable(notificationCommand.Path)
		if err != nil {
			msg := fmt.Sprintf("Encountered the following error when checking if notification script '%s' "+
				"is executable: %s", notificationCommand.Path, err)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		// check "Type" entry is valid
		for _, entryType := range notificationCommand.Type {
			foundMatch := false
			for _, allowed := range NotificationTypes {
				if entryType == allowed {
					foundMatch = true
					break
				}
			}
			if foundMatch {
				continue
			} else {
				msg := fmt.Sprintf("Notification script '%s' has mentioned a notification event type of '%s' "+
					"which is not an allowed value. Allowed values are: %+v", notificationCommand.Path, entryType,
					NotificationTypes)
				if logError {
					logger.Error(msg)
				}
				return errors.New("chosen notification type value is not valid")
			}
		}
	}
	return nil
}

func ValidateNotificationEmail(emails []shared.ConfigNotificationEmail, logError bool) error {
	for _, notificationEmail := range emails {
		// check that we're using authentication if not connecting to the localhost SMTP server
		if notificationEmail.User == "" && notificationEmail.Pass == "" && !isLocalhost(notificationEmail.Server) {
			msg := fmt.Sprintf("Notification email entry has 'user' and 'pass' fields empty (or not defined) "+
				"but the specified "+
				"email(SMTP) server is '%s' and not one of 'localhost', '127.0.0.1' and '::1' which means "+
				"authentication is mandatory. Please fill in the User and Pass fields", notificationEmail.Server)
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		// if either the Pass or User field are filled in then require the other one too
		if notificationEmail.User != "" && notificationEmail.Pass == "" {
			msg := fmt.Sprintf("Notification email entry has 'user' field filled in but the 'pass' field isn't. " +
				"Please fill in also the 'pass' field")
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if notificationEmail.User == "" && notificationEmail.Pass != "" {
			msg := fmt.Sprintf("Notification email entry has 'pass' field filled in but the 'user' field isn't. " +
				"Please fill in also the 'user' field")
			if logError {
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		// check "Type" entry is valid
		for _, entryType := range notificationEmail.Type {
			foundMatch := false
			for _, allowed := range NotificationTypes {
				if entryType == allowed {
					foundMatch = true
					break
				}
			}
			if foundMatch {
				continue
			} else {
				msg := fmt.Sprintf("Notification email entry has mentioned a notification event type of '%s' "+
					"which is not an allowed value. Allowed values are: %+v", entryType, NotificationTypes)
				if logError {
					logger.Error(msg)
				}
				return errors.New(msg)
			}
		}
	}
	return nil

}

// Validate directory path
func ValidateDir(dir string, paramName string, logError bool) error {
	_, err := utils.DirExists(dir, true)
	if err != nil {
		msg := ""
		switch err {
		case utils.ErrNoSuchDir:
			{
				msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter does not exist or can not be accessed. "+
					"In case '%s' is a default value for '%s' then you will not notice it in the configuration file.",
					dir, paramName, dir, paramName)
			}
		case utils.ErrUnusableDirPath:
			{
				msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter can not be used. In case '%s' is a "+
					"default value for '%s' then you will not notice it in the configuration file.",
					dir, paramName, dir, paramName)
			}
		case utils.ErrNoSuchRelativeDir:
			{
				absPath, _ := filepath.Abs(dir) // #nosec
				msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter does not exist or can not be accessed. "+
					"The absolute is: '%s'. In case '%s' is a default value for '%s' then you will not notice it in the"+
					" configuration file.", dir, paramName, absPath, dir, paramName)
			}
		case utils.ErrNotADir:
			{
				msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter exists but it is not a directory. In case "+
					"'%s' is a default value for '%s' then you will not notice it in the configuration file.",
					dir, paramName, dir, paramName)
			}
		}

		if logError {
			logger.Error(msg)
		}
		return errors.New(msg)
	} else {
		return nil
	}

}

// validate User section
// params: config struct to validate; logError is if to log errors or not; hiddenPass is if to allow obfuscated
// passwords (meaning strings with value *****)
func ValidateUser(config shared.CfgTemplate, logError bool, hiddenPass bool) error {
	if len(config.User) > 0 {
		names := make([]string, 0)
		for _, user := range config.User {
			if utils.StringInSlice(user.Name, names) {
				msg := fmt.Sprintf("more than one users have the same 'name=%s' . User 'name' values must"+
					" be unique", user.Name)
				if logError {
					logger.Error(msg)
				}
				return errors.New(msg)
			} else {
				names = append(names, user.Name)
			}
			// brcypt hashes should start with $2
			if strings.Index(user.Pass, "$2") != 0 {
				// if hiddenPass then we allow the password to be like *****
				if hiddenPass && CheckStringIsOnly(user.Pass, "*") {
					// do nothing
				} else {
					msg := fmt.Sprintf("The password hash of user %s should start with $2 but it doesn't. Bcrypt "+
						"password hashes start with $2", user.Name)
					if logError {
						logger.Error(msg)
					}
					return errors.New(msg)
				}
			}
			// Access field has only two options allowed: "read" or "write"
			if strings.ToLower(user.Access) != "read" && strings.ToLower(user.Access) != "write" {
				msg := fmt.Sprintf("Username '%s' has field 'access' set to value '%s' but the only two allowed "+
					"values are 'read' or 'write'", user.Name, user.Access)
				if logError {
					logger.Error(msg)
				}
				return errors.New(msg)
			}
		}
	}
	return nil
}

// validateBackupOrTargetName rejects names that would produce ambiguous or unsafe
// on-disk filenames for the per-target restore SQLite databases. Per restore DB
// naming (restore__<backupName>__<targetName>.sqlite), the separator token MUST NOT
// appear inside either component, and neither component may contain a path
// separator since that would let a crafted name escape the data directory.
// $kind is "Backup" or "Target" and is used solely in the error message.
func validateBackupOrTargetName(kind string, name string, logError bool) error {
	if strings.Contains(name, database.RestoreDbSeparator) {
		msg := fmt.Sprintf("%s 'name=%s' contains the reserved substring '%s' which is used internally as a "+
			"separator in restore database filenames. Choose a different name",
			kind, name, database.RestoreDbSeparator)
		if logError {
			logger.Error(msg)
		}
		return errors.New(msg)
	}
	if strings.ContainsAny(name, `/\`) {
		msg := fmt.Sprintf("%s 'name=%s' contains a path separator character (one of '/' or '\\'). %s names are "+
			"used as filename components and are not allowed to contain path separators",
			kind, name, kind)
		if logError {
			logger.Error(msg)
		}
		return errors.New(msg)
	}
	if name == "." || name == ".." {
		msg := fmt.Sprintf("%s 'name=%s' is a reserved name that cannot be used because it would produce an "+
			"ambiguous filename", kind, name)
		if logError {
			logger.Error(msg)
		}
		return errors.New(msg)
	}
	return nil
}

// checks if a sql database exists for each "backup" section and if it doesn't then it attempts to create it
// params: config struct to validate; because NO LOCKING IS USED the config struct should not be in use by anything else
// this function is not called from Validate() as it actually changes things on disk (aka creates DBs) so we want it
// called only after Validate() and only in specific cases
func ValidateAndCreateDB(config shared.CfgTemplate, backupJobsState *shared.BackupJobsState) error {
	if len(config.User) > 0 {
		for _, backup := range config.Backup {
			err := database.ValidateAndCreate(config.DataDir, backup.Name, true, backupJobsState)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// replace passwords or secrets with **************** within an instance of CfgTemplate type
// Unfortunately this function doesn't have any smarts so whenever the config struct is changed then also an update to
// the function is needed.
func SanitizeCfgTemplate(config shared.CfgTemplate) shared.CfgTemplate {
	// overwrite User.Pass
	for i := 0; i < len(config.User); i++ {
		if config.User[i].Pass != "" {
			config.User[i].Pass = SecretReplace
		}
	}
	// overwrite ConfigBackup.EncryptPass and ConfigBackup.Target.Pass
	for i := 0; i < len(config.Backup); i++ {
		if config.Backup[i].EncryptPass != "" {
			config.Backup[i].EncryptPass = SecretReplace
		}
		for j := 0; j < len(config.Backup[i].Target); j++ {
			for k := 0; k < len(config.Backup[i].Target[j].Parameters); k++ {
				for _, val := range ParemtersWithSecrets {
					if strings.EqualFold(config.Backup[i].Target[j].Parameters[k].Name, val) {
						config.Backup[i].Target[j].Parameters[k].Value = SecretReplace
					}
				}
			}
		}
	}
	// overwrite notifications.[]Email.Pass
	for i := 0; i < len(config.Notifications.Email); i++ {
		if config.Notifications.Email[i].Pass != "" {
			config.Notifications.Email[i].Pass = SecretReplace
		}
	}
	return config
}

// checks if a string is made up only of a given character (or substring)
func CheckStringIsOnly(val string, chars string) bool {
	if utf8.RuneCountInString(val) > 0 && val == strings.Repeat(chars, utf8.RuneCountInString(val)) {
		return true
	} else {
		return false
	}
}

// reads passwords from the old config. If the new config has an entry which matches (meaning both have the same name)
// one from the old config and in the new config this entry has a password of "*****" (more or less stars) then copy
// Returns an error if one or more **** based passwords don't have a counterpart in the old config so the old password
// can't be extracted
func CopyPasswordsFromOldConfig(newConfig *shared.CfgTemplate, oldConfig shared.CfgTemplate) error {
	// compare User.Password entries
	err := CopyPasswordsFromOldConfigUser(newConfig.User, oldConfig.User)
	if err != nil {
		return err
	}

	// compare Backup.EncryptPass and Backup.Target.Params."different secrets" entries
	err = CopyPasswordsFromOldConfigBackup(newConfig.Backup, oldConfig.Backup)
	if err != nil {
		return err
	}

	// compared config.Notification.Email.Pass entries
	err = CopyPasswordsFromOldConfigNotificationsEmails(newConfig.Notifications.Email, oldConfig.Notifications.Email)
	if err != nil {
		return err
	}

	return nil
}

// reads passwords from the old config User type entries.
// If the new config has an entry which matches (meaning both have the same name)
// one from the old config and in the new config this entry has a password of "*****" (more or less stars) then copy
// Returns an error if one or more **** based passwords don't have a counterpart in the old config so the old password
// can't be extracted
//
// a slice is a kind of pointer hence we don't pass in "newConfigUser" as a pointer
func CopyPasswordsFromOldConfigUser(newConfigUser []shared.ConfigUser, oldConfigUser []shared.ConfigUser) error {
	// compare User.Password entries
	for i := 0; i < len(newConfigUser); i++ {
		if CheckStringIsOnly(newConfigUser[i].Pass, "*") {
			foundMatch := false
			// search for a match in the old(active) config
			for j := 0; j < len(oldConfigUser); j++ {
				if oldConfigUser[j].Name == newConfigUser[i].Name {
					foundMatch = true
					newConfigUser[i].Pass = oldConfigUser[j].Pass
					break
				}
			}
			if !foundMatch {
				return fmt.Errorf("username '%s' has a password of '%s' which implies the password "+
					"should be copied from the current(active) configuration but no such username was found in "+
					"the current configuration", newConfigUser[i].Name, newConfigUser[i].Pass)
			}
		}
	}
	return nil
}

// reads passwords from the old config.Notification.Email.User type entries.
// If the new config has an entry which matches (meaning both have the same User name & the same server address)
// one from the old config and in the new config this entry has a password of "*****" (more or less stars) then copy
// Returns an error if one or more **** based passwords don't have a counterpart in the old config so the old password
// can't be extracted
//
// a slice is a kind of pointer hence we don't pass in "newConfigUser" as a pointer
func CopyPasswordsFromOldConfigNotificationsEmails(newNotificationsEmail []shared.ConfigNotificationEmail, oldNotificationsEmail []shared.ConfigNotificationEmail) error {
	// compare Notification.Email.Pass entries
	for i := 0; i < len(newNotificationsEmail); i++ {
		if CheckStringIsOnly(newNotificationsEmail[i].Pass, "*") {
			foundMatch := false
			// search for a match in the old(active) config. Because as opposed to other places where we do this,
			// here we don't have a unique field per entry, we consider the $server to be this (despite the fact that
			// we don't enforce uniqueness or imply it)
			for j := 0; j < len(oldNotificationsEmail); j++ {
				if oldNotificationsEmail[j].Server == newNotificationsEmail[i].Server && oldNotificationsEmail[j].User == newNotificationsEmail[i].User {
					foundMatch = true
					newNotificationsEmail[i].Pass = oldNotificationsEmail[j].Pass
					break
				}
			}
			if !foundMatch {
				return fmt.Errorf("in the notifications section, username '%s' configured for "+
					"email(SMTP) server '%s' has a password "+
					"of '%s' which implies the password should be copied from the current(active) configuration but no "+
					"such username + server address was found in the current configuration",
					newNotificationsEmail[i].User, newNotificationsEmail[i].Server, oldNotificationsEmail[i].Pass)
			}
		}
	}
	return nil
}

// reads passwords from the old config Backup type entries.
// If the new config has an entry which matches (meaning both have the same name)
// one from the old config and in the new config this entry has a password of "*****" (more or less stars) then copy
// Returns an error if one or more **** based passwords don't have a counterpart in the old config so the old password
// can't be extracted
//
// a slice is a kind of pointer hence we don't pass in "newConfigBackup" as a pointer
func CopyPasswordsFromOldConfigBackup(newConfigBackup []shared.ConfigBackup, oldConfigBackup []shared.ConfigBackup) error {
	// compare ConfigBackup.EncryptPass entries
	for i := 0; i < len(newConfigBackup); i++ {
		// compare ConfigBackup.EncryptPass
		if CheckStringIsOnly(newConfigBackup[i].EncryptPass, "*") {
			foundMatch := false
			// search for a match in the old(active) config
			for j := 0; j < len(oldConfigBackup); j++ {
				if oldConfigBackup[j].Name == newConfigBackup[i].Name {
					if oldConfigBackup[j].EncryptPass != "" {
						foundMatch = true
						newConfigBackup[i].EncryptPass = oldConfigBackup[j].EncryptPass
						break
					} else {
						return fmt.Errorf("backup having name '%s' has an 'encrypt_pass' of '%s' "+
							"which implies the password should be copied from the current(active) configuration but "+
							"in the current configuration there isn't a password set for 'encrypt_pass' so there "+
							"is nothing to copy from", newConfigBackup[i].Name, newConfigBackup[i].EncryptPass)
					}
				}
			}
			if !foundMatch {
				return fmt.Errorf("backup having name '%s' has an 'encrypt_pass' of '%s' which implies the password "+
					"should be copied from the current(active) configuration but no backup with the same name was found in "+
					"the current configuration", newConfigBackup[i].Name, newConfigBackup[i].EncryptPass)
			}
		}

		// compare ConfigBackup.Target.Parameters.* entries and look for the ones with secrets
		for j := 0; j < len(newConfigBackup[i].Target); j++ {
			for k := 0; k < len(newConfigBackup[i].Target[j].Parameters); k++ {
				for _, parameter := range ParemtersWithSecrets {
					if strings.EqualFold(newConfigBackup[i].Target[j].Parameters[k].Name, parameter) {
						// if the new value contains only "*" then copy the secret from the old Value
						if CheckStringIsOnly(newConfigBackup[i].Target[j].Parameters[k].Value, "*") {
							// check if the old config has a Backup with the same name
							foundMatchingBackupName := false
							var matchingOldBackup shared.ConfigBackup
							for _, entry := range oldConfigBackup {
								if entry.Name == newConfigBackup[i].Name {
									foundMatchingBackupName = true
									matchingOldBackup = entry
									break
								}
							}
							if !foundMatchingBackupName {
								return fmt.Errorf("backup having name '%s' and target '%s' has secret "+
									"parameter '%s' with value '%s' which implies the value "+
									"should be copied from the current(active) configuration but in the current "+
									"configuration there isn't a backup section having the same name so there "+
									"is nothing to copy from", newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
									newConfigBackup[i].Target[j].Parameters[k].Name, newConfigBackup[i].Target[j].Parameters[k].Value)
							}
							foundMatchingTarget := false
							var matchingOldTarget shared.ConfigBackupTarget
							// check if the old config has a target with the same Name and Type
							for _, target := range matchingOldBackup.Target {
								if target.Name == newConfigBackup[i].Target[j].Name && target.Type == newConfigBackup[i].Target[j].Type {
									foundMatchingTarget = true
									matchingOldTarget = target
									break
								}
							}
							if !foundMatchingTarget {
								return fmt.Errorf("backup having name '%s' and target '%s' has secret "+
									"parameter '%s' with value '%s' which implies the value "+
									"should be copied from the current(active) configuration but in the current "+
									"configuration the backup section having the same name does not also have a target "+
									"with a matching name and matching type so there is nothing to copy from",
									newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
									newConfigBackup[i].Target[j].Parameters[k].Name,
									newConfigBackup[i].Target[j].Parameters[k].Value)
							}
							foundMatchingParameter := false
							// check if the old config has Parameter with the same name
							for _, oldParameter := range matchingOldTarget.Parameters {
								if strings.EqualFold(oldParameter.Name, newConfigBackup[i].Target[j].Parameters[k].Name) {
									if oldParameter.Value == "" {
										return fmt.Errorf("backup having name '%s' and target '%s' has secret "+
											"parameter '%s' with value '%s' which implies the value "+
											"should be copied from the current(active) configuration but in the current "+
											"configuration the backup section having the same name and target "+
											"with a matching name and matching type does have a '%s' parameter but its value is an empty string",
											newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
											newConfigBackup[i].Target[j].Parameters[k].Name,
											newConfigBackup[i].Target[j].Parameters[k].Value,
											newConfigBackup[i].Target[j].Parameters[k].Name)
									}
									newConfigBackup[i].Target[j].Parameters[k].Value = oldParameter.Value
									foundMatchingParameter = true
									break
								}
							}
							if !foundMatchingParameter {
								return fmt.Errorf("backup having name '%s' and target '%s' has secret "+
									"parameter '%s' with value '%s' which implies the value "+
									"should be copied from the current(active) configuration but in the current "+
									"configuration the backup section having the same name and target "+
									"with a matching name and matching type does not have a '%s' parameter so the value can't be copied",
									newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
									newConfigBackup[i].Target[j].Parameters[k].Name,
									newConfigBackup[i].Target[j].Parameters[k].Value,
									newConfigBackup[i].Target[j].Parameters[k].Name)
							}
						}
					}
				}

			}
		}
	}
	return nil
}

func isLocalhost(name string) bool {
	return strings.ToLower(name) == "localhost" || name == "127.0.0.1" || name == "::1"
}

// saves in $datadir/config.yaml a sanitised version of the config file (meaning all secrets are replaced with ****) and returns the path to the file
// It's up to the caller to remove the TMP file (if no error was returned)
func SaveSanitizedCfgToTmpFile(serverConfigCopy shared.CfgTemplate) (string, error) {
	sanitizedCfg := SanitizeCfgTemplate(serverConfigCopy)
	toWrite, err := yaml.Marshal(sanitizedCfg)
	if err != nil {
		return fmt.Sprintf("could not marshall YAML when preparing for write the copy of the configuration "+
			"file. The error received was: %s", err), err
	}
	filePath := filepath.Join(serverConfigCopy.DataDir, "config.yaml")
	err = os.WriteFile(filePath, toWrite, 0600)
	if err != nil {
		return fmt.Sprintf("While trying to save a copy of the configuration "+
			"file, encountered error: %s", err), err
	}
	return filePath, nil
}

// given a string parameter which has a value of "yes"(any case), "no"(any case), "1", "t", "T", "true", "TRUE",
// "True", "0", "f", "F", "false", "FALSE", "False":  it converts it to a bool. Returns error if conversion is not possible
func StringParameterToBoolean(parameter string) (bool, error) {
	switch strings.ToLower(parameter) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	default:
		{
			return strconv.ParseBool(parameter)
		}
	}
}
