package httpd

import (
	"cloudbackup/config"
	"github.com/julienschmidt/httprouter"
	"fmt"
	"net/http"
	"encoding/json"
	"cloudbackup/utils"
	"os"
	"github.com/jinzhu/configor"
	"bytes"
)

// serve $api_prefix/config and logger.Info requester
func (srvSrc SrvData) handlerGetConfig(w http.ResponseWriter, r *http.Request, _ httprouter.Params){
	srv := srvSrc.GetWithLock(loggingContext + ".pageRoot")
	runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".handlerGetConfig")

	// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
	//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
	sanitizedcfg := config.SanitizeCfgTemplate(runtimeCfg)

	JSONSuccessWithResult(w, "success", "successfully retrieved server configuration", sanitizedcfg)
}

// process POST for $api_prefix/config . If successful then it updates the whole daemon config
func (srvSrc SrvData) handlerPutConfig(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	bodyBytes , err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}

	tmpFilePath, err := utils.SetupTmpFileWithContent(bodyBytes, "new_config_")
	if err != nil {
		msg := fmt.Sprintf("Error writing temporary file to hold the new configuration. The encountered" +
			" error was: %s", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		logger.Error(msg)
		return
	}
	// remove tmpfile which holds the new config as it has been parsed and loaded. The context will be kept so despite
	// renaming tmpFilePath (later below) the correct file will be deleted
	defer func() {
		err := os.Remove(tmpFilePath)
		if err != nil {
			logger.Errorf("Encountered error: '%s' when trying to delete temporary file %s", err, tmpFilePath)
		}
	}()

	// rename file so it has .json extension . Otherwise the "configor" library needs to guess if it's a YAML or JSON
	// file
	err = os.Rename(tmpFilePath, tmpFilePath + ".json")
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to rename temporary file %s to  %s", err,
			tmpFilePath, tmpFilePath + ".json")
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"manipulating new configuration file")
		return
	}
	tmpFilePath = tmpFilePath + ".json"
	var NewConfig = config.CfgTemplate{}

	// load new config from tmp file and perform basic validation
	err = configor.New(&configor.Config{ENVPrefix: config.EnvPrefix}).Load(&NewConfig, tmpFilePath)
	if err != nil {
		msg := fmt.Sprintf("When validating the new configuration the following error was encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}

	// perform advanced validation of the above loaded (in a struct) config
	err = config.Validate(NewConfig, true)
	if err != nil {
		msg := fmt.Sprintf("When validating the new configuration the following error was encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}

	srv := srvSrc.GetWithLock(loggingContext + ".handlerPutConfig")
	oldConfig := srv.globalcfg.GetWithLock(loggingContext + ".handlerPutConfig")

	// for password fields containing only asterisks ('*******') attempt to read the actual password from the old config
	err = config.CopyPasswordsFromOldConfig(&NewConfig, oldConfig)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, err.Error())
		logger.Debug(err.Error())
		return
	}

	oldConfigMarshalled, err := json.Marshal(oldConfig)
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to json.Marshall() existing config in order to " +
			"compare it with the new config", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"trying to compare old config with the new one in order to establish if they differ")
		return
	}
	NewConfigMarshalled, err := json.Marshal(NewConfig)
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to json.Marshall() new config in order to " +
			"compare it with the old config", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"trying to compare new config with the old one in order to establish if they differ")
		return
	}
	// TODO - do not allow configuration changes for a given Backup if there are in progress backups or restores for
	// that section

	var writeErr error
	// compare new and old config and if there is no difference then don't rewrite the config file
	if bytes.Equal(oldConfigMarshalled, NewConfigMarshalled) {
		logger.Debug("old and new config match")
		JSONSuccess(w, "success", "The supplied configuration matches the existing one so no actual " +
			"changes are going to take effect")
		return
	} else {
		logger.Debug("Acquiring lock for HTTP server config before writing config file and updating in-memory " +
			"configuration")
		srvSrc.Mutex.Lock()
		defer func() {
			srvSrc.Mutex.Unlock()
			logger.Debug("HTTP server lock released after attempting to write config file")
		}()
		writeErr = config.Save(srvSrc.globalcfg, NewConfig)
		if writeErr != nil {
			logger.Error(writeErr.Error())
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, writeErr.Error())
			return
		} else {
			// log that a config change happened
			httpUser, _, _ := r.BasicAuth()
			logger.Infof("Configuration file '%s' updated with new content as requested by user '%s' from '%s' via" +
				" '%s' to '%s%s'", srv.globalcfg.Path, httpUser, r.RemoteAddr, r.Method, r.Host, r.RequestURI)
		}
	}

	// notify daemon(master) that a config change happened. The only reason to do so would be to notify the
	// builtin "cron"(scheduling) daemon
	srvSrc.sndCfgChange <- true

	JSONSuccess(w, "success", "Successfully updated server configuration. Any changes to SSL " +
		"certificates, ports and addresses to listen on and if to use http or https will require a server restart in" +
		" order to take effect")
	return
}

// process POST for $api_prefix/config/backup . If successful then it updates the whole daemon config
func (srvSrc SrvData) handlerPutConfigBackup(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	bodyBytes , err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}

	tmpFilePath, err := utils.SetupTmpFileWithContent(bodyBytes, "new_config_backup_")
	if err != nil {
		msg := fmt.Sprintf("Error writing temporary file to hold the new backup section configuration. The " +
			"encountered error was: %s", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		logger.Error(msg)
		return
	}
	// remove tmpfile which holds the new config as it has been parsed and loaded. The context will be kept so despite
	// renaming tmpFilePath (later below) the correct file will be deleted
	defer func() {
		err := os.Remove(tmpFilePath)
		if err != nil {
			logger.Errorf("Encountered error: '%s' when trying to delete temporary file %s", err, tmpFilePath)
		}
	}()

	// rename file so it has .json extension . Otherwise the "configor" library needs to guess if it's a YAML or JSON
	// file
	err = os.Rename(tmpFilePath, tmpFilePath + ".json")
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to rename temporary file %s to  %s", err,
			tmpFilePath, tmpFilePath + ".json")
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"manipulating new configuration file")
		return
	}
	tmpFilePath = tmpFilePath + ".json"
	var NewConfigBackup = config.Backup{}

	// load new config from tmp file and perform basic validation
	err = configor.New(&configor.Config{ENVPrefix: config.EnvPrefix}).Load(&NewConfigBackup, tmpFilePath)
	if err != nil {
		msg := fmt.Sprintf("When validating the new backup configuration section the following error was" +
			" encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}
	// slice to hold the Backup object as the Validate function expects a slice as input
	backups := make([]config.Backup, 0)
	backups = append(backups, NewConfigBackup)
	// perform advanced validation of the above loaded (in a struct) config
	err = config.ValidateBackup(backups, true)
	if err != nil {
		msg := fmt.Sprintf("When validating the new backup configuration section the following error was " +
			"encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}

	srv := srvSrc.GetWithLock(loggingContext + ".handlerPutConfig")
	oldConfig := srv.globalcfg.GetWithLock(loggingContext + ".handlerPutConfig")
	// we'll put any config changes in the "copy" of the old config
	NewConfig := srv.globalcfg.GetWithLock(loggingContext + ".handlerPutConfig")

	// for password fields containing only asterisks ('*******') attempt to read the actual password from the old config
	err = config.CopyPasswordsFromOldConfigBackup(backups, oldConfig.Backup)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, err.Error())
		logger.Debug(err.Error())
		return
	}
	// processed element may contain overwritten passwords so copy them back to the original Object
	NewConfigBackup = backups[0]
	// check if in the active config (aka old one) we have backup with the same name and if so then overwrite it
	matchFound := false
	for i := 0; i < len(oldConfig.Backup); i++ {
		if NewConfig.Backup[i].Name == NewConfigBackup.Name {
			NewConfig.Backup[i] = NewConfigBackup
			matchFound = true
			break
		}
	}

	// if no match found then this is a new "Backup" entry so append it to the existing slice of Backup entries
	if matchFound == false {
		NewConfig.Backup = append(NewConfig.Backup, NewConfigBackup)
	}

	oldConfigMarshalled, err := json.Marshal(oldConfig)
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to json.Marshall() existing config in order to " +
			"compare it with the new config", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"trying to compare old config with the new one in order to establish if they differ")
		return
	}
	NewConfigMarshalled, err := json.Marshal(NewConfig)
	if err != nil {
		logger.Errorf("Encountered error: '%s' when trying to json.Marshall() new config in order to " +
			"compare it with the old config", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "internal error when " +
			"trying to compare new config with the old one in order to establish if they differ")
		return
	}
	var writeErr error
	if bytes.Equal(oldConfigMarshalled, NewConfigMarshalled) {
		logger.Debug("old and new config match")
		JSONSuccess(w, "success", "The supplied configuration matches the existing one so no actual " +
			"changes are going to take effect")
		return
	} else {
		logger.Debug("Acquiring lock for HTTP server config before writing config file and updating in-memory " +
			"configuration")
		srvSrc.Mutex.Lock()
		defer func() {
			srvSrc.Mutex.Unlock()
			logger.Debug("HTTP server lock released after attempting to write config file")
		}()
		writeErr = config.Save(srvSrc.globalcfg, NewConfig)
		if writeErr != nil {
			logger.Error(writeErr.Error())
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, writeErr.Error())
			return
		} else {
			// log that a config change happened
			httpUser, _, _ := r.BasicAuth()
			logger.Infof("Configuration file '%s' updated with new content as requested by user '%s' from '%s' via" +
				" '%s' to '%s%s'", srv.globalcfg.Path, httpUser, r.RemoteAddr, r.Method, r.Host, r.RequestURI)
		}
	}

	// notify daemon(master) that a config change happened. The only reason to do so would be to notify the
	// builtin "cron"(scheduling) daemon
	srvSrc.sndCfgChange <- true

	JSONSuccess(w, "success", "Successfully updated server configuration. Any changes to SSL " +
		"certificates, ports and addresses to listen on and if to use http or https will require a server restart in" +
		" order to take effect")
	return
}
