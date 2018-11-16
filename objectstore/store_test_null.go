package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
)

type StoreTestNull struct {
	ctx context.Context
	backupConfig config.Backup
	storeType string
}

func InitialiseStoreTestNull (ctx context.Context, backupConfig config.Backup, storeType string) (*StoreTestNull) {
	result := &StoreTestNull{
		ctx: ctx,
		backupConfig: backupConfig,
		storeType: storeType,
	}
	// actual backends will also setup the connection client in this section
	return result
}

func (object *StoreTestNull) Upload (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, nil
}

func (object *StoreTestNull) MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, nil
}
