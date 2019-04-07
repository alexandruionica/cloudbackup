package config

import (
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"os"
	"reflect"
	"runtime"
	"testing"
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

	result, _, err := Load(path, false, "", "", "")
	if err != nil {
		t.Fatalf("Could not load fake client config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result))
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

	result, _, err := Load(path, true, "", "", "")
	if err != nil {
		t.Fatalf("Could not load fake client config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result))
	}
}

// test loading missing config file
func TestLoad3(t *testing.T) {
	_, _, err := Load("a/file/which/does/not/exist", true, "", "", "")
	if err == nil {
		t.Fatal("Client config file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
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

	_, _, err = Load(path, true, "", "", "")
	if err == nil {
		t.Fatal("Invalid yaml config file should have caused an error but didn't")
	}
}

// test loading client config file with regular reporting from configor library and all overrides specified on command line
func TestLoad5(t *testing.T) {
	testData := Client{
		Username: "someuser",
		Password: "somepass",
		Address:  "http://7.7.7.7:9999",
	}
	result, _, err := Load("", false, testData.Username, testData.Password, testData.Address)
	if err != nil {
		t.Fatalf("Could not load fake configuration options. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(testData) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(testData),
			reflect.TypeOf(result))
	}
	// check data matches
	if result.Username != testData.Username || result.Password != testData.Password || result.Address != testData.Address {
		t.Fatalf("Result from Load() was: %+v but we were expecting %+v", result, testData)
	}
}

// test loading client config file with regular reporting from configor library and all overrides specified on command
// line but the address override is incorrect in format
func TestLoad6(t *testing.T) {
	testData := Client{
		Username: "someuser",
		Password: "somepass",
		Address:  "ftp://1.2.3.4:21",
	}
	result, _, err := Load("", false, testData.Username, testData.Password, testData.Address)
	if err == nil {
		t.Fatal("Expected error for Load() when passing in an address in an incorrect format but we didn't " +
			"get any error")
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(testData) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(testData),
			reflect.TypeOf(result))
	}
}

func TestCheckConfigOptionNotEmpty(t *testing.T) {
	err := CheckConfigOptionNotEmpty("bla", "bla_option")
	if err != nil {
		t.Fatalf("CheckConfigOptionNotEmpty() should not have raised error for non empty string but we got: "+
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
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: "+
			"'%s'", testAddr, err)
	}

	testAddr = "https://blabla.org:82"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: "+
			"'%s'", testAddr, err)
	}

	testAddr = "http://1.2.3.4:83"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: "+
			"'%s'", testAddr, err)
	}

	testAddr = "https://1.2.3.4:84"
	err = ValidateAddress(testAddr)
	if err != nil {
		t.Fatalf("ValidateAddress() should not have raised error for address '%s' but we got: "+
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
		Address:  "http://127.0.0.1:880",
	}
	err := Validate(clientConfig)
	if err != nil {
		t.Fatalf("Could not validate mock client config. Error was: %s", err)
	}

	clientConfig.Address = "google.com"
	err = Validate(clientConfig)
	if err == nil {
		t.Fatalf("Validate mock client config should have failed but didn't. "+
			"Input structure was: %+v", clientConfig)
	}

	clientConfig = Client{
		Username: "blablauser",
		Password: "34sdf234",
		Address:  "",
	}
	err = Validate(clientConfig)
	if err == nil {
		t.Fatalf("Validate mock client config should have failed but didn't. "+
			"Input structure was: %+v", clientConfig)
	}

	clientConfig = Client{
		Username: "blablauser",
		Password: "",
		Address:  "http://127.0.0.1:880",
	}
	err = Validate(clientConfig)
	if err == nil {
		t.Fatalf("Validate mock client config should have failed but didn't. "+
			"Input structure was: %+v", clientConfig)
	}

	clientConfig = Client{
		Username: "",
		Password: "34sdf234",
		Address:  "http://127.0.0.1:880",
	}
	err = Validate(clientConfig)
	if err == nil {
		t.Fatalf("Validate mock client config should have failed but didn't. "+
			"Input structure was: %+v", clientConfig)
	}
}

func TestSanitizeClientConfig(t *testing.T) {
	var clientConfig = Client{
		Username: "blablauser",
		Password: "34sdf234",
		Address:  "http://127.0.0.1:880",
	}
	result := SanitizeClientConfig(clientConfig)
	if result.Password != SecretReplace {
		t.Fatalf("Expected client config password to be %s but it remains %s", SecretReplace, result.Password)
	}
}

func TestSanitizeCheckIfOptionOrEnvVars1(t *testing.T) {
	envVar1 := "ENVVAR1"
	envVar2 := "EnvVar2"
	err := os.Unsetenv(envVar1)
	if err != nil {
		t.Fatalf("Recevied error while trying to unset an environment variable as part of the test setup."+
			" Err was: %s", err)
	}
	err = os.Unsetenv(envVar2)
	if err != nil {
		t.Fatalf("1. Recevied error while trying to unset an environment variable as part of the test setup."+
			" Err was: %s", err)
	}
	result := CheckIfOptionOrEnvVars("", envVar1, envVar2)
	if result {
		t.Fatalf("2. Expected CheckIfOptionOrEnvVars() to return false but it returned %+v", result)
	}

	result = CheckIfOptionOrEnvVars("bla", envVar1, envVar2)
	if result == false {
		t.Fatalf("3. Expected CheckIfOptionOrEnvVars() to return true but it returned %+v", result)
	}

	err = os.Setenv(envVar1, "somevalue")
	if err != nil {
		t.Fatalf("4. Recevied error while trying to set an environment variable as part of the test."+
			" Err was: %s", err)
	}
	result = CheckIfOptionOrEnvVars("", envVar1, envVar2)
	if result == false {
		t.Fatalf("5. Expected CheckIfOptionOrEnvVars() to return true but it returned %+v", result)
	}
	err = os.Unsetenv(envVar1)
	if err != nil {
		t.Fatalf("6. Recevied error while trying to unset an environment variable as part of the test cleanup."+
			" Err was: %s", err)
	}

	err = os.Setenv(envVar2, "SomeOtherValue")
	if err != nil {
		t.Fatalf("7. Recevied error while trying to set an environment variable as part of the test."+
			" Err was: %s", err)
	}
	result = CheckIfOptionOrEnvVars("", envVar1, envVar2)
	if result == false {
		t.Fatalf("8. Expected CheckIfOptionOrEnvVars() to return true but it returned %+v", result)
	}
	err = os.Unsetenv(envVar2)
	if err != nil {
		t.Fatalf("9. Recevied error while trying to unset an environment variable as part of the test cleanup."+
			" Err was: %s", err)
	}
}

func TestRetrieveClientConfigFilePath1(t *testing.T) {
	result, err := RetrieveClientConfigFilePath("somepath")
	if err != nil {
		t.Fatalf("Recevied error while trying running RetrieveClientConfigFilePath(). Error was: %s", err)
	}
	if result != "somepath" {
		t.Fatalf("RetrieveClientConfigFilePath() was expected to return 'somepath' but it returned: %s", result)
	}
}

func TestRetrieveClientConfigFilePath2(t *testing.T) {
	if runtime.GOOS == "windows" {
		err := os.Setenv("HomeDrive", "C:")
		if err != nil {
			t.Fatalf("1. Recevied error while trying to set an environment variable as part of the test."+
				" Err was: %s", err)
		}
		err = os.Setenv("HomePath", `\somepath\someuser`)
		if err != nil {
			t.Fatalf("2. Recevied error while trying to set an environment variable as part of the test."+
				" Err was: %s", err)
		}
		expectedResult := `C:\somepath\someuser` + string(os.PathSeparator) + defaultClientConfigFile
		result, err := RetrieveClientConfigFilePath("")
		if err != nil {
			t.Fatalf("Recevied error while trying running RetrieveClientConfigFilePath(). Error was: %s", err)
		}
		if result != expectedResult {
			t.Fatalf("RetrieveClientConfigFilePath() was expected to return %s but it returned: %s",
				expectedResult, result)
		}
		// otherwise we're running some kind of Unix or Linux
	} else {
		err := os.Setenv("HOME", `/home/someuser`)
		if err != nil {
			t.Fatalf("1. Recevied error while trying to set an environment variable as part of the test."+
				" Err was: %s", err)
		}
		expectedResult := `/home/someuser` + string(os.PathSeparator) + defaultClientConfigFile
		result, err := RetrieveClientConfigFilePath("")
		if err != nil {
			t.Fatalf("Recevied error while trying running RetrieveClientConfigFilePath(). Error was: %s", err)
		}
		if result != expectedResult {
			t.Fatalf("RetrieveClientConfigFilePath() was expected to return %s but it returned: %s",
				expectedResult, result)
		}
	}
}

func TestRetrieveClientConfigFilePath3(t *testing.T) {
	if runtime.GOOS == "windows" {
		err := os.Unsetenv("HomeDrive")
		if err != nil {
			t.Fatalf("1. Recevied error while trying to unset an environment variable as part of the test."+
				" Err was: %s", err)
		}
		err = os.Unsetenv("HomePath")
		if err != nil {
			t.Fatalf("2. Recevied error while trying to set an environment variable as part of the test."+
				" Err was: %s", err)
		}
		_, err = RetrieveClientConfigFilePath("")
		if err == nil {
			t.Fatal("Did not recevied error when running RetrieveClientConfigFilePath() without input and when" +
				" ensuring required environment variables were missing.")
		}
		// otherwise we're running some kind of Unix or Linux
	} else {
		err := os.Unsetenv("HOME")
		if err != nil {
			t.Fatalf("Recevied error while trying to unset an environment variable as part of the test."+
				" Err was: %s", err)
		}
		_, err = RetrieveClientConfigFilePath("")
		if err == nil {
			t.Fatal("Did not recevied error when running RetrieveClientConfigFilePath() without input and when" +
				" ensuring required environment variables were missing.")
		}
	}
}
