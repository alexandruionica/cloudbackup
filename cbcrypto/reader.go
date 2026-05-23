package cbcrypto

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// EncryptingReader wraps a plaintext io.Reader and emits the ciphertext
// stream (header followed by chunked AES-256-GCM). It is one-shot and not
// seekable.
//
// A fresh per-file content-encryption-key (CEK) and 4-byte nonce prefix are
// generated at construction. The CEK is wrapped under the supplied KEK and
// embedded in the header along with the supplied keystoreUUID; the prefix is
// embedded in the header for use during decryption.
type EncryptingReader struct {
	src    *bufio.Reader
	cipher cipher.AEAD

	noncePrefix [NoncePrefixSize]byte
	counter     uint64

	header        []byte // remaining unsent header bytes
	headerEmitted bool

	chunkPlain  []byte // ChunkSize-byte scratch
	pendingOut  []byte // ciphertext bytes ready to return from Read
	lastEmitted bool   // we've encrypted the final chunk
}

// NewEncryptingReader builds an EncryptingReader from a plaintext source,
// a 32-byte KEK, and the per-target keystore UUID.
func NewEncryptingReader(src io.Reader, kek []byte, keystoreUUID [KeystoreUUIDSize]byte) (*EncryptingReader, error) {
	if len(kek) != KEKSize {
		return nil, fmt.Errorf("kek length %d, want %d", len(kek), KEKSize)
	}
	cek := make([]byte, CEKSize)
	if _, err := rand.Read(cek); err != nil {
		return nil, err
	}
	wrapNonce, wrappedCEK, err := WrapCEK(kek, cek)
	if err != nil {
		return nil, err
	}
	var noncePrefix [NoncePrefixSize]byte
	if _, err := rand.Read(noncePrefix[:]); err != nil {
		return nil, err
	}
	hdr := &Header{
		Version:      1,
		KeystoreUUID: keystoreUUID,
		WrapNonce:    wrapNonce,
		WrappedCEK:   wrappedCEK,
		NoncePrefix:  noncePrefix,
		ChunkSize:    ChunkSize,
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &EncryptingReader{
		src:         bufio.NewReader(src),
		cipher:      gcm,
		noncePrefix: noncePrefix,
		header:      hdr.MarshalBinary(),
		chunkPlain:  make([]byte, ChunkSize),
	}, nil
}

// Read implements io.Reader. Output: header bytes first, then ciphertext
// chunks in order. Returns io.EOF once the last chunk's tag has been emitted.
func (r *EncryptingReader) Read(p []byte) (int, error) {
	for {
		// Emit header before any chunk.
		if !r.headerEmitted {
			n := copy(p, r.header)
			r.header = r.header[n:]
			if len(r.header) == 0 {
				r.headerEmitted = true
			}
			if n > 0 {
				return n, nil
			}
			// n == 0 means p is empty; let the caller try again with a real buffer.
			return 0, nil
		}
		// Drain anything already encrypted but not yet returned.
		if len(r.pendingOut) > 0 {
			n := copy(p, r.pendingOut)
			r.pendingOut = r.pendingOut[n:]
			return n, nil
		}
		if r.lastEmitted {
			return 0, io.EOF
		}
		// Produce the next chunk.
		if err := r.encryptNextChunk(); err != nil {
			return 0, err
		}
	}
}

func (r *EncryptingReader) encryptNextChunk() error {
	n, eof, err := readFullOrEOF(r.src, r.chunkPlain)
	if err != nil {
		return err
	}
	isLast := eof
	if !isLast {
		// Got a full chunk; check if there's anything after it.
		_, peekErr := r.src.Peek(1)
		switch peekErr {
		case nil:
			isLast = false
		case io.EOF:
			isLast = true
		default:
			return peekErr
		}
	}
	nonce, err := buildChunkNonce(r.noncePrefix, r.counter, isLast)
	if err != nil {
		return err
	}
	// Seal returns plaintext||tag. Allocate fresh each time; chunks are 64KiB
	// + 16, so this avoids subtle aliasing bugs with a reused scratch buffer.
	ct := r.cipher.Seal(nil, nonce[:], r.chunkPlain[:n], nil)
	r.pendingOut = ct
	r.counter++
	if isLast {
		r.lastEmitted = true
	}
	return nil
}

// DecryptingReader wraps a ciphertext io.Reader and emits the plaintext
// stream. It parses the header on first Read, then verifies and emits each
// chunk in order. Detects truncation via the per-chunk last-flag in the nonce.
//
// Callers should verify the header's KeystoreUUID against the expected
// keystore identity (the sidecar's keystore_uuid) before consuming further
// plaintext, to surface a clear "keystore mismatch" error rather than a
// cryptic AEAD tag failure.
type DecryptingReader struct {
	src    *bufio.Reader
	cipher cipher.AEAD

	noncePrefix [NoncePrefixSize]byte
	chunkSize   int
	counter     uint64

	header       *Header
	headerParsed bool

	chunkCT      []byte // (ChunkSize + TagSize)-byte scratch
	pendingOut   []byte // decrypted plaintext bytes ready to return
	lastConsumed bool
}

// NewDecryptingReader builds a DecryptingReader from a ciphertext source and
// a 32-byte KEK. The header is parsed lazily on the first Read so that
// callers can construct the reader cheaply and surface header errors as
// regular Read errors.
func NewDecryptingReader(src io.Reader, kek []byte) (*DecryptingReader, error) {
	if len(kek) != KEKSize {
		return nil, fmt.Errorf("kek length %d, want %d", len(kek), KEKSize)
	}
	return &DecryptingReader{
		src: bufio.NewReader(src),
		// cipher and noncePrefix are populated by parseHeader on first Read;
		// stash KEK temporarily in chunkCT to avoid an extra field.
		chunkCT: kek,
	}, nil
}

// Header returns the parsed header. Returns nil if the header has not yet
// been read (i.e. before the first Read or PeekHeader call).
func (r *DecryptingReader) Header() *Header { return r.header }

// PeekHeader forces the header to be parsed without consuming any plaintext.
// Useful for early keystore-UUID validation.
func (r *DecryptingReader) PeekHeader() (*Header, error) {
	if r.headerParsed {
		return r.header, nil
	}
	if err := r.parseHeader(); err != nil {
		return nil, err
	}
	r.headerParsed = true
	return r.header, nil
}

// Read implements io.Reader.
func (r *DecryptingReader) Read(p []byte) (int, error) {
	for {
		if !r.headerParsed {
			if err := r.parseHeader(); err != nil {
				return 0, err
			}
			r.headerParsed = true
		}
		if len(r.pendingOut) > 0 {
			n := copy(p, r.pendingOut)
			r.pendingOut = r.pendingOut[n:]
			return n, nil
		}
		if r.lastConsumed {
			return 0, io.EOF
		}
		if err := r.decryptNextChunk(); err != nil {
			return 0, err
		}
	}
}

func (r *DecryptingReader) parseHeader() error {
	// chunkCT temporarily holds the KEK until we use it to unwrap.
	kek := r.chunkCT
	r.chunkCT = nil

	hdrBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r.src, hdrBuf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("ciphertext too short for header: %w", err)
		}
		return err
	}
	hdr, err := UnmarshalHeader(hdrBuf)
	if err != nil {
		return err
	}
	cek, err := UnwrapCEK(kek, hdr.WrapNonce, hdr.WrappedCEK)
	if err != nil {
		return fmt.Errorf("unwrap CEK: %w", err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	r.header = hdr
	r.cipher = gcm
	r.noncePrefix = hdr.NoncePrefix
	r.chunkSize = int(hdr.ChunkSize)
	if r.chunkSize <= 0 || r.chunkSize > 64*1024*1024 {
		return fmt.Errorf("header chunk_size %d out of range", r.chunkSize)
	}
	r.chunkCT = make([]byte, r.chunkSize+TagSize)
	return nil
}

func (r *DecryptingReader) decryptNextChunk() error {
	n, eof, err := readFullOrEOF(r.src, r.chunkCT)
	if err != nil {
		return err
	}
	if n < TagSize {
		return fmt.Errorf("chunk %d truncated: only %d bytes (need at least %d for tag)", r.counter, n, TagSize)
	}
	isLast := eof
	if !isLast {
		_, peekErr := r.src.Peek(1)
		switch peekErr {
		case nil:
			isLast = false
		case io.EOF:
			isLast = true
		default:
			return peekErr
		}
	}
	nonce, err := buildChunkNonce(r.noncePrefix, r.counter, isLast)
	if err != nil {
		return err
	}
	pt, err := r.cipher.Open(nil, nonce[:], r.chunkCT[:n], nil)
	if err != nil {
		return fmt.Errorf("decrypt chunk %d (last=%v): %w", r.counter, isLast, err)
	}
	r.pendingOut = pt
	r.counter++
	if isLast {
		r.lastConsumed = true
	}
	return nil
}
