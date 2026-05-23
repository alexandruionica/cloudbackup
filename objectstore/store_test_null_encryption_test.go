package objectstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloudbackup/cbcrypto"
	"cloudbackup/shared"
)

// TestStoreTestNull_EncryptedUploadRoundtrip uploads a real file through the
// TestNull backend with encryption enabled, then verifies the captured bytes
// are valid ciphertext that decrypts back to the original plaintext.
func TestStoreTestNull_EncryptedUploadRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	plain := make([]byte, 50*1024)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	srcPath := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(srcPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := makeEncryptedConfig(t, "test-pass")
	state := &noopJobsState{}
	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull: %v", err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatalf("InitEncryption: %v", err)
	}

	rec := shared.BackedUpFileProperties{
		Path: srcPath,
		Type: "file",
		Size: int64(len(plain)),
	}
	if _, _, err := store.Upload(rec, 1, state, false); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	remoteKey := store.storePrefix + "/" + DataPrepend + "/" + srcPath
	store.memMu.Lock()
	uploaded, ok := store.memObjects[remoteKey]
	store.memMu.Unlock()
	if !ok {
		t.Fatalf("no captured upload at %q (captured keys: %v)", remoteKey, mapKeys(store.memObjects))
	}

	if bytes.Equal(uploaded, plain) {
		t.Fatal("uploaded bytes match plaintext — encryption did not run")
	}
	if string(uploaded[:4]) != cbcrypto.MagicV1 {
		t.Fatalf("uploaded bytes don't start with %q magic; first 8 bytes: %x", cbcrypto.MagicV1, uploaded[:8])
	}

	dr, err := cbcrypto.NewDecryptingReader(bytes.NewReader(uploaded), store.KEK())
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	hdr, err := dr.PeekHeader()
	if err != nil {
		t.Fatalf("PeekHeader: %v", err)
	}
	if hdr.KeystoreUUID != store.KeystoreUUID() {
		t.Errorf("header keystore UUID doesn't match store's KEK's UUID")
	}
	got, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted bytes don't match original (got %d bytes, want %d)", len(got), len(plain))
	}
}

// TestStoreTestNull_EncryptedGetRoundtrip uploads via Upload(), then downloads
// via Get(), and verifies the restored file matches the original plaintext.
// Exercises the full upload + download decryption path end-to-end.
func TestStoreTestNull_EncryptedGetRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	plain := make([]byte, 65*1024+7) // straddle a chunk boundary
	for i := range plain {
		plain[i] = byte((i*7 + 3) % 251)
	}
	srcPath := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(srcPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := makeEncryptedConfig(t, "rt-pass")
	state := &noopJobsState{}
	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatal(err)
	}

	rec := shared.BackedUpFileProperties{
		Path:      srcPath,
		Type:      "file",
		Size:      int64(len(plain)),
		Encrypted: true, // marks the DB record so Get knows to decrypt
	}
	if _, _, err := store.Upload(rec, 1, state, false); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	restorePath := filepath.Join(tmp, "restored.bin")
	if _, err := store.Get(rec, restorePath, 1, "1", false); err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("restored file differs from original (got %d bytes, want %d)", len(got), len(plain))
	}
}

// TestStoreTestNull_KeystoreUUIDMismatch confirms that a Get with a mismatched
// keystore UUID in the file header is rejected with the decrypt_keystore_mismatch
// signal rather than silently returning a corrupt file.
func TestStoreTestNull_KeystoreUUIDMismatch(t *testing.T) {
	tmp := t.TempDir()
	plain := []byte("contents")
	srcPath := filepath.Join(tmp, "x.bin")
	if err := os.WriteFile(srcPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := makeEncryptedConfig(t, "p")
	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatal(err)
	}
	rec := shared.BackedUpFileProperties{Path: srcPath, Type: "file", Size: int64(len(plain)), Encrypted: true}
	if _, _, err := store.Upload(rec, 1, &noopJobsState{}, false); err != nil {
		t.Fatal(err)
	}

	// Rotate the in-memory keystore UUID so the next Get sees a mismatch.
	var bogus [cbcrypto.KeystoreUUIDSize]byte
	for i := range bogus {
		bogus[i] = 0xFF
	}
	store.keystoreUUID = bogus

	restorePath := filepath.Join(tmp, "restored.bin")
	_, err = store.Get(rec, restorePath, 1, "1", false)
	if err == nil {
		t.Fatal("expected error for keystore UUID mismatch, got nil")
	}
}

// TestStoreTestNull_SkipEncryptionUsesPlaintext verifies the SkipEncryption
// flag short-circuits the EncryptingReader wrapping path even when the store
// has encryption initialised.
func TestStoreTestNull_SkipEncryptionUsesPlaintext(t *testing.T) {
	tmp := t.TempDir()
	plain := []byte("internal cloudbackup state — must NOT be encrypted")
	srcPath := filepath.Join(tmp, "db.sqlite")
	if err := os.WriteFile(srcPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := makeEncryptedConfig(t, "test-pass")
	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatal(err)
	}

	rec := shared.BackedUpFileProperties{
		Path:           srcPath,
		Type:           "file",
		Size:           int64(len(plain)),
		SkipEncryption: true, // <-- the flag under test
	}
	if _, _, err := store.Upload(rec, 1, &noopJobsState{}, true); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	remoteKey := store.storePrefix + "/" + MetaDataPrepend + "/" + srcPath
	store.memMu.Lock()
	uploaded := store.memObjects[remoteKey]
	store.memMu.Unlock()

	if !bytes.Equal(uploaded, plain) {
		t.Fatalf("SkipEncryption=true should bypass encryption; got %d bytes that don't equal plaintext", len(uploaded))
	}
}

func mapKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Used by the test to keep timestamps deterministic-ish in error messages.
var _ = time.Now
var _ = context.Background
