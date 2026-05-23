// Package keystore implements the per-target sidecar that holds the salt,
// KDF parameters, keystore UUID, and password verifier needed to bootstrap
// client-side encryption.
//
// The sidecar is a small YAML file persisted at
// <storePrefix>/.cbcrypt/keystore.v1.yaml in every encryption-enabled target's
// bucket. Its contents are non-sensitive (salt and KDF parameters are public
// by design; the verifier is a known plaintext sealed with the KEK), so it
// can be stored cleartext.
//
// This package is pure data + crypto; it has no knowledge of object stores
// or networking. The fetch/write/race-handling lifecycle belongs to whatever
// component owns the bucket client (see Phase 6 in the implementation plan).
package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"gopkg.in/yaml.v2"

	"cloudbackup/cbcrypto"
)

const (
	// CurrentVersion is the sidecar schema version this binary writes.
	CurrentVersion = 1

	// KDFAlgorithmArgon2id is the only KDF algorithm currently supported in
	// the sidecar's kdf.algorithm field.
	KDFAlgorithmArgon2id = "argon2id"

	// ReservedPathSegment is the directory name (relative to a target's
	// storePrefix) used by cloudbackup to hold encryption-related metadata.
	// File-path mapping in the upload pipeline must reject any plaintext
	// remote path that would land under this segment.
	ReservedPathSegment = ".cbcrypt"

	// SidecarFileName is the leaf file name of the sidecar object.
	SidecarFileName = "keystore.v1.yaml"

	// verifierPlaintextSize is the fixed length of the known plaintext the
	// verifier seals. 16 zero bytes — value is irrelevant; any constant
	// works, this one is conventional.
	verifierPlaintextSize = 16

	// VerifierCiphertextSize is the on-disk length of the encrypted verifier.
	VerifierCiphertextSize = verifierPlaintextSize + cbcrypto.TagSize

	// SaltSize is the byte length of the per-target argon2id salt.
	SaltSize = 32
)

// SidecarRelativePath is the bucket-relative path of the sidecar object
// under a target's storePrefix.
var SidecarRelativePath = ReservedPathSegment + "/" + SidecarFileName

// verifierPlaintext is the fixed plaintext the verifier seals. It must
// outlive any verifier check; never mutate the returned slice.
var verifierPlaintext = make([]byte, verifierPlaintextSize)

// Sidecar mirrors the on-disk YAML structure.
type Sidecar struct {
	Version      int       `yaml:"version"`
	KeystoreUUID string    `yaml:"keystore_uuid"` // base64-encoded 16 bytes
	KDF          KDFConfig `yaml:"kdf"`
	Verifier     Verifier  `yaml:"verifier"`
}

// KDFConfig holds the parameters fed to argon2id during KEK derivation.
type KDFConfig struct {
	Algorithm string `yaml:"algorithm"`
	Time      uint32 `yaml:"time"`
	MemoryKiB uint32 `yaml:"memory_kib"`
	Threads   uint8  `yaml:"threads"`
	Salt      string `yaml:"salt"` // base64-encoded
}

// Verifier holds the encrypted "did we derive the right KEK?" probe.
type Verifier struct {
	Nonce      string `yaml:"nonce"`      // base64-encoded 12 bytes
	Ciphertext string `yaml:"ciphertext"` // base64-encoded 32 bytes
}

// ErrWrongPassword is returned by DeriveAndVerify when the verifier check
// fails after a successful argon2id derivation. Callers should surface this
// as a clear "wrong encrypt_pass" error rather than a generic crypto failure.
var ErrWrongPassword = errors.New("keystore verifier failed: wrong password or sidecar tampered")

// New generates a fresh sidecar from password using DefaultKDFParams.
// Returns the populated Sidecar struct and the derived KEK so callers don't
// need to re-derive immediately.
func New(password []byte) (*Sidecar, []byte, error) {
	return NewWithParams(password, cbcrypto.DefaultKDFParams)
}

// NewWithParams is the explicit-KDF-params form of New.
func NewWithParams(password []byte, params cbcrypto.KDFParams) (*Sidecar, []byte, error) {
	if len(password) == 0 {
		return nil, nil, errors.New("password must not be empty")
	}
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, fmt.Errorf("read salt: %w", err)
	}
	uuid := make([]byte, cbcrypto.KeystoreUUIDSize)
	if _, err := rand.Read(uuid); err != nil {
		return nil, nil, fmt.Errorf("read keystore uuid: %w", err)
	}

	kek := cbcrypto.DeriveKEK(password, salt, params)

	verifNonce, verifCT, err := sealVerifier(kek)
	if err != nil {
		return nil, nil, err
	}

	s := &Sidecar{
		Version:      CurrentVersion,
		KeystoreUUID: base64.StdEncoding.EncodeToString(uuid),
		KDF: KDFConfig{
			Algorithm: KDFAlgorithmArgon2id,
			Time:      params.Time,
			MemoryKiB: params.Memory,
			Threads:   params.Threads,
			Salt:      base64.StdEncoding.EncodeToString(salt),
		},
		Verifier: Verifier{
			Nonce:      base64.StdEncoding.EncodeToString(verifNonce),
			Ciphertext: base64.StdEncoding.EncodeToString(verifCT),
		},
	}
	return s, kek, nil
}

// Marshal serialises the sidecar to YAML bytes.
func (s *Sidecar) Marshal() ([]byte, error) {
	return yaml.Marshal(s)
}

// Unmarshal parses YAML bytes into a Sidecar and validates structural
// invariants (version, algorithm, decoded field lengths). It does NOT
// verify the password — call DeriveAndVerify for that.
func Unmarshal(b []byte) (*Sidecar, error) {
	var s Sidecar
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Sidecar) validate() error {
	if s.Version != CurrentVersion {
		return fmt.Errorf("unsupported sidecar version %d (want %d)", s.Version, CurrentVersion)
	}
	if s.KDF.Algorithm != KDFAlgorithmArgon2id {
		return fmt.Errorf("unsupported KDF algorithm %q", s.KDF.Algorithm)
	}
	if s.KDF.Time == 0 || s.KDF.MemoryKiB == 0 || s.KDF.Threads == 0 {
		return errors.New("KDF parameters time/memory_kib/threads must be non-zero")
	}
	if _, err := s.KeystoreUUIDBytes(); err != nil {
		return err
	}
	if _, err := s.SaltBytes(); err != nil {
		return err
	}
	if _, err := s.verifierNonceBytes(); err != nil {
		return err
	}
	if _, err := s.verifierCiphertextBytes(); err != nil {
		return err
	}
	return nil
}

// KeystoreUUIDBytes returns the decoded 16-byte keystore identifier.
func (s *Sidecar) KeystoreUUIDBytes() ([cbcrypto.KeystoreUUIDSize]byte, error) {
	var out [cbcrypto.KeystoreUUIDSize]byte
	raw, err := base64.StdEncoding.DecodeString(s.KeystoreUUID)
	if err != nil {
		return out, fmt.Errorf("decode keystore_uuid: %w", err)
	}
	if len(raw) != cbcrypto.KeystoreUUIDSize {
		return out, fmt.Errorf("keystore_uuid length %d, want %d", len(raw), cbcrypto.KeystoreUUIDSize)
	}
	copy(out[:], raw)
	return out, nil
}

// SaltBytes returns the decoded argon2id salt.
func (s *Sidecar) SaltBytes() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s.KDF.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	if len(raw) < 16 {
		return nil, fmt.Errorf("salt length %d, want at least 16", len(raw))
	}
	return raw, nil
}

func (s *Sidecar) verifierNonceBytes() ([cbcrypto.NonceSize]byte, error) {
	var out [cbcrypto.NonceSize]byte
	raw, err := base64.StdEncoding.DecodeString(s.Verifier.Nonce)
	if err != nil {
		return out, fmt.Errorf("decode verifier nonce: %w", err)
	}
	if len(raw) != cbcrypto.NonceSize {
		return out, fmt.Errorf("verifier nonce length %d, want %d", len(raw), cbcrypto.NonceSize)
	}
	copy(out[:], raw)
	return out, nil
}

func (s *Sidecar) verifierCiphertextBytes() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s.Verifier.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode verifier ciphertext: %w", err)
	}
	if len(raw) != VerifierCiphertextSize {
		return nil, fmt.Errorf("verifier ciphertext length %d, want %d", len(raw), VerifierCiphertextSize)
	}
	return raw, nil
}

// KDFParams returns the argon2id parameters as a cbcrypto.KDFParams.
func (s *Sidecar) KDFParams() cbcrypto.KDFParams {
	return cbcrypto.KDFParams{
		Time:    s.KDF.Time,
		Memory:  s.KDF.MemoryKiB,
		Threads: s.KDF.Threads,
	}
}

// DeriveAndVerify re-derives the KEK from password using the sidecar's KDF
// parameters and salt, then decrypts the verifier. On success returns the
// 32-byte KEK; on verifier failure returns ErrWrongPassword.
func (s *Sidecar) DeriveAndVerify(password []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("password must not be empty")
	}
	salt, err := s.SaltBytes()
	if err != nil {
		return nil, err
	}
	nonce, err := s.verifierNonceBytes()
	if err != nil {
		return nil, err
	}
	ct, err := s.verifierCiphertextBytes()
	if err != nil {
		return nil, err
	}
	kek := cbcrypto.DeriveKEK(password, salt, s.KDFParams())

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce[:], ct, nil)
	if err != nil {
		return nil, ErrWrongPassword
	}
	if len(pt) != verifierPlaintextSize {
		return nil, fmt.Errorf("verifier plaintext length %d, want %d", len(pt), verifierPlaintextSize)
	}
	for _, b := range pt {
		if b != 0 {
			return nil, ErrWrongPassword
		}
	}
	return kek, nil
}

// sealVerifier encrypts the fixed verifier plaintext (16 zero bytes) under
// kek with a random nonce. Returns (nonce, ciphertext||tag).
func sealVerifier(kek []byte) (nonce []byte, ciphertext []byte, err error) {
	nonce = make([]byte, cbcrypto.NonceSize)
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, verifierPlaintext, nil)
	return nonce, ciphertext, nil
}
