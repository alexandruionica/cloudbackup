package config

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
)

var logger = log.WithFields(log.Fields{
	"context": "config",
	})

type Configuration struct {
	// lock this before reading or writing the config
	mutex *sync.Mutex
	path string
}

func Load(path string) *Configuration {
	logger.Info(fmt.Sprintf("Loading config file %s", path))
	return &Configuration{mutex: &sync.Mutex{},
					      path: path,
	}
}