package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
)

type StoreTestNull struct {
	ctx context.Context
	backupConfig config.Backup
	storeName string
	storeType string
}

func InitialiseStoreTestNull (ctx context.Context, backupConfig config.Backup, storeName string, storeType string) (*StoreTestNull) {
	result := &StoreTestNull{
		ctx: ctx,
		backupConfig: backupConfig,
		storeName: storeName,
		storeType: storeType,
	}
	// actual backends will also setup the connection client in this section
	return result
}

func (object *StoreTestNull) Upload (path string, newDbRecord shared.BackedUpFileProperties, backupJobsState shared.BackupJobsStateInterface)  (result string, cancelled bool, err error) {
	return "", false, nil
}

func (object *StoreTestNull) MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, nil
}

func (object *StoreTestNull) GetStoreDetails ()  (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}