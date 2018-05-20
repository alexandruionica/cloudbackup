package config

import (
	"testing"
	"cloudbackup/utils"
	"cloudbackup/testutils"
	"os"
	"reflect"
)

// test loading client config file with regular reporting from configor library
func TestLoad1(t *testing.T) {
	var compare = Client{}
	path, err := utils.SetupTmpFileWithContent(testutils.MockClientYaml, "unittest_client_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, false)
	if err != nil {
		t.Fatalf("Could not load fake client config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result) )
	}
}

// test loading client config file with DEBUG(actually called Verbose) reporting from configor library
func TestLoad2(t *testing.T) {
	var compare = Client{}
	path, err := utils.SetupTmpFileWithContent(testutils.MockClientYaml, "unittest_client_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, true)
	if err != nil {
		t.Fatalf("Could not load fake client config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result) )
	}
}

// test loading missing config file
func TestLoad3(t *testing.T) {
	_, err := Load("a/file/which/does/not/exist", true)
	if err == nil {
		t.Fatal("Client config file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
	var compare= Client{}
	var invalidConfig = []byte(`zzzzzz
some: value`)
	path, err := utils.SetupTmpFileWithContent(invalidConfig, "unittest_client_config_test_")
	if err != nil {
		t.Fatal(err)
	}
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, true)
	if err == nil {
		t.Fatal("Invalid yaml config file should have caused an eror but didn't")
	}
	// we just ensure that we have the same type in the result as what we expect
	if compare == result {
	}
}

func TestCheckConfigOptionNotEmpty(t *testing.T) {
	err := CheckConfigOptionNotEmpty("bla", "bla_option")
	if err != nil {
		t.Fatalf("CheckConfigOptionNotEmpty() should not have raised error for non empty string but we got: " +
			"'%s'", err)
	}

	err = CheckConfigOptionNotEmpty("", "empty_option")
	if err == nil {
		t.Fatal("CheckConfigOptionNotEmpty() should  have raised error for empty string but it didn't")
	}

	err = CheckConfigOptionNotEmpty("  	   ", "bla_option")
	if err == nil {
		t.Fatal("CheckConfigOptionNotEmpty() should  have raised error for string containing whitespace and a" +
			" 'tab' but it didn't")
	}
}

func TestValidateAddress(t *testing.T) {
	testAddr := "http://blabla.org:81"
	err := ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: " +
			"'%s'", testAddr, err)
	}

	testAddr = "https://blabla.org:82"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: " +
			"'%s'", testAddr, err)
	}

	testAddr = "http://1.2.3.4:83"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: " +
			"'%s'", testAddr, err)
	}

	testAddr = "https://1.2.3.4:84"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: " +
			"'%s'", testAddr, err)
	}

	testAddr = "xyz://blabla.org:82"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}

	testAddr = "blabla.org:82"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}

	testAddr = "http://bla_bla.org:82"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}

	// missing colon and port number
	testAddr = "http://blabla.org"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}

	// missing port number (but colon present)
	testAddr = "http://blabla.org:"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}

	// missing protocol
	testAddr = "://blabla.org:443"
	err = ValidateAddress(testAddr)
	if err == nil {
		t.Fatalf("ValidateAddress() should have raised error for address '%s' but it didn't", testAddr)
	}
}

func TestValidate1(t *testing.T) {
	var clientConfig = Client{
		Username: "blablauser",
		Password: "34sdf234",
		Address: "http://127.0.0.1:880",
	}
	err := Validate(clientConfig, false)
	if err != nil {
		t.Fatalf("Could not validate mock client config. Error was: %s", err)
	}

	err = Validate(clientConfig, true)
	if err != nil {
		t.Fatalf("Could not validate mock client config. Error was: %s", err)
	}

	clientConfig.Address = "google.com"
	err = Validate(clientConfig, false)
	if err == nil {
		t.Fatalf("Validate mock client config should have failed but didn't. " +
			"Input structure was: %+v", clientConfig)
	}
}