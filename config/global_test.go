package config

import (
	"testing"
	"cloudbackup/testutils"
	"os"
	"reflect"
)

// test loading config file with regular reporting from configor library
func TestLoad1(t *testing.T) {
	var compare = &Configuration{}
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_global_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, false)
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result) )
	}
}

// test loading config file with DEBUG(actually called Verbose) reporting from configor library
func TestLoad2(t *testing.T) {
	var compare = &Configuration{}
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_global_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, true)
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
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
		t.Fatal("Configuration file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
	var compare = &Configuration{}
	var invalid_config = []byte(`---
some: value`)
	var path = testutils.SetupFakeFile(invalid_config, "unittest_global_test_", t)
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