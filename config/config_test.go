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
	var compare = &RuntimeConfig{}
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
	var compare = &RuntimeConfig{}
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
		t.Fatal("RuntimeConfig file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
	var compare = &RuntimeConfig{}
	var invalidConfig = []byte(`zzzzzz
some: value`)
	var path = testutils.SetupTmpFileWithContent(invalidConfig, "unittest_config_test_", t)
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
	var path = testutils.SetupTmpFileWithContent(testutils.MockYamlInvalidConfig1, "unittest_config_test_", t)
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
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

// validate valid config yaml
func TestValidate1(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
	err = ValidateBackup(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to missing encyption password")
	}
}

// valid yaml with invalid versioning setting
func TestValidate2(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
	err = ValidateBackup(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to versioning being enabled but not" +
			" versions_max_age or versions_max_num are having default values")
	}
}

func TestValidate3(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
		t.Fatal("Config file loaded successfully but should have failed due to versions_max_age being set and" +
			" versioning being disabled ")
	}
	err = ValidateBackup(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to versions_max_age being set and" +
			" versioning being disabled ")
	}
}

func TestValidate4(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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
		t.Fatal("Config file loaded successfully but should have failed due to VersionsMaxNum > 0  and" +
			" versioning being disabled ")
	}
	err = ValidateBackup(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to VersionsMaxNum > 0  and" +
			" versioning being disabled ")
	}
}

// validate data dir using absolute path which does not exist
func TestValidate5(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.DataDir = "/a/missing/folder/which/should/not/exist"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to data_dir using absolute path which " +
			"does not exist")
	}

	err = ValidateTopLevelDataDir(result.Config, true)
	if err == nil {
		t.Fatal("data_dir validates successfully but should have failed due to using absolute path which " +
			"does not exist")
	}
}

func TestValidate6(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.DataDir = "relative_path_which_does_not_exist"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to data_dir using a relative path " +
			"which does not exist")
	}

	err = ValidateTopLevelDataDir(result.Config, true)
	if err == nil {
		t.Fatal("data_dir validates successfully but should have failed due to using a relative path which " +
			"does not exist")
	}
}

func TestValidate7(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.Https.Enabled = true
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to HTTPS being enabled but " +
			"ssl_cert_path not being specified")
	}

	err = ValidateHttps(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validates but should have failed due to HTTPS being enabled but ssl_cert_path not " +
			"being specified")
	}
}

func TestValidate8(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/a/missing/file"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to HTTPS being enabled but " +
			"ssl_key_path not being specified")
	}

	err = ValidateHttps(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validates but should have failed due to HTTPS being enabled but ssl_key_path not " +
			"being specified")
	}
}

func TestValidate9(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/a/missing/file"
	result.Config.Https.SslKeyPath = "/another/missing/file"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to HTTPS being enabled due to " +
			"inexistent file specified as value of ssl_cert_path")
	}

	err = ValidateHttps(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validates but should have failed due to HTTPS being enabled due to " +
			"inexistent file specified as value of ssl_cert_path")
	}
}

func TestValidate10(t *testing.T) {
	var path = testutils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_", t)
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

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/etc/services"
	result.Config.Https.SslKeyPath = "/another/missing/file"
	err = Validate(result.Config)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to HTTPS being enabled due to " +
			"inexistent file specified as value of ssl_key_path")
	}

	err = ValidateHttps(result.Config, true)
	if err == nil {
		t.Fatal("Config struct validates but should have failed due to HTTPS being enabled due to " +
			"inexistent file specified as value of ssl_key_path")
	}
}