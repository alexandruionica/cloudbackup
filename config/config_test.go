package config

import (
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"os"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

// test loading config file with regular reporting from configor library
func TestLoad1(t *testing.T) {
	var compare = &shared.RuntimeConfig{}
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result))
	}
}

// test loading config file with DEBUG(actually called Verbose) reporting from configor library
func TestLoad2(t *testing.T) {
	var compare = &shared.RuntimeConfig{}
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, true, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	// we just ensure that we have the same type in the result as what we expect
	if reflect.TypeOf(compare) != reflect.TypeOf(result) {
		t.Fatalf("Type of result should have been '%s' but is '%s' ", reflect.TypeOf(compare),
			reflect.TypeOf(result))
	}
}

// test loading missing config file
func TestLoad3(t *testing.T) {
	_, err := Load("a/file/which/does/not/exist", true, &sync.RWMutex{})
	if err == nil {
		t.Fatal("RuntimeConfig file load should have failed due to missing file but instead succeeded")
	}
}

// test loading valid yaml but invalid config file
func TestLoad4(t *testing.T) {
	var invalidConfig = []byte(`zzzzzz
some: value`)
	path, err := utils.SetupTmpFileWithContent(invalidConfig, "unittest_config_test_")
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

	_, err = Load(path, true, &sync.RWMutex{})
	if err == nil {
		t.Fatal("Invalid yaml config file should have caused an eror but didn't")
	}
}

// test loading config file with missing encryption password
func TestLoad5(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYamlInvalidConfig1, "unittest_config_test_")
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

	_, err = Load(path, false, &sync.RWMutex{})
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to missing encyption password")
	}
}

// test loading config file with invalid user password hash. This should trigger a different kind of validation failure
func TestLoad6(t *testing.T) {
	path, err := utils.SetupTmpFileWithContent(testutils.MockYamlInvalidConfig2, "unittest_config_test_")
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

	_, err = Load(path, false, &sync.RWMutex{})
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to a user having an invalid password" +
			" hash")
	}
}

func TestConfiguration_GetCopyWithLock(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result.GetCopyWithLock(loggingContext)
	expectedString := "first_backup"
	// we just ensure that we have the same string in the result as what we expect
	if result.GetCopyWithLock(loggingContext).Backup[0].Name != expectedString {
		t.Fatalf("The result should have been '%s' but is '%s' ", expectedString,
			result.GetCopyWithLock(loggingContext).Backup[0].Name)
	}
}

// validate valid config yaml
func TestValidate0(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file did not load successfully but should have")
	}
	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatal("Config struct did not validat but should have")
	}
}

// validate invalid config (yaml is valid but once loaded we change a setting to make Struct fail validation)
func TestValidate1(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Encrypt = true
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to missing encyption password")
	}
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to missing encyption password")
	}
}

// TODO - validate various values for VersionsMaxAge parameter (below is an old test which needs adjusting)
//func TestValidate3(t *testing.T) {
//	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_config_test_")
//	if err != nil {
//		t.Fatal(err)
//	}
//	// remove tmpfile which holds the yaml as the config has been parsed and loaded
//	defer func() {
//		err := os.Remove(path)
//		if err != nil {
//			t.Fatal(err)
//		}
//	}()
//
//	result , err := Load(path, false, &sync.RWMutex{})
//	if err != nil {
//		t.Fatalf("Could not load fake config file. Error was: %s", err)
//	}
//
//	result.Config.ConfigBackup[0].VersionsMaxAge = "10w"
//	err = Validate(result.Config, false)
//	if err == nil {
//		t.Fatal("Config file loaded successfully but should have failed due to versions_max_age being set and" +
//			" versioning being disabled ")
//	}
//	err = ValidateBackup(result.Config.ConfigBackup, true)
//	if err == nil {
//		t.Fatal("Config struct validated but should have failed due to versions_max_age being set and" +
//			" versioning being disabled ")
//	}
//}

// validate data_dir using absolute path which does not exist
func TestValidate5(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.DataDir = "/a/missing/folder/which/should/not/exist"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to data_dir using absolute path which " +
			"does not exist")
	}

	err = ValidateDir(result.Config.DataDir, "data_dir", true)
	if err == nil {
		t.Fatal("data_dir validates successfully but should have failed due to using absolute path which " +
			"does not exist")
	}
}

func TestValidate6(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.DataDir = "relative_path_which_does_not_exist"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to data_dir using a relative path " +
			"which does not exist")
	}

	err = ValidateDir(result.Config.DataDir, "data_dir", true)
	if err == nil {
		t.Fatal("data_dir validates successfully but should have failed due to using a relative path which " +
			"does not exist")
	}
}

func TestValidate7(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Https.Enabled = true
	err = Validate(result.Config, false)
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
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/a/missing/file"
	err = Validate(result.Config, false)
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
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/a/missing/file"
	result.Config.Https.SslKeyPath = "/another/missing/file"
	err = Validate(result.Config, false)
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
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Https.Enabled = true
	result.Config.Https.SslCertPath = "/etc/services"
	result.Config.Https.SslKeyPath = "/another/missing/file"
	err = Validate(result.Config, false)
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

// two users with the same name
func TestValidate11(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	result.Config.User = append(result.Config.User, result.Config.User[0])
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to two users having the same name")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, false)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to two users having the same name")
	}
}

// user with invalid password hash
func TestValidate12(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	result.Config.User[0].Pass = "blabla"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to invalid password hash")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, false)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to invalid password hash")
	}
}

// two backups with the same name
func TestValidate13(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	result.Config.Backup = append(result.Config.Backup, result.Config.Backup[0])
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to two backups having the same name")
	}
	// validate also individual function
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to two backups having the same name")
	}
}

// two backups targets the same name belonging to one backup
func TestValidate14(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	result.Config.Backup[0].Target = append(result.Config.Backup[0].Target, result.Config.Backup[0].Target[0])
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to two backups targets the same " +
			"name belonging to one backup")
	}
	// validate also individual function
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatal("Config struct validated but should have failed due to two backups targets the same name " +
			"belonging to one backup")
	}
}

// users with password hash set to "******"
func TestValidate15(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	result.Config.User[0].Pass = "***************"
	err = Validate(result.Config, true)
	if err != nil {
		t.Fatal("Config file did not load successfully but should have when a users passsowrd is set to '******'" +
			" and Validate is called with hiddenPass=true")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, true)
	if err != nil {
		t.Fatal("Config struct did not load successfully but should have when a users passsowrd is set to '******'" +
			" and ValidateUser is called with hiddenPass=true")
	}
}

// users with password hash set to "******BLA"
func TestValidate16(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	result.Config.User[0].Pass = "******BLA"
	err = Validate(result.Config, true)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have not when a users passsowrd is set to '******BLA'" +
			" and Validate is called with hiddenPass=true")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, true)
	if err == nil {
		t.Fatal("Config struct loaded successfully but should have not when a users passsowrd is set to '******BLA'" +
			" and ValidateUser is called with hiddenPass=true")
	}
}

// validate User's 'access' key (allowed values should be only 'read', 'write')
func TestValidate17(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	// #### invalid value
	result.Config.User[0].Access = "bla"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have not when a users 'access' key is set to 'bla'")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, false)
	if err == nil {
		t.Fatal("Config struct loaded successfully but should have not when a users 'access' key is set to 'bla")
	}

	// ##### valid value 'read'
	result.Config.User[0].Access = "read"
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file failed to load but should have when a users 'access' key is set to 'read'")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, false)
	if err != nil {
		t.Fatal("Config struct failed to load but should have when a users 'access' key is set to 'read'")
	}
	// ##### valid value 'write'
	result.Config.User[0].Access = "write"
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file failed to load but should have when a users 'access' key is set to 'write'")
	}
	// validate also individual function
	err = ValidateUser(result.Config, true, false)
	if err != nil {
		t.Fatal("Config struct failed to load but should have when a users 'access' key is set to 'write'")
	}
}

// validate html_dir and html_dir using absolute path which does not exist
func TestValidate18(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.HtmlDir = "/a/missing/folder/which/should/not/exist"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to html_dir using absolute path which " +
			"does not exist")
	}

	err = ValidateDir(result.Config.HtmlDir, "html_dir", true)
	if err == nil {
		t.Fatal("html_dir validates successfully but should have failed due to using absolute path which " +
			"does not exist")
	}
}

func TestValidate19(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.HtmlDir = "relative_path_which_does_not_exist"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to html_dir using a relative path " +
			"which does not exist")
	}

	err = ValidateDir(result.Config.HtmlDir, "html_dir", true)
	if err == nil {
		t.Fatal("html_dir validates successfully but should have failed due to using a relative path which " +
			"does not exist")
	}
}

// check that a backup "Name" containing non ASCII characters is not permitted
func TestValidate21(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	result.Config.Backup[0].Name = "backupöüÂ"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file loaded successfully but should have failed due to the backup 'Name' of the first " +
			"backup containing non ASCII characters")
	}
}

// backup target type which is known to be working
func TestValidate22(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	targetType := "aws_s3"
	result.Config.Backup[0].Target[0].Type = targetType
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatalf("Config file failed to load successfully but should have because target type '%s' should "+
			"be allowed", targetType)
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatalf("Config struct failed to validate but should have because target type '%s' should "+
			"be allowed", targetType)
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("Targets config struct failed to validate but should have because target type '%s' should "+
			"be allowed", targetType)
	}
}

// HIDDEN backup target type which is known to be working
func TestValidate23(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	targetType := "test_null"
	result.Config.Backup[0].Target[0].Type = targetType
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatalf("Config file failed to load successfully but should have because target type '%s' should "+
			"be allowed", targetType)
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatalf("Config struct failed to validate but should have because target type '%s' should "+
			"be allowed", targetType)
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatalf("Targets config struct failed to validate but should have because target type '%s' should "+
			"be allowed", targetType)
	}
}

// backup target type which is UNKNOWN
func TestValidate24(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	targetType := "madeup_type"
	result.Config.Backup[0].Target[0].Type = targetType
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatalf("Config file should have failed to load successfully but didn't despite target type '%s' not "+
			"being  allowed", targetType)
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatalf("Config struct should have failed to load successfully but didn't despite target type '%s' not "+
			"being  allowed", targetType)
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err == nil {
		t.Fatalf("Targets config struct should have failed to load successfully but didn't despite target type"+
			" '%s' not being  allowed", targetType)
	}
}

// rate limit < 0
func TestValidate25(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	result.Config.Backup[0].Target[0].RateLimit = "-100 KB"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file should have failed to load successfully but didn't despite rate limit < 0 which " +
			"is not allowed")
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatal("Config struct should have failed to load successfully but didn't despite rate limit < 0 " +
			"which is not allowed")
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err == nil {
		t.Fatal("Targets config struct should have failed to load successfully but didn't despite rate limit" +
			" < 0 which is not  allowed")
	}
}

// rate limit is not decodable
func TestValidate26(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	result.Config.Backup[0].Target[0].RateLimit = "101 BB"
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatal("Config file should have failed to load successfully but didn't despite rate limit < 0 which " +
			"is not allowed")
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatal("Config struct should have failed to load successfully but didn't despite rate limit < 0 " +
			"which is not allowed")
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err == nil {
		t.Fatal("Targets config struct should have failed to load successfully but didn't despite rate limit" +
			" < 0 which is not  allowed")
	}
}

// rate limit is valid decodable
func TestValidate27(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	result.Config.Backup[0].Target[0].RateLimit = "703 KB"
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err != nil {
		t.Fatal("Config file should have loaded successfully but didn't despite rate limit being valid")
	}
}

// PostRunScript fails because the script doesn't exist
func TestValidate28(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	scriptPath := testutils.GenerateTmpFilePath(t, "TestValidate28_missing_file_", ".sh")
	result.Config.Backup[0].PostRunScript = scriptPath
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatalf("Validate() reports that config file loaded successfully; but shouldn't have because "+
			"'PostRunScript' '%s' does not exist", scriptPath)
	}

	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatalf("ValidateBackup() reports that config file loaded successfully; but shouldn't have because "+
			"'PostRunScript' '%s' does not exist", scriptPath)
	}

	err = ValidatePrePostRunScript(scriptPath, "post", "test_backup", true)
	if err == nil {
		t.Fatalf("ValidatePrePostRunScript() reports that '%s' exists despite it not", scriptPath)
	}

	err = isExecutable(scriptPath)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatalf("isExecutable() reports that '%s' is executable despite it not even existing", scriptPath)
		}
	}
}

// PreRunScript fails because the script doesn't exist
func TestValidate29(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	scriptPath := testutils.GenerateTmpFilePath(t, "TestValidate29_missing_file_", ".sh")
	result.Config.Backup[0].PreRunScript = scriptPath
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatalf("Validate() reports that config file loaded successfully; but shouldn't have because "+
			"'PreRunScript' '%s' does not exist", scriptPath)
	}

	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatalf("ValidateBackup() reports that config file loaded successfully; but shouldn't have because "+
			"'PreRunScript' '%s' does not exist", scriptPath)
	}

	err = ValidatePrePostRunScript(scriptPath, "pre", "test_backup", true)
	if err == nil {
		t.Fatalf("ValidatePrePostRunScript() reports that '%s' exists despite it not", scriptPath)
	}

	err = isExecutable(scriptPath)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatalf("isExecutable() reports that '%s' is executable despite it not even existing", scriptPath)
		}
	}
}

// PostRunScript fails because the script doesn't exist
func TestValidate30(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	var scriptPath string
	if runtime.GOOS == "windows" {
		scriptPath = testutils.GenerateTmpFilePath(t, "TestValidate30_existing_file_", ".ps1")
	} else {
		scriptPath = testutils.GenerateTmpFilePath(t, "TestValidate30_existing_file_", ".sh")
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	f, err := os.Create(scriptPath)
	if err != nil {
		t.Fatalf("Could not create file %s", scriptPath)
	}
	defer func() {
		_ = f.Close() // #nosec
	}()
	if _, err := f.Write([]byte("some content")); err != nil {
		t.Fatalf("Could not write to %s", scriptPath)
	}

	err = isExecutable(scriptPath)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatalf("isExecutable() should have reported an error as %s is not executable; but it didn't",
				scriptPath)
		}
	}

	// make ScriptPath executable and repeat test; on Windows it's not needed (or supported to make executable)
	if runtime.GOOS != "windows" {
		err = os.Chmod(scriptPath, 0700)
		if err != nil {
			t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
		}
	}
	err = isExecutable(scriptPath)
	if err != nil {
		t.Fatalf("isExecutable() produced error while evaluation %s but it was expected to have the execute bit"+
			" set. The reported error was: %s", scriptPath, err)
	}

	result.Config.Backup[0].PostRunScript = scriptPath
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatalf("Validate() reports that config file did not load successfully due to err: %s", err)
	}

	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatalf("ValidateBackup() reports that config file did not load successfully due to err: %s", err)
	}

	err = ValidatePrePostRunScript(scriptPath, "post", "test_backup", true)
	if err != nil {
		t.Fatalf("ValidatePrePostRunScript() errored with: %s", err)
	}
}

// PreRunScript fails because the script doesn't exist
func TestValidate31(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}
	var scriptPath string
	if runtime.GOOS == "windows" {
		scriptPath = testutils.GenerateTmpFilePath(t, "TestValidate31_existing_file_", ".ps1")
	} else {
		scriptPath = testutils.GenerateTmpFilePath(t, "TestValidate31_existing_file_", ".sh")
	}
	defer testutils.DeleteTestFilesAndDirs([]string{scriptPath})

	f, err := os.Create(scriptPath)
	if err != nil {
		t.Fatalf("Could not create file %s", scriptPath)
	}
	defer func() {
		_ = f.Close() // #nosec
	}()
	if _, err := f.Write([]byte("some content")); err != nil {
		t.Fatalf("Could not write to %s", scriptPath)
	}

	err = isExecutable(scriptPath)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatalf("isExecutable() should have reported an error as %s is not executable; but it didn't",
				scriptPath)
		}
	}

	// make ScriptPath executable and repeat test; on Windows it's not needed (or supported to make executable)
	if runtime.GOOS != "windows" {
		err = os.Chmod(scriptPath, 0700)
		if err != nil {
			t.Fatalf("Could not make executable %s due to error: %s", scriptPath, err)
		}
	}

	err = isExecutable(scriptPath)
	if err != nil {
		t.Fatalf("isExecutable() produced error while evaluation %s but it was expected to have the execute bit"+
			" set. The reported error was: %s", scriptPath, err)
	}

	result.Config.Backup[0].PreRunScript = scriptPath
	err = Validate(result.Config, false)
	if err != nil {
		t.Fatalf("Validate() reports that config file did not load successfully due to err: %s", err)
	}

	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatalf("ValidateBackup() reports that config file did not load successfully due to err: %s", err)
	}

	err = ValidatePrePostRunScript(scriptPath, "pre", "test_backup", true)
	if err != nil {
		t.Fatalf("ValidatePrePostRunScript() errored with: %s", err)
	}
}

// duplicate parameter in config/backup/target/params
func TestValidate33(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have a backup section but we're trying to validate backup related code")
	}

	if len(result.Config.Backup[0].Target[0].Parameters) == 0 {
		t.Fatalf("No custom parameters define so there is nothing to test on. Fix the config file used as input")
	}
	result.Config.Backup[0].Target[0].Parameters = append(result.Config.Backup[0].Target[0].Parameters, result.Config.Backup[0].Target[0].Parameters[0])
	err = Validate(result.Config, false)
	if err == nil {
		t.Fatalf("1. Config file should have failed to validate successfully but didn't despite having " +
			"duplicate parameters in the Backup.Target.Parameters section")
	}
	// validate also individual functions
	err = ValidateBackup(result.Config.Backup, true)
	if err == nil {
		t.Fatalf("2. Config file should have failed to validate successfully but didn't despite having " +
			"duplicate parameters in the Backup.Target.Parameters section")
	}
	err = ValidateBackupTarget(result.Config.Backup[0].Target, true, result.Config.Backup[0].Name)
	if err == nil {
		t.Fatalf("3. Config file should have failed to validate successfully but didn't despite having " +
			"duplicate parameters in the Backup.Target.Parameters section")
	}
}

func TestCheckStringIsOnly(t *testing.T) {
	if CheckStringIsOnly("************", "*") != true {
		t.Fatal("CheckStringIsOnly() did not return a match as expected")
	}
}

func TestCheckStringIsOnly2(t *testing.T) {
	if CheckStringIsOnly("*******ERWER", "*") {
		t.Fatal("CheckStringIsOnly() did return a match but this should not happened")
	}
}

func TestCheckStringIsOnly3(t *testing.T) {
	if CheckStringIsOnly("", "*") {
		t.Fatal("CheckStringIsOnly() did return a match but this should not happened as we passed in an empty " +
			"string to check")
	}
}

// check that for fully populated configs (actual hash in the password field) we don't get an error
func TestCopyPasswordsFromOldConfig(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have Backup section but we're trying to validate Backup related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err != nil {
		t.Fatalf("Old config and new config both have password hashes for various entries but "+
			"CopyPasswordsFromOldConfig() returned error: %s", err)
	}
}

// check that "****" password actually get replaced with hashes
func TestCopyPasswordsFromOldConfig2(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}
	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}
	if len(result.Config.Backup[0].Target) == 0 {
		t.Fatal("Config file doesn't have 'Backup[0].Target' section but we're trying to validate Backup.Target" +
			" related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	NewConfig.User[0].Pass = SecretReplace
	NewConfig.Backup[1].EncryptPass = SecretReplace
	foundSecretKey := false
	for _, entry := range NewConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			entry.Value = SecretReplace
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sample backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err != nil {
		t.Fatalf("CopyPasswordsFromOldConfig() returned error: %s", err)
	}
	if NewConfig.User[0].Pass == SecretReplace {
		t.Fatalf("CopyPasswordsFromOldConfig() should have replaced the NewConfig's User[0].Pass (this is user "+
			"having name: '%s') from '%s' to an actual hash but did not do so", NewConfig.User[0].Name, SecretReplace)
	}

	if NewConfig.Backup[1].EncryptPass == SecretReplace {
		t.Fatalf("CopyPasswordsFromOldConfig() should have replaced the NewConfig's Backup[1].EncryptPass (this is backup "+
			"having name: '%s') from '%s' to an actual password but did not do so", NewConfig.Backup[1].Name, SecretReplace)
	}

	foundSecretKey = false
	for _, entry := range NewConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			if entry.Value == SecretReplace {
				t.Fatalf("CopyPasswordsFromOldConfig() should have replaced the NewConfig's Backup[0].Target[0].Parameters.'AWS_SECRET_ACCESS_KEY "+
					"(this is backup having name: '%s' and target name '%s') from '%s' to an actual secret but did not do so",
					NewConfig.Backup[0].Name, NewConfig.Backup[0].Target[0].Name, SecretReplace)
			}
			foundSecretKey = true
		}
	}
	if !foundSecretKey {
		t.Fatal("CopyPasswordsFromOldConfig() seems to somehow made 'AWS_SECRET_ACCESS_KEY' disappear")
	}
}

// check that for a user with pass=*** that we get an error if the user doesn't exist in the old config
func TestCopyPasswordsFromOldConfig3(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.User) == 0 {
		t.Fatal("Config file doesn't have user section but we're trying to validate User related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	NewConfig.User[0].Name = "bla5345345BlaUser"
	NewConfig.User[0].Pass = SecretReplace

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check that for a Backup with EncryptPass=*** that we get an error if the Backup doesn't exist in the old config
func TestCopyPasswordsFromOldConfig4(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	NewConfig.Backup[1].Name = "bla4353424Backup"
	NewConfig.Backup[1].EncryptPass = SecretReplace

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check that for a Backup with EncryptPass=*** that we get an error if the Backup doesn't have a password in the
// old config
func TestCopyPasswordsFromOldConfig5(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	oldConfig.Backup[1].EncryptPass = ""
	NewConfig.Backup[1].EncryptPass = SecretReplace

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check that for a Backup.Target.Params with value for AWS_SECRET_ACCESS_KEY *** that we get an error if the Target doesn't exist in the old config
func TestCopyPasswordsFromOldConfig6(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}
	if len(result.Config.Backup[0].Target) == 0 {
		t.Fatal("Config file doesn't have 'Backup[0].Target' section but we're trying to validate Backup.Target" +
			" related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	NewConfig.Backup[0].Target[0].Name = "bla4832094Target"
	foundSecretKey := false
	for i, entry := range NewConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			NewConfig.Backup[0].Target[0].Parameters[i].Value = SecretReplace
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sample backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check that for a Backup.Target with Pass=*** that we get an error if the Backup with that name doesn't exist in the
// old config
func TestCopyPasswordsFromOldConfig7(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}
	if len(result.Config.Backup[0].Target) == 0 {
		t.Fatal("Config file doesn't have 'Backup[0].Target' section but we're trying to validate Backup.Target" +
			" related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config
	NewConfig.Backup[0].Name = "bla32847234blaBackup"
	foundSecretKey := false
	for i, entry := range NewConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			NewConfig.Backup[0].Target[0].Parameters[i].Value = SecretReplace
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sample backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check that for a Backup.Target with Pass=*** that we get an error if the Target doesn't have a password in the
// old config
func TestCopyPasswordsFromOldConfig8(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a second time the fake config file. Error was: %s", err)
	}

	if len(result.Config.Backup) == 0 {
		t.Fatal("Config file doesn't have 'Backup' section but we're trying to validate Backup related code")
	}
	if len(result.Config.Backup[0].Target) == 0 {
		t.Fatal("Config file doesn't have 'Backup[0].Target' section but we're trying to validate Backup.Target" +
			" related code")
	}

	oldConfig := result.Config
	NewConfig := result2.Config

	// change in new config
	foundSecretKey := false
	for i, entry := range NewConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			NewConfig.Backup[0].Target[0].Parameters[i].Value = SecretReplace
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sample backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	// change in old config
	foundSecretKey = false
	for i, entry := range oldConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			oldConfig.Backup[0].Target[0].Parameters[i].Value = ""
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sample backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	err = CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err == nil {
		t.Fatal("CopyPasswordsFromOldConfig() did not return error but one was expected")
	}
}

// check passwords get replaced with *****
func TestSanitizeCfgTemplate(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file did not load successfully but should have")
	}
	err = ValidateBackup(result.Config.Backup, true)
	if err != nil {
		t.Fatal("Config struct did not validat but should have")
	}
	// actual test
	sanitizedConfig := SanitizeCfgTemplate(result.Config)
	if sanitizedConfig.User[0].Pass != SecretReplace {
		t.Fatalf("Expected user password to be %s but it remains %s", SecretReplace, sanitizedConfig.User[0].Pass)
	}

	foundSecretKey := false
	for _, entry := range sanitizedConfig.Backup[0].Target[0].Parameters {
		if entry.Name == "AWS_SECRET_ACCESS_KEY" {
			if entry.Value != SecretReplace {
				t.Fatalf("Expected target parameter 'AWS_SECRET_ACCESS_KEY' to be %s but it remains %s", SecretReplace, entry.Value)
			}
			foundSecretKey = true
		}
	}

	if !foundSecretKey {
		t.Fatal("Sanitised backup config used does not have a 'AWS_SECRET_ACCESS_KEY' parameter for the first " +
			"target of the first backup and we expect the config to be built this way, for testing")
	}

	if sanitizedConfig.Backup[1].EncryptPass != SecretReplace {
		t.Fatalf("Expected Encrypt password to be %s but it remains %s", SecretReplace, sanitizedConfig.Backup[1].EncryptPass)
	}
}

// validate ValidateDir() using file instead of dir
func TestValidateDir(t *testing.T) {
	err := ValidateDir("/etc/services", "data_dir", true)
	if err == nil {
		t.Fatal("data_dir validates successfully but should have failed due to providing file path instead of" +
			" directory path")
	}
}

// save  config, load again an compare settings got saved
func TestSave(t *testing.T) {
	const tmpName = "cHanGedName"
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_config_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	result, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	err = Validate(result.Config, false)
	if err != nil {
		t.Fatal("Config file did not load successfully but should have")
	}

	// load again as we need a 2nd variable to hold the "new" config we're going to write
	result2, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a 2nd time the fake config file. Error was: %s", err)
	}

	err = Validate(result2.Config, false)
	if err != nil {
		t.Fatal("Config file did not load successfully the 2nd time but should have")
	}
	// change something in the new config
	result2.Config.Backup[0].Name = tmpName

	// save
	err = Save(result, result2.Config)
	if err != nil {
		t.Fatalf("Could not save the fake config file. Error was: %s", err)
	}

	// load again config from file to check changes were saved
	result3, err := Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load a 3rd time the fake config file. Error was: %s", err)
	}

	err = Validate(result3.Config, false)
	if err != nil {
		t.Fatal("Config file did not load successfully the 3rd time but should have")
	}

	if result3.Config.Backup[0].Name != tmpName {
		t.Fatal("The content of the saved configuration does not match expectation")
	}

}

// check validateTargetParametersAreKnown() works when supplied with parameters slice which matches allowed values
func TestValidateTargetParametersAreKnown1(t *testing.T) {
	parameters := make([]shared.ConfigBackupTargetParams, 0)
	parameters = append(parameters, shared.ConfigBackupTargetParams{Name: "a_name1", Value: "a_value1"},
		shared.ConfigBackupTargetParams{Name: "a_name2", Value: "a_value2"})
	allowedParameters := [...]string{"a_name1", "a_name2", "a_name3"}
	err := validateTargetParametersAreKnown(parameters, allowedParameters[:], "backup1", "target1", "some_type")
	if err != nil {
		t.Fatal(err)
	}
}

// check validateTargetParametersAreKnown() works when supplied with parameters slice which DON'T match allowed values
func TestValidateTargetParametersAreKnown2(t *testing.T) {
	parameters := make([]shared.ConfigBackupTargetParams, 0)
	parameters = append(parameters, shared.ConfigBackupTargetParams{Name: "a_name1", Value: "a_value1"},
		shared.ConfigBackupTargetParams{Name: "a_name2", Value: "a_value2"})
	allowedParameters := [...]string{"a_name2", "a_name3", "a_name4"}
	err := validateTargetParametersAreKnown(parameters, allowedParameters[:], "backup1", "target1", "some_type")
	if err == nil {
		t.Fatalf("Was expecting validateTargetParametersAreKnown() to return an error as we passed it one " +
			"unknown parameter; but it didn't error out")
	}
}

func TestValidateBackupOrTargetNameAcceptsGoodNames(t *testing.T) {
	for _, name := range []string{"backup1", "aTarget", "a_single_underscore_is_fine", "x"} {
		if err := validateBackupOrTargetName("Backup", name, false); err != nil {
			t.Errorf("name %q unexpectedly rejected: %s", name, err)
		}
	}
}

func TestValidateBackupOrTargetNameRejectsReservedSeparator(t *testing.T) {
	// The double-underscore is reserved because per-target restore DB files use
	// it as a separator: restore__<backup>__<target>.sqlite
	for _, name := range []string{"bad__name", "__leading", "trailing__", "a__b__c"} {
		if err := validateBackupOrTargetName("Backup", name, false); err == nil {
			t.Errorf("name %q should have been rejected for containing the reserved separator", name)
		}
	}
}

func TestValidateBackupOrTargetNameRejectsPathSeparators(t *testing.T) {
	for _, name := range []string{"a/b", "a\\b", "../etc", "./local"} {
		if err := validateBackupOrTargetName("Target", name, false); err == nil {
			t.Errorf("name %q should have been rejected for containing a path separator", name)
		}
	}
}

func TestValidateBackupOrTargetNameRejectsDotAndDotDot(t *testing.T) {
	for _, name := range []string{".", ".."} {
		if err := validateBackupOrTargetName("Backup", name, false); err == nil {
			t.Errorf("name %q should have been rejected", name)
		}
	}
}

func TestValidateBackupScheduleAcceptsValidExpressions(t *testing.T) {
	good := []string{
		"05 01 * * *",
		"0 8 1 * *",
		"@daily",
		"@hourly",
		"@every 1m",
	}
	for _, expr := range good {
		if err := ValidateBackupSchedule([]string{expr}, "job", false); err != nil {
			t.Errorf("expression %q unexpectedly rejected: %s", expr, err)
		}
	}
}

func TestValidateBackupScheduleRejectsBadExpressions(t *testing.T) {
	bad := []string{
		"bogus * * * *",
		"99 * * * *",  // minute out of range
		"* * * * * *", // 6 fields, not supported by standard parser
		"@nonsense",
	}
	for _, expr := range bad {
		if err := ValidateBackupSchedule([]string{expr}, "job", false); err == nil {
			t.Errorf("expression %q should have been rejected", expr)
		}
	}
}

func TestValidateBackupScheduleRejectsEmptyEntry(t *testing.T) {
	if err := ValidateBackupSchedule([]string{""}, "job", false); err == nil {
		t.Error("empty schedule entry should have been rejected")
	}
	if err := ValidateBackupSchedule([]string{"   "}, "job", false); err == nil {
		t.Error("whitespace-only schedule entry should have been rejected")
	}
}

func TestValidateBackupScheduleAcceptsEmptyList(t *testing.T) {
	// a backup with no schedule at all is fine — it just won't be triggered automatically
	if err := ValidateBackupSchedule(nil, "job", false); err != nil {
		t.Errorf("nil schedule list unexpectedly rejected: %s", err)
	}
	if err := ValidateBackupSchedule([]string{}, "job", false); err != nil {
		t.Errorf("empty schedule list unexpectedly rejected: %s", err)
	}
}
