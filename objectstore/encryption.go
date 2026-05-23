package objectstore

import (
	"errors"
	"fmt"

	"cloudbackup/cbcrypto"
	"cloudbackup/cbcrypto/keystore"
)

// EncryptionInitOptions controls per-target keystore bring-up.
type EncryptionInitOptions struct {
	// HasEncryptedFiles reports whether the local SQLite DB already knows about
	// encrypted files for this target's backup job. Consulted only when the
	// sidecar is missing and AllowBootstrap is true: a true value blocks
	// bootstrap to avoid silently orphaning previously-encrypted data.
	HasEncryptedFiles bool

	// AllowBootstrap, when true, permits this call to CREATE a fresh sidecar
	// if none exists. Use true for backup (where new encrypted uploads need a
	// keystore); use false for restore and config-validate (which must never
	// produce write side-effects on the bucket).
	AllowBootstrap bool
}

// ErrSidecarMissingForRestore is returned by InitEncryption when the sidecar
// is missing and bootstrap was not permitted (typically: restore or
// validate-config). Callers should surface this with a clear "keystore not
// found" message so operators understand the bucket state.
var ErrSidecarMissingForRestore = errors.New("keystore sidecar missing; cannot decrypt without it")

// ErrKeystoreInconsistent is returned when the sidecar is missing but the
// local DB references encrypted files. Bumps the keystore_inconsistent
// counter. Recovery: restore the sidecar from a backup, or run
// `cloudbackup reset-keystore <target>` if accepting data loss.
var ErrKeystoreInconsistent = errors.New("keystore inconsistent: sidecar missing but local DB references encrypted files")

// errSidecarNotFound is the canonical "object does not exist" sentinel each
// backend's sidecarIO.Fetch must return when the keystore object is absent.
// Distinguished from transport errors so the bootstrap path can fire.
var errSidecarNotFound = errors.New("sidecar not found in bucket")

// errSidecarConflict is returned by sidecarIO.PutIfNotExists when another
// writer beat us to creating the sidecar (precondition-failed). Triggers the
// "lost the race, adopt the winner" branch of the lifecycle.
var errSidecarConflict = errors.New("sidecar already exists at remote (conditional PUT conflict)")

// sidecarIO abstracts the per-backend bucket operations that runEncryptionInit
// needs. Each cloud backend implements a thin wrapper around its SDK client.
type sidecarIO interface {
	// Fetch returns the sidecar object bytes, or errSidecarNotFound, or any
	// other error encountered.
	Fetch() ([]byte, error)
	// PutIfNotExists writes body as the sidecar object using a conditional-
	// create precondition (If-None-Match: * / IfGenerationMatch=0). Returns
	// errSidecarConflict if the object already exists at the remote.
	PutIfNotExists(body []byte) error
}

// encryptionState holds per-target keystore material. Each backend's store
// struct embeds this so they share field shape and lifecycle logic.
type encryptionState struct {
	enabled      bool
	password     []byte // stashed at construction; cleared after Init
	kek          []byte // 32 bytes, valid after successful InitEncryption
	keystoreUUID [cbcrypto.KeystoreUUIDSize]byte
	initialised  bool
}

// initEncryption is the common entry point each backend's InitEncryption
// method delegates to. It is idempotent: subsequent calls are no-ops once
// the first one returned nil.
func (es *encryptionState) initEncryption(io sidecarIO, opts EncryptionInitOptions, bumpCounter func(name, msg string)) error {
	if !es.enabled {
		return nil
	}
	if es.initialised {
		return nil
	}
	kek, sc, err := runEncryptionInit(io, es.password, opts, bumpCounter)
	if err != nil {
		return err
	}
	uuid, err := sc.KeystoreUUIDBytes()
	if err != nil {
		return err
	}
	es.kek = kek
	es.keystoreUUID = uuid
	es.initialised = true
	// Wipe the stashed password — KEK is what we need from here on, and the
	// password should not linger in memory longer than necessary.
	for i := range es.password {
		es.password[i] = 0
	}
	es.password = nil
	return nil
}

// KEK returns the cached key-encryption-key for this target's encryption
// state. Returns nil if encryption is disabled or InitEncryption has not
// succeeded yet.
func (es *encryptionState) KEK() []byte { return es.kek }

// KeystoreUUID returns the cached 16-byte sidecar identifier. Used by
// upload to stamp file headers and by download to detect mismatches.
func (es *encryptionState) KeystoreUUID() [cbcrypto.KeystoreUUIDSize]byte { return es.keystoreUUID }

// EncryptionEnabled reports whether this target is configured for CSE.
func (es *encryptionState) EncryptionEnabled() bool { return es.enabled }

// EncryptionReady reports whether the keystore has been initialised
// successfully. Returns true also when encryption is disabled (nothing to
// initialise).
func (es *encryptionState) EncryptionReady() bool { return !es.enabled || es.initialised }

// runEncryptionInit executes the per-target keystore lifecycle.
//
// Returns the 32-byte KEK and the parsed sidecar on success. The caller
// should cache these on the store struct for the daemon's lifetime; the KEK
// will be used to wrap per-file CEKs at upload time and unwrap them at
// download time.
//
// bumpCounter is invoked (when non-nil) with (counter_name, message) for the
// keystore_inconsistent case so the backup report surfaces the issue.
// Callers are free to pass nil if no counter machinery is available (e.g.
// validate-config path); the function still returns the appropriate error.
func runEncryptionInit(io sidecarIO, password []byte, opts EncryptionInitOptions, bumpCounter func(name, msg string)) ([]byte, *keystore.Sidecar, error) {
	if len(password) == 0 {
		return nil, nil, errors.New("empty encrypt_pass; refusing to initialise keystore")
	}

	body, err := io.Fetch()
	if err == nil {
		return verifySidecar(body, password)
	}
	if !errors.Is(err, errSidecarNotFound) {
		return nil, nil, fmt.Errorf("fetch sidecar: %w", err)
	}

	// Sidecar missing.
	if !opts.AllowBootstrap {
		return nil, nil, ErrSidecarMissingForRestore
	}
	if opts.HasEncryptedFiles {
		msg := "keystore sidecar missing but local DB has encrypted-file records for this target; refusing to bootstrap a new keystore. Recovery: restore the sidecar from a backup, or run 'cloudbackup reset-keystore <target>' if accepting data loss."
		if bumpCounter != nil {
			bumpCounter("keystore_inconsistent", msg)
		}
		return nil, nil, fmt.Errorf("%w: %s", ErrKeystoreInconsistent, msg)
	}

	// First-time bootstrap.
	sc, kek, err := keystore.New(password)
	if err != nil {
		return nil, nil, fmt.Errorf("generate new sidecar: %w", err)
	}
	scBytes, err := sc.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal new sidecar: %w", err)
	}
	switch err := io.PutIfNotExists(scBytes); {
	case err == nil:
		return kek, sc, nil
	case errors.Is(err, errSidecarConflict):
		// Lost the race against another daemon writing the sidecar concurrently.
		// Adopt the winner's sidecar.
		body, fetchErr := io.Fetch()
		if fetchErr != nil {
			return nil, nil, fmt.Errorf("fetch sidecar after PUT conflict: %w", fetchErr)
		}
		return verifySidecar(body, password)
	default:
		return nil, nil, fmt.Errorf("write new sidecar: %w", err)
	}
}

// verifySidecar parses the YAML bytes and validates the password via the
// embedded verifier. Returns the derived KEK and parsed Sidecar.
func verifySidecar(body []byte, password []byte) ([]byte, *keystore.Sidecar, error) {
	sc, err := keystore.Unmarshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("parse sidecar: %w", err)
	}
	kek, err := sc.DeriveAndVerify(password)
	if err != nil {
		return nil, nil, fmt.Errorf("derive/verify KEK: %w", err)
	}
	if len(kek) != cbcrypto.KEKSize {
		return nil, nil, fmt.Errorf("derived KEK length %d, want %d", len(kek), cbcrypto.KEKSize)
	}
	return kek, sc, nil
}

// sidecarBucketKey computes the well-known object key for a target's
// keystore sidecar from its storePrefix. All four backends use the same
// path layout so this lives in shared code.
func sidecarBucketKey(storePrefix string) string {
	return storePrefix + "/" + keystore.SidecarRelativePath
}
