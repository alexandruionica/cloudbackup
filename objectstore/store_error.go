package objectstore

import (
	"cloudbackup/shared"
	"context"
	"fmt"
)

// this object store is used only for when we have errors due to an unknown store being supplied. Beside satisfying
// signatures this store is useless (except its also used for testing when we want to emulate a store error for a given operation)
type StoreError struct {
	storeName string
	storeType string
}

func InitialiseStoreError(ctx context.Context, backupConfig shared.ConfigBackup, storeName string, storeType string, rateLimitVal int64) *StoreError {
	result := &StoreError{
		storeName: storeName,
		storeType: storeType,
	}
	// actual backends will also setup the connection client in this section
	return result
}

func (object *StoreError) Upload(newDbRecord shared.BackedUpFileProperties, version int, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}

// GetStoreDetails()(StoreName string, StoreType string)
func (object *StoreError) GetStoreDetails() (StoreName string, StoreType string) {
	return object.storeName, object.storeType
}

func (object *StoreError) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, version int, metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}

func (object *StoreError) Delete(path string, objType string, version int, remoteVersion string, metadata bool) error {
	return fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}

func (object *StoreError) Validate() (string, error) {
	return "", fmt.Errorf("unsupported backend of type: '%s'", object.storeType)
}
