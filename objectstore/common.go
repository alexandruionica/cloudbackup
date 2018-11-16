package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "objectstore"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ObjectStore interface {
	Upload (path string, newDbRecord shared.BackedUpFileProperties) (result string, cancelled bool, err error)
	// update or create a metadata entry
	MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties) (result string, cancelled bool, err error)
}

func GetObjectStores (ctx context.Context, backupConfig config.Backup) ([]ObjectStore, error) {
	results := make([]ObjectStore, 0)
	for _, backupTarget := range backupConfig.Target {
		switch backupTarget.Type {
			case "test_null": {
				results = append(results, InitialiseStoreTestNull(ctx, backupConfig, backupTarget.Type))
			}
			// TODO: when implementing aws_s3 backend go back to the config file used for unit tests and add it back there too as it was removed due to tests failing because it was yet to be implemented
			// also update the config file used be the Python integration tests
			default: {
				logger.Errorf("unknown backend of type: '%s'", backupTarget.Type)
				return results, errors.New("unknown backend type")
			}
		}
	}
	return results, nil
}