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

func (objStore *StoreError) Upload(newDbRecord shared.BackedUpFileProperties, version int64, backupJobsState shared.BackupJobsStateInterface,
	metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

// GetStoreDetails()(StoreName string, StoreType string)
func (objStore *StoreError) GetStoreDetails() (StoreName string, StoreType string) {
	return objStore.storeName, objStore.storeType
}

func (objStore *StoreError) MarkDeleted(existingDbRecord shared.BackedUpFileProperties, markerVersion int64, metadata bool) (remoteVersion string, cancelled bool, err error) {
	return "", false, fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreError) Delete(existingDbRecord shared.BackedUpFileProperties, version int64, remoteVersion string, metadata bool) error {
	return fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreError) Get(existingDbRecord shared.BackedUpFileProperties, restorePath string, version int64, remoteVersion string, metadata bool) (cancelled bool, err error) {
	return false, fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}

func (objStore *StoreError) Validate() (string, error) {
	return "", fmt.Errorf("unsupported backend of type: '%s'", objStore.storeType)
}
