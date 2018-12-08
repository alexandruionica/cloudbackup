package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "objectstore"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type ObjectStore interface {
	Upload (path string, newDbRecord shared.BackedUpFileProperties, backupJobsState shared.BackupJobsStateInterface) (result string, cancelled bool, err error)
	// update or create a metadata entry
	MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties) (result string, cancelled bool, err error)
	//
	GetStoreDetails()(StoreName string, StoreType string)
}

func GetObjectStores (ctx context.Context, backupConfig config.Backup) ([]ObjectStore, error) {
	results := make([]ObjectStore, 0)
	for _, backupTarget := range backupConfig.Target {
		switch backupTarget.Type {
			case "test_null": {
				results = append(results, InitialiseStoreTestNull(ctx, backupConfig, backupTarget.Name, backupTarget.Type))
				ratelimit, err := humanize.ParseBytes(backupTarget.RateLimit)
				if err != nil {
					return results, errors.New(fmt.Sprintf("While trying to convert the rate limit '%s' to a number the following error was encountered: %s", backupTarget.RateLimit, err))
				}
				if ratelimit > 0 {
					logger.Infof("Target '%s' for backup '%s' has defined a rate limit of %s", backupTarget.Name, backupConfig.Name, humanize.Bytes(ratelimit))
				}
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