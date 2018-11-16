package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"context"
	"errors"
	"fmt"
)

// this object store is used only for when we have errors due to an unknown store being supplied. Beside satisfying
// signatures this store is useless
type StoreError struct {
	storeType string
}

func InitialiseStoreError (ctx context.Context, backupConfig config.Backup, storeType string) (*StoreError) {
	result := &StoreError{
		storeType: storeType,
	}
	// actual backends will also setup the connection client in this section
	return result
}

func (object *StoreError) Upload (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, errors.New(fmt.Sprintf("unsupported backend of type: '%s'", object.storeType))
}

func (object *StoreError) MetadataUpdate (path string, newDbRecord shared.BackedUpFileProperties)  (result string, cancelled bool, err error) {
	return "", false, errors.New(fmt.Sprintf("unsupported backend of type: '%s'", object.storeType))
}
