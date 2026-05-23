package cbcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"testing"
)

func TestEncryptedSize(t *testing.T) {
	cases := []struct {
		plain int64
		want  int64
	}{
		{0, int64(HeaderSize) + int64(TagSize)},
		{1, int64(HeaderSize) + 1 + int64(TagSize)},
		{int64(ChunkSize) - 1, int64(HeaderSize) + int64(ChunkSize) - 1 + int64(TagSize)},
		{int64(ChunkSize), int64(HeaderSize) + int64(ChunkSize) + int64(TagSize)},
		{int64(ChunkSize) + 1, int64(HeaderSize) + int64(ChunkSize) + 1 + 2*int64(TagSize)},
		{2 * int64(ChunkSize), int64(HeaderSize) + 2*int64(ChunkSize) + 2*int64(TagSize)},
		{2*int64(ChunkSize) + 1, int64(HeaderSize) + 2*int64(ChunkSize) + 1 + 3*int64(TagSize)},
	}
	for _, c := range cases {
		got := EncryptedSize(c.plain)
		if got != c.want {
			t.Errorf("EncryptedSize(%d) = %d, want %d", c.plain, got, c.want)
		}
	}
}

func TestHeaderRoundtrip(t *testing.T) {
	want := Header{
		Version:     1,
		ChunkSize:   ChunkSize,
		NoncePrefix: [NoncePrefixSize]byte{0xDE, 0xAD, 0xBE, 0xEF},
	}
	mustRead(t, want.KeystoreUUID[:])
	mustRead(t, want.WrapNonce[:])
	mustRead(t, want.WrappedCEK[:])

	buf := want.MarshalBinary()
	if len(buf) != HeaderSize {
		t.Fatalf("header length %d, want %d", len(buf), HeaderSize)
	}
	if string(buf[0:4]) != MagicV1 {
		t.Errorf("magic %q, want %q", buf[0:4], MagicV1)
	}
	if binary.BigEndian.Uint32(buf[88:92]) != ChunkSize {
		t.Errorf("chunk_size field mismatch")
	}

	got, err := UnmarshalHeader(buf)
	if err != nil {
		t.Fatalf("UnmarshalHeader: %v", err)
	}
	if got.Version != want.Version {
		t.Errorf("Version: got %d, want %d", got.Version, want.Version)
	}
	if got.KeystoreUUID != want.KeystoreUUID {
		t.Errorf("KeystoreUUID mismatch")
	}
	if got.WrapNonce != want.WrapNonce {
		t.Errorf("WrapNonce mismatch")
	}
	if got.WrappedCEK != want.WrappedCEK {
		t.Errorf("WrappedCEK mismatch")
	}
	if got.NoncePrefix != want.NoncePrefix {
		t.Errorf("NoncePrefix mismatch")
	}
	if got.ChunkSize != want.ChunkSize {
		t.Errorf("ChunkSize: got %d, want %d", got.ChunkSize, want.ChunkSize)
	}
}

func TestHeaderRejectsBadMagic(t *testing.T) {
	h := Header{Version: 1, ChunkSize: ChunkSize}
	buf := h.MarshalBinary()
	buf[0] = 'X'
	if _, err := UnmarshalHeader(buf); err == nil {
		t.Fatal("expected error for bad magic, got nil")
	}
}

func TestHeaderRejectsUnknownVersion(t *testing.T) {
	h := Header{Version: 1, ChunkSize: ChunkSize}
	buf := h.MarshalBinary()
	buf[4] = 99
	if _, err := UnmarshalHeader(buf); err == nil {
		t.Fatal("expected error for unknown version, got nil")
	}
}

func TestHeaderRejectsZeroChunkSize(t *testing.T) {
	h := Header{Version: 1, ChunkSize: 0}
	buf := h.MarshalBinary()
	if _, err := UnmarshalHeader(buf); err == nil {
		t.Fatal("expected error for zero chunk_size, got nil")
	}
}

func TestHeaderRejectsShortBuffer(t *testing.T) {
	if _, err := UnmarshalHeader(make([]byte, HeaderSize-1)); err == nil {
		t.Fatal("expected error for short buffer, got nil")
	}
}

func TestKDFDeterministic(t *testing.T) {
	salt := mustRandom(t, 32)
	password := []byte("hunter2")
	params := KDFParams{Time: 1, Memory: 8 * 1024, Threads: 1}
	a := DeriveKEK(password, salt, params)
	b := DeriveKEK(password, salt, params)
	if !bytes.Equal(a, b) {
		t.Fatal("DeriveKEK is not deterministic for the same inputs")
	}
	if len(a) != KEKSize {
		t.Fatalf("KEK length %d, want %d", len(a), KEKSize)
	}
	c := DeriveKEK(password, mustRandom(t, 32), params)
	if bytes.Equal(a, c) {
		t.Fatal("DeriveKEK produced the same KEK for different salts")
	}
}

func TestWrapUnwrapCEKRoundtrip(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	cek := mustRandom(t, CEKSize)
	wrapNonce, wrappedCEK, err := WrapCEK(kek, cek)
	if err != nil {
		t.Fatalf("WrapCEK: %v", err)
	}
	got, err := UnwrapCEK(kek, wrapNonce, wrappedCEK)
	if err != nil {
		t.Fatalf("UnwrapCEK: %v", err)
	}
	if !bytes.Equal(got, cek) {
		t.Fatal("unwrapped CEK does not match original")
	}
}

func TestUnwrapCEKWrongKEK(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	wrongKEK := mustRandom(t, KEKSize)
	cek := mustRandom(t, CEKSize)
	wrapNonce, wrappedCEK, err := WrapCEK(kek, cek)
	if err != nil {
		t.Fatalf("WrapCEK: %v", err)
	}
	if _, err := UnwrapCEK(wrongKEK, wrapNonce, wrappedCEK); err == nil {
		t.Fatal("UnwrapCEK with wrong KEK succeeded; expected tag failure")
	}
}

func TestUnwrapCEKTampered(t *testing.T) {
	kek := mustRandom(t, KEKSize)
	cek := mustRandom(t, CEKSize)
	wrapNonce, wrappedCEK, err := WrapCEK(kek, cek)
	if err != nil {
		t.Fatalf("WrapCEK: %v", err)
	}
	wrappedCEK[0] ^= 0x01
	if _, err := UnwrapCEK(kek, wrapNonce, wrappedCEK); err == nil {
		t.Fatal("UnwrapCEK with tampered ciphertext succeeded; expected tag failure")
	}
}

func TestWrapCEKRejectsBadLengths(t *testing.T) {
	if _, _, err := WrapCEK(make([]byte, KEKSize-1), make([]byte, CEKSize)); err == nil {
		t.Error("expected error for short KEK")
	}
	if _, _, err := WrapCEK(make([]byte, KEKSize), make([]byte, CEKSize-1)); err == nil {
		t.Error("expected error for short CEK")
	}
}

func TestBuildChunkNonce(t *testing.T) {
	prefix := [NoncePrefixSize]byte{0x01, 0x02, 0x03, 0x04}
	n, err := buildChunkNonce(prefix, 0, false)
	if err != nil {
		t.Fatalf("buildChunkNonce: %v", err)
	}
	want := [NonceSize]byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0, 0, 0, 0, 0}
	if n != want {
		t.Errorf("nonce(0, false) = %x, want %x", n, want)
	}
	nLast, err := buildChunkNonce(prefix, 0, true)
	if err != nil {
		t.Fatalf("buildChunkNonce: %v", err)
	}
	wantLast := [NonceSize]byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0, 0, 0, 0, 1}
	if nLast != wantLast {
		t.Errorf("nonce(0, true) = %x, want %x", nLast, wantLast)
	}
	// Counter at MaxChunkCounter fits.
	if _, err := buildChunkNonce(prefix, MaxChunkCounter, false); err != nil {
		t.Errorf("counter at MaxChunkCounter rejected: %v", err)
	}
	// One past MaxChunkCounter should fail.
	if _, err := buildChunkNonce(prefix, MaxChunkCounter+1, false); err == nil {
		t.Error("counter past MaxChunkCounter accepted; expected error")
	}
}

// --- helpers ---

func mustRandom(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	mustRead(t, b)
	return b
}

func mustRead(t *testing.T, b []byte) {
	t.Helper()
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
}
