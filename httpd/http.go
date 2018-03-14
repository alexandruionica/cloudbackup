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
)
const loggingContext = "httpd"
// various "code" messages the API can return
const (
	HttpErrBadContentType = "bad content type"
	HttpErrInvalidJson = "invalid json"
	HttpErrInvalidConfig = "invalid config"
	HttpErrUnauthorized = "Unauthorized"
	HttpErrInternalServerError = "Internal Server Error"
)
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})


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

// serve / and logger.Info requester
func (srvSrc SrvData) handlerRoot(w http.ResponseWriter, r *http.Request, _ httprouter.Params){
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
	srv := srvSrc.GetWithLock(loggingContext + "_pageRoot")
	runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".handlerGetConfig")

	// config.SanitizeCfgTemplate takes care of replacing passwords with *** . Unfortunately this function doesn't have
	//  any smarts so whenever the config struct is changed then also config.SanitizeCfgTemplate needs updating
	sanitizedcfg := config.SanitizeCfgTemplate(runtimeCfg)
	status := HttpStatusReply{
		HTTPCode: 200,
		Code: "success",
		Message: "successfully retrieved server configuration",
	}
	result := struct {
		HttpStatusReply
		Result config.CfgTemplate `json:"result"`
	} {status,
	sanitizedcfg}
	js, err := json.Marshal(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(js)
	if err != nil {
		logger.Debug("handlerGetConfig() - could not write response back to client ")
	}

	logger.Info(fmt.Sprintf("HTTP request for RequestURI: %s from requester: %s ", r.RequestURI, r.RemoteAddr))
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

	// load config file and perform basic validation
	err = configor.New(&configor.Config{ENVPrefix: config.EnvPrefix}).Load(&NewConfig, tmpFilePath)
	if err != nil {
		msg := fmt.Sprintf("When validating the new configuration the following error was encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}

	// perform advanced validation of the already loaded (in a struct) config
	err = config.Validate(NewConfig)
	if err != nil {
		msg := fmt.Sprintf("When validating the new configuration the following error was encountered: %s", err)
		JSONError(w, http.StatusBadRequest, HttpErrInvalidConfig, msg)
		logger.Debug(msg)
		return
	}
	utils.Pp(NewConfig)

}

// provides basic Authentication against username + password hashes stored in the config
// returns a httprouter.Handle function
func (srvSrc *SrvData) BasicAuth(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Get the Basic Authentication credentials
		httpUser, httpPassword, hasAuth := r.BasicAuth()
		srv := srvSrc.GetWithLock(loggingContext + ".BasicAuth")
		runtimeCfg := srv.globalcfg.GetWithLock(loggingContext + ".BasicAuth")
		isAuthenticated := false
		errmsg := "Basic authentication is required. Please provide an username and password"

		if hasAuth {
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
		} else {
			// Request Basic Authentication otherwise
			w.Header().Set("WWW-Authenticate", "Basic realm=Restricted")
			JSONError(w, http.StatusUnauthorized, HttpErrUnauthorized, errmsg)
		}
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
	apiPrefix := "/api/v1"
	router := httprouter.New()
	router.GET("/", srv.handlerRoot)
	router.GET(apiPrefix + "/config", srv.BasicAuth(srv.handlerGetConfig))
	// handlerPutConfig
	router.POST(apiPrefix + "/config", srv.BasicAuth(srv.handlerPutConfig))

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

// send HTTP error back to user in JSON format; httpcode is code to reply with, "code" is a short message to show,
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