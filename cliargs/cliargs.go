package cliargs

import (
	log "github.com/sirupsen/logrus"
	clientConfig "cloudbackup/client/config"
	clientBackup "cloudbackup/client/backup"
	"cloudbackup/config"
	"cloudbackup/daemon"
	"cloudbackup/misc"
	"cloudbackup/password"
	"cloudbackup/utils"
	"sync"
	"fmt"
	"os"
)

const loggingContext = "cliargs"

// top level CLI options and arguments
type Args struct {
	Server ArgsCommandServer  `command:"server" description:"Backup server related commands and options"`
	Client ArgsCommandClient  `command:"client" description:"Backup client related commands and options"`
	Misc   ArgsCommandMisc    `command:"misc" description:"Miscellaneous commands"`
}

type ArgsCommandServer struct {
	Config ArgsCommandServerConfig `command:"config" description:"Server configuration file related options"`
	Start  ArgsCommandServerStart  `command:"start" description:"Start the backup server"`
}

type ArgsCommandServerConfig struct {
	Validate ArgsCommandServerConfigValidate `command:"validate" description:"Validate provided yaml configuration file"`
	Dump     ArgsCommandServerConfigDump     `command:"dump" description:"Dumps the merged configuration. This is a merge of command line arguments, environment variables and then the supplied .yaml config file. Priority is from left to right of the given list. The result will include default values too."`
	Example  ArgsCommandServerConfigExample  `command:"example" description:"Show an example .yaml config file with all possible statements"`
}

type ArgsCommandServerConfigValidate struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration. WARNING! Secrets and passwords will be shown when using log level debug."`
}

type ArgsCommandServerConfigDump struct {
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration. WARNING! Secrets and passwords will be shown when using log level debug."`
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
}

// arguments for an actual Daemon start
type ArgsCommandServerStart struct {
	ConfigFile string `short:"c" long:"configfile" description:"Server configuration file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Quiet bool `short:"q" long:"quiet" description:"Set logging to quiet: don't show any log messages"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug. WARNING! Secrets and passwords will be shown when using log level debug while the configuration information is being parsed and potentially later on."`
	TextLog bool `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
	LogFile string `short:"l" long:"logfile" description:"Output logging messages to this file instead of stdout. If the target file can't be created or can't be written to then it will output an error and revert back to using stdout for logging output."`
}

type ArgsCommandServerConfigExample struct {}

type ArgsCommandMisc struct {
	HashPassword ArgsCommandMiscHash `command:"hash-password" description:"Hash a password using bcrypt. This is a convenience function so you can easily hash passwords before adding them to the yaml config file of the server."`
}

type ArgsCommandMiscHash struct {
}

type ArgsCommandClient struct {
	Config ArgsCommandClientConfig `command:"config" description:"Client configuration file related options"`
	Backup ArgsCommandClientBackup `command:"backup" description:"Interact with backup jobs (start/stop/status)"`
}

type ArgsCommandClientConfig struct {
	Validate ArgsCommandClientConfigValidate `command:"validate" description:"Validate provided yaml configuration file"`
	Dump     ArgsCommandClientConfigDump     `command:"dump" description:"Dumps the merged configuration. This is a merge of command line arguments, environment variables and then the supplied .yaml config file. Priority is from left to right of the given list. The result will include default values too."`
	Example  ArgsCommandClientConfigExample  `command:"example" description:"Show an example .yaml config file with all possible statements"`
}

// this one is included in multiple structs which themselves are defined below
type ArgsCommandClientBackupCommonOptions struct {
	ConfigFile string `short:"c" long:"configfile" description:"Client configuration file expected to be in YAML format and have .yml or .yaml extension. If unspecified then the default is to attempt to use $HOME/.cloudbackup.yaml on Linux or Unixes and %HomeDrive%%HomePath% on Microsoft Windows" required:"false"`
	Username   string `short:"u" long:"username" description:"Username to use when connecting to the server. If not specified then an attempt will be made to use environment variable CLOUDBACKUP_CLIENT_USERNAME followed by an attempt to use the command line specified configuration file (if not specified then a configuration file will be searched at the default location)"`
	Password   string `short:"p" long:"password" description:"Password to use when connecting to the server. If not specified then an attempt will be made to use environment variable CLOUDBACKUP_CLIENT_PASSWORD followed by an attempt to use the command line specified configuration file (if not specified then a configuration file will be searched at the default location)"`
	Address    string `short:"a" long:"address" description:"Address to use when connecting to the server. The format expect is one of 'https://1.2.3.4:8443' or 'http://127.0.0.1:8080'. If not specified then an attempt will be made to use environment variable CLOUDBACKUP_CLIENT_ADDRESS followed by an attempt to use the command line specified configuration file (if not specified then a configuration file will be searched at the default location)"`
	Debug      bool   `short:"d" long:"debug" description:"Set logging to debug. WARNING! Secrets and passwords will be shown when using log level debug"`
	JsonLog    bool   `long:"jsonlog" description:"Set logging to JSON. Defaults to plaintext"`
}

type ArgsCommandClientBackup struct {
	Start ArgsCommandClientBackupStart `command:"start" description:"Start a backup job"`
	Stop  ArgsCommandClientBackupStop `command:"stop" description:"Stop a running backup job"`
	List  ArgsCommandClientBackupList `command:"list" description:"List all backup jobs and a brief status for each of them"`

}

type ArgsCommandClientBackupStart struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job struct {
		Name   string `positional-arg-name:"job_name" description:"Name of the backup job to start. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
}

type ArgsCommandClientBackupStop struct {
	ArgsCommandClientBackupCommonOptions
}

type ArgsCommandClientBackupList struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output is in a table like format"`
}

type ArgsCommandClientConfigValidate struct {
	ArgsCommandClientBackupCommonOptions
}

type ArgsCommandClientConfigDump struct {
	ArgsCommandClientBackupCommonOptions
}

type ArgsCommandClientConfigExample struct {
}

func (command *ArgsCommandServerConfigValidate) Execute(args []string) error {
	if command.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	_, err := config.Load(command.ConfigFile, command.Debug, &sync.RWMutex{})
	if err != nil{
		fmt.Printf("Server configuration file %s did not pass validation\n", command.ConfigFile)
		os.Exit(1)
	} else {
		fmt.Printf("Server configuration %s is valid\n", command.ConfigFile)
		os.Exit(0)
	}
	return nil
}

func (command *ArgsCommandServerConfigDump) Execute(args []string) error {
	if command.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	configuration, err := config.Load(command.ConfigFile, command.Debug, &sync.RWMutex{})
	if err == nil {
		// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
		//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
		utils.Pp(config.SanitizeCfgTemplate(configuration.GetWithLock(loggingContext)))
		os.Exit(0)
	} else {
		fmt.Printf("Config file %s did not pass validation\n", command.ConfigFile)
		os.Exit(1)
	}
	return nil
}

// this is where the main stuff actually starts
func (command *ArgsCommandServerStart) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet: command.Quiet,
		Debug: command.Debug,
		TextLog: command.TextLog,
		LogFile: command.LogFile,
	}
	misc.SetupLogging(loggingArgs)
	daemon.Start(command.ConfigFile, command.Debug)
	return nil
}

func (command *ArgsCommandServerConfigExample) Execute(args []string) error {
	fmt.Println(misc.SampleYamlConfig)
	os.Exit(0)
	return nil
}

func (command *ArgsCommandMiscHash) Execute(args []string) error {
	hash, err := password.ReadPassFromCli()
	if err != nil {
		os.Exit(1)
	}
	fmt.Printf("The hashed password is: %s \n", hash)
	os.Exit(0)
	return nil
}

func (command *ArgsCommandClientConfigValidate) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet: true,
		Debug: command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	_, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil{
		fmt.Printf("Client configuration using file %s and optional environment variables and command line " +
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	} else {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line " +
			"switches is valid\n", path)
		os.Exit(0)
	}
	return nil
}

func (command *ArgsCommandClientConfigDump) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet: true,
		Debug: command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	configData, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err == nil {
		// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
		//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
		utils.Pp(clientConfig.SanitizeClientConfig(configData))
		os.Exit(0)
	} else {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line " +
			"switches did not pass validation\nThe encountered error was: %s\n",
			path, err)
		os.Exit(1)
	}
	return nil
}


func (command *ArgsCommandClientBackupList) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clientConfig , path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.List(clientConfig, command.Json)
	return nil
}


func (command *ArgsCommandClientBackupStart) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clientConfig , path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.Start(clientConfig, command.Json, command.Job.Name)
	return nil
}