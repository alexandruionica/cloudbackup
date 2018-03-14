package config

import (
	"cloudbackup/utils"
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
	"github.com/jinzhu/configor"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const loggingContext = "config"
// used for looking up environment variables holding configuration data
const EnvPrefix = "CLOUDBACKUP"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
	})


// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
type Backup struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Paths []string `required:"true" yaml:"paths" json:"paths"`
	Exclusions []string `yaml:"exclusions" json:"exclusions"`
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
type User struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Pass string `required:"true" yaml:"pass" json:"pass"`
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
type CfgTemplate = struct {
	DataDir string `required:"true" yaml:"data_dir" json:"data_dir"`
	User []User `yaml:"user" json:"user"`
	Http Http `yaml:"http" json:"http"`
	Https Https `yaml:"https" json:"https"`
	Backup []Backup `yaml:"backup" json:"backup"`
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
func (cfg *RuntimeConfig) GetWithLock(logContext string) CfgTemplate {
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying config struct")
	cfg.Mutex.RLock()
	defer func() {
		cfg.Mutex.RUnlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying config struct")
	}()
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
	return cfgCopy
}

// load configuration from yaml file at "path" and if boolean "debug" is set then also enable debugging in the yaml
// config parser library
func Load(path string, debug bool, mutex *sync.RWMutex) (*RuntimeConfig, error) {
	logger.Info(fmt.Sprintf("Loading config file %s", path))

	var Config = CfgTemplate{}
	var err error

	if _, err := utils.FileExists(path, true); err != nil {
		logger.Error(err)
		return &RuntimeConfig{}, err
	}

	logger.Debug("Acquiring lock before reading config file")
	mutex.RLock()
	defer func() {
		mutex.RUnlock()
		logger.Debug("Lock released after reading config file")
		}()
	// if debug then also adjust logging level of configor library (set library to Verbose not Debug as
	// Verbose is actually what we expect when using  "debug")
	if debug {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix, Verbose: true}).Load(&Config, path)
	} else {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix}).Load(&Config, path)
	}

	if err != nil {
		msg := fmt.Sprintf("When parsing the configuration file %s the following error was encountered: %s",
			path, err)
		logger.Error(msg)
		return &RuntimeConfig{}, errors.New(msg)
	}

	err = Validate(Config)
	if err != nil {
		return &RuntimeConfig{}, err
	}

	return &RuntimeConfig{Mutex: mutex,
					      Path: path,
					      Config: Config,
	}, nil
}

// validate several config options which depends on other options having certain values. Trying to do this with
// reflection ends up being harder to understand and still requires application logic in the validator
func Validate(config CfgTemplate) error {
	// check if "data_dir" exists
	if err := ValidateTopLevelDataDir(config, true); err != nil {
		return err
	}
	// validate HTTPS section of the config
	if err := ValidateHttps(config, true); err != nil {
		return err
	}
	// validate "Backup" section of the config
	if err := ValidateBackup(config, true); err != nil {
		return err
	}
	// validate "User" section of the config
	if err := ValidateUser(config, true); err != nil {
		return err
	}
	return nil
}

// validate "Backup" section of the config
func ValidateBackup(config CfgTemplate, logError bool) error {
	names := make([]string, 0)
	i:=0
	for _, backup := range config.Backup {
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

// Validate DataDir top level config entry
func ValidateTopLevelDataDir(config CfgTemplate, logError bool) error {
	stat, err := os.Stat(config.DataDir)
	if err != nil{
		msg := ""
		if filepath.IsAbs(config.DataDir){
			msg = fmt.Sprintf("Path %s supplied for 'data_dir' parameter does not exist or can not be accessed",
				config.DataDir )
		} else {
			path, err := filepath.Abs(config.DataDir)
			if err != nil{
				msg = fmt.Sprintf("Path %s supplied for 'data_dir' parameter can not be used",
					config.DataDir)
			} else {
				msg = fmt.Sprintf("Path %s supplied for 'data_dir' parameter does not exist or can not be " +
					"accessed. The absolute is: %s", config.DataDir, path )
				if logError{
					logger.Error(msg)
				}
				return errors.New(msg)
			}
		}
		if logError{
			logger.Error(msg)
		}
		return err
	}

	if stat.IsDir() {
		return nil
	} else {
		msg := fmt.Sprintf("Path %s supplied for 'data_dir' parameter exists but it is not a directory",
			config.DataDir)
		if logError{
			logger.Error(msg)
		}
		return errors.New(msg)
	}
}

// validate User section
func ValidateUser(config CfgTemplate, logError bool) error {
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
				msg := fmt.Sprintf("The password hash of user %s should start with $2 but it doesn't. Bcrypt " +
					"password hashes start with $2", user.Name)
				if logError{
					logger.Error(msg)
				}
				return errors.New(msg)
			}
		}
	}
	return nil
}

// replace passwords or secrets with **************** within an instance of CfgTemplate type
// Unfortunately this function doesn't have any smarts so whenever the config struct is changed then also an update to
// the function is needed
func SanitizeCfgTemplate (config CfgTemplate) CfgTemplate {
	const SecretReplace = "****************"
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