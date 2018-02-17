package config

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
	"github.com/jinzhu/configor"
	"os"
	"errors"
)

var logger = log.WithFields(log.Fields{
	"context": "config",
	})

type CfgTemplate = struct {
	Backup []struct {
		Name     string `required:"true"`
		Paths     []string `required:"true"`
		Exclusions []string
		Targets []struct {
			Name  string `required:"true"`
			Type string `required:"true"`
			User string
			Pass string `secret:"true"`
			Bucket string `required:"true"`
			Prefix string `required:"true"`
			StorageClass string `yaml:"storage_class"`
		}
		Schedule []string `required:"true"`
		Encrypt bool `default:"false"`
		EncryptPass string `secret:"true" dependsOn:"Encrypt" yaml:"encrypt_pass"`
		Versioning bool `default:"false"`
		VersionsMaxNum uint `dependsOn:"Versioning" yaml:"versions_max_num"`
		VersionsMaxAge string `dependsOn:"Versioning" yaml:"versions_max_age"`
	} `required:"true"`
}

type Configuration struct {
	// lock this before reading or writing the config file or reading / writing the loaded config variables
	Mutex *sync.Mutex
	// path to config file
	Path string
	// actual config file
	Config CfgTemplate
}

// return a copy of the config struct. A lock while reading the struct
func (cfg *Configuration) GetWithLock() CfgTemplate {
	logger.Debug("Acquiring lock before copying config struct")
	cfg.Mutex.Lock()
	defer func() {
		cfg.Mutex.Unlock()
		logger.Debug("Lock released after copying config struct")
	}()
	cfgCopy := cfg.Config
	return cfgCopy
}

// load configuration from yaml file at "path" and if boolean "debug" is set then also enable debugging in the yaml
// config parser library
func Load(path string, debug bool, mutex *sync.Mutex) (*Configuration, error) {
	logger.Info(fmt.Sprintf("Loading config file %s", path))
	const envPrefix = "CLOUDBACKUP"
	var Config = CfgTemplate{}
	var err error

	if _, err := os.Stat(path); os.IsNotExist(err) {
		msg := fmt.Sprintf("File %s does not exist", path)
		logger.Error(msg)
		return &Configuration{}, errors.New(msg)
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
		return &Configuration{}, errors.New(msg)
	}

	return &Configuration{Mutex: mutex,
					      Path: path,
					      Config: Config,
	}, err
}