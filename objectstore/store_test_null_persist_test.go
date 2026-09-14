package objectstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cloudbackup/shared"
)

// persistConfig returns a config whose single test_null target keeps objects under $dir.
func persistConfig(t *testing.T, dir string, password string) shared.ConfigBackup {
	t.Helper()
	cfg := makeEncryptedConfig(t, password)
	cfg.Target[0].Parameters = []shared.ConfigBackupTargetParams{{Name: "persist_dir", Value: dir}}
	return cfg
}

func writeSource(t *testing.T, dir string, name string, size int) (string, []byte) {
	t.Helper()
	plain := make([]byte, size)
	for i := range plain {
		plain[i] = byte((i * 7) % 253)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	return p, plain
}

// TestStoreTestNull_PersistDir_RoundTripAcrossInstances uploads with one store instance and
// downloads with a brand new one, which is exactly what the daemon does between a backup job and
// a later restore job.
func TestStoreTestNull_PersistDir_RoundTripAcrossInstances(t *testing.T) {
	tmp := t.TempDir()
	persist := filepath.Join(tmp, "objects")
	srcPath, plain := writeSource(t, tmp, "src.bin", 300*1024)
	cfg := persistConfig(t, persist, "")
	state := &noopJobsState{}

	up, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull: %v", err)
	}
	rec := shared.BackedUpFileProperties{Path: srcPath, Type: "file", Size: int64(len(plain))}
	if _, _, err := up.Upload(rec, 1, state, false); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(up.memObjects) != 0 {
		t.Fatalf("persistent store must not capture into memObjects, got %d entries", len(up.memObjects))
	}
	entries, err := os.ReadDir(persist)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one object file in persist_dir, found %d", len(entries))
	}

	down, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull (second instance): %v", err)
	}
	restorePath := filepath.Join(tmp, "restored.bin")
	if cancelled, err := down.Get(rec, restorePath, 1, "1", false); err != nil || cancelled {
		t.Fatalf("Get: cancelled=%v err=%v", cancelled, err)
	}
	got, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("restored bytes differ from source (%d vs %d bytes)", len(got), len(plain))
	}
}

// TestStoreTestNull_PersistDir_EncryptedRoundTripAcrossInstances covers the sidecar bootstrap on
// the first instance, sidecar rehydrate on the second, and ciphertext decrypting back to plaintext.
func TestStoreTestNull_PersistDir_EncryptedRoundTripAcrossInstances(t *testing.T) {
	tmp := t.TempDir()
	persist := filepath.Join(tmp, "objects")
	srcPath, plain := writeSource(t, tmp, "src.bin", 120*1024)
	cfg := persistConfig(t, persist, "hunter2-but-longer")
	state := &noopJobsState{}

	up, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull: %v", err)
	}
	if err := up.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatalf("InitEncryption (bootstrap): %v", err)
	}
	rec := shared.BackedUpFileProperties{Path: srcPath, Type: "file", Size: int64(len(plain)), Encrypted: true}
	if _, _, err := up.Upload(rec, 1, state, false); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// The bytes on "disk" must be ciphertext, not the plaintext.
	stored, found, err := up.blobGet(up.storePrefix + "/" + DataPrepend + "/" + srcPath)
	if err != nil || !found {
		t.Fatalf("blobGet: found=%v err=%v", found, err)
	}
	if bytes.Contains(stored, plain[:64]) {
		t.Fatal("persisted object contains plaintext; encryption was not applied")
	}

	down, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull (second instance): %v", err)
	}
	// AllowBootstrap=false: the second instance must find the sidecar written by the first.
	if err := down.InitEncryption(EncryptionInitOptions{AllowBootstrap: false}); err != nil {
		t.Fatalf("InitEncryption (rehydrate): %v", err)
	}
	if down.KeystoreUUID() != up.KeystoreUUID() {
		t.Fatalf("keystore UUID differs across instances: %x vs %x", down.KeystoreUUID(), up.KeystoreUUID())
	}
	restorePath := filepath.Join(tmp, "restored.bin")
	if cancelled, err := down.Get(rec, restorePath, 1, "1", false); err != nil || cancelled {
		t.Fatalf("Get: cancelled=%v err=%v", cancelled, err)
	}
	got, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("decrypted restore differs from source")
	}
}

// TestStoreTestNull_PersistDir_SidecarConflict proves the conditional PUT semantics survive the
// move to disk: a second bootstrap against the same persist_dir must be refused.
func TestStoreTestNull_PersistDir_SidecarConflict(t *testing.T) {
	persist := filepath.Join(t.TempDir(), "objects")
	cfg := persistConfig(t, persist, "pw")
	a, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatal(err)
	}
	key := sidecarBucketKey(a.storePrefix)
	if err := a.blobPutIfNotExists(key, []byte("first")); err != nil {
		t.Fatalf("first conditional put: %v", err)
	}
	if err := b.blobPutIfNotExists(key, []byte("second")); !errors.Is(err, errSidecarConflict) {
		t.Fatalf("second conditional put: want errSidecarConflict, got %v", err)
	}
	body, found, err := b.blobGet(key)
	if err != nil || !found || string(body) != "first" {
		t.Fatalf("sidecar after conflict: body=%q found=%v err=%v", body, found, err)
	}
}

// TestStoreTestNull_PersistDir_OverwriteAndMissing: a re-upload replaces the object in place and
// Get on a never-uploaded key stays a silent no-op, matching the in-memory behaviour.
func TestStoreTestNull_PersistDir_OverwriteAndMissing(t *testing.T) {
	tmp := t.TempDir()
	persist := filepath.Join(tmp, "objects")
	cfg := persistConfig(t, persist, "")
	state := &noopJobsState{}
	st, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatal(err)
	}
	srcPath, _ := writeSource(t, tmp, "src.bin", 1024)
	rec := shared.BackedUpFileProperties{Path: srcPath, Type: "file", Size: 1024}
	if _, _, err := st.Upload(rec, 1, state, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcPath, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec.Size = 2
	if _, _, err := st.Upload(rec, 2, state, false); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "out.bin")
	if _, err := st.Get(rec, out, 2, "2", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "v2" {
		t.Fatalf("expected overwritten object, got %q", got)
	}
	missing := shared.BackedUpFileProperties{Path: filepath.Join(tmp, "never-uploaded"), Type: "file", Size: 1}
	if _, err := st.Get(missing, filepath.Join(tmp, "unused"), 1, "1", false); err != nil {
		t.Fatalf("Get of a missing key must be a no-op, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "unused")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Get of a missing key must not create the restore file")
	}
}

// TestStoreTestNull_PersistDir_CreatedOnInit: the directory is created by the constructor so a
// config can point at a not-yet-existing path.
func TestStoreTestNull_PersistDir_CreatedOnInit(t *testing.T) {
	persist := filepath.Join(t.TempDir(), "a", "b", "objects")
	cfg := persistConfig(t, persist, "")
	if _, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(persist); err != nil || !fi.IsDir() {
		t.Fatalf("persist_dir not created: %v", err)
	}
}
