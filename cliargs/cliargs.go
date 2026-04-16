package cliargs

import (
	"cloudbackup/client"
	clientBackup "cloudbackup/client/backup"
	clientBackupReport "cloudbackup/client/backup/report"
	clientBackupTarget "cloudbackup/client/backup/target"
	clientConfig "cloudbackup/client/config"
	clientNotification "cloudbackup/client/notification"
	clientRestore "cloudbackup/client/restore"
	"cloudbackup/config"
	"cloudbackup/daemon"
	"cloudbackup/misc"
	"cloudbackup/password"
	"cloudbackup/utils"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const loggingContext = "cliargs"

// top level CLI options and arguments
type Args struct {
	Server ArgsCommandServer `command:"server" description:"Backup server related commands and options"`
	Client ArgsCommandClient `command:"client" description:"Backup client related commands and options"`
	Misc   ArgsCommandMisc   `command:"misc" description:"Miscellaneous commands"`
}

type ArgsCommandServer struct {
	Config  ArgsCommandServerConfig  `command:"config" description:"Server configuration file related options"`
	Start   ArgsCommandServerStart   `command:"start" description:"Start the backup server"`
	Version ArgsCommandServerVersion `command:"version" description:"Show server version"`
}

type ArgsCommandServerConfig struct {
	Validate ArgsCommandServerConfigValidate `command:"validate" description:"Validate provided yaml configuration file"`
	Dump     ArgsCommandServerConfigDump     `command:"dump" description:"Dumps the merged configuration. This is a merge of command line arguments, environment variables and then the supplied .yaml config file. Priority is from left to right of the given list. The result will include default values too."`
	Example  ArgsCommandServerConfigExample  `command:"example" description:"Show an example .yaml config file with all possible statements"`
}

type ArgsCommandServerConfigValidate struct {
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Debug      bool   `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration. WARNING! Secrets and passwords will be shown when using log level debug."`
}

type ArgsCommandServerConfigDump struct {
	Debug      bool   `short:"d" long:"debug" description:"Set logging to debug in order to see more details about the build up of the configuration. WARNING! Secrets and passwords will be shown when using log level debug."`
	ConfigFile string `short:"c" long:"configfile" description:"RuntimeConfig file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
}

// arguments for an actual Daemon start
type ArgsCommandServerStart struct {
	ConfigFile string `short:"c" long:"configfile" description:"Server configuration file expected to be in YAML format and have .yml or .yaml extension" required:"true"`
	Quiet      bool   `short:"q" long:"quiet" description:"Set logging to quiet: don't show any log messages"`
	Debug      bool   `short:"d" long:"debug" description:"Set logging to debug. WARNING! Secrets and passwords will be shown when using log level debug while the configuration information is being parsed and potentially later on."`
	TextLog    bool   `short:"t" long:"textlog" description:"Set logging to plaintext. Defaults to false which means JSON formatting is used"`
	LogFile    string `short:"l" long:"logfile" description:"Output logging messages to this file instead of stdout. If the target file can't be created or can't be written to then it will output an error and revert back to using stdout for logging output."`
}

type ArgsCommandServerConfigExample struct{}

type ArgsCommandServerVersion struct{}

type ArgsCommandMisc struct {
	HashPassword ArgsCommandMiscHash `command:"hash-password" description:"Hash a password using bcrypt. This is a convenience function so you can easily hash passwords before adding them to the yaml config file of the server."`
}

type ArgsCommandMiscHash struct{}

type ArgsCommandClient struct {
	Config        ArgsCommandClientConfig        `command:"config" description:"Client configuration file related options"`
	Backup        ArgsCommandClientBackup        `command:"backup" description:"Interact with backup jobs (start/stop/status)"`
	Restore       ArgsCommandClientRestore       `command:"restore" description:"Interact with restore jobs (start/stop/list/watch)"`
	Notification  ArgsCommandClientNotification  `command:"notification" description:"Interact with server generated notifications"`
	Version       ArgsCommandClientVersion       `command:"version" description:"Client version"`
	ServerVersion ArgsCommandClientServerVersion `command:"server-version" description:"Retrieve and show server version"`
}

type ArgsCommandClientVersion struct{}

type ArgsCommandClientServerVersion struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
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
	Start  ArgsCommandClientBackupStart  `command:"start" description:"Start a backup job"`
	Stop   ArgsCommandClientBackupStop   `command:"stop" description:"Stop a running backup job"`
	List   ArgsCommandClientBackupList   `command:"list" description:"List all backup jobs and a brief status for each of them"`
	Status ArgsCommandClientBackupStatus `command:"status" description:"Show details about a specific backup job"`
	Watch  ArgsCommandClientBackupWatch  `command:"watch" description:"Continuously watches a specific backup job in order to show file, directory and symlinks backup progress. This is a best effort operation meaning that events will get discarded and not sent to the client if either the server produces more events per second than it can handle or if the client can't receive quickly enough events produced by the server"`
	DryRun ArgsCommandClientBackupDryRun `command:"dryrun" description:"Dry run a backup job in order to see what files and directories get evaluated"`
	Target ArgsCommandClientBackupTarget `command:"target" description:"Backup target (object store) related commands"`
	Report ArgsCommandClientBackupReport `command:"report" description:"Access reports of previously ran backup jobs"`
}

type ArgsCommandClientBackupTarget struct {
	Test ArgsCommandClientBackupTargetTest `command:"test" description:"For a given backup section name, it will test all defined targets in order to check that the object stores are usable for storing backed up files"`
}

type ArgsCommandClientBackupReport struct {
	List ArgsCommandClientBackupReportList `command:"list" description:"List available reports of previously ran backup jobs"`
	Show ArgsCommandClientBackupReportShow `command:"show" description:"Show report of a previously ran backup job"`
}

type ArgsCommandClientBackupReportList struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job for which to show the list of available reports. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	StartTime string `long:"from-start-time" description:"Select only reports which have a job start_time which equals or is newer than the supplied value. If unspecified then a value equal to '30 days ago' will be used. Date+time format is RFC3339 with nanosecond support"`
	EndTime   string `long:"until-start-time" description:"Select only reports which have a job start_time which equals or is up to (earlier than) the supplied value. If unspecified then a value equal to 'now' will be used. Date+time format is RFC3339 with nanosecond support"`
}

type ArgsCommandClientBackupReportShow struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job for which to show the list of available reports. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	JobId string `short:"i" long:"job-id" required:"yes" description:"Id of the job for which to show the report"`
}

type ArgsCommandClientBackupStart struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job to start. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	Watch bool `short:"w" long:"watch" description:"If the backup is successfully started then watch the backup job in order to show progress. Please see the description of the command 'client backup watch' for more details."`
}

type ArgsCommandClientBackupStop struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job to start. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	JobId string `short:"i" long:"job-id" description:"Id of the job to stop. Using this ensures that only a particular job is stopped. If the job id doesn't match the id of the running job having the same name then the stop operation will not proceed"`
}

type ArgsCommandClientBackupWatch struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation is successful then print JSON responses as they are received from server. If this option is not specified then the response is processed and the output is a plaintext table."`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job to dry run. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	JobId string `short:"i" long:"job-id" description:"Id of the job to watch. Using this ensures that only a particular job is watched. If the job id doesn't match the id of the running job having the same name then the watch operation will not proceed"`
}

type ArgsCommandClientBackupDryRun struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation is successful then print JSON responses as they are received from server. If this option is not specified then the response is processed and the output is a plaintext table followed by a summary at the end."`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job to dry run. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
}

type ArgsCommandClientBackupList struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output is in a table like format"`
}

type ArgsCommandClientBackupStatus struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print only the part of the JSON response which is related to your selected job. If this option is not specified then the response is processed and the output is in a table like format"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job for which to get the status. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	JobId string `short:"i" long:"job-id" description:"Id of the job to stop. Using this ensures that only a particular running job is showed. If the job id doesn't match the id of the running job having the same name then the status operation will exit."`
}

type ArgsCommandClientBackupTargetTest struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup job for which to test all defined targets. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
}

type ArgsCommandClientConfigValidate struct {
	ArgsCommandClientBackupCommonOptions
}

type ArgsCommandClientConfigDump struct {
	ArgsCommandClientBackupCommonOptions
}

type ArgsCommandClientConfigExample struct {
}

type ArgsCommandClientNotification struct {
	Test ArgsCommandClientNotificationTest `command:"test" description:"Trigger a test of each notification defined on the backup server"`
}

type ArgsCommandClientNotificationTest struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
}

func (command *ArgsCommandServerConfigValidate) Execute(args []string) error {
	if command.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	_, err := config.Load(command.ConfigFile, command.Debug, &sync.RWMutex{})
	if err != nil {
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
		utils.Pp(config.SanitizeCfgTemplate(configuration.GetCopyWithLock(loggingContext)))
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
		Quiet:   command.Quiet,
		Debug:   command.Debug,
		TextLog: command.TextLog,
		LogFile: command.LogFile,
	}
	misc.SetupLogging(loggingArgs)
	daemon.Start(command.ConfigFile, command.Debug)
	return nil
}

func (command *ArgsCommandServerConfigExample) Execute(args []string) error {
	if strings.ToLower(runtime.GOOS) == "windows" {
		fmt.Print(misc.SampleWindowsServerYamlConfig)
	} else {
		fmt.Print(misc.SampleUnixServerYamlConfig)
	}
	os.Exit(0)
	return nil
}

func (command *ArgsCommandServerVersion) Execute(args []string) error {
	v := misc.CloudBackupVersion()
	v.OS = runtime.GOOS
	v.Arch = runtime.GOARCH
	v.Runtime = runtime.Version()
	fmt.Printf("Server version: %s\nBuild date: %s\nOS: %s\nArch: %s\nRuntime: %s\nAWS SDK: %s\nAzure Blob "+
		"Storage SDK: %s\nGoogle Cloud Platform SDK: %s\n", v.CloudBackup, v.BuildDate, v.OS, v.Arch, v.Runtime,
		v.AwsSdk, v.AzureBlobStorageSdk, v.GcpStorageSdk)
	os.Exit(0)

	return nil
}

func (command *ArgsCommandClientServerVersion) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	client.RetrieveServerVersion(clConfig, command.Json)
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

func (command *ArgsCommandClientVersion) Execute(args []string) error {
	fmt.Printf("Client version: %s\nBuild date: %s\n", misc.CloudBackupVersion().CloudBackup, misc.CloudBackupVersion().BuildDate)
	os.Exit(0)
	return nil
}

func (command *ArgsCommandClientConfigValidate) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	_, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	} else {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches is valid\n", path)
		os.Exit(0)
	}
	return nil
}

func (command *ArgsCommandClientConfigDump) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
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
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
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

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.List(clConfig, command.Json)
	return nil
}

func (command *ArgsCommandClientBackupStatus) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.Status(clConfig, command.Json, command.Job.Name, command.JobId)
	return nil
}

func (command *ArgsCommandClientBackupStart) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.Start(clConfig, command.Json, command.Job.Name, command.Watch)
	return nil
}

func (command *ArgsCommandClientBackupStop) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.Stop(clConfig, command.Json, command.Job.Name, command.JobId)
	return nil
}

func (command *ArgsCommandClientBackupWatch) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.Watch(clConfig, command.Json, command.Job.Name, command.JobId)
	return nil
}

func (command *ArgsCommandClientBackupDryRun) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackup.DryRun(clConfig, command.Json, command.Job.Name)
	return nil
}

func (command *ArgsCommandClientBackupTargetTest) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientBackupTarget.Test(clConfig, command.Json, command.Job.Name)
	return nil
}

func (command *ArgsCommandClientConfigExample) Execute(args []string) error {
	fmt.Println(misc.SampleClientYamlConfig)
	os.Exit(0)
	return nil
}

func (command *ArgsCommandClientNotificationTest) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientNotification.Test(clConfig, command.Json)
	return nil
}

func (command *ArgsCommandClientBackupReportList) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}

	var FromStartTime, UntilStartTime time.Time
	if command.StartTime == "" {
		FromStartTime = time.Now().AddDate(0, 0, -30) // 30 days ago
	} else {
		FromStartTime, err = time.Parse(time.RFC3339Nano, command.StartTime)
		if err != nil {
			fmt.Printf("Provided value '%s' for '--from-start-time' could not be parsed into a time object which "+
				"is RFC3339 with nanoseconds compliant due to error: %s", command.StartTime, err.Error())
			os.Exit(1)
		}
	}

	if command.EndTime == "" {
		UntilStartTime = time.Now()
	} else {
		UntilStartTime, err = time.Parse(time.RFC3339Nano, command.EndTime)
		if err != nil {
			fmt.Printf("Provided value '%s' for '--until-start-time' could not be parsed into a time object which "+
				"is RFC3339 with nanoseconds compliant due to error: %s", command.EndTime, err.Error())
			os.Exit(1)
		}
	}

	if UntilStartTime.Before(FromStartTime) {
		fmt.Printf("Provided value '%s' for '--until-start-time' represents a value which is earlier "+
			"than '--from-start-time' 's own value of %s . Please specify a value which is equal or older for "+
			"'--until-start-time'", UntilStartTime, FromStartTime)
		os.Exit(1)
	}

	clientBackupReport.List(clConfig, command.Json, command.Job.Name, FromStartTime, UntilStartTime)
	return nil
}

func (command *ArgsCommandClientBackupReportShow) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}

	clientBackupReport.Show(clConfig, command.Json, command.Job.Name, command.JobId)
	return nil
}

// -----------------------------------------------------------------------------
// Restore commands
// -----------------------------------------------------------------------------

type ArgsCommandClientRestore struct {
	Start  ArgsCommandClientRestoreStart  `command:"start" description:"Start a restore job. You may supply files explicitly via --file, use --all-files to restore everything, or omit both to launch an interactive TUI file browser for selecting files from the chosen backup job run."`
	Stop   ArgsCommandClientRestoreStop   `command:"stop" description:"Stop a running restore job"`
	List   ArgsCommandClientRestoreList   `command:"list" description:"List all currently running restore jobs"`
	Watch  ArgsCommandClientRestoreWatch  `command:"watch" description:"Continuously watches a specific restore job in order to show file, directory and symlinks restore progress. Uses the same SSE protocol as backup watch"`
	Report ArgsCommandClientRestoreReport `command:"report" description:"Access reports of previously ran restore jobs"`
}

type ArgsCommandClientRestoreReport struct {
	List ArgsCommandClientRestoreReportList `command:"list" description:"List available reports of previously ran restore jobs"`
	Show ArgsCommandClientRestoreReportShow `command:"show" description:"Show report of a previously ran restore job"`
}

type ArgsCommandClientRestoreReportList struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup definition for which to show the list of available restore reports. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	StartTime string `long:"from-start-time" description:"Select only reports which have a job start_time which equals or is newer than the supplied value. If unspecified then a value equal to '30 days ago' will be used. Date+time format is RFC3339 with nanosecond support"`
	EndTime   string `long:"until-start-time" description:"Select only reports which have a job start_time which equals or is up to (earlier than) the supplied value. If unspecified then a value equal to 'now' will be used. Date+time format is RFC3339 with nanosecond support"`
}

type ArgsCommandClientRestoreReportShow struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup definition for which to show the restore report. This needs to match a backup job as defined in the configuration of the server"`
	} `positional-args:"yes" required:"yes"`
	JobId string `short:"i" long:"job-id" required:"yes" description:"Id of the restore job for which to show the report"`
}

type ArgsCommandClientRestoreStart struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup definition from which to restore. This needs to match a backup definition on the server"`
	} `positional-args:"yes" required:"yes"`
	SourceJobId    string   `short:"i" long:"job-id" required:"yes" description:"UUID of the previously ran backup job run from which the files will be fetched. Use 'backup report list' to discover available job ids"`
	Target         string   `long:"target" description:"Optional name of the target (as defined in the backup definition) from which to fetch files. If empty, the first target is used"`
	RestoreDir     string   `long:"restore-dir" description:"Optional override of the server side destination directory into which to write restored files"`
	Files          []string `long:"file" description:"Absolute source path to restore. May be specified multiple times. Mutually exclusive with --all-files"`
	AllFiles       bool     `long:"all-files" description:"Restore every file, directory and symlink recorded for the source job id. Mutually exclusive with --file"`
	Exclusions     []string `long:"exclusion" description:"Bourne-Again shell like globing pattern for files to exclude from the restore. May be specified multiple times"`
	NonInteractive bool     `short:"N" long:"non-interactive" description:"Do not launch the interactive TUI file browser even if no --file and no --all-files were supplied. In that case the request is rejected"`
	Watch          bool     `short:"w" long:"watch" description:"If the restore is successfully started then watch the restore job in order to show progress"`
}

type ArgsCommandClientRestoreStop struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output unstructured plaintext"`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup definition for which the running restore should be stopped"`
	} `positional-args:"yes" required:"yes"`
	RestoreJobId string `short:"i" long:"restore-job-id" description:"Optional UUID of the restore job. If supplied, the restore is only stopped if both name and id match the running job"`
}

type ArgsCommandClientRestoreList struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation was successful then print JSON response as received from server. If this option is not specified then the response is processed and the output is in a table like format"`
}

type ArgsCommandClientRestoreWatch struct {
	ArgsCommandClientBackupCommonOptions
	Json bool `long:"json" description:"If the operation is successful then print JSON responses as they are received from server. If this option is not specified then the response is processed and the output is a plaintext table."`
	Job  struct {
		Name string `positional-arg-name:"job_name" description:"Name of the backup definition for which a restore is running and should be watched"`
	} `positional-args:"yes" required:"yes"`
	RestoreJobId string `short:"i" long:"restore-job-id" description:"Optional UUID of the restore job to watch. If omitted, the currently running restore job for the given name is watched"`
}

func (command *ArgsCommandClientRestoreStart) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}

	if command.AllFiles && len(command.Files) > 0 {
		fmt.Println("--all-files and --file are mutually exclusive")
		os.Exit(1)
	}

	files := command.Files
	allFiles := command.AllFiles

	// If neither was supplied, launch the TUI browser (unless --non-interactive was set).
	if !allFiles && len(files) == 0 {
		if command.NonInteractive {
			fmt.Println("Neither --all-files nor --file were supplied and --non-interactive prevents the TUI " +
				"file browser from running. Either supply files or remove --non-interactive.")
			os.Exit(1)
		}
		selected, err := clientRestore.Browse(clConfig, command.Job.Name, command.SourceJobId)
		if err != nil {
			fmt.Printf("File browser did not complete successfully: %s\n", err)
			os.Exit(1)
		}
		if len(selected) == 0 {
			fmt.Println("No files were selected. Aborting restore.")
			os.Exit(1)
		}
		files = selected
		fmt.Printf("Selected %d items for restore.\n", len(files))
	}

	clientRestore.Start(clConfig, command.Json, command.Job.Name, command.SourceJobId, command.Target,
		command.RestoreDir, files, allFiles, command.Exclusions, command.Watch)
	return nil
}

func (command *ArgsCommandClientRestoreStop) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientRestore.Stop(clConfig, command.Json, command.Job.Name, command.RestoreJobId)
	return nil
}

func (command *ArgsCommandClientRestoreList) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientRestore.List(clConfig, command.Json)
	return nil
}

func (command *ArgsCommandClientRestoreWatch) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}
	clientRestore.Watch(clConfig, command.Json, command.Job.Name, command.RestoreJobId)
	return nil
}

func (command *ArgsCommandClientRestoreReportList) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}

	var FromStartTime, UntilStartTime time.Time
	if command.StartTime == "" {
		FromStartTime = time.Now().AddDate(0, 0, -30)
	} else {
		FromStartTime, err = time.Parse(time.RFC3339Nano, command.StartTime)
		if err != nil {
			fmt.Printf("Provided value '%s' for '--from-start-time' could not be parsed into a time object which "+
				"is RFC3339 with nanoseconds compliant due to error: %s", command.StartTime, err.Error())
			os.Exit(1)
		}
	}

	if command.EndTime == "" {
		UntilStartTime = time.Now()
	} else {
		UntilStartTime, err = time.Parse(time.RFC3339Nano, command.EndTime)
		if err != nil {
			fmt.Printf("Provided value '%s' for '--until-start-time' could not be parsed into a time object which "+
				"is RFC3339 with nanoseconds compliant due to error: %s", command.EndTime, err.Error())
			os.Exit(1)
		}
	}

	if UntilStartTime.Before(FromStartTime) {
		fmt.Printf("Provided value '%s' for '--until-start-time' represents a value which is earlier "+
			"than '--from-start-time' 's own value of %s . Please specify a value which is equal or older for "+
			"'--until-start-time'", UntilStartTime, FromStartTime)
		os.Exit(1)
	}

	clientRestore.ReportList(clConfig, command.Json, command.Job.Name, FromStartTime, UntilStartTime)
	return nil
}

func (command *ArgsCommandClientRestoreReportShow) Execute(args []string) error {
	loggingArgs := misc.LoggingArgs{
		Quiet:   true,
		Debug:   command.Debug,
		TextLog: !command.JsonLog,
	}
	misc.SetupLogging(loggingArgs)

	clConfig, path, err := clientConfig.Load(command.ConfigFile, command.Debug, command.Username, command.Password, command.Address)
	if err != nil {
		fmt.Printf("Client configuration using file %s and optional environment variables and command line "+
			"switches did not pass validation\nThe encountered error was: %s\n", path, err)
		os.Exit(1)
	}

	clientRestore.ReportShow(clConfig, command.Json, command.Job.Name, command.JobId)
	return nil
}
