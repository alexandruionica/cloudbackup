package dbops

import (
	"database/sql"
)

// HasAnyEncryptedFiles reports whether the local files table has any rows
// where encrypted = 1.
//
// Consulted by the keystore lifecycle (objectstore.InitEncryption) to detect
// "sidecar was lost on the bucket but the local DB still references files we
// previously uploaded as encrypted". Triggering a fresh sidecar bootstrap in
// that situation would silently orphan those existing encrypted objects, so
// we refuse to do so and surface the keystore_inconsistent counter.
func HasAnyEncryptedFiles(db *sql.DB) (bool, error) {
	var present int
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE encrypted = 1 LIMIT 1)").Scan(&present)
	if err != nil {
		return false, err
	}
	return present != 0, nil
}

// ResetEncryptedFlags clears the encrypted=1 marker from every row in the
// files table. Returns the number of affected rows.
//
// Used by `cloudbackup server reset-keystore <job>` after the operator has
// removed the keystore sidecar from the bucket. The next backup run will see
// an empty encrypted-files set in the DB, allow a fresh sidecar bootstrap,
// and re-upload all files under the new keystore via the existing
// flag-mismatch re-upload path (backup.go:594).
func ResetEncryptedFlags(db *sql.DB) (int64, error) {
	res, err := db.Exec("UPDATE files SET encrypted = 0 WHERE encrypted = 1")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
