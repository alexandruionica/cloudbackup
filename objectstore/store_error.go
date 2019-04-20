package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"fmt"
)

// this object store is used only for when we have errors due to an unknown store being supplied. Beside satisfying
// signatures this store is useless
type StoreError struct {
	storeName string
	storeType string
}

func InitialiseStoreError(ctx context.Context, backupConfig config.Backup, storeName string, storeType string, rateLimitVal int64) *StoreError {
	result := &StoreError{
		storeName: storeName,
		storeType: storeType,
	}
	// actual backends will also setup the connection client in this section
	return result
}

func (object *StoreError) Upload(path string, newDbRecord shared.BackedUpFileProperties, version int, backupJobsState shared.BackupJobsStateInterface) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}

// GetStoreDetails()(StoreName string, StoreType string)
func (object *StoreError) GetStoreDetails() (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}

func (object *StoreError) MarkDeleted(path string, existingDbRecord shared.BackedUpFileProperties, version int) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}
