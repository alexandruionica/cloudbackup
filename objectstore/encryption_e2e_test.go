package objectstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"cloudbackup/shared"
)

// TestEncryptionE2E_PlaintextThenEncryptedMigration walks a representative
// upgrade path: backup a file plaintext, then enable encryption and back up
// again, then restore. Confirms:
//   - the same TestNull store can be reused with a new encryption state on a
//     fresh instance,
//   - the post-encryption upload produces ciphertext (different bytes from
//     the plaintext upload),
//   - the restore path through Get() decrypts back to the original bytes.
//
// This covers the "config flag flip → re-upload as encrypted" scenario that
// the production code relies on via backup/needsUpload's flag-mismatch path.
func TestEncryptionE2E_PlaintextThenEncryptedMigration(t *testing.T) {
	tmp := t.TempDir()
	plain := []byte("the quick brown fox jumps over the lazy dog (and again, more bytes to span a few blocks)")
	for i := 0; i < 200; i++ {
		plain = append(plain, byte(i))
	}
	srcPath := filepath.Join(tmp, "data.bin")
	if err := os.WriteFile(srcPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}

	// Phase 1: plaintext backup.
	ctx := context.Background()
	plainCfg := shared.ConfigBackup{
		Name:    "test-job",
		Encrypt: false,
		Target: []shared.ConfigBackupTarget{
			{Name: "t1", Type: "test_null", Prefix: "prefix"},
		},
	}
	state := &noopJobsState{}
	plainStore, err := InitialiseStoreTestNull(ctx, plainCfg, plainCfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := plainStore.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatal(err)
	}
	plainRec := shared.BackedUpFileProperties{Path: srcPath, Type: "file", Size: int64(len(plain))}
	if _, _, err := plainStore.Upload(plainRec, 1, state, false); err != nil {
		t.Fatalf("plaintext Upload: %v", err)
	}
	plainKey := plainStore.storePrefix + "/" + DataPrepend + "/" + srcPath
	plainStore.memMu.Lock()
	plainCaptured := plainStore.memObjects[plainKey]
	plainStore.memMu.Unlock()
	if !bytes.Equal(plainCaptured, plain) {
		t.Fatal("plaintext upload should produce bytes identical to the source file")
	}

	// Phase 2: same backup job switches to encryption — a fresh store
	// instance picks up the new config and bootstraps a keystore.
	encCfg := shared.ConfigBackup{
		Name:        "test-job",
		Encrypt:     true,
		EncryptPass: "migration-pass",
		Target:      plainCfg.Target,
	}
	encStore, err := InitialiseStoreTestNull(ctx, encCfg, encCfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := encStore.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatalf("InitEncryption (post-migration): %v", err)
	}
	// Same record but marked as encrypted now (mirroring what backup.go does
	// once the config flag flips and PrepareFileRecord sets Encrypted=true).
	encRec := plainRec
	encRec.Encrypted = true
	if _, _, err := encStore.Upload(encRec, 2, state, false); err != nil {
		t.Fatalf("encrypted Upload: %v", err)
	}
	encStore.memMu.Lock()
	encCaptured := encStore.memObjects[plainKey]
	encStore.memMu.Unlock()
	if bytes.Equal(encCaptured, plain) {
		t.Fatal("encrypted upload should NOT produce plaintext bytes")
	}

	// Phase 3: restore via Get on the encrypted store.
	restorePath := filepath.Join(tmp, "restored.bin")
	if _, err := encStore.Get(encRec, restorePath, 2, "2", false); err != nil {
		t.Fatalf("Get: %v", err)
	}
	restored, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, plain) {
		t.Fatal("restored bytes differ from original plaintext")
	}
}

// TestEncryptionE2E_SkipEncryptionAlongsideEncrypted exercises the mixed-
// stream case that arises every backup run: user content goes through the
// EncryptingReader, but the DB-copy and config-copy use SkipEncryption=true
// and land plaintext in the same bucket. Both must be retrievable from the
// same store with the same configuration.
func TestEncryptionE2E_SkipEncryptionAlongsideEncrypted(t *testing.T) {
	tmp := t.TempDir()
	state := &noopJobsState{}
	cfg := shared.ConfigBackup{
		Name:        "test-job",
		Encrypt:     true,
		EncryptPass: "mixed-pass",
		Target: []shared.ConfigBackupTarget{
			{Name: "t1", Type: "test_null", Prefix: "prefix"},
		},
	}
	store, err := InitialiseStoreTestNull(context.Background(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatal(err)
	}

	// User file: encrypted.
	userPath := filepath.Join(tmp, "user.txt")
	userContent := []byte("user data must be encrypted at rest")
	if err := os.WriteFile(userPath, userContent, 0o600); err != nil {
		t.Fatal(err)
	}
	userRec := shared.BackedUpFileProperties{Path: userPath, Type: "file", Size: int64(len(userContent)), Encrypted: true}
	if _, _, err := store.Upload(userRec, 1, state, false); err != nil {
		t.Fatal(err)
	}

	// Internal state file: plaintext (SkipEncryption=true).
	dbPath := filepath.Join(tmp, "metadata.sqlite")
	dbContent := []byte("internal cloudbackup state — readable from bucket")
	if err := os.WriteFile(dbPath, dbContent, 0o600); err != nil {
		t.Fatal(err)
	}
	dbRec := shared.BackedUpFileProperties{Path: dbPath, Type: "file", Size: int64(len(dbContent)), SkipEncryption: true, Encrypted: false}
	if _, _, err := store.Upload(dbRec, 1, state, true); err != nil {
		t.Fatal(err)
	}

	// Read both back from in-memory store and verify their states.
	userKey := store.storePrefix + "/" + DataPrepend + "/" + userPath
	dbKey := store.storePrefix + "/" + MetaDataPrepend + "/" + dbPath
	store.memMu.Lock()
	uploadedUser := store.memObjects[userKey]
	uploadedDB := store.memObjects[dbKey]
	store.memMu.Unlock()

	if bytes.Equal(uploadedUser, userContent) {
		t.Error("user file should NOT have been uploaded plaintext")
	}
	if !bytes.Equal(uploadedDB, dbContent) {
		t.Error("internal state file SHOULD have been uploaded plaintext")
	}

	// Round-trip the user file via Get.
	restoreUser := filepath.Join(tmp, "restored-user.txt")
	if _, err := store.Get(userRec, restoreUser, 1, "1", false); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(restoreUser)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, userContent) {
		t.Fatal("user file roundtrip failed")
	}

	// Round-trip the DB file via Get (plaintext, no decryption needed).
	restoreDB := filepath.Join(tmp, "restored-db.sqlite")
	if _, err := store.Get(dbRec, restoreDB, 1, "1", true); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := os.ReadFile(restoreDB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restoredDB, dbContent) {
		t.Fatal("DB file roundtrip failed")
	}
}

// guard that the imports above are referenced even if test bodies are pruned.
var _ = io.Discard
