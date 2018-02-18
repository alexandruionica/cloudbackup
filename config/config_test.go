package config

import (
	"testing"
	"cloudbackup/testutils"
	"os"
	"reflect"
	"sync"
)

// test loading config file with regular reporting from configor library
func TestLoad1(t *testing.T) {
	var compare = &Configuration{}
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, false, &sync.Mutex{})
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
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, true, &sync.Mutex{})
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
	_, err := Load("a/file/which/does/not/exist", true, &sync.Mutex{})
	if err == nil {
		t.Fatal("Configuration file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
	var compare = &Configuration{}
	var invalidConfig = []byte(`---
some: value`)
	var path = testutils.SetupFakeFile(invalidConfig, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, true, &sync.Mutex{})
	if err == nil {
		t.Fatal("Invalid yaml config file should have caused an eror but didn't")
	}
	// we just ensure that we have the same type in the result as what we expect
	if compare == result {
	}
}

// test loading config file with missing encryption password
func TestLoad5(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYamlInvalidConfig1, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err := Load(path, false, &sync.Mutex{})
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to missing encyption password")
	}
}

func TestConfiguration_GetWithLock(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result, err := Load(path, false, &sync.Mutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.GetWithLock(loggingContext)
	expectedString := "first_backup"
	// we just ensure that we have the same string in the result as what we expect
	if result.GetWithLock(loggingContext).Backup[0].Name != expectedString {
		t.Fatalf("The result should have been '%s' but is '%s' ", expectedString,
			result.GetWithLock(loggingContext).Backup[0].Name )
	}
}

func TestValidate1(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := Load(path, false, &sync.Mutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Encrypt = true
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to missing encyption password")
	}
}

func TestValidate2(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := Load(path, false, &sync.Mutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Versioning = true
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to versioning being enabled but not" +
			" versions_max_age or versions_max_num are having default values")
	}
}

func TestValidate3(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := Load(path, false, &sync.Mutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].VersionsMaxAge = "10w"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to versions_max_age being set but" +
			" versioning being disabled ")
	}
}

func TestValidate4(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_config_test_", t)
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()

	result , err := Load(path, false, &sync.Mutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].VersionsMaxNum = 5
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to VersionsMaxNum > 0  but" +
			" versioning being disabled ")
	}
}