package objectstore

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"cloudbackup/cbcrypto"
	"cloudbackup/cbcrypto/keystore"
	"cloudbackup/shared"
)

// noopJobsState satisfies shared.BackupJobsStateInterface with no-op methods,
// for use in encryption tests that don't care about counter accounting.
type noopJobsState struct{}

func (noopJobsState) AddBytesRead(string, uint64)                                            {}
func (noopJobsState) IncrementCounter(string, string, string, string, string, string)        {}
func (noopJobsState) IncrementRateCounter(string, string, string, int64, string, uint, bool) {}
func (noopJobsState) IncrementSequence(string)                                               {}
func (noopJobsState) UpdateStatsText(string, string, string, string, string)                 {}

var _ shared.BackupJobsStateInterface = (*noopJobsState)(nil)

// fakeSidecarIO is a thread-safe in-memory sidecarIO used to exercise the
// shared lifecycle without invoking real cloud SDKs.
type fakeSidecarIO struct {
	mu        sync.Mutex
	body      []byte
	exists    bool
	fetchErr  error
	putErr    error
	fetchHits int
	putHits   int
}

func (f *fakeSidecarIO) Fetch() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchHits++
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	if !f.exists {
		return nil, errSidecarNotFound
	}
	out := make([]byte, len(f.body))
	copy(out, f.body)
	return out, nil
}

func (f *fakeSidecarIO) PutIfNotExists(body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putHits++
	if f.putErr != nil {
		return f.putErr
	}
	if f.exists {
		return errSidecarConflict
	}
	f.body = append([]byte(nil), body...)
	f.exists = true
	return nil
}

// fastParams keeps unit tests responsive without changing the production
// argon2 defaults baked into cbcrypto.DefaultKDFParams.
var fastParams = cbcrypto.KDFParams{Time: 1, Memory: 8 * 1024, Threads: 1}

// makeExistingSidecar primes a fakeSidecarIO with a real sidecar generated
// under password+fastParams. Returns the expected KEK for assertions.
func makeExistingSidecar(t *testing.T, password []byte) (*fakeSidecarIO, []byte) {
	t.Helper()
	sc, kek, err := keystore.NewWithParams(password, fastParams)
	if err != nil {
		t.Fatalf("NewWithParams: %v", err)
	}
	body, err := sc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return &fakeSidecarIO{body: body, exists: true}, kek
}

func TestRunEncryptionInit_ExistingSidecarCorrectPassword(t *testing.T) {
	password := []byte("right-password")
	io, _ := makeExistingSidecar(t, password)
	kek, sc, err := runEncryptionInit(io, password, EncryptionInitOptions{AllowBootstrap: true}, nil)
	if err != nil {
		t.Fatalf("runEncryptionInit: %v", err)
	}
	if len(kek) != cbcrypto.KEKSize {
		t.Errorf("KEK length %d, want %d", len(kek), cbcrypto.KEKSize)
	}
	if sc == nil {
		t.Fatal("got nil sidecar")
	}
}

func TestRunEncryptionInit_ExistingSidecarWrongPassword(t *testing.T) {
	io, _ := makeExistingSidecar(t, []byte("right"))
	_, _, err := runEncryptionInit(io, []byte("wrong"), EncryptionInitOptions{AllowBootstrap: true}, nil)
	if err == nil {
		t.Fatal("expected error with wrong password, got nil")
	}
	if !errors.Is(err, keystore.ErrWrongPassword) {
		t.Fatalf("expected error chain to contain ErrWrongPassword, got: %v", err)
	}
}

func TestRunEncryptionInit_MissingSidecarBootstrap(t *testing.T) {
	io := &fakeSidecarIO{}
	password := []byte("brand-new")
	kek, sc, err := runEncryptionInit(io, password, EncryptionInitOptions{AllowBootstrap: true, HasEncryptedFiles: false}, nil)
	if err != nil {
		t.Fatalf("runEncryptionInit (bootstrap): %v", err)
	}
	if len(kek) != cbcrypto.KEKSize {
		t.Fatalf("KEK length %d, want %d", len(kek), cbcrypto.KEKSize)
	}
	if !io.exists {
		t.Fatal("sidecar was not persisted after bootstrap")
	}
	// Verify the persisted sidecar would decrypt with the same password.
	if _, err := sc.DeriveAndVerify(password); err != nil {
		t.Errorf("DeriveAndVerify on bootstrapped sidecar: %v", err)
	}
}

func TestRunEncryptionInit_MissingSidecarDBHasEncrypted(t *testing.T) {
	io := &fakeSidecarIO{}
	var capturedName, capturedMsg string
	bump := func(name, msg string) {
		capturedName = name
		capturedMsg = msg
	}
	_, _, err := runEncryptionInit(io, []byte("p"), EncryptionInitOptions{AllowBootstrap: true, HasEncryptedFiles: true}, bump)
	if err == nil {
		t.Fatal("expected error for keystore_inconsistent, got nil")
	}
	if !errors.Is(err, ErrKeystoreInconsistent) {
		t.Errorf("expected ErrKeystoreInconsistent in chain, got: %v", err)
	}
	if capturedName != "keystore_inconsistent" {
		t.Errorf("counter name = %q, want keystore_inconsistent", capturedName)
	}
	if !strings.Contains(capturedMsg, "reset-keystore") {
		t.Errorf("counter message should mention reset-keystore, got: %s", capturedMsg)
	}
	if io.exists {
		t.Error("sidecar must NOT have been created when DB has encrypted records")
	}
}

func TestRunEncryptionInit_MissingSidecarNoBootstrap(t *testing.T) {
	io := &fakeSidecarIO{}
	_, _, err := runEncryptionInit(io, []byte("p"), EncryptionInitOptions{AllowBootstrap: false}, nil)
	if !errors.Is(err, ErrSidecarMissingForRestore) {
		t.Fatalf("expected ErrSidecarMissingForRestore, got: %v", err)
	}
	if io.exists {
		t.Error("sidecar must NOT have been created when AllowBootstrap=false")
	}
}

func TestRunEncryptionInit_ConflictAdoptsWinner(t *testing.T) {
	// Pre-existing sidecar with the same password — emulates the case where
	// another daemon won the conditional-PUT race a moment ago.
	io, expectedKEK := makeExistingSidecar(t, []byte("shared-pass"))

	kek, sc, err := runEncryptionInit(io, []byte("shared-pass"), EncryptionInitOptions{AllowBootstrap: true, HasEncryptedFiles: false}, nil)
	if err != nil {
		t.Fatalf("runEncryptionInit (existing sidecar): %v", err)
	}
	if len(kek) != len(expectedKEK) {
		t.Errorf("KEK length mismatch")
	}
	if sc == nil {
		t.Fatal("sidecar nil")
	}
	// Should have fetched at least once and not written anything.
	if io.fetchHits == 0 {
		t.Error("expected at least one Fetch call")
	}
	if io.putHits != 0 {
		t.Errorf("expected zero PutIfNotExists calls when sidecar already exists, got %d", io.putHits)
	}
}

func TestRunEncryptionInit_RaceMidBootstrap(t *testing.T) {
	// Simulate: Fetch returns not-found (sidecar truly absent at start),
	// but by the time we PUT, another writer has created it.
	// We do this by pre-loading a sidecar body, then forcing exists=false at
	// fetch time, then having PUT fail with errSidecarConflict.
	password := []byte("p")
	priorSc, priorKEK, err := keystore.NewWithParams(password, fastParams)
	if err != nil {
		t.Fatal(err)
	}
	priorBytes, err := priorSc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	io := &fakeSidecarIOSeq{
		fetchResults:    [][]byte{nil, priorBytes}, // first fetch: not-found; second fetch (after PUT conflict): winner's bytes
		fetchErrs:       []error{errSidecarNotFound, nil},
		putReturnsError: errSidecarConflict,
	}
	kek, _, err := runEncryptionInit(io, password, EncryptionInitOptions{AllowBootstrap: true, HasEncryptedFiles: false}, nil)
	if err != nil {
		t.Fatalf("runEncryptionInit on race: %v", err)
	}
	if len(kek) != len(priorKEK) {
		t.Errorf("adopted KEK length mismatch")
	}
}

func TestRunEncryptionInit_EmptyPassword(t *testing.T) {
	io := &fakeSidecarIO{}
	_, _, err := runEncryptionInit(io, nil, EncryptionInitOptions{AllowBootstrap: true}, nil)
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

// fakeSidecarIOSeq returns a different result on successive Fetch calls,
// for exercising the race-adoption path which fetches twice.
type fakeSidecarIOSeq struct {
	mu              sync.Mutex
	fetchResults    [][]byte
	fetchErrs       []error
	fetchCalls      int
	putReturnsError error
}

func (f *fakeSidecarIOSeq) Fetch() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.fetchCalls
	f.fetchCalls++
	if i >= len(f.fetchResults) {
		return nil, errSidecarNotFound
	}
	return f.fetchResults[i], f.fetchErrs[i]
}

func (f *fakeSidecarIOSeq) PutIfNotExists(body []byte) error {
	return f.putReturnsError
}

func TestSidecarBucketKey(t *testing.T) {
	got := sidecarBucketKey("backups/job-x")
	want := "backups/job-x/.cbcrypt/keystore.v1.yaml"
	if got != want {
		t.Errorf("sidecarBucketKey = %q, want %q", got, want)
	}
}

// End-to-end exercise of InitEncryption through StoreTestNull using its
// in-memory sidecarIO. Confirms the public method wires shared lifecycle
// correctly and that the KEK is cached on the embedded encryptionState.
func TestStoreTestNull_InitEncryption_BootstrapThenRehydrate(t *testing.T) {
	cfg := makeEncryptedConfig(t, "password-one")
	state := &noopJobsState{}

	// First store: empty bucket, allow bootstrap, no prior encrypted files.
	store1, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull #1: %v", err)
	}
	if err := store1.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatalf("InitEncryption (bootstrap): %v", err)
	}
	if !store1.EncryptionReady() {
		t.Fatal("store should report EncryptionReady after successful init")
	}
	kek1 := store1.KEK()
	if len(kek1) == 0 {
		t.Fatal("KEK should be populated after init")
	}
	uuid1 := store1.KeystoreUUID()

	// Second store sharing the same sidecar bytes (simulating two daemons
	// pointing at the same bucket): InitEncryption should find the existing
	// sidecar and derive the same KEK.
	store2, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", state)
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull #2: %v", err)
	}
	// Copy the sidecar from store1 to store2 so they share the same "bucket".
	store2.memObjects[sidecarBucketKey(store2.storePrefix)] = append([]byte(nil), store1.memObjects[sidecarBucketKey(store1.storePrefix)]...)

	if err := store2.InitEncryption(EncryptionInitOptions{AllowBootstrap: false}); err != nil {
		t.Fatalf("InitEncryption (rehydrate): %v", err)
	}
	kek2 := store2.KEK()
	if string(kek1) != string(kek2) {
		t.Fatal("rehydrated KEK differs from bootstrap KEK")
	}
	if store1.KeystoreUUID() != uuid1 || store2.KeystoreUUID() != uuid1 {
		t.Fatal("keystore UUID drifted between bootstrap and rehydrate")
	}
}

func TestStoreTestNull_InitEncryption_DisabledIsNoOp(t *testing.T) {
	cfg := makeEncryptedConfig(t, "")
	cfg.Encrypt = false
	cfg.EncryptPass = ""

	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull: %v", err)
	}
	if err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true}); err != nil {
		t.Fatalf("InitEncryption with encryption disabled should be a no-op, got: %v", err)
	}
	if !store.EncryptionReady() {
		t.Fatal("EncryptionReady should be true even when encryption is disabled")
	}
	if store.EncryptionEnabled() {
		t.Fatal("EncryptionEnabled should be false")
	}
	if store.KEK() != nil {
		t.Fatal("KEK should be nil when encryption disabled")
	}
}

func TestStoreTestNull_InitEncryption_RestoreFailsOnMissing(t *testing.T) {
	cfg := makeEncryptedConfig(t, "p")
	store, err := InitialiseStoreTestNull(t.Context(), cfg, cfg.Target[0], "0", &noopJobsState{})
	if err != nil {
		t.Fatalf("InitialiseStoreTestNull: %v", err)
	}
	err = store.InitEncryption(EncryptionInitOptions{AllowBootstrap: false})
	if !errors.Is(err, ErrSidecarMissingForRestore) {
		t.Fatalf("expected ErrSidecarMissingForRestore for missing sidecar in restore mode, got: %v", err)
	}
}

func makeEncryptedConfig(t *testing.T, password string) shared.ConfigBackup {
	t.Helper()
	return shared.ConfigBackup{
		Name:        "test-job",
		Encrypt:     password != "",
		EncryptPass: password,
		Target: []shared.ConfigBackupTarget{
			{Name: "t1", Type: "test_null", Prefix: "prefix"},
		},
	}
}
