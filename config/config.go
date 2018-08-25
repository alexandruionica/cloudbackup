package config

import (
	"cloudbackup/utils"
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
	"github.com/jinzhu/configor"
	"errors"
	"path/filepath"
	"strings"
    "unicode/utf8"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"cloudbackup/database"
)

const loggingContext = "config"
const SecretReplace = "****************"
// used for looking up environment variables holding configuration data
const EnvPrefix = "CLOUDBACKUP"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
	})


// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating
type Backup struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Paths []string `required:"true" yaml:"paths" json:"paths"`
	Exclusions []string `yaml:"exclusions" json:"exclusions"`
	// TODO - fix library bug - https://github.com/jinzhu/configor/issues/34
	Dereference bool `default:"true" yaml:"dereference" json:"dereference"`
	Target []Target `required:"true" yaml:"target" json:"target"`
	Schedule []string `yaml:"schedule" json:"schedule"`
	Encrypt bool `default:"false" yaml:"encrypt" json:"encrypt"`
	EncryptPass string `yaml:"encrypt_pass" json:"encrypt_pass"`
	Versioning bool `default:"false" yaml:"versioning" json:"versioning"`
	VersionsMaxNum uint `yaml:"versions_max_num" json:"versions_max_num"`
	VersionsMaxAge string `yaml:"versions_max_age" json:"versions_max_age"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating
type Target struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Type string `required:"true" yaml:"type" json:"type"`
	User string `yaml:"user" json:"user"`
	Pass string `yaml:"pass" json:"pass"`
	Bucket string `required:"true" yaml:"bucket" json:"bucket"`
	Prefix string `required:"true" yaml:"prefix" json:"prefix"`
	StorageClass string `yaml:"storage_class" json:"storage_class"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating
type User struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Pass string `required:"true" yaml:"pass" json:"pass"`
	// allowed options are read or write (write implies read)
	Access string `default:"read" yaml:"access" json:"access"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
type Http struct {
	BindAddress string `default:"127.0.0.1:8080" yaml:"bind_address" json:"bind_address"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
type Https struct {
	Enabled bool `default:"false" yaml:"enabled" json:"enabled"`
	BindAddress string `default:"127.0.0.1:8443" yaml:"bind_address" json:"bind_address"`
	SslCertPath string `yaml:"ssl_cert_path" json:"ssl_cert_path"`
	SslKeyPath string `yaml:"ssl_key_path" json:"ssl_key_path"`
}

// this is the "master" struct which keeps all of the config settings (as specified in the config file + env vars)
// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
type CfgTemplate struct {
	DataDir string `required:"true" yaml:"data_dir" json:"data_dir"`
	HtmlDir string `default:"webstatic" yaml:"html_dir" json:"html_dir"`
	User []User `yaml:"user" json:"user"`
	Http Http `yaml:"http" json:"http"`
	Https Https `yaml:"https" json:"https"`
	Backup []Backup `yaml:"backup" json:"backup"`
	// the mutex is used for locking mainly only when we deal with copies of this struct (which may not have the parent
	// RuntimeConfig struct containing this struct)
	Mutex *sync.RWMutex `yaml:"-" json:"-"`
}


// this struct contains the above "master" config struct and also some runtime related parameters and settings
type RuntimeConfig struct {
	// lock this before reading or writing the config file or reading / writing the loaded config variables
	Mutex *sync.RWMutex
	// path to config file
	Path string
	// actual config file
	Config CfgTemplate
}

// return a copy of the config struct. Lock while reading the struct. logContext is used for passing the caller's
// logging context as to make it clear where the call is coming from
func (cfg *RuntimeConfig) GetCopyWithLock(logContext string) CfgTemplate {
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying server config" +
		" struct")
	cfg.Mutex.RLock()
	defer func() {
		cfg.Mutex.RUnlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying server " +
			"config struct")
	}()
	log.WithFields(log.Fields{"context": logContext}).Debug("Read lock for copying server config acquired")
	cfgCopy := cfg.Config

	// we need to manually copy slices because by default a pointer to the slice is copied
	cfgCopy.User = make([]User, len(cfg.Config.User))
	copy(cfgCopy.User, cfg.Config.User)
	cfgCopy.Backup = make([]Backup, len(cfg.Config.Backup))
	copy(cfgCopy.Backup, cfg.Config.Backup)
	// deepcopy various slices part of the Backup{} struct
	for i := 0; i < len(cfg.Config.Backup); i++ {
		// deepcopy the []Target slice
		cfgCopy.Backup[i].Target = make([]Target, len(cfg.Config.Backup[i].Target))
		copy(cfgCopy.Backup[i].Target, cfg.Config.Backup[i].Target)
		// deepcopy the []Path slice
		cfgCopy.Backup[i].Paths = make([]string, len(cfg.Config.Backup[i].Paths))
		copy(cfgCopy.Backup[i].Paths, cfg.Config.Backup[i].Paths)
		// deepcopy the []Exclusions slice
		cfgCopy.Backup[i].Exclusions = make([]string, len(cfg.Config.Backup[i].Exclusions))
		copy(cfgCopy.Backup[i].Exclusions, cfg.Config.Backup[i].Exclusions)
		// deepcopy the []Schedule slice
		cfgCopy.Backup[i].Schedule = make([]string, len(cfg.Config.Backup[i].Schedule))
		copy(cfgCopy.Backup[i].Schedule, cfg.Config.Backup[i].Schedule)
	}
	// new mutex for locking
	cfgCopy.Mutex = &sync.RWMutex{}
	return cfgCopy
}

// load configuration from yaml file at "path" and if boolean "debug" is set then also enable debugging in the yaml
// config parser library
func Load(path string, debug bool, mutex *sync.RWMutex) (*RuntimeConfig, error) {
	logger.Info(fmt.Sprintf("Loading server config file %s", path))

	var Config = CfgTemplate{}
	var err error

	if _, err := utils.FileExists(path, true); err != nil {
		logger.Error(err)
		return &RuntimeConfig{}, err
	}

	logger.Debug("Acquiring read lock before reading config file")
	mutex.RLock()
	defer func() {
		mutex.RUnlock()
		logger.Debug("Read lock released after reading config file")
		}()
	logger.Debug("Acquired read lock for reading config file")
	// if debug then also adjust logging level of configor library (set library to Verbose not Debug as
	// Verbose is actually what we expect when using  "debug")
	if debug {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix, Verbose: true}).Load(&Config, path)
	} else {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix}).Load(&Config, path)
	}

	if err != nil {
		msg := fmt.Sprintf("When parsing the server configuration file %s the following error was encountered:" +
			" %s", path, err)
		logger.Error(msg)
		return &RuntimeConfig{}, errors.New(msg)
	}

	err = Validate(Config, false)
	if err != nil {
		return &RuntimeConfig{}, err
	}

	// rarely used lock (this is to be used only by functions which get the Config struct but don't get also the
	// parent struct (which has a different lock)
	Config.Mutex = &sync.RWMutex{}

	return &RuntimeConfig{Mutex: mutex,
					      Path: path,
					      Config: Config,
	}, nil
}

// saves new configuration to file
func Save(runtimeCfg *RuntimeConfig, newConfig CfgTemplate) error {
	logger.Debug("Acquiring lock before writing config file")
	runtimeCfg.Mutex.Lock()
	defer func() {
		runtimeCfg.Mutex.Unlock()
		logger.Debug("Lock released after attempting to write config file")
	}()
	toWrite, err := yaml.Marshal(newConfig)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not marshall YAML when preparing for write the configuration " +
			"file. The error received was: %s", err.Error()))
	}
	if err := ioutil.WriteFile(runtimeCfg.Path, toWrite, 0644); err != nil {
		return errors.New(fmt.Sprintf("Could not write to configuration file '%s' . Received error was: %s",
			runtimeCfg.Path, err))
	}
	logger.Debug("Updating in-memory configuration")
	runtimeCfg.Config = newConfig
	return nil
}

// validate several config options which depends on other options having certain values. Trying to do this with
// reflection ends up being harder to understand and still requires application logic in the validator
// params: config struct to validate; hiddenPass is if to allow obfuscated passwords (meaning strings with value *****)
func Validate(config CfgTemplate, hiddenPass bool) error {
	// check if "data_dir" exists
	if err := ValidateDir(config.DataDir, "data_dir", true); err != nil {
		return err
	}
	// check if "html_dir" exists
	if err := ValidateDir(config.HtmlDir, "html_dir", true); err != nil {
		return err
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
	return nil
}

// validate "Backup" section of the config
func ValidateBackup(backups []Backup, logError bool) error {
	names := make([]string, 0)
	i:=0
	for _, backup := range backups {
		// have this as the first check as subsequent ones use the Backup name in error output in order to indicate
		// where did things go wrong
		if utils.StringInSlice(backup.Name, names){
			msg := fmt.Sprintf("more than one Backups have the same 'name=%s' . Backup 'name' values must" +
				" be unique", backup.Name)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		} else {
			names = append(names, backup.Name)
		}
		if backup.Encrypt && backup.EncryptPass == ""{
			msg := fmt.Sprintf("backup[%d] having 'name=%s' has setting 'encrypt=true' but 'encrypt_pass' is not" +
				" set. Set a password or disable encryption", i, backup.Name)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == false && backup.VersionsMaxNum > 0 {
			msg := fmt.Sprintf("backup[%d] having 'name=%s' has setting 'versioning=false' but " +
				"'versions_max_num=%d' . Enable versioning or remove the 'versions_max_num' setting",
					i, backup.Name, backup.VersionsMaxNum)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == false && backup.VersionsMaxAge != "" {
			msg := fmt.Sprintf("backup[%d] having 'name=%s' has setting 'versioning=false' but " +
				"'versions_max_age=%s' . Enable versioning or remove the 'versions_max_age' setting", i, backup.Name,
					backup.VersionsMaxAge)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == true && backup.VersionsMaxAge == "" && backup.VersionsMaxNum == 0 {
			msg := fmt.Sprintf("backup[%d] having 'name=%s' has setting 'versioning=true' but " +
				"'versions_max_num=0' or is unset and 'versions_max_age' is unset. Disable versioning or set " +
					"'versions_max_num' > 0 or set 'versions_max_age'", i, backup.Name)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		err := ValidateBackupTarget(backup.Target, logError, backup.Name)
		if err != nil {
			return err
		}
		i+=1
	}
	return nil
}

// validate HTTPS section of the config
func ValidateHttps(config CfgTemplate, logError bool) error {
	if config.Https.Enabled == true {
		if config.Https.SslCertPath == "" {
			msg := fmt.Sprintf("https: enabled=true  but https: ssl_cert_path  is not set")
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if config.Https.SslKeyPath == "" {
			msg := fmt.Sprintf("https: enabled=true  but https: ssl_key_path  is not set")
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if _, err := utils.FileExists(config.Https.SslCertPath, true); err != nil {
			msg := fmt.Sprintf("https: enabled=true  and https: ssl_cert_path=%s but when evaluating " +
				"the latter the following error ocurred: %s", config.Https.SslCertPath, err)
			if logError{
				logger.Error(msg)
			}
			return err
		}
		if _, err := utils.FileExists(config.Https.SslKeyPath, true); err != nil {
			msg := fmt.Sprintf("https: enabled=true  and https: ssl_key_path=%s but when evaluating " +
				"the latter the following error ocurred: %s", config.Https.SslKeyPath, err)
			if logError{
				logger.Error(msg)
			}
			return err
		}
	}
	return nil
}

// validate "Backup/Target" section of the config
func ValidateBackupTarget(targets []Target, logError bool, BackupName string) error {
	names := make([]string, 0)
	for _, target := range targets {
		// have this as the first check as subsequent ones use the Target name in error output in order to indicate
		// where did things go wrong

		// check uniqueness of backup Target name
		if utils.StringInSlice(target.Name, names){
			msg := fmt.Sprintf("more than one 'target' of the same 'backup' (belonging to backup section having" +
				" 'name=%s') have the same 'name=%s' . Target 'name' values must be unique within a 'backup'" +
					" section", BackupName, target.Name)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		} else {
			names = append(names, target.Name)
		}
	}
	return nil
}

// Validate directory path
func ValidateDir(dir string, paramName string, logError bool) error {
	_, err := utils.DirExists(dir, true)
	if err != nil{
		msg := ""
		switch err {
		case utils.ErrNoSuchDir: {
			msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter does not exist or can not be accessed. " +
				"In case '%s' is a default value for '%s' then you will not notice it in the configuration file.",
				dir, paramName, dir, paramName )
			}
		case utils.ErrUnusableDirPath: {
			msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter can not be used. In case '%s' is a " +
				"default value for '%s' then you will not notice it in the configuration file.",
				dir, paramName, dir, paramName)
		}
		case utils.ErrNoSuchRelativeDir: {
			absPath, _ := filepath.Abs(dir) // #nosec
			msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter does not exist or can not be accessed. " +
				"The absolute is: '%s'. In case '%s' is a default value for '%s' then you will not notice it in the" +
					" configuration file.", dir, paramName, absPath, dir, paramName )
			}
		case utils.ErrNotADir: {
			msg = fmt.Sprintf("Path '%s' supplied for '%s' parameter exists but it is not a directory. In case " +
				"'%s' is a default value for '%s' then you will not notice it in the configuration file.",
				dir, paramName, dir, paramName)
			}
		}

		if logError{
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
func ValidateUser(config CfgTemplate, logError bool, hiddenPass bool) error {
	if len(config.User) > 0 {
		names := make([]string, 0)
		for _, user := range config.User {
			if utils.StringInSlice(user.Name, names){
				msg := fmt.Sprintf("more than one users have the same 'name=%s' . User 'name' values must" +
					" be unique", user.Name)
				if logError{
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
				msg := fmt.Sprintf("Username '%s' has field 'access' set to value '%s' but the only two allowed " +
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

// checks if a sql database exists for each "backup" section and if it doesn't then it attempts to create it
// params: config struct to validate; because NO LOCKING IS USED the config struct should not be in use by anything else
// this function is not called from Validate() as it actually changes things on disk (aka creates DBs) so we want it
// called only after Validate() and only in specific cases
func ValidateAndCreateDB(config CfgTemplate) error {
	if len(config.User) > 0 {
		for _, backup := range config.Backup {
			err := database.ValidateAndCreate(config.DataDir, backup.Name, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// replace passwords or secrets with **************** within an instance of CfgTemplate type
// Unfortunately this function doesn't have any smarts so whenever the config struct is changed then also an update to
// the function is needed
func SanitizeCfgTemplate (config CfgTemplate) CfgTemplate {
	// overwrite User.Pass
	for i := 0; i < len(config.User); i++ {
		if config.User[i].Pass != "" {
			config.User[i].Pass = SecretReplace
		}
	}
	// overwrite Backup.EncryptPass and Backup.Target.Pass
	for i := 0; i < len(config.Backup); i++ {
		if config.Backup[i].EncryptPass != "" {
			config.Backup[i].EncryptPass = SecretReplace
		}
		for j := 0; j < len(config.Backup[i].Target); j++ {
			if config.Backup[i].Target[j].Pass != "" {
				config.Backup[i].Target[j].Pass = SecretReplace
			}
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
func CopyPasswordsFromOldConfig(newConfig *CfgTemplate, oldConfig CfgTemplate) error {
	// compare User.Password entries
	err := CopyPasswordsFromOldConfigUser(newConfig.User, oldConfig.User)
	if err != nil {
		return err
	}

	// compare Backup.EncryptPass and Backup.Target.Pass entries
	err = CopyPasswordsFromOldConfigBackup(newConfig.Backup, oldConfig.Backup)
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
func CopyPasswordsFromOldConfigUser(newConfigUser []User, oldConfigUser []User) error {
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
			if foundMatch != true {
				return errors.New(fmt.Sprintf("Username '%s' has a password of '%s' which implies the password "+
					"should be copied from the current(active) configuration but no such username was found in "+
					"the current configuration", newConfigUser[i].Name, newConfigUser[i].Pass))
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
func CopyPasswordsFromOldConfigBackup(newConfigBackup []Backup, oldConfigBackup []Backup) error {
	// compare Backup.EncryptPass and Backup.Target.Pass entries
	for i := 0; i < len(newConfigBackup); i++ {
		// compare Backup.EncryptPass
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
						return errors.New(fmt.Sprintf("Backup having name '%s' has an 'encrypt_pass' of '%s' " +
							"which implies the password should be copied from the current(active) configuration but " +
							"in the current configuration there isn't a password set for 'encrypt_pass' so there " +
							"is nothing to copy from", newConfigBackup[i].Name, newConfigBackup[i].EncryptPass))
					}
				}
			}
			if foundMatch != true {
				return errors.New(fmt.Sprintf("Backup having name '%s' has an 'encrypt_pass' of '%s' which implies the password " +
					"should be copied from the current(active) configuration but no backup with the same name was found in " +
					"the current configuration", newConfigBackup[i].Name, newConfigBackup[i].EncryptPass))
			}
		}

		// compare Backup.Target.Pass entries
		for j := 0; j < len(newConfigBackup[i].Target); j++ {
			if CheckStringIsOnly(newConfigBackup[i].Target[j].Pass, "*") {
				foundMatch := false
				// search for a match in the old(active) config - check if we have a backup with the same name
				for k := 0; k < len(oldConfigBackup); k++ {
					if oldConfigBackup[k].Name == newConfigBackup[i].Name {
						foundMatch = true
						// search for a target with the same name in the old config
						foundTargetMatch := false
						for l := 0; l < len(oldConfigBackup[k].Target); l++ {
							if oldConfigBackup[k].Target[l].Name == newConfigBackup[i].Target[j].Name {
								// check if old config Target has a pass and if so copy it
								if oldConfigBackup[k].Target[l].Pass != "" {
									foundTargetMatch = true
									newConfigBackup[i].Target[j].Pass = oldConfigBackup[k].Target[l].Pass
									break
								} else {
									return errors.New(fmt.Sprintf("Backup having name '%s' and target '%s' has an " +
										"'pass' of '%s' which implies the password " +
										"should be copied from the current(active) configuration but in the current " +
										"configuration there isn't a password set for the same target name so there " +
										"is nothing to copy from", newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
										newConfigBackup[i].Target[j].Pass))
								}
							}
						}
						if foundTargetMatch != true {
							return errors.New(fmt.Sprintf("Backup having name '%s' and target '%s' has an " +
								"'pass' of '%s' which implies the password " +
								"should be copied from the current(active) configuration but no 'target' with the " +
								"same name was found in the current configuration for a Backup having the same" +
								" name", newConfigBackup[i].Name, newConfigBackup[i].Target[j].Name,
								newConfigBackup[i].Target[j].Pass))
						}

					}
				}
				if foundMatch != true {
					return errors.New(fmt.Sprintf("Backup having name '%s' and target '%s' has an " +
						"'pass' of '%s' which implies the password " +
						"should be copied from the current(active) configuration but no 'backup' with the " +
						"same name was found in the current configuration", newConfigBackup[i].Name,
						newConfigBackup[i].Target[j].Name, newConfigBackup[i].Target[j].Pass))
				}
			}
		}
	}
	return nil
}
