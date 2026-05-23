// Package cbcrypto implements client-side encryption for cloudbackup.
//
// File format (single ciphertext stream per encrypted object):
//
//	Header (HeaderSize bytes, fixed):
//	  0..3   magic "CBE1"
//	  4      version (currently 1)
//	  5..7   reserved (zero)
//	  8..23  keystore_uuid (16 bytes)
//	  24..35 wrap_nonce (12 bytes, AES-GCM nonce that protects the wrapped CEK)
//	  36..83 wrapped_cek (48 bytes = 32-byte CEK + 16-byte AES-GCM tag)
//	  84..87 nonce_prefix (4 random bytes, per-file)
//	  88..91 chunk_size (uint32, big-endian, plaintext bytes per chunk)
//
//	Body (zero or more chunks; always at least one chunk, even for empty plaintext):
//	  Each chunk: plaintext (1..ChunkSize bytes; final chunk may be shorter) || tag (16 bytes)
//	  Chunk nonce (12 bytes) = nonce_prefix(4) || counter(7, big-endian) || last_flag(1)
//	  last_flag = 0x01 only on the final chunk; otherwise 0x00. This prevents truncation
//	  attacks: a chopped stream cannot present a non-final chunk as final without failing
//	  the AEAD tag check.
package cbcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	// MagicV1 is the four-byte header prefix identifying a cloudbackup v1
	// client-side-encrypted stream.
	MagicV1 = "CBE1"

	// HeaderSize is the fixed on-the-wire size of the file header in bytes.
	HeaderSize = 92

	// ChunkSize is the plaintext chunk size in bytes. Each chunk is encrypted
	// independently with its own AES-GCM tag.
	ChunkSize = 64 * 1024

	// TagSize is the AES-GCM authentication tag size in bytes.
	TagSize = 16

	// NonceSize is the AES-GCM nonce size in bytes.
	NonceSize = 12

	// NoncePrefixSize is the per-file random prefix portion of each chunk nonce.
	NoncePrefixSize = 4

	// KeystoreUUIDSize is the size of the per-target keystore identifier in bytes.
	KeystoreUUIDSize = 16

	// KEKSize is the byte length of the key-encryption-key (AES-256).
	KEKSize = 32

	// CEKSize is the byte length of the content-encryption-key (AES-256).
	CEKSize = 32

	// WrappedCEKSize is the byte length of the AES-GCM-wrapped CEK
	// (CEKSize plaintext + TagSize tag).
	WrappedCEKSize = CEKSize + TagSize

	// MaxChunkCounter is the largest chunk counter value representable in the
	// 7-byte big-endian counter field of a chunk nonce.
	MaxChunkCounter = uint64(1)<<56 - 1
)

// KDFParams are the argon2id parameters used to derive a KEK from a password.
// They are persisted in the per-target sidecar so that future re-derivation
// uses the same cost factor regardless of binary version.
type KDFParams struct {
	Time    uint32
	Memory  uint32 // KiB
	Threads uint8
}

// DefaultKDFParams is the parameter set used when bootstrapping a new
// keystore sidecar. 3 iterations, 64 MiB memory, 4 lanes per the OWASP 2023
// guidance for argon2id.
var DefaultKDFParams = KDFParams{
	Time:    3,
	Memory:  64 * 1024,
	Threads: 4,
}

// DeriveKEK runs argon2id over (password, salt) using the given parameters
// and returns a 32-byte key-encryption-key.
func DeriveKEK(password, salt []byte, params KDFParams) []byte {
	return argon2.IDKey(password, salt, params.Time, params.Memory, uint8(params.Threads), KEKSize)
}

// WrapCEK encrypts cek under kek using AES-256-GCM with a freshly generated
// random nonce. Returns the nonce and the (CEK || tag) ciphertext.
func WrapCEK(kek, cek []byte) (wrapNonce [NonceSize]byte, wrappedCEK [WrappedCEKSize]byte, err error) {
	if len(kek) != KEKSize {
		err = fmt.Errorf("kek length %d, want %d", len(kek), KEKSize)
		return
	}
	if len(cek) != CEKSize {
		err = fmt.Errorf("cek length %d, want %d", len(cek), CEKSize)
		return
	}
	if _, err = rand.Read(wrapNonce[:]); err != nil {
		return
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return
	}
	sealed := gcm.Seal(nil, wrapNonce[:], cek, nil)
	if len(sealed) != WrappedCEKSize {
		err = fmt.Errorf("wrapped cek length %d, want %d", len(sealed), WrappedCEKSize)
		return
	}
	copy(wrappedCEK[:], sealed)
	return
}

// UnwrapCEK reverses WrapCEK, returning the 32-byte CEK or an error if the
// authentication tag does not verify (wrong KEK, corruption, etc.).
func UnwrapCEK(kek []byte, wrapNonce [NonceSize]byte, wrappedCEK [WrappedCEKSize]byte) ([]byte, error) {
	if len(kek) != KEKSize {
		return nil, fmt.Errorf("kek length %d, want %d", len(kek), KEKSize)
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, wrapNonce[:], wrappedCEK[:], nil)
}

// Header is the parsed form of the fixed-size file header.
type Header struct {
	Version      uint8
	KeystoreUUID [KeystoreUUIDSize]byte
	WrapNonce    [NonceSize]byte
	WrappedCEK   [WrappedCEKSize]byte
	NoncePrefix  [NoncePrefixSize]byte
	ChunkSize    uint32
}

// MarshalBinary serialises h into a HeaderSize-byte buffer.
func (h *Header) MarshalBinary() []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[0:4], MagicV1)
	buf[4] = h.Version
	// buf[5:8] reserved, left zero
	copy(buf[8:24], h.KeystoreUUID[:])
	copy(buf[24:36], h.WrapNonce[:])
	copy(buf[36:84], h.WrappedCEK[:])
	copy(buf[84:88], h.NoncePrefix[:])
	binary.BigEndian.PutUint32(buf[88:92], h.ChunkSize)
	return buf
}

// UnmarshalHeader parses a HeaderSize-byte buffer into a Header. Returns an
// error if the magic or version field is unrecognised, or if the buffer is
// too short.
func UnmarshalHeader(buf []byte) (*Header, error) {
	if len(buf) < HeaderSize {
		return nil, fmt.Errorf("header buffer length %d, want at least %d", len(buf), HeaderSize)
	}
	if string(buf[0:4]) != MagicV1 {
		return nil, fmt.Errorf("invalid magic %q (want %q)", buf[0:4], MagicV1)
	}
	if buf[4] != 1 {
		return nil, fmt.Errorf("unsupported header version %d", buf[4])
	}
	h := &Header{Version: buf[4]}
	copy(h.KeystoreUUID[:], buf[8:24])
	copy(h.WrapNonce[:], buf[24:36])
	copy(h.WrappedCEK[:], buf[36:84])
	copy(h.NoncePrefix[:], buf[84:88])
	h.ChunkSize = binary.BigEndian.Uint32(buf[88:92])
	if h.ChunkSize == 0 {
		return nil, errors.New("header chunk_size is zero")
	}
	return h, nil
}

// EncryptedSize returns the on-the-wire size in bytes of an encrypted stream
// produced from a plaintext of plaintextSize bytes, using the package's
// default ChunkSize. An empty plaintext still produces one chunk (containing
// only the AES-GCM tag) so that decryption always has a final-chunk marker.
func EncryptedSize(plaintextSize int64) int64 {
	if plaintextSize < 0 {
		return 0
	}
	if plaintextSize == 0 {
		return int64(HeaderSize) + int64(TagSize)
	}
	chunks := plaintextSize / int64(ChunkSize)
	if plaintextSize%int64(ChunkSize) != 0 {
		chunks++
	}
	return int64(HeaderSize) + plaintextSize + chunks*int64(TagSize)
}

// buildChunkNonce constructs the 12-byte AES-GCM nonce for a single chunk.
// Layout: nonce_prefix(4) || counter(7, big-endian) || last_flag(1).
// Returns an error if counter exceeds MaxChunkCounter.
func buildChunkNonce(prefix [NoncePrefixSize]byte, counter uint64, last bool) ([NonceSize]byte, error) {
	var nonce [NonceSize]byte
	if counter > MaxChunkCounter {
		return nonce, fmt.Errorf("chunk counter %d exceeds max %d", counter, MaxChunkCounter)
	}
	copy(nonce[0:NoncePrefixSize], prefix[:])
	// 7-byte big-endian counter at indices 4..10.
	nonce[4] = byte(counter >> 48)
	nonce[5] = byte(counter >> 40)
	nonce[6] = byte(counter >> 32)
	nonce[7] = byte(counter >> 24)
	nonce[8] = byte(counter >> 16)
	nonce[9] = byte(counter >> 8)
	nonce[10] = byte(counter)
	if last {
		nonce[11] = 0x01
	}
	return nonce, nil
}

// readFullOrEOF is io.ReadFull with EOF and ErrUnexpectedEOF folded into a
// single "got n bytes, source exhausted" return path.
func readFullOrEOF(src io.Reader, buf []byte) (n int, eof bool, err error) {
	n, err = io.ReadFull(src, buf)
	switch err {
	case nil:
		return n, false, nil
	case io.EOF, io.ErrUnexpectedEOF:
		return n, true, nil
	default:
		return n, false, err
	}
}
