package backup

import (
	"context"
	"strings"
	"testing"

	"cloudbackup/cbcrypto"
	"cloudbackup/objectstore"
	"cloudbackup/shared"
)

// fakeStore is a minimal ObjectStore for unit-testing pre-upload gating
// logic without invoking real cloud SDKs. Only MaxObjectSize and
// GetStoreDetails are consulted by the code under test; the I/O methods are
// stubs that panic if called.
type fakeStore struct {
	name         string
	plainMax     int64
	encryptedMax int64
}

func (f *fakeStore) Upload(shared.BackedUpFileProperties, int64, shared.BackupJobsStateInterface, bool) (string, bool, error) {
	panic("fakeStore.Upload should not be called from preUploadSizeCheck tests")
}
func (f *fakeStore) GetStoreDetails() (string, string) { return f.name, "fake" }
func (f *fakeStore) MarkDeleted(shared.BackedUpFileProperties, int64, bool) (string, bool, error) {
	panic("unused")
}
func (f *fakeStore) Delete(shared.BackedUpFileProperties, int64, string, bool) error { panic("unused") }
func (f *fakeStore) Get(shared.BackedUpFileProperties, string, int64, string, bool) (bool, error) {
	panic("unused")
}
func (f *fakeStore) Validate() (string, error) { return "ok", nil }
func (f *fakeStore) MaxObjectSize(encrypted bool) int64 {
	if encrypted {
		return f.encryptedMax
	}
	return f.plainMax
}
func (f *fakeStore) InitEncryption(objectstore.EncryptionInitOptions) error { return nil }

var _ objectstore.ObjectStore = (*fakeStore)(nil)
var _ = context.Background // keep import even if other tests don't use ctx

func TestPreUploadSizeCheck_FileWithinLimit(t *testing.T) {
	store := &fakeStore{name: "t1", plainMax: 1 << 20, encryptedMax: 1 << 20}
	cfg := shared.ConfigBackup{Encrypt: false}
	rec := shared.BackedUpFileProperties{Type: "file", Size: 1024}
	if got := preUploadSizeCheck(rec, cfg, store); got.Skip() {
		t.Fatalf("expected no skip, got %+v", got)
	}
}

func TestPreUploadSizeCheck_PlaintextOverLimit(t *testing.T) {
	store := &fakeStore{name: "tiny", plainMax: 1000, encryptedMax: 1000}
	cfg := shared.ConfigBackup{Encrypt: false}
	rec := shared.BackedUpFileProperties{Type: "file", Size: 5000, Path: "/x/big.bin"}
	got := preUploadSizeCheck(rec, cfg, store)
	if !got.Skip() {
		t.Fatalf("expected skip, got %+v", got)
	}
	if got.CounterName != "skipped_too_large_for_target" {
		t.Errorf("CounterName = %q, want skipped_too_large_for_target", got.CounterName)
	}
	if !strings.Contains(got.Message, "tiny") {
		t.Errorf("Message should mention target name %q, got: %s", "tiny", got.Message)
	}
}

func TestPreUploadSizeCheck_EncryptionInflatesSize(t *testing.T) {
	// Plaintext just under the encrypted cap, but EncryptedSize tips it
	// over because of header + chunk tags.
	plain := int64(1000)
	encrypted := cbcrypto.EncryptedSize(plain)
	store := &fakeStore{name: "t", plainMax: encrypted * 10, encryptedMax: encrypted - 1}
	cfg := shared.ConfigBackup{Encrypt: true}
	rec := shared.BackedUpFileProperties{Type: "file", Size: plain, Path: "/x/y"}
	got := preUploadSizeCheck(rec, cfg, store)
	if !got.Skip() {
		t.Fatalf("expected skip when encrypted size exceeds encrypted cap, got %+v", got)
	}
}

func TestPreUploadSizeCheck_EncryptionUnderLimit(t *testing.T) {
	plain := int64(1000)
	encrypted := cbcrypto.EncryptedSize(plain)
	store := &fakeStore{name: "t", plainMax: 0, encryptedMax: encrypted + 1}
	cfg := shared.ConfigBackup{Encrypt: true}
	rec := shared.BackedUpFileProperties{Type: "file", Size: plain}
	if got := preUploadSizeCheck(rec, cfg, store); got.Skip() {
		t.Fatalf("expected no skip when encrypted size <= encrypted cap, got %+v", got)
	}
}

func TestPreUploadSizeCheck_NonFileNeverSkipped(t *testing.T) {
	store := &fakeStore{name: "t", plainMax: 0, encryptedMax: 0}
	cfg := shared.ConfigBackup{Encrypt: true}
	// directory and symlink records carry no payload — never gated by size.
	for _, kind := range []string{"directory", "symlink"} {
		rec := shared.BackedUpFileProperties{Type: kind, Size: 999999999}
		if got := preUploadSizeCheck(rec, cfg, store); got.Skip() {
			t.Errorf("type %q got unexpected skip: %+v", kind, got)
		}
	}
}

func TestPreUploadSizeCheck_SkipEncryptionUsesPlaintextLimit(t *testing.T) {
	// File that would be too big under the encrypted cap, but well within
	// the plaintext cap. With SkipEncryption=true the upload is plaintext
	// even when backupConfig.Encrypt is true, so the plaintext cap applies.
	plain := int64(1 << 20) // 1 MiB
	store := &fakeStore{
		name:         "t",
		plainMax:     plain * 10,                        // plenty of room
		encryptedMax: cbcrypto.EncryptedSize(plain) - 1, // tight if encryption applied
	}
	cfg := shared.ConfigBackup{Encrypt: true}
	rec := shared.BackedUpFileProperties{Type: "file", Size: plain, SkipEncryption: true}
	if got := preUploadSizeCheck(rec, cfg, store); got.Skip() {
		t.Fatalf("SkipEncryption=true should bypass encrypted cap, got skip %+v", got)
	}
	// Sanity: same record without SkipEncryption should skip.
	rec.SkipEncryption = false
	if got := preUploadSizeCheck(rec, cfg, store); !got.Skip() {
		t.Fatalf("SkipEncryption=false with encrypted cap exceeded should skip, got proceed")
	}
}

func TestPreUploadReservedPathCheck(t *testing.T) {
	cases := []struct {
		path     string
		wantSkip bool
	}{
		{"/etc/hosts", false},
		{"/home/user/photos/img.jpg", false},
		{"/.cbcrypt", true}, // root-level reserved dir
		{".cbcrypt", true},  // bare reserved name
		{"/.cbcrypt/keystore.v1.yaml", true},
		{"/home/x/.cbcrypt/y", true},    // nested reserved component
		{"/var/data/.cbcrypt", true},    // reserved leaf dir
		{"/.cbcryptish/file", false},    // similar but not reserved
		{"/notreserved/cbcrypt", false}, // missing leading dot
		{"/home/.CBCRYPT/case", false},  // case-sensitive
	}
	for _, c := range cases {
		got := preUploadReservedPathCheck(shared.BackedUpFileProperties{Path: c.path})
		if got.Skip() != c.wantSkip {
			t.Errorf("path %q: got skip=%v, want %v (outcome=%+v)", c.path, got.Skip(), c.wantSkip, got)
		}
		if c.wantSkip && got.CounterName != "skipped_reserved_path" {
			t.Errorf("path %q: CounterName=%q, want skipped_reserved_path", c.path, got.CounterName)
		}
	}
}
