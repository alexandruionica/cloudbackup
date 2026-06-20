package objectstore

import (
	"cloudbackup/shared"
	"fmt"
	"math"
)

// this object store is used only for when we have errors due to an unknown store being supplied. Beside satisfying
// signatures this store is useless (except its also used for testing when we want to emulate a store error for a given operation)
type StoreError struct {
	storeName string
	storeType string
}

// InitialiseStoreError builds the sentinel "error" store returned for unknown/unsupported
// backend types (and used in tests to emulate store failures). Unlike the real backend
// constructors it intentionally takes no ctx/config/rate-limit args — it never opens a
// connection, so requiring them would be misleading.
func InitialiseStoreError(storeName string, storeType string) *StoreError {
	result := &StoreError{
		storeName: storeName,
		storeType: storeType,
	}
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

// MaxObjectSize: see ObjectStore.MaxObjectSize. The error backend returns MaxInt64 so callers
// reach the Upload() failure path (which is the whole point of this store as a test injector)
// rather than being short-circuited by a pre-flight size check.
func (objStore *StoreError) MaxObjectSize(encrypted bool) int64 {
	return math.MaxInt64
}

// InitEncryption: no-op. The error store carries no real configuration and never reaches
// encrypted I/O paths; tests that combine encryption with this store should set up a fake
// store with the desired KEK state directly.
func (objStore *StoreError) InitEncryption(opts EncryptionInitOptions) error {
	return nil
}
