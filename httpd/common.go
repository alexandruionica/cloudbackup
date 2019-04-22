package httpd

import (
	"cloudbackup/backup/scan"
	"cloudbackup/config"
	"cloudbackup/objectstore"
	"cloudbackup/password"
	"cloudbackup/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

// various "code" messages the API can return
const (
	HttpErrBadContentType      = "bad content type"
	HttpErrInvalidJson         = "invalid json"
	HttpErrInvalidConfig       = "invalid config"
	HttpErrUnauthorized        = "unauthorized"
	HttpErrInternalServerError = "internal server error"
	HttpErrForbidden           = "access denied"
	HttpErrIncorrectClientData = "client supplied incorrect data"
	HttpErrNotFound            = "not found"
	HttpErrInternalError       = "internal server error"
)

type HttpStatusReply struct {
	HTTPCode int    `json:"-"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type SrvData struct {
	// if we receive something over the channel then a configuration change happened and we are being notified
	rcvCfgChange chan bool
	// we send something over the channel in order to notify when we adjusted the global config
	sndCfgChange chan bool
	httpsrv      *http.Server
	SslCertPath  string
	SslKeyPath   string
	httpsEnabled bool
	// when true then the web server is already being shutdown and cleanup is in progress
	serverExiting bool
	// pointer to the main configuration object shared across go routines. We use this to read and change configuration
	globalcfg *config.RuntimeConfig
	// lock this before reading or writing the loaded config variables
	Mutex *sync.RWMutex
	// used to send backup (start/stop) commands to the scheduler routine
	commWithSchedulerForBackup *shared.CommWithSchedulerForBackup
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState *shared.BackupJobsState
}

func (srv *SrvData) GetCopyWithLock(logContext string) SrvData {
	//log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read lock before copying HTTPD config " +
	//	"struct")
	srv.Mutex.RLock()
	defer func() {
		srv.Mutex.RUnlock()
		//log.WithFields(log.Fields{"context": logContext}).Debug("Read lock released after copying HTTPD " +
		//	"config struct")
	}()
	//log.WithFields(log.Fields{"context": logContext}).Debug("Read lock for copying HTTPD config acquired")
	cfgCopy := *srv
	return cfgCopy
}

// provides basic Authentication against username + password hashes stored in the config
// returns a httprouter.Handle function
func (srvSrc *SrvData) BasicAuth(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		LogHttpRequest(r)
		// Get the Basic Authentication credentials
		httpUser, httpPassword, hasAuth := r.BasicAuth()
		srv := srvSrc.GetCopyWithLock(loggingContext + ".BasicAuth")
		runtimeCfg := srv.globalcfg.GetCopyWithLock(loggingContext + ".BasicAuth")
		isAuthenticated := false
		errmsg := "Basic authentication is required. Please provide an username and password"

		if hasAuth && httpUser != "" {
			errmsg = "Invalid username or password"
			logger.Debugf("Checking if user: '%s' provided via HTTP(S) matches any username + password hash "+
				"from the config", httpUser)
			// while "runtimeCfg" is a copy, some of the data is pointers so locking is still needed as it may be
			// shared with other functions (running in other routines)
			runtimeCfg.Mutex.RLock()
			if len(runtimeCfg.User) == 0 {
				errmsg = "The server configuration doesn't have a 'User' section defined so http(s) authentication " +
					"will fail"
				logger.Debug(errmsg)
			} else {
				// check if a matching username + pass exists
				for _, user := range runtimeCfg.User {
					if user.Name == httpUser {
						logger.Debugf("Username '%s' matches an entry from the config, checking if password"+
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
			runtimeCfg.Mutex.RUnlock()

			if !isAuthenticated {
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
		if !hasAuth {
			msg := fmt.Sprintf("CheckAccess() called for an unauthenticated session for path '%s' and HTTP "+
				"method '%s'. Please submit a bug report", r.URL.Path, r.Method)
			logger.Error(msg)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
			return
		}

		srv := srvSrc.GetCopyWithLock(loggingContext + ".CheckAccess")
		runtimeCfg := srv.globalcfg.GetCopyWithLock(loggingContext + ".CheckAccess")
		hasAccess := false
		// while "runtimeCfg" is a copy, some of the data is pointers so locking is still needed as it may be
		// shared with other functions (running in other routines)
		runtimeCfg.Mutex.RLock()
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
						if r.URL.Path == ApiPrefix+path {
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
		runtimeCfg.Mutex.RUnlock()
		if hasAccess {
			// Delegate request to the given handle
			handle(w, r, ps)
			return
		} else {
			msg := fmt.Sprintf("User '%s' does not have access to '%s' using http method '%s'. If access is "+
				"needed then please request 'write' privileges from your systems admininistrator", httpUser,
				r.URL.Path, r.Method)
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
		Code:     code,
		Message:  message,
	}
	b, err := json.Marshal(e)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPCode)
	_, err = fmt.Fprint(w, string(b))
	if err != nil {
		logger.Debugf("Encountered an error while writing reply to client: %s", err)
	}
}

// send HTTP success back to user in JSON format; "code" is a short message to show, "message" is a detailed explanation
func JSONSuccess(w http.ResponseWriter, code string, message string) {
	status := HttpStatusReply{
		HTTPCode: 200,
		Code:     code,
		Message:  message,
	}

	b, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "Internal Server Error when trying to reply that requested operation was successful",
			500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPCode)
	_, err = fmt.Fprint(w, string(b))
	if err != nil {
		logger.Debugf("Encountered an error while writing reply to client: %s", err)
	}
}

// send HTTP success back to user in JSON format; "code" is a short message to show, "message" is a detailed explanation
// result is any Struct which can be json.Marshall-ed and it contains operation specific data
func JSONSuccessWithResult(w http.ResponseWriter, code string, message string, result interface{}) {
	status := HttpStatusReply{
		HTTPCode: 200,
		Code:     code,
		Message:  message,
	}
	response := struct {
		HttpStatusReply
		Result interface{} `json:"result"`
	}{status, result}

	b, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Internal Server Error when trying to reply that requested operation was successful",
			500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.HTTPCode)
	_, err = fmt.Fprint(w, string(b))
	if err != nil {
		logger.Debugf("Encountered an error while writing reply to client: %s", err)
	}
}

// validate HTTP input of type JSON. We Buffer the request body so we can process it multiple times. This is bad if
// someone sends a very large payload as they could cause a DOS by crashing the daemon so
// ONLY AUTHENTICATED SESSIONS SHOULD USE THIS FUNCTION
func ValidateJsonHTTPInput(w http.ResponseWriter, r *http.Request) (bodyBytes []byte, err error) {
	if r.Header.Get("Content-Type") != "application/json" {
		msg := fmt.Sprintf("%s 'Content-Type' is not of type 'application/json'", r.Method)
		JSONError(w, http.StatusBadRequest, HttpErrBadContentType, msg)
		return bodyBytes, errors.New(msg)
	}
	// Buffer the request body so we can process it multiple times.
	bodyBytes, err = ioutil.ReadAll(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Error reading request body with containing new config. The encountered error"+
			" was: %s", err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		logger.Warn(msg)
		return bodyBytes, err
	}
	var decodedJson interface{}
	// err = json.NewDecoder(bodyBytes).Decode(&decodedJson)
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprintf("Request body is not valid JSON. "+
			"While attempting to decode the JSON payload the following error was encountered: %s", err))
		return bodyBytes, err
	}
	return bodyBytes, nil
}

// basic logging of http requests. Does not include response code. Requests wrapped with BasicAuth() will get logged but
// otherwise you must call this function
func LogHttpRequest(r *http.Request) {
	log.WithFields(log.Fields{"context": loggingContext + ".access"}).Infof("%s %s %s %s", r.RemoteAddr,
		r.Method, r.Host, r.RequestURI)
}

// calls an "evaluate" of the backup paths for a particular job
func dryRunBackupPaths(ctx context.Context, backupConfig config.Backup, backupJobsState *shared.DryRunBackupJobsState,
	scanPathExit chan bool) {
	for _, path := range backupConfig.Paths {
		// empty objectStores object as a dry run should never reach an upload function anyway
		objectStores := make([]objectstore.ObjectStore, 0)
		u, err := uuid.NewV4()
		if err != nil {
			logger.Errorf("Could not generate a UUID so the dry run can't proceed. Encountered error was: %s", err)
			scanPathExit <- true
			return
		}
		jobId := u.String()
		// backupJobsState MUST be a pointer
		exiting, err := scan.Path(ctx, path, backupConfig, backupJobsState, true, shared.DbData{Connected: false}, objectStores, jobId)
		// Examine FIRST $exit and then $err ;  $exiting means that a signal was sent so scan.Path() exits, on request,
		// 	early
		if exiting {
			logger.Debug("scan.Path() reported to dryRunBackupPaths() that it was requested to cancel its run")
			scanPathExit <- true
			logger.Debug("dryRunBackupPaths() successfully messaged that it's exiting and so it's exiting")
			return
		}
		if err != nil {
			logger.Debug("dryRunBackupPaths() has encountered and error and is exiting")
			scanPathExit <- true
			return
		}
	}
	// message HttpEval handler that scan.Path() has completed successfully its run
	scanPathExit <- true
	logger.Debug("dryRunBackupPaths() has successfully completed its run and is exiting")
}
