package password

import (
	"log"
	"os"
	"strings"
	"testing"
)

// check if a generated hash looks like a hash; do not attempt to decrypt hash
func TestHashPassword(t *testing.T) {
	hashedPass, err := HashPassword("testpassword")
	if err != nil {
		t.Fatalf("Could not hash password. The encountered error was: %s \n", err)
	}
	// brcypt hashes should start with $2
	if strings.Index(hashedPass, "$2") != 0 {
		t.Fatalf("The password hash should start with '$2' but it starts with '%s' . Bcrypt "+
			"password hashes start with $2", hashedPass[0:2])
	}
}

func TestCheckPasswordHash1(t *testing.T) {
	// this is a hash of the string "testpassword"
	hash := "$2a$05$ErmQ.rgdVIrYJdGM8HEPguQSspE4NgegvXHCqcXnQqmlC79jAE64W"
	result := CheckPasswordHash("testpassword", hash)
	if result == false {
		t.Fatal("Could not validate a known hash and it's plaintext source")
	}
}

// hash and plaintext should not match
func TestCheckPasswordHash2(t *testing.T) {
	// this is a hash of the string "blabla"
	hash := "$2a$05$i1z/9.yRZbdmOpRiEy6rmO3wCpwiE7eKXgECei7LAfqAsaUJYUjd2"
	result := CheckPasswordHash("testpassword", hash)
	if result {
		t.Fatal("Hash and plaintext should have not validated but they did")
	}
}

// generate hash and try to then validate it
func TestCheckPasswordHash3(t *testing.T) {
	// this is a hash of the string "testpassword"
	hashedPass, err := HashPassword("testpassword")
	if err != nil {
		t.Fatalf("Could not hash password. The encountered error was: %s \n", err)
	}
	result := CheckPasswordHash("testpassword", hashedPass)
	if result == false {
		t.Fatal("Could not validate a known hash and it's plaintext source")
	}
}

// ReadPassFromCli succeeds
func TestReadPassFromCli1(t *testing.T) {
	content := []byte("testpassword\n")
	// we will use a file as a mock stdin
	tmpfile, err := os.CreateTemp("", "unittest_password_user_test_")
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = tmpfile.Close()
		err := os.Remove(tmpfile.Name())
		if err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := tmpfile.Write(content); err != nil {
		log.Fatal(err)
	}
	// file needs to be set to position 0 for the read
	if _, err := tmpfile.Seek(0, 0); err != nil {
		log.Fatal(err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }() // Restore original Stdin

	os.Stdin = tmpfile
	if _, err := ReadPassFromCli(); err != nil {
		t.Errorf("ReadPassFromCli failed with: '%v' but it was expected to succeed", err)
	}
}

// test ReadPassFromCli fails due to empty password
func TestReadPassFromCli2(t *testing.T) {
	content := []byte("\n")
	// we will use a file as a mock stdin
	tmpfile, err := os.CreateTemp("", "unittest_password_user_test_")
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = tmpfile.Close()
		err := os.Remove(tmpfile.Name())
		if err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := tmpfile.Write(content); err != nil {
		log.Fatal(err)
	}
	// file needs to be set to position 0 for the read
	if _, err := tmpfile.Seek(0, 0); err != nil {
		log.Fatal(err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }() // Restore original Stdin

	os.Stdin = tmpfile
	if _, err := ReadPassFromCli(); err == nil {
		t.Error("ReadPassFromCli succeed but it should have failed")
	}
}

// test ReadPassFromCli fails due to stdin being by default /dev/null during unit testing
func TestReadPassFromCli3(t *testing.T) {
	if _, err := ReadPassFromCli(); err == nil {
		t.Error("ReadPassFromCli succeed but it should have failed")
	}
}

// ReadPassFromCli succeeds and resulted hash is usable
func TestReadPassFromCli4(t *testing.T) {
	content := []byte("testpassword\n")
	// we will use a file as a mock stdin
	tmpfile, err := os.CreateTemp("", "unittest_password_user_test_")
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = tmpfile.Close()
		err := os.Remove(tmpfile.Name())
		if err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := tmpfile.Write(content); err != nil {
		log.Fatal(err)
	}
	// file needs to be set to position 0 for the read
	if _, err := tmpfile.Seek(0, 0); err != nil {
		log.Fatal(err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }() // Restore original Stdin

	os.Stdin = tmpfile
	hashedPass, err := ReadPassFromCli()
	if err != nil {
		t.Errorf("ReadPassFromCli failed with: '%v' but it was expected to succeed", err)
	}

	result := CheckPasswordHash("testpassword", hashedPass)
	if result == false {
		t.Fatal("Could not validate a known hash and it's plaintext source")
	}
}
