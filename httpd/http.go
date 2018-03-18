package httpd

import (
	"cloudbackup/config"
	"cloudbackup/password"
	"context"
	log "github.com/sirupsen/logrus"
	"github.com/julienschmidt/httprouter"
	"fmt"
	"net/http"
	"time"
	"sync"
	"encoding/json"
	"io/ioutil"
	"cloudbackup/utils"
	"os"
	"github.com/jinzhu/configor"
	"errors"
	"bytes"
	"strings"
)
const (
	loggingContext = "httpd"
	ApiPrefix = "/api/v1"
)
// various "code" messages the API can return
const (
	HttpErrBadContentType = "bad content type"
	HttpErrInvalidJson = "invalid json"
	HttpErrInvalidConfig = "invalid config"
	HttpErrUnauthorized = "unauthorized"
	HttpErrInternalServerError = "internal server error"
	HttpErrForbidden = "access denied"
)
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// used to check if a user has access to a given PATH . Path will be prefixed with $ApiPrefix in the calling function
var ReadAccess = map[string][]string{
	//"POST": []string{"aaa", "bbb"},
	"GET":  {"/config"},
}


//type SrvData interface {
//	New(chan bool, int, string)
//}

type SrvData struct {
	// if we receive something over the channel then a configuration change happened and we are being notified
	rcvCfgChange chan bool
	// we send something over the channel in order to notify when we adjusted the global config
	sndCfgChange chan bool
	httpsrv *http.Server
	SslCertPath string
	SslKeyPath string
	httpsEnabled bool
	// when true then the web server is already being shutdown and cleanup is in progress
	serverExiting bool
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	globalcfg *config.RuntimeConfig
	// lock this before reading or writing the loaded config variables
	Mutex *sync.RWMutex
}

type HttpStatusReply struct {
	HTTPCode int `json:"-"`
	Code string `json:"code"`
	Message  string `json:"message"`
}

func (srv *SrvData) GetWithLock(logContext string) SrvData {
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying HTTPD config " +
		"struct")
	srv.Mutex.RLock()
	defer func() {
		srv.Mutex.RUnlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying HTTPD " +
			"config struct")
	}()
	cfgCopy := *srv
	return cfgCopy
}

// pseudo constructor to setup a new http server
func New(rcvCfgChange chan bool, sndCfgChange chan bool, globalcfg *config.RuntimeConfig, addr string,
	httpsEnabled bool, SslCertPath string, SslKeyPath string ) (*SrvData) {

	return &SrvData{rcvCfgChange: rcvCfgChange,
		sndCfgChange: sndCfgChange,
		serverExiting: false,
		globalcfg: globalcfg,
		httpsrv: &http.Server{
			Addr: addr,
			Handler: nil,
		},
		SslCertPath: SslCertPath,
		SslKeyPath: SslKeyPath,
		httpsEnabled: httpsEnabled,
		Mutex: &sync.RWMutex{},
	}
}

// start http server
func (srv *SrvData) Start() {
	var protocol, msg string
	if srv.httpsEnabled {
		protocol = "https://"
		msg = fmt.Sprintf(" using ssl certificate %s and ssl key %s", srv.SslCertPath, srv.SslKeyPath)
	} else {
		msg = ""
		protocol = "http://"
	}
	logger.Infof("Starting web server to listen on %s%s%s", protocol, srv.httpsrv.Addr, msg)
	router := httprouter.New()
	router.GET("/", srv.handlerRoot)
	router.GET(ApiPrefix+ "/config", srv.BasicAuth(srv.CheckAccess(srv.handlerGetConfig)))
	// handlerPutConfig
	router.POST(ApiPrefix+ "/config", srv.BasicAuth(srv.CheckAccess(srv.handlerPutConfig)))

	// put a write lock and update the router - by this point all routes should have been added
	srv.Mutex.Lock()
	srv.httpsrv.Handler = router
	srv.Mutex.Unlock()
	logger.Debug(fmt.Sprintf("%+v", srv))
	go func() {
		var err error
		var extraMsg string
		if srv.httpsEnabled {
			extraMsg = "HTTPS"
			err = srv.httpsrv.ListenAndServeTLS(srv.SslCertPath, srv.SslKeyPath)
		} else {
			extraMsg = "HTTP"
			err = srv.httpsrv.ListenAndServe()
		}
		srvCopy := srv.GetWithLock(loggingContext)
		if err != nil && srvCopy.serverExiting == false {
			logger.Errorf("%s server could not be started or encountered an error during it's operation",
				extraMsg)
			logger.Error(err)
		}
	}()
}

// shutdown gracefully the http server using 30 sec timeout
func (srv *SrvData) Stop(){
	logger.Info("Shutting down the http server...")
	srv.Mutex.Lock()
	srv.serverExiting = true
	srv.Mutex.Unlock()

	// preparation to exit with grace period of 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := srv.httpsrv.Shutdown(ctx)
	if err != nil {
		logger.Error(err)
	}

}

// serve / and logger.Info requester
func (srvSrc SrvData) handlerRoot(w http.ResponseWriter, r *http.Request, _ httprouter.Params){
	LogHttpRequest(r)
	srv := srvSrc.GetWithLock(loggingContext + ".handlerRoot")
	if srv.httpsEnabled{
		_, err := w.Write([]byte("HTTPS server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	} else {
		_, err := w.Write([]byte("HTTP server is running\n"))
		if err != nil {
			logger.Debug("handlerRoot() - could not write response back to client ")
		}
	}
	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", r.RequestURI, r.RemoteAddr))
}

// serve $api_prefix/config and logger.Info requester
func (srvSrc SrvData) handlerGetConfig(w http.ResponseWriter, r *http.Request, _ httprouter.Params){
	srv := srvSrc.GetWithLock(loggingContext + ".pageRoot")
	runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".handlerGetConfig")

	// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
	//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
	sanitizedcfg := config.SanitizeCfgTemplate(runtimeCfg)

	JSONSuccessWithResult(w, "success", "successfully retrieved server configuration", sanitizedcfg)
	}

// process POST for $api_prefix/config . If susccessful then it updates the whole daemon config
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
	// TODO - compare new and old config and if there is no difference then don't rewrite the config file
	if bytes.Equal(oldConfigMarshalled, NewConfigMarshalled) {
		logger.Debug("old and new config match")
		JSONSuccess(w, "success", "The supplied configuration matches the existing one so no actual " +
			"changes are going to take effect")
	}


	// TODO - add code to copy the new values over the old config structure or better just write the file and tell the
	// daemon to reload config
}

// provides basic Authentication against username + password hashes stored in the config
// returns a httprouter.Handle function
func (srvSrc *SrvData) BasicAuth(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		LogHttpRequest(r)
		// Get the Basic Authentication credentials
		httpUser, httpPassword, hasAuth := r.BasicAuth()
		srv := srvSrc.GetWithLock(loggingContext + ".BasicAuth")
		runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".BasicAuth")
		isAuthenticated := false
		errmsg := "Basic authentication is required. Please provide an username and password"

		if hasAuth && httpUser !="" {
			errmsg = "Invalid username or password"
			logger.Debugf("Checking if user: '%s' provided via HTTP(S) matches any username + password hash " +
				"from the config", httpUser)
			if len(runtimeCfg.User) == 0 {
				errmsg = "The server configuration doesn't have a 'User' section defined so http(s) authentication " +
					"will fail"
				logger.Debug(errmsg)
			} else {
				// check if a matching username + pass exists
				for _, user := range runtimeCfg.User {
					if user.Name == httpUser {
						logger.Debugf("Username '%s' matches an entry from the config, checking if password" +
							" matches the stored hash", httpUser)
						if password.CheckPasswordHash(httpPassword, user.Pass) {
							logger.Debugf("Password provided for username '%s' matches stored password hash",
								httpUser)
							isAuthenticated = true
							break
						}
					}
				}
			}

			if isAuthenticated == false {
				logger.Debug("Could not find any matching username + password(hash) in the config")
			}
		}

		if isAuthenticated {
			// Delegate request to the given handle
			handle(w, r, ps)
			return
		} else {
			// Request Basic Authentication otherwise
			w.Header().Set("WWW-Authenticate", "Basic realm=Restricted")
			JSONError(w, http.StatusUnauthorized, HttpErrUnauthorized, errmsg)
			return
		}
	}
}

// check if the user has access to the given path and method. The session MUST already be authenticated
func (srvSrc *SrvData) CheckAccess(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Get the Basic Authentication credentials
		httpUser, _, hasAuth := r.BasicAuth()
		if hasAuth != true {
			msg := fmt.Sprintf("CheckAccess() called for an unauthenticated session for path '%s' and HTTP " +
				"method '%s'. Please submit a bug report", r.URL.Path, r.Method)
			logger.Error(msg)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
			return
		}

		srv := srvSrc.GetWithLock(loggingContext + ".CheckAccess")
		runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".CheckAccess")
		hasAccess := false
		for _, user := range runtimeCfg.User {
			if user.Name == httpUser {
				if strings.ToLower(user.Access) == "write" {
					// "write" grants access to everything
					hasAccess = true
					break
				}
				// check we have a key with the method value in ReadAccess
				if _, ok := ReadAccess[r.Method]; ok {
					for _, path := range ReadAccess[r.Method] {
						if r.URL.Path == ApiPrefix + path {
							logger.Infof("found match for %s", r.URL.Path)
							hasAccess = true
							break
						}
					}
					if hasAccess {
						break
					}
				}

			}
		}
		if hasAccess {
			// Delegate request to the given handle
			handle(w, r, ps)
			return
		} else {
			msg := fmt.Sprintf("User '%s' does not have access to '%s' using http method '%s'. Request 'write'" +
				" privileges from your Admin if access is needed", httpUser, r.URL.Path, r.Method)
			logger.Debug(msg)
			JSONError(w, http.StatusForbidden, HttpErrForbidden, msg)
			return
		}

	}
}

// send HTTP error back to user in JSON format; "httpcode" is HTTP status code to reply with, "code" is a short message to show,
// "message" is a detailed explanation of what when wrong
func JSONError(w http.ResponseWriter, httpcode int, code string, message string) {
	e := HttpStatusReply{
		HTTPCode: httpcode,
		Code: code,
		Message: message,
	}
	b, err := json.Marshal(e)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPCode)
	fmt.Fprint(w, string(b))
}

// send HTTP success back to user in JSON format; "code" is a short message to show, "message" is a detailed explanation
func JSONSuccess(w http.ResponseWriter, code string, message string) {
	status := HttpStatusReply{
		HTTPCode: 200,
		Code: code,
		Message: message,
	}

	b, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "Internal Server Error when trying to reply that requested operation was successful",
			500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPCode)
	fmt.Fprint(w, string(b))
}

// send HTTP success back to user in JSON format; "code" is a short message to show, "message" is a detailed explanation
// result is any Struct which can be json.Marshall-ed and it contains operation specific data
func JSONSuccessWithResult(w http.ResponseWriter, code string, message string, result interface{}) {
	status := HttpStatusReply{
		HTTPCode: 200,
		Code: code,
		Message: message,
	}
	response := struct {
		HttpStatusReply
		Result interface{} `json:"result"`
	}{ status, result}

	b, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Internal Server Error when trying to reply that requested operation was successful",
			500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.HTTPCode)
	fmt.Fprint(w, string(b))
}

// validate HTTP input of type JSON. We Buffer the request body so we can process it multiple times. This is bad if
// someone sends a very large payload as they could cause a DOS by crashing the daemon so
// ONLY AUTHENTICATED SESSIONS SHOULD USE THIS FUNCTION
func ValidateJsonHTTPInput (w http.ResponseWriter, r *http.Request) (bodyBytes []byte, err error) {
	if r.Header.Get("Content-Type") != "application/json" {
		msg := fmt.Sprintf("%s 'Content-Type' is not of type 'application/json'", r.Method)
		JSONError(w, http.StatusBadRequest, HttpErrBadContentType, msg)
		return bodyBytes, errors.New(msg)
	}
	// Buffer the request body so we can process it multiple times.
	bodyBytes , err = ioutil.ReadAll(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Error reading request body with containing new config. The encountered error" +
			" was: %s", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		logger.Warn(msg)
		return bodyBytes, err
	}
	var decodedJson interface{}
	// err = json.NewDecoder(bodyBytes).Decode(&decodedJson)
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprintf("Request body is not valid JSON. " +
			"While attempting to decode the JSON payload the following error was encountered: %s", err))
		return bodyBytes, err
	}
	return bodyBytes, nil
}

// basic logging of http requests. Does not include response code. Requests wrapped with BasicAuth() will get logged but
// otherwise you must call this function
func LogHttpRequest(r *http.Request){
	log.WithFields(log.Fields{"context": loggingContext + ".access"}).Infof("%s %s %s %s", r.RemoteAddr,
		r.Method, r.Host, r.RequestURI)
}