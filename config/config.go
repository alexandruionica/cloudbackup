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
)

const loggingContext = "config"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
	})


// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
type Backup struct {
	Name     string `required:"true" yaml:"name" json:"name"`
	Paths     []string `required:"true" yaml:"paths" json:"paths"`
	Exclusions []string `yaml:"exclusions" json:"exclusions"`
	Targets []Targets `required:"true" yaml:"targets" json:"targets"`
	Schedule []string `yaml:"schedule" json:"schedule"`
	Encrypt bool `default:"false" yaml:"encrypt" json:"encrypt"`
	EncryptPass string `secret:"true" yaml:"encrypt_pass" json:"encrypt_pass"`
	Versioning bool `default:"false" yaml:"versioning" json:"versioning"`
	VersionsMaxNum uint `yaml:"versions_max_num" json:"versions_max_num"`
	VersionsMaxAge string `yaml:"versions_max_age" json:"versions_max_age"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
type Targets struct {
	Name  string `required:"true" yaml:"name" json:"name"`
	Type string `required:"true" yaml:"type" json:"type"`
	User string `yaml:"user" json:"user"`
	Pass string `secret:"true" yaml:"pass" json:"pass"`
	Bucket string `required:"true" yaml:"bucket" json:"bucket"`
	Prefix string `required:"true" yaml:"prefix" json:"prefix"`
	StorageClass string `yaml:"storage_class" json:"storage_class"`
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
	Http Http `yaml:"http" json:"http"`
	Https Https `yaml:"https" json:"https"`
	Backup []Backup `yaml:"backup" json:"backup"`
}


// this struct contains the above "master" config struct and also some runtime related parameters and settings
type RuntimeConfig struct {
	// lock this before reading or writing the config file or reading / writing the loaded config variables
	Mutex *sync.Mutex
	// path to config file
	Path string
	// actual config file
	Config CfgTemplate
}

// return a copy of the config struct. Lock while reading the struct. logContext is used for passing the caller's
// logging context as to make it clear where the call is coming from
func (cfg *RuntimeConfig) GetWithLock(logContext string) CfgTemplate {
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring lock before copying config struct")
	cfg.Mutex.Lock()
	defer func() {
		cfg.Mutex.Unlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("Lock released after copying config struct")
	}()
	cfgCopy := cfg.Config
	return cfgCopy
}

// load configuration from yaml file at "path" and if boolean "debug" is set then also enable debugging in the yaml
// config parser library
func Load(path string, debug bool, mutex *sync.Mutex) (*RuntimeConfig, error) {
	logger.Info(fmt.Sprintf("Loading config file %s", path))
	const envPrefix = "CLOUDBACKUP"
	var Config = CfgTemplate{}
	var err error

	if _, err := utils.FileExists(path, true); err != nil {
		logger.Error(err)
		return &RuntimeConfig{}, err
	}

	logger.Debug("Acquiring lock before reading config file")
	mutex.Lock()
	defer func() {
		mutex.Unlock()
		logger.Debug("Lock released after reading config file")
		}()
	// if debug then also adjust logging level of configor library (set library to Verbose not Debug as
	// Verbose is actually what we expect when using  "debug")
	if debug {
		err = configor.New(&configor.Config{ENVPrefix: envPrefix, Verbose: true}).Load(&Config, path)
	} else {
		err = configor.New(&configor.Config{ENVPrefix: envPrefix}).Load(&Config, path)
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
	return nil
}

// validate "Backup" section of the config
func ValidateBackup(config CfgTemplate, logError bool) error {
	i:=0
	for _, backup := range config.Backup {
		if backup.Encrypt && backup.EncryptPass == ""{
			msg := fmt.Sprintf("backup[%d]: encrypt=true but backup[%d]: encrypt_pass is not set. Set a password" +
				" or disable encryption", i, i)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == false && backup.VersionsMaxNum > 0 {
			msg := fmt.Sprintf("backup[%d]: versioning=false but backup[%d]: versions_max_num is %d . Enable " +
				"versioning or remove the 'versions_max_num' setting", i, i, backup.VersionsMaxNum)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == false && backup.VersionsMaxAge != "" {
			msg := fmt.Sprintf("backup[%d]: versioning=false but backup[%d]: versions_max_age is %s . Enable " +
				"versioning or remove the 'versions_max_age' setting", i, i, backup.VersionsMaxAge)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
		}
		if backup.Versioning == true && backup.VersionsMaxAge == "" && backup.VersionsMaxNum == 0 {
			msg := fmt.Sprintf("backup[%d]: versioning=true but backup[%d]: versions_max_num is 0 or unset and" +
				" backup[%d]: versions_max_age is unset. Disable versioning or set 'versions_max_num' > 0 or set " +
				"'versions_max_age'", i, i, i)
			if logError{
				logger.Error(msg)
			}
			return errors.New(msg)
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