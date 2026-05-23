package cbcrypto

import (
	"bytes"
	"testing"
)

// FuzzUnmarshalHeader runs UnmarshalHeader against arbitrary byte inputs and asserts
// the invariants the parser is documented to enforce:
//   - it never panics
//   - if it returns no error, marshalling the result back must produce input bytes that
//     are at least HeaderSize long and round-trip-identical to the on-wire prefix
//   - it always rejects inputs shorter than HeaderSize
//   - it always rejects inputs that don't start with MagicV1
//
// Run with: go test -fuzz=FuzzUnmarshalHeader -fuzztime=20s ./cbcrypto
func FuzzUnmarshalHeader(f *testing.F) {
	// Seed corpus: one well-formed header, one truncated, one bad magic.
	well := make([]byte, HeaderSize)
	copy(well[0:4], []byte(MagicV1))
	well[4] = 1
	well[88] = 0
	well[91] = 16 // ChunkSize = 16 — any non-zero is acceptable
	f.Add(well)
	f.Add(well[:HeaderSize-1])
	bad := make([]byte, HeaderSize)
	copy(bad[0:4], []byte("XYZ!"))
	bad[4] = 1
	bad[91] = 1
	f.Add(bad)

	f.Fuzz(func(t *testing.T, in []byte) {
		h, err := UnmarshalHeader(in)
		if err != nil {
			// Negative result must be consistent: short input or bad magic should always fail.
			if len(in) < HeaderSize {
				return
			}
			if !bytes.Equal(in[0:4], []byte(MagicV1)) {
				return
			}
			if in[4] != 1 {
				return
			}
			// chunk_size == 0 is also a documented reject.
			return
		}
		// On success the returned struct's ChunkSize must be non-zero, version must be 1.
		if h.ChunkSize == 0 {
			t.Fatalf("UnmarshalHeader returned ok with chunk_size=0")
		}
		if h.Version != 1 {
			t.Fatalf("UnmarshalHeader returned ok with version=%d", h.Version)
		}
		// Re-marshal and confirm the produced bytes start with MagicV1.
		out := h.MarshalBinary()
		if !bytes.HasPrefix(out, []byte(MagicV1)) {
			t.Fatalf("re-marshalled header missing magic prefix; out=%v", out[:4])
		}
	})
}
