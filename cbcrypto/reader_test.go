package cbcrypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
)

// roundtripSizes spans the chunk boundaries that historically trip up
// chunked-AEAD implementations.
var roundtripSizes = []int{
	0,
	1,
	ChunkSize - 1,
	ChunkSize,
	ChunkSize + 1,
	2*ChunkSize - 1,
	2 * ChunkSize,
	2*ChunkSize + 1,
	3 * ChunkSize,
	1024*1024 + 7, // ~1 MiB, off-boundary
}

func TestRoundtripVariousSizes(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	mustRead(t, uuid[:])

	for _, sz := range roundtripSizes {
		t.Run(sizeName(sz), func(t *testing.T) {
			plain := mustRandom(t, sz)
			ct := encryptAll(t, plain, kek, uuid)

			if int64(len(ct)) != EncryptedSize(int64(sz)) {
				t.Errorf("ciphertext length %d, EncryptedSize predicted %d", len(ct), EncryptedSize(int64(sz)))
			}

			got := decryptAll(t, ct, kek)
			if !bytes.Equal(got, plain) {
				t.Fatalf("roundtrip mismatch for size %d", sz)
			}
		})
	}
}

func TestRoundtripPreservesHeader(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	mustRead(t, uuid[:])

	ct := encryptAll(t, []byte("hello"), kek, uuid)
	dr, err := NewDecryptingReader(bytes.NewReader(ct), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	hdr, err := dr.PeekHeader()
	if err != nil {
		t.Fatalf("PeekHeader: %v", err)
	}
	if hdr.KeystoreUUID != uuid {
		t.Errorf("KeystoreUUID in header doesn't match input")
	}
	if hdr.ChunkSize != ChunkSize {
		t.Errorf("ChunkSize in header = %d, want %d", hdr.ChunkSize, ChunkSize)
	}
	if hdr.Version != 1 {
		t.Errorf("Version = %d, want 1", hdr.Version)
	}
}

func TestDecryptWrongKEK(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	wrongKEK := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte

	ct := encryptAll(t, []byte("secret data"), kek, uuid)
	dr, err := NewDecryptingReader(bytes.NewReader(ct), wrongKEK)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	_, err = io.ReadAll(dr)
	if err == nil {
		t.Fatal("decryption with wrong KEK succeeded; expected unwrap failure")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	plain := mustRandom(t, ChunkSize*2+100)
	ct := encryptAll(t, plain, kek, uuid)

	// Flip a byte well past the header to land inside chunk ciphertext.
	tamperIdx := HeaderSize + 50
	ct[tamperIdx] ^= 0x01

	dr, err := NewDecryptingReader(bytes.NewReader(ct), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	if _, err := io.ReadAll(dr); err == nil {
		t.Fatal("decryption of tampered ciphertext succeeded; expected tag failure")
	}
}

func TestDecryptTruncatedLastChunk(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	// Two-chunk file so that lopping off chunk 2 leaves chunk 1 looking
	// non-final on the wire. The chunk-1 nonce was sealed with last=0;
	// the decryptor will peek EOF and try last=1 → tag fails.
	plain := mustRandom(t, ChunkSize+100)
	ct := encryptAll(t, plain, kek, uuid)

	// Drop the final chunk entirely (truncate to header + chunk 1).
	truncated := ct[:HeaderSize+ChunkSize+TagSize]

	dr, err := NewDecryptingReader(bytes.NewReader(truncated), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	if _, err := io.ReadAll(dr); err == nil {
		t.Fatal("decryption of truncated ciphertext succeeded; expected tag failure")
	}
}

func TestDecryptShortHeader(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	dr, err := NewDecryptingReader(bytes.NewReader([]byte("too short")), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	_, err = io.ReadAll(dr)
	if err == nil {
		t.Fatal("expected error reading from too-short ciphertext")
	}
}

func TestDecryptEmptyChunkAfterHeader(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	ct := encryptAll(t, nil, kek, uuid)
	// Truncate to header only — no chunk tag.
	truncated := ct[:HeaderSize]
	dr, err := NewDecryptingReader(bytes.NewReader(truncated), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	if _, err := io.ReadAll(dr); err == nil {
		t.Fatal("expected error decrypting header-only ciphertext")
	}
}

func TestReadInSmallBuffers(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	plain := mustRandom(t, ChunkSize*3+17)
	ct := encryptAll(t, plain, kek, uuid)

	dr, err := NewDecryptingReader(bytes.NewReader(ct), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	// Read into 7-byte windows to exercise the pendingOut drain path.
	var out bytes.Buffer
	buf := make([]byte, 7)
	for {
		n, err := dr.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	if !bytes.Equal(out.Bytes(), plain) {
		t.Fatal("small-buffer Read produced mismatched output")
	}
}

func TestEncryptingReaderRejectsBadKEK(t *testing.T) {
	if _, err := NewEncryptingReader(bytes.NewReader(nil), make([]byte, KEKSize-1), [KeystoreUUIDSize]byte{}); err == nil {
		t.Fatal("expected error for short KEK")
	}
}

func TestDecryptingReaderRejectsBadKEK(t *testing.T) {
	if _, err := NewDecryptingReader(bytes.NewReader(nil), make([]byte, KEKSize-1)); err == nil {
		t.Fatal("expected error for short KEK")
	}
}

func TestTwoIdenticalEncryptionsDifferInCiphertext(t *testing.T) {
	// Defense against deterministic-ciphertext leaks: the same plaintext +
	// same KEK should produce different ciphertext on every encryption,
	// because both the per-file CEK and the per-file nonce_prefix are random.
	kek := mustRandom(t, KEKSize)
	var uuid [KeystoreUUIDSize]byte
	plain := bytes.Repeat([]byte{0xAB}, 1024)

	a := encryptAll(t, plain, kek, uuid)
	b := encryptAll(t, plain, kek, uuid)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext")
	}
}

// --- helpers ---

func encryptAll(t *testing.T, plain []byte, kek []byte, uuid [KeystoreUUIDSize]byte) []byte {
	t.Helper()
	er, err := NewEncryptingReader(bytes.NewReader(plain), kek, uuid)
	if err != nil {
		t.Fatalf("NewEncryptingReader: %v", err)
	}
	out, err := io.ReadAll(er)
	if err != nil {
		t.Fatalf("read all from EncryptingReader: %v", err)
	}
	return out
}

func decryptAll(t *testing.T, ct []byte, kek []byte) []byte {
	t.Helper()
	dr, err := NewDecryptingReader(bytes.NewReader(ct), kek)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	out, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("read all from DecryptingReader: %v", err)
	}
	return out
}

func sizeName(n int) string {
	switch n {
	case 0:
		return "empty"
	case 1:
		return "byte"
	default:
		// Avoid pulling in fmt; just enumerate by size buckets.
		if n < ChunkSize {
			return "sub-chunk"
		}
		if n == ChunkSize {
			return "exact-chunk"
		}
		if n < 2*ChunkSize {
			return "chunk-plus"
		}
		if n == 2*ChunkSize {
			return "exact-two-chunks"
		}
		if n < 3*ChunkSize {
			return "two-chunks-plus"
		}
		return "many-chunks"
	}
}

// Verify rand.Read is wired in the helpers we use (compile-time check).
var _ = rand.Reader
