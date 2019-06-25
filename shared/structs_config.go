package shared

import (
	"sync"
)

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigBackup struct {
	Name       string   `required:"true" yaml:"name" json:"name"`
	Paths      []string `required:"true" yaml:"paths" json:"paths"`
	Exclusions []string `yaml:"exclusions" json:"exclusions"`
	// TODO - fix library bug - https://github.com/jinzhu/configor/issues/34
	Dereference bool                 `default:"true" yaml:"dereference" json:"dereference"`
	Checksum    bool                 `default:"false" yaml:"checksum" json:"checksum"`
	Target      []ConfigBackupTarget `required:"true" yaml:"target" json:"target"`
	Schedule    []string             `yaml:"schedule" json:"schedule"`
	Encrypt     bool                 `default:"false" yaml:"encrypt" json:"encrypt"`
	EncryptPass string               `yaml:"encrypt_pass" json:"encrypt_pass"`
	// 0 means unlimited number of versions
	VersionsMaxNum uint `default:"0" yaml:"versions_max_num" json:"versions_max_num"`
	// 0 means unlimited number of age
	VersionsMaxAge string `default:"0" yaml:"versions_max_age" json:"versions_max_age"`
	//
	PreRunScript  string `yaml:"pre_run_script" json:"pre_run_script"`
	PostRunScript string `yaml:"post_run_script" json:"post_run_script"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigBackupTarget struct {
	Name       string                     `required:"true" yaml:"name" json:"name"`
	Type       string                     `required:"true" yaml:"type" json:"type"`
	Prefix     string                     `required:"true" yaml:"prefix" json:"prefix"`
	Bucket     string                     `required:"true" yaml:"bucket" json:"bucket"`
	Parameters []ConfigBackupTargetParams `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	RateLimit  string                     `default:"0" yaml:"ratelimit" json:"ratelimit"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigBackupTargetParams struct {
	Name  string `required:"true" yaml:"name" json:"name"`
	Value string `required:"true" yaml:"value" json:"value"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
// CopyPasswordsFromOldConfig replaces ***** with actual passwords so whenever the config struct is changed then
// also config.CopyPasswordsFromOldConfig needs updating; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigUser struct {
	Name string `required:"true" yaml:"name" json:"name"`
	Pass string `required:"true" yaml:"pass" json:"pass"`
	// allowed options are read or write (write implies read)
	Access string `default:"read" yaml:"access" json:"access"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigHttp struct {
	BindAddress string `default:"127.0.0.1:8080" yaml:"bind_address" json:"bind_address"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigHttps struct {
	Enabled     bool   `default:"false" yaml:"enabled" json:"enabled"`
	BindAddress string `default:"127.0.0.1:8443" yaml:"bind_address" json:"bind_address"`
	SslCertPath string `yaml:"ssl_cert_path" json:"ssl_cert_path"`
	SslKeyPath  string `yaml:"ssl_key_path" json:"ssl_key_path"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigNotification struct {
	Email  []ConfigNotificationEmail  `yaml:"email,omitempty" json:"email,omitempty"`
	Script []ConfigNotificationScript `yaml:"script,omitempty" json:"script,omitempty"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigNotificationEmail struct {
	Server string   `required:"true" yaml:"server" json:"server"`
	User   string   `yaml:"user,omitempty" json:"user,omitempty"`
	Pass   string   `yaml:"pass,omitempty" json:"pass,omitempty"`
	Port   string   `yaml:"port" json:"port" default:"25"`
	From   string   `yaml:"from,omitempty" json:"from,omitempty"`
	To     string   `required:"true" yaml:"to" json:"to"`
	CC     []string `yaml:"cc,omitempty" json:"cc,omitempty"`
	// type is one of: started, finished, failed, cancelled, crashed
	Type []string `yaml:"type" json:"type" default:"[failed,crashed]"`
}

// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// IF ANY NEW notification mechanism is added here then you MUST updated the function notifications.GetNumNotificators()
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type ConfigNotificationScript struct {
	Path string `required:"true" yaml:"path" json:"path"`
	// type is one of: started, finished, failed, cancelled, crashed
	Type []string `yaml:"type" json:"type" default:"[failed,crashed]"`
}

// this is the "master" struct which keeps all of the config settings (as specified in the config file + env vars)
// ANY CHANGE in this struct REQUIRES also an update to the Swagger YAML file to ensure the API is kept in sync
// ; also GetCopyWithLock() needs updating to ensure a deep copy is done
type CfgTemplate struct {
	DataDir       string             `required:"true" yaml:"data_dir" json:"data_dir"`
	HtmlDir       string             `default:"webstatic" yaml:"html_dir" json:"html_dir"`
	User          []ConfigUser       `yaml:"user" json:"user"`
	Http          ConfigHttp         `yaml:"http" json:"http"`
	Https         ConfigHttps        `yaml:"https" json:"https"`
	Backup        []ConfigBackup     `yaml:"backup" json:"backup"`
	Notifications ConfigNotification `yaml:"notification,omitempty" json:"notification,omitempty"`
	// the mutex is used for locking mainly only when we deal with copies of this struct (which may not have the parent
	// RuntimeConfig struct containing this struct)
	Mutex *sync.RWMutex `yaml:"-" json:"-"`
}

// this struct contains the above "master" config struct and also some runtime related parameters and settings
type RuntimeConfig struct {
	// lock this before reading or writing the config file or reading / writing the loaded config variables
	Mutex *sync.RWMutex
	// path to config file
	Path string
	// actual config file
	Config CfgTemplate
}

// return a copy of the config struct. Lock while reading the struct. logContext is used for passing the caller's
// logging context as to make it clear where the call is coming from
func (cfg *RuntimeConfig) GetCopyWithLock(logContext string) CfgTemplate {
	// log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying server config struct")
	cfg.Mutex.RLock()
	defer func() {
		cfg.Mutex.RUnlock()
		//log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying server " +
		//	"config struct")
	}()
	//log.WithFields(log.Fields{"context": logContext}).Debug("Read lock for copying server config acquired")
	cfgCopy := cfg.Config

	// we need to manually copy slices because by default a pointer to the slice is copied
	cfgCopy.User = make([]ConfigUser, len(cfg.Config.User))
	copy(cfgCopy.User, cfg.Config.User)
	cfgCopy.Backup = make([]ConfigBackup, len(cfg.Config.Backup))
	copy(cfgCopy.Backup, cfg.Config.Backup)
	// deepcopy the Notification.Email
	cfgCopy.Notifications.Email = make([]ConfigNotificationEmail, len(cfg.Config.Notifications.Email))
	copy(cfgCopy.Notifications.Email, cfg.Config.Notifications.Email)
	// deepcopy the Notification.Script
	cfgCopy.Notifications.Script = make([]ConfigNotificationScript, len(cfg.Config.Notifications.Script))
	copy(cfgCopy.Notifications.Script, cfg.Config.Notifications.Script)
	// deepcopy various slices part of the ConfigBackup{} struct
	for i := 0; i < len(cfg.Config.Backup); i++ {
		cfgCopy.Backup[i] = CopyConfigBackupStruct(cfg.Config.Backup[i])
	}
	// new mutex for locking
	cfgCopy.Mutex = &sync.RWMutex{}
	return cfgCopy
}

// makes a deep copy of a ConfigBackup struct
func CopyConfigBackupStruct(source ConfigBackup) ConfigBackup {
	result := source
	result.Paths = make([]string, len(source.Paths))
	copy(result.Paths, source.Paths)

	result.Exclusions = make([]string, len(source.Exclusions))
	copy(result.Exclusions, source.Exclusions)

	result.Schedule = make([]string, len(source.Schedule))
	copy(result.Schedule, source.Schedule)

	result.Target = make([]ConfigBackupTarget, len(source.Target))
	copy(result.Target, source.Target)

	// copy the Parameters slice for each target
	for k, v := range result.Target {
		result.Target[k].Parameters = make([]ConfigBackupTargetParams, len(v.Parameters))
		copy(result.Target[k].Parameters, source.Target[k].Parameters)
	}

	return result
}
