package keystore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"cloudbackup/cbcrypto"
)

// fastParams keeps unit tests snappy; production uses cbcrypto.DefaultKDFParams.
var fastParams = cbcrypto.KDFParams{Time: 1, Memory: 8 * 1024, Threads: 1}

func TestNewRoundtripVerify(t *testing.T) {
	password := []byte("correct horse battery staple")
	sc, kek1, err := NewWithParams(password, fastParams)
	if err != nil {
		t.Fatalf("NewWithParams: %v", err)
	}
	if len(kek1) != cbcrypto.KEKSize {
		t.Fatalf("KEK length %d, want %d", len(kek1), cbcrypto.KEKSize)
	}

	// Marshal to YAML, then parse back.
	buf, err := sc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	sc2, err := Unmarshal(buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// DeriveAndVerify should yield the same KEK.
	kek2, err := sc2.DeriveAndVerify(password)
	if err != nil {
		t.Fatalf("DeriveAndVerify: %v", err)
	}
	if !bytes.Equal(kek1, kek2) {
		t.Fatal("KEK mismatch after roundtrip")
	}
}

func TestDeriveAndVerifyWrongPassword(t *testing.T) {
	sc, _, err := NewWithParams([]byte("right"), fastParams)
	if err != nil {
		t.Fatalf("NewWithParams: %v", err)
	}
	_, err = sc.DeriveAndVerify([]byte("wrong"))
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("DeriveAndVerify with wrong password: got err=%v, want ErrWrongPassword", err)
	}
}

func TestDeriveAndVerifyEmptyPassword(t *testing.T) {
	sc, _, err := NewWithParams([]byte("x"), fastParams)
	if err != nil {
		t.Fatalf("NewWithParams: %v", err)
	}
	if _, err := sc.DeriveAndVerify(nil); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestNewEmptyPassword(t *testing.T) {
	if _, _, err := NewWithParams(nil, fastParams); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestKeystoreUUIDIsRandomAndStable(t *testing.T) {
	a, _, err := NewWithParams([]byte("p"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := NewWithParams([]byte("p"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if a.KeystoreUUID == b.KeystoreUUID {
		t.Fatal("two fresh sidecars produced identical keystore UUIDs")
	}
	// And same sidecar twice produces same UUID bytes.
	u1, err := a.KeystoreUUIDBytes()
	if err != nil {
		t.Fatal(err)
	}
	u2, err := a.KeystoreUUIDBytes()
	if err != nil {
		t.Fatal(err)
	}
	if u1 != u2 {
		t.Fatal("KeystoreUUIDBytes is not stable across calls")
	}
}

func TestSaltIsFreshPerSidecar(t *testing.T) {
	a, _, _ := NewWithParams([]byte("p"), fastParams)
	b, _, _ := NewWithParams([]byte("p"), fastParams)
	if a.KDF.Salt == b.KDF.Salt {
		t.Fatal("two fresh sidecars produced identical salt")
	}
}

func TestUnmarshalRejectsBadVersion(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.Version = 99
	buf, _ := yaml.Marshal(sc) // bypass our Marshal to keep bad value
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestUnmarshalRejectsBadAlgorithm(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.KDF.Algorithm = "bcrypt"
	buf, _ := yaml.Marshal(sc)
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for unknown algorithm")
	}
}

func TestUnmarshalRejectsZeroKDFParams(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.KDF.Time = 0
	buf, _ := yaml.Marshal(sc)
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for zero KDF time")
	}
}

func TestUnmarshalRejectsBadUUIDLength(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.KeystoreUUID = base64.StdEncoding.EncodeToString([]byte("too short"))
	buf, _ := yaml.Marshal(sc)
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for wrong UUID length")
	}
}

func TestUnmarshalRejectsBadVerifierLength(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.Verifier.Nonce = base64.StdEncoding.EncodeToString([]byte("nope"))
	buf, _ := yaml.Marshal(sc)
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for wrong verifier nonce length")
	}
}

func TestUnmarshalRejectsMalformedBase64(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	sc.KDF.Salt = "@@@not-base64@@@"
	buf, _ := yaml.Marshal(sc)
	if _, err := Unmarshal(buf); err == nil {
		t.Fatal("expected error for malformed base64")
	}
}

func TestTamperedSaltCausesVerifierFail(t *testing.T) {
	password := []byte("hunter2")
	sc, _, err := NewWithParams(password, fastParams)
	if err != nil {
		t.Fatal(err)
	}
	// Generate a different salt, splice it in.
	otherSc, _, _ := NewWithParams(password, fastParams)
	sc.KDF.Salt = otherSc.KDF.Salt
	if _, err := sc.DeriveAndVerify(password); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("expected ErrWrongPassword, got %v", err)
	}
}

func TestTamperedVerifierCausesFail(t *testing.T) {
	password := []byte("p")
	sc, _, err := NewWithParams(password, fastParams)
	if err != nil {
		t.Fatal(err)
	}
	ct, _ := base64.StdEncoding.DecodeString(sc.Verifier.Ciphertext)
	ct[0] ^= 0x01
	sc.Verifier.Ciphertext = base64.StdEncoding.EncodeToString(ct)
	if _, err := sc.DeriveAndVerify(password); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("expected ErrWrongPassword, got %v", err)
	}
}

func TestMarshalIsHumanReadable(t *testing.T) {
	sc, _, _ := NewWithParams([]byte("p"), fastParams)
	buf, err := sc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// Spot-check that field names appear as expected — guards against an
	// accidental yaml tag rename breaking interop.
	for _, want := range []string{"version:", "keystore_uuid:", "kdf:", "algorithm:", "salt:", "verifier:", "nonce:", "ciphertext:"} {
		if !strings.Contains(string(buf), want) {
			t.Errorf("marshaled YAML missing %q\n--- yaml ---\n%s", want, buf)
		}
	}
}

func TestSidecarRelativePathStable(t *testing.T) {
	// This constant is part of the on-bucket layout; changing it without a
	// migration plan would orphan existing sidecars.
	if SidecarRelativePath != ".cbcrypt/keystore.v1.yaml" {
		t.Fatalf("SidecarRelativePath drifted to %q", SidecarRelativePath)
	}
}
