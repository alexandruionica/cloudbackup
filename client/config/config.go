package config


import (
	"errors"
	"fmt"

	"cloudbackup/utils"
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
	"regexp"
	"os"
	"runtime"
)

const loggingContext = "client.config"
const SecretReplace = "****************"
// used for looking up environment variables holding configuration data
const EnvPrefix = "CLOUDBACKUP_CLIENT"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

type Client struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	// expected values are "https://12.34.56.7:8443" or "http://12.34.56.7:8080"
	Address string `yaml:"address" json:"address"`
}

func Load(path string, debug bool) (Client, error) {
	logger.Info(fmt.Sprintf("Loading client config file %s", path))

	var Config = Client{}
	var err error

	if _, err := utils.FileExists(path, true); err != nil {
		logger.Error(err)
		return Client{}, err
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
		return Client{}, errors.New(msg)
	}

	err = Validate(Config, false)
	if err != nil {
		return Client{}, err
	}

	return Config, nil
}

func Validate(config Client, hiddenPass bool) error {
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

func GetOSSpecificDefaultClientConfigFile() (string, error){
	const defaultClientConfigFile = ".cloudbackup.yaml"
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