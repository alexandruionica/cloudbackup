package cliargs

import (
	log "github.com/sirupsen/logrus"
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
	Example  ArgsCommandServerExample        `command:"example" description:"Show an example .yaml config file with all possible statements"`
}

type ArgsCommandServerConfigValidate struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration"`
}

type ArgsCommandServerConfigDump struct {
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration"`
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
}

// arguments for an actual Daemon start
type ArgsCommandServerStart struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Quiet bool `short:"q" long:"quiet" description:"Set logging to quiet: show only Warning or above log level messages"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug"`
	TextLog bool `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
}

type ArgsCommandServerExample struct {}

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
	Validate ArgsCommandServerConfigValidate `command:"validate" description:"Validate provided yaml configuration file"`
	Dump     ArgsCommandServerConfigDump     `command:"dump" description:"Dumps the merged configuration. This is a merge of command line arguments, environment variables and then the supplied .yaml config file. Priority is from left to right of the given list. The result will include default values too."`
	Example  ArgsCommandServerExample        `command:"example" description:"Show an example .yaml config file with all possible statements"`
}

type ArgsCommandClientBackup struct {
	Start ArgsCommandClientBackupStart `command:"start" description:"Start a backup job"`
	Stop  ArgsCommandClientBackupStop `command:"stop" description:"Stop a running backup job"`
	List  ArgsCommandClientBackupList `command:"list" description:"List all backup jobs and a brief status for each of them"`
}

type ArgsCommandClientBackupStart struct {
}

type ArgsCommandClientBackupStop struct {
}

type ArgsCommandClientBackupList struct {
}

func (command *ArgsCommandServerConfigValidate) Execute(args []string) error {
	if command.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	_, err := config.Load(command.ConfigFile, command.Debug, &sync.RWMutex{})
	if err != nil{
		fmt.Printf("Config file %s did not pass validation\n", command.ConfigFile)
		os.Exit(1)
	} else {
		fmt.Printf("Config file %s is valid\n", command.ConfigFile)
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
	}
	misc.SetupLogging(loggingArgs)
	daemon.Start(command.ConfigFile, command.Debug)
	return nil
}

func (command *ArgsCommandServerExample) Execute(args []string) error {
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