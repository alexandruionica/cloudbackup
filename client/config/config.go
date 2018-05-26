package config


import (
	"errors"
	"fmt"

	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
	"regexp"
	"os"
	"runtime"
	"cloudbackup/utils"
)

const loggingContext = "client.config"
const SecretReplace = "****************"
// used for looking up environment variables holding configuration data
const EnvPrefix = "CLOUDBACKUP_CLIENT"
// default name of client configuration file . This is expected to be in the user's Homedir
const defaultClientConfigFile = ".cloudbackup.yaml"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type Client struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	// expected values are "https://12.34.56.7:8443" or "http://12.34.56.7:8080"
	Address string `yaml:"address" json:"address"`
}

// attempt to load configuration file and/or configuration supplied via command line options
// returns loaded config, path of loaded config file (if any) and error (if any)
func Load(path string, debug bool, cliUsername string, cliPassword string, cliAddress string) (Client, string, error) {
	var Config = Client{}
	var err error
	fileCheckRequired := false

	// if a mix of command line options + env vars supplies all needed options then don't check if the config file
	// exists
	if CheckIfOptionOrEnvVars(cliUsername, EnvPrefix + "_Username", EnvPrefix + "_USERNAME") == false {
		fileCheckRequired = true
	}
	if CheckIfOptionOrEnvVars(cliPassword, EnvPrefix + "_Password", EnvPrefix + "_PASSWORD") == false {
		fileCheckRequired = true
	}
	if CheckIfOptionOrEnvVars(cliAddress, EnvPrefix + "_Address", EnvPrefix + "_ADDRESS") == false {
		fileCheckRequired = true
	}

	if fileCheckRequired {
		// if path = "" then this gets the default config file location, based on OS
		path, err = RetrieveClientConfigFilePath(path)
		if err != nil {
			return Client{}, path, err
		}
		logger.Info(fmt.Sprintf("Loading client config file %s", path))

		if _, err := utils.FileExists(path, true); err != nil {
			logger.Error(err)
			return Client{}, path, errors.New(path + " " + err.Error())
		}
	}

	// if debug then also adjust logging level of configor library (set library to Verbose not Debug as
	// Verbose is actually what we expect when using  "debug")
	if debug {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix, Verbose: true}).Load(&Config, path)
	} else {
		err = configor.New(&configor.Config{ENVPrefix: EnvPrefix}).Load(&Config, path)
	}

	if err != nil {
		msg := fmt.Sprintf("When parsing the client configuration file %s the following error was encountered:" +
			" %s", path, err)
		logger.Error(msg)
		return Client{}, path, errors.New(msg)
	}

	// any non empty command line options override ENV variables + actual config file
	if cliUsername != "" {
		Config.Username = cliUsername
	}
	if cliPassword != "" {
		Config.Password = cliPassword
	}
	if cliAddress != "" {
		Config.Address = cliAddress
	}

	err = Validate(Config)
	if err != nil {
		return Client{}, path, err
	}

	return Config, path, nil
}

func Validate(config Client) error {
	err := CheckConfigOptionNotEmpty(config.Username, "username")
	if err != nil {
		return err
	}

	err = CheckConfigOptionNotEmpty(config.Password, "password")
	if err != nil {
		return err
	}

	err = CheckConfigOptionNotEmpty(config.Address, "address")
	if err != nil {
		return err
	}

	err = ValidateAddress(config.Address)
	if err != nil {
		return err
	}

	return nil
}

func CheckConfigOptionNotEmpty(value string, name string) error {
	if value == "" {
		return errors.New(fmt.Sprintf("no '%s' was provided or provided value is an empty string", name))
	}

	r, err := regexp.Compile(`^[[:space:]]+$`)
	// if the err is not nil then we have bug
	if err != nil {
		logger.Errorf("Encountered error while compiling the regular expression used to evaluate the '%s'" +
			" field. The error was: %s \n", name, err)
		os.Exit(2)
	}

	if r.MatchString(value) {
		return errors.New(fmt.Sprintf("value of '%s' is made only out of whitespace", name))
	}

	return nil
}


func ValidateAddress(address string) error {
	r, err := regexp.Compile(`^https?://[a-zA-Z0-9-.]+:[0-9]+$`)
	// if the err is not nil then we have bug
	if err != nil {
		logger.Errorf("Encountered error while compiling the regular expression used to evaluate the 'address'" +
			" field. The error was: %s \n", err)
		os.Exit(2)
	}
	if r.MatchString(address) == false {
		return errors.New(fmt.Sprintf("supplied 'address' having value '%s' does not match the pattern " +
			"'http://IP-ADDRESS:port' 'https://IP-ADDRESS:port' 'http://hostname:port' 'https://hostname:port'",
			address))
	}

	return nil
}

// if inPath == "" then return default config file path for OS ; if inPath != "" then return inPath
func RetrieveClientConfigFilePath(inPath string) (string, error){
	if inPath != "" {
		return inPath, nil
	}
	if runtime.GOOS == "windows" {
		// %HomeDrive%%HomePath%
		homeDrive, found := os.LookupEnv("HomeDrive")
		if found == false {
			return "", errors.New("environment variable %HomeDrive% is not set so the path to the default client " +
				"configuration file can't be established")
		}
		homePath, found := os.LookupEnv("HomePath")
		if found == false {
			return "", errors.New("environment variable %HomePath% is not set so the path to the default client " +
				"configuration file can't be established")
		}
		return homeDrive + homePath + string(os.PathSeparator) + defaultClientConfigFile, nil
	// otherwise we're running Linux or some kind of Unix derivate so $HOME is the path to the user's home
	} else {
		home, found := os.LookupEnv("HOME")
		if found == false {
			return "", errors.New("environment variable %HOME% is not set so the path to the default client " +
				"configuration file can't be established")
		}
		return home + string(os.PathSeparator) + defaultClientConfigFile, nil
	}
}

// check if an option (type string) was passed as command line or if an environment var option is set
func CheckIfOptionOrEnvVars(cliOption string, envVar1 string, envVar2 string) bool {
	_, envOk1 := os.LookupEnv(envVar1)
	_, envOk2 := os.LookupEnv(envVar2)
	if cliOption == "" && envOk1 == false && envOk2 == false {
		return false
	}
	return true
}

// replace passwords or secrets with **************** within an instance of Client type
// Unfortunately this function doesn't have any smarts so whenever the config struct is changed then also an update to
// the function is needed
func SanitizeClientConfig (config Client) Client {
    config.Password = SecretReplace
	return config
}