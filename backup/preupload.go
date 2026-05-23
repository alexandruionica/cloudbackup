package backup

import (
	"fmt"
	"path/filepath"
	"strings"

	"cloudbackup/cbcrypto"
	"cloudbackup/cbcrypto/keystore"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
)

// preUploadCheckOutcome describes the result of per-target gating performed
// before invoking ObjectStore.Upload. An empty CounterName means "proceed".
type preUploadCheckOutcome struct {
	// CounterName is the BackupJobsState counter to bump when skipping.
	// Empty when the file should be uploaded.
	CounterName string
	// Message is a human-readable explanation suitable for logs and the
	// error column of the counter event.
	Message string
}

func (o preUploadCheckOutcome) Skip() bool { return o.CounterName != "" }

// preUploadSizeCheck refuses files whose ciphertext (or plaintext, when
// encryption is off) would exceed the target's MaxObjectSize. The bound is
// per-target so a file too big for S3+encryption may still ship to Azure or
// GCP in the same job.
//
// Skipping at this layer is distinct from a transfer failure: it leaves no
// remote_files row, contributes no rollback work, and does not abort other
// targets in the loop.
func preUploadSizeCheck(DbRecord shared.BackedUpFileProperties, backupConfig shared.ConfigBackup, objStore objectstore.ObjectStore) preUploadCheckOutcome {
	if DbRecord.Type != "file" {
		return preUploadCheckOutcome{}
	}
	// Internal-state uploads bypass encryption (and therefore the encrypted
	// size cap) regardless of the backup config's encrypt flag.
	encrypted := backupConfig.Encrypt && !DbRecord.SkipEncryption
	wireSize := DbRecord.Size
	if encrypted {
		wireSize = cbcrypto.EncryptedSize(DbRecord.Size)
	}
	maxSize := objStore.MaxObjectSize(encrypted)
	if wireSize <= maxSize {
		return preUploadCheckOutcome{}
	}
	targetName, _ := objStore.GetStoreDetails()
	return preUploadCheckOutcome{
		CounterName: "skipped_too_large_for_target",
		Message: fmt.Sprintf(
			"plaintext size %d (on-the-wire size %d with encrypted=%t) exceeds target '%s' MaxObjectSize %d",
			DbRecord.Size, wireSize, encrypted, targetName, maxSize),
	}
}

// preUploadReservedPathCheck refuses any local path whose name components
// include the keystore-reserved segment (e.g. "/some/.cbcrypt/file"). The
// keystore sidecar lives under <storePrefix>/.cbcrypt/ in every encryption-
// enabled target's bucket; refusing reserved-segment paths protects against
// silent collisions if the on-bucket layout ever changes, and against
// pathological backup configs (e.g. a backup job literally named ".cbcrypt").
//
// Defense-in-depth: under the current "<storePrefix>/data|metadata/<localPath>"
// layout, no natural file mapping reaches the reserved namespace, so this
// check is expected to be silent in practice.
func preUploadReservedPathCheck(DbRecord shared.BackedUpFileProperties) preUploadCheckOutcome {
	// Normalise to forward slashes so the same check holds on Windows.
	parts := strings.Split(filepath.ToSlash(DbRecord.Path), "/")
	for _, p := range parts {
		if p == keystore.ReservedPathSegment {
			return preUploadCheckOutcome{
				CounterName: "skipped_reserved_path",
				Message: fmt.Sprintf(
					"path %q contains reserved segment %q which is used by cloudbackup for client-side encryption metadata",
					DbRecord.Path, keystore.ReservedPathSegment),
			}
		}
	}
	return preUploadCheckOutcome{}
}
