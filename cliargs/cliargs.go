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
	Config ArgsCommandConfig `command:"config" description:"configuration file related options"`
	Start ArgsCommandStart `command:"start" description:"Start the backup daemon"`
	HashPassword ArgsCommandHash `command:"hash-password" description:"Hash a password using bcrypt. This is a convenience function so you can easily hash passwords before adding them to the yaml config file."`
}

type ArgsCommandConfig struct {
	Validate ArgsCommandConfigCommandValidate `command:"validate" description:"validate provided yaml configuration file"`
	Dump ArgsCommandConfigCommandDump `command:"dump" description:"dumps the merged configuration. This is a merge of command line arguments, environment variables and then the supplied .yaml config file. Priority is from left to right of the given list. The result will include default values too."`
	Example ArgsCommandConfigCommandExample `command:"example" description:"show an example .yaml config file with all possible statements"`
}

type ArgsCommandConfigCommandValidate struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration"`
}

type ArgsCommandConfigCommandDump struct {
	Debug bool `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration"`
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
}

type ArgsCommandHash struct {
}

// arguments for an actual Daemon start
type ArgsCommandStart struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Verbose bool `short:"v" long:"verbose" description:"Set logging to verbose"`
	Debug bool `short:"d" long:"debug" description:"Set logging to debug"`
	TextLog bool `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
}

type ArgsCommandConfigCommandExample struct {}

func (command *ArgsCommandConfigCommandValidate) Execute(args []string) error {
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

func (command *ArgsCommandConfigCommandDump) Execute(args []string) error {
	if command.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	configuration, err := config.Load(command.ConfigFile, command.Debug, &sync.RWMutex{})
	if err == nil {
		utils.Pp(configuration.GetWithLock(loggingContext))
		os.Exit(0)
	} else {
		fmt.Printf("Config file %s did not pass validation\n", command.ConfigFile)
		os.Exit(1)
	}
	return nil
}

// this is where the main stuff actually starts
func (command *ArgsCommandStart) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Verbose: command.Verbose,
		Debug: command.Debug,
		TextLog: command.TextLog,
	}
	misc.SetupLogging(loggingArgs)
	daemon.Start(command.ConfigFile, command.Debug)
	return nil
}

func (command *ArgsCommandConfigCommandExample) Execute(args []string) error {
	fmt.Println(misc.SampleYamlConfig)
	os.Exit(0)
	return nil
}

func (command *ArgsCommandHash) Execute(args []string) error {
	hash, err := password.ReadPassFromCli()
	if err != nil {
		os.Exit(1)
	}
	fmt.Printf("The hashed password is: %s \n", hash)
	os.Exit(0)
	return nil
}