package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/notifications"
	"cloudbackup/shared"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ReportBackupList struct {
	// Job definition's name for which to build a report list
	Name string `json:"name"`
	// token to use when pagination is needed. Token format is:   offset:max_results
	Next string `json:"next"`
	// max number of results to return. Ignored when $Next is supplied
	MaxResults uint64 `json:"max_results"`
	// select only reports which have a start_time >= $FromStartTime
	FromStartTime string `json:"from_start_time"`
	// select only reports which have a start_time <= $ToStartTime
	UntilStartTime string `json:"until_start_time"`
}

type ReportBackupListDbResults struct {
	Name      string `json:"name"`
	JobId     string `json:"job_id"`
	StartTime string `json:"start_time"` // Format MUST be time.RFC3339Nano
	EndTime   string `json:"end_time"`   // Format MUST be time.RFC3339Nano
	State     string `json:"state"`
}

type ReportBackupJob struct {
	Name  string `json:"name"`
	JobId string `json:"job_id"`
}

type ReportBackupFileList struct {
	// Job definition's name for which to build a report list
	Name string `json:"name"`
	// token to use when pagination is needed. Token format is:   offset:max_results
	Next string `json:"next"`
	// max number of results to return. Ignored when $Next is supplied
	MaxResults uint64 `json:"max_results"`
	JobId      string `json:"job_id"`
	Path       string `json:"path"`
	Descend    bool   `json:"descend"`
}

type ReportBackupFileListInstanceDbResults struct {
	JobId string `json:"job_id"`
	// Job definition's name for which to build a report list
	JobName      string `json:"job_name"`
	JobStartTime string `json:"job_start_time"` // Format MUST be time.RFC3339Nano
	// Backup target name as defined in the configuration file, at the time of the backup
	JobTarget string `json:"target"`
	// File size. For directories this will be 0.
	Size uint64 `json:"size"`
	// One of file|directory|symlink
	Type string `json:"type"`
	// when was this particular version of the file/dir/symlink uploaded to the remote object store. It is very likely
	// that for example a backup done yesterday would have a file unchanged so it would lets say have been initially
	// backed up 3 weeks ago so this upload_date would be the date of 3 weeks ago
	UploadDate string `json:"upload_date"` // Format MUST be time.RFC3339Nano
	// same meaning as in the DB - if true then the file/dir/symlink is in a deleted state
	DeleteMarker bool `json:"deleted"`
}

type ReportBackupFileListDbResults struct {
	// Absolute path of the file/dir/symlink
	Path string `json:"path"`
	// One or more backed up versions of this item
	Instances []ReportBackupFileListInstanceDbResults `json:"instances"`
}

const LimitReportResults = 1000
const LimitReportResultsDefault = 100
const LimitReportFileResults = 10000
const LimitReportFileResultsDefault = 1000

// runs all Notification definitions from the config file, wait for them to complete(or fail) and reply to the client
func (srvSrc SrvData) handlerPostNotificationTest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostNotificationTest")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostNotificationTest")

	if notifications.GetNumNotificators(configCopy.Notifications) == 0 {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, "Notification test can not be run as there "+
			"are no notification entries in the server's configuration file")
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		msg := fmt.Sprintf("Could not generate a UUID so the notification test operation can't be started. Encountered error was: %s", err)
		logger.Error(msg)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		return
	}
	jobId := u.String()
	_, err = notifications.Execute(configCopy, jobId, "backup", "test", "notifications_test", "", "")
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	JSONSuccess(w, "success", fmt.Sprintf("Test completed successfully for job id '%s'", jobId))
}

func (srvSrc SrvData) handlerPostReportBackupList(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson ReportBackupList
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name"+
			" is needed in order to know what backup definition you're requesting reports for"))
		return
	}
	jobName := decodedJson.Name

	var limit uint64 = LimitReportResultsDefault
	if decodedJson.MaxResults > LimitReportResults {
		logger.Debugf("Requested max results of '%d' is larger then hardcoded limit of '%d'. Will return up"+
			" to '%d' results", decodedJson.MaxResults, LimitReportResults, LimitReportResults)
		limit = LimitReportResults
	} else {
		if decodedJson.MaxResults > 0 {
			limit = decodedJson.MaxResults
		}
	}

	var FromStartTime, UntilStartTime int64
	if decodedJson.FromStartTime == "" {
		FromStartTime = time.Now().AddDate(0, 0, -30).UnixNano() // 30 days ago
	} else {
		tmpTime, err := time.Parse(time.RFC3339Nano, decodedJson.FromStartTime)
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson,
				fmt.Sprintf("Provided value '%s' for 'from_start_time' could not be parsed into a time object which "+
					"is RFC3339 with nanoseconds compliant due to error: %s", decodedJson.FromStartTime, err.Error()))
			return
		} else {
			FromStartTime = tmpTime.UnixNano()
		}
	}

	if decodedJson.UntilStartTime == "" {
		UntilStartTime = time.Now().UnixNano()
	} else {
		tmpTime, err := time.Parse(time.RFC3339Nano, decodedJson.UntilStartTime)
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson,
				fmt.Sprintf("Provided value '%s' for 'until_start_time' could not be parsed into a time object which "+
					"is RFC3339 with nanoseconds compliant due to error: %s", decodedJson.UntilStartTime, err.Error()))
			return
		} else {
			UntilStartTime = tmpTime.UnixNano()
		}
		if UntilStartTime < FromStartTime {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson,
				fmt.Sprintf("Provided value '%s' for 'until_start_time' represents a value which is earlier "+
					"than 'from_start_time' 's own value of %s", decodedJson.UntilStartTime,
					time.Unix(0, FromStartTime).Format(time.RFC3339Nano)))
			return
		}
	}

	var offset uint64 = 0
	if decodedJson.Next != "" {
		logger.Debugf("Ignoring any values passed in for 'max_results', 'from_start_time' and 'until_start_time'" +
			" as we had a valid 'next' supplied too")
		limit, offset, FromStartTime, UntilStartTime, err = decodeNextTokenOfReportBackupList(decodedJson.Next)
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
			return
		}
	}

	dbData, backupJobsState, err := getDbAccess(srvSrc, jobName, w, loggingContext+".handlerPostReportBackupList")
	if err == nil {
		// TODO - there is a potentially big problem here if a client disconnects before we setup the below defer() as
		// the structure which records how many DB connected clients we have would end up with extra marked clients
		// (but the actual clients have long disconnected). This leads to any attempt to close the database to hang as
		// the closing function waits for all clients to disconnect
		defer dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	} else {
		// getDbAccess() takes care of passing the error back to the client via $w so there isn't anything else to do
		return
	}

	dbResults, err := getRowsForHandlerPostReportBackupList(dbData, jobName, limit, offset, FromStartTime, UntilStartTime)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	next := ""
	// if returned results == $limit  then sent back a "next" value to client so it knows it doesn't have the full result set
	if uint64(len(dbResults)) == limit {
		next = buildNextTokenOfReportBackupList(limit, offset+limit, FromStartTime, UntilStartTime)
	}

	JSONSuccessWithResultPaginated(w, "success", "success", next, dbResults)
}

// retrieves results from the database and reports any errors. If an error is returned then the
// []ReportBackupListDbResults should be discarded/disregarded
// $limit represents how many records to retrieve in one go; $offset is the offset to start request records from
// (this is related to $limit); $earliestStart is the start date+time(Unix time in nanoseconds) of the oldest backup
// to consider for reporting back; $latestStart is the start date+time(Unix time in nanoseconds) of the most recent
// backup to consider. $earliestStart togeher with $latestStart defines the interval to report for
func getRowsForHandlerPostReportBackupList(dbData shared.DbData, jobName string, limit uint64, offset uint64, earliestStart int64, latestStart int64) ([]ReportBackupListDbResults, error) {
	results := make([]ReportBackupListDbResults, 0)
	rows, err := dbData.Db.Query(dbData.PreparedStatements.ReportBackupJobsListQuery, jobName, earliestStart,
		latestStart, limit, offset)
	if err != nil {
		logger.Errorf("While querying the database in order to get the list of reports, the "+
			"following error was encountered: %s", err)
		return results, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a the database query used to get the list of reports for "+
				"backup definition '%s', the following error was encountered: %s", jobName, err)
		}
	}()

	var tmpStartTime, tmpEndTime int64
	for rows.Next() {
		rowResult := ReportBackupListDbResults{}
		err := rows.Scan(&rowResult.Name, &rowResult.JobId, &tmpStartTime, &tmpEndTime, &rowResult.State)
		if err != nil {
			logger.Errorf("While retrieving the database records in order to build report list for backup "+
				"definition '%s', the following error was encountered: '%s'", jobName, err)
			return results, err
		}
		rowResult.StartTime = time.Unix(0, tmpStartTime).Format(time.RFC3339Nano)
		rowResult.EndTime = time.Unix(0, tmpEndTime).Format(time.RFC3339Nano)
		results = append(results, rowResult)
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to build report list for "+
			"backup definition '%s', the following error was encountered: '%s'", jobName, err)
		return results, err
	}

	return results, nil
}

// builds a next token by concatenating $limit + ":" + $offset + ":" + $earliestStart + ":" $latestStart and then base64 encoding the result
func buildNextTokenOfReportBackupList(limit uint64, offset uint64, earliestStart int64, latestStart int64) string {
	// format is limit:offset:earliestStart:latestStart
	plain := strconv.FormatUint(limit, 10) + ":" + strconv.FormatUint(offset, 10) + ":" +
		strconv.FormatInt(earliestStart, 10) + ":" + strconv.FormatInt(latestStart, 10)
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

// decodes a $nextToken produced by buildNextTokenOfReportBackupList(); if returned $err is not nil then all other returned values should be ignored
func decodeNextTokenOfReportBackupList(nextToken string) (limit uint64, offset uint64, earliestStart int64, latestStart int64, err error) {
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("could not base64 decode 'Next' token '%s' due to error: %s", nextToken, err)
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("base64 decoded token '%s' to '%+v' is "+
			"not made up of four parts separated by ':'", nextToken, parts)
	}
	limit, err = strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("base64 decoded token '%s' to '%+v'. First "+
			"part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[0], err)
	}

	offset, err = strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("base64 decoded token '%s' to '%+v'. Second"+
			" part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[1], err)
	}

	earliestStart, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("base64 decoded token '%s' to '%+v'. Third "+
			"part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[2], err)
	}

	latestStart, err = strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return limit, offset, earliestStart, latestStart, fmt.Errorf("base64 decoded token '%s' to '%+v'. Fourth "+
			"part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[3], err)
	}

	return limit, offset, earliestStart, latestStart, nil
}

func (srvSrc SrvData) handlerPostReportBackupShow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson ReportBackupJob
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name"+
			" is needed in order to know what backup definition you're requesting reports for"))
		return
	}
	jobName := decodedJson.Name

	if decodedJson.JobId == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprintf("'job_id' key is mandatory. The job id"+
			" is needed in order to know what specific back job run of backup '%s' you're requesting reports for", jobName))
		return
	}
	jobId := decodedJson.JobId

	dbData, backupJobsState, err := getDbAccess(srvSrc, jobName, w, loggingContext+".handlerPostReportBackupShow")
	if err == nil {
		// TODO - there is a potentially big problem here if a client disconnects before we setup the below defer() as
		// the structure which records how many DB connected clients we have would end up with extra marked clients
		// (but the actual clients have long disconnected). This leads to any attempt to close the database to hang as
		// the closing function waits for all clients to disconnect
		defer dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	} else {
		// getDbAccess() takes care of passing the error back to the client via $w so there isn't anything else to do
		return
	}

	dbResult, numResults, err := getRowsForHandlerPostReportBackupShow(dbData, jobName, jobId)
	if err != nil {
		if numResults == 0 {
			JSONError(w, http.StatusNotFound, HttpErrNotFound, err.Error())
			return
		}
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	JSONSuccessWithResult(w, "success", "success", dbResult)
}

// retrieves result from the database and reports any errors. If an error is returned then the
// shared.BackupJobStatus result should be discarded/disregarded . The second returned field is the number of results
// found (anything beside 1 is an error); last returned value is an error object
func getRowsForHandlerPostReportBackupShow(dbData shared.DbData, jobName string, jobId string) (shared.BackupJobStatus, int, error) {
	rows, err := dbData.Db.Query(dbData.PreparedStatements.ReportBackupJobsShowQuery, jobName, jobId)
	if err != nil {
		logger.Errorf("While querying the database in order to get report for backup definition '%s' having job id '%s', the "+
			"following error was encountered: %s", jobName, jobId, err)
		return shared.BackupJobStatus{}, 0, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a the database query used to get the job report for "+
				"backup definition '%s' having job id '%s', the following error was encountered: %s", jobName, jobId, err)
		}
	}()

	numRows := 0
	var rawResult, state string
	for rows.Next() {
		numRows += 1
		err := rows.Scan(&rawResult, &state)
		if err != nil {
			logger.Errorf("While retrieving the database record in with the job report for backup "+
				"definition '%s' having job id '%s', the following error was encountered: '%s'", jobName, jobId, err)
			return shared.BackupJobStatus{}, 0, err
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to build report list for "+
			"backup definition '%s', the following error was encountered: '%s'", jobName, err)
		return shared.BackupJobStatus{}, 0, err
	}

	if numRows == 0 {
		msg := fmt.Sprintf("The database query used to retrieve the job report for backup "+
			"definition '%s' having job id '%s' didn't return any result. Please check that the supplied job id is "+
			"correct", jobName, jobId)
		logger.Error(msg)
		return shared.BackupJobStatus{}, numRows, errors.New(msg)
	}
	if numRows != 1 {
		msg := fmt.Sprintf("The database query used to retrieve the job report for backup "+
			"definition '%s' having job id '%s' returned %d results. This is an internal error as only one result is "+
			"expected", jobName, jobId, numRows)
		logger.Error(msg)
		return shared.BackupJobStatus{}, numRows, errors.New(msg)
	}

	if state == "crashed" && rawResult == "" {
		msg := fmt.Sprintf("Job report for backup "+
			"definition '%s' having job id '%s' could not be retreived because the job is marked as 'crashed' and no "+
			"report is available for it", jobName, jobId)
		logger.Debugf(msg)
		return shared.BackupJobStatus{}, numRows, errors.New(msg)
	}

	var result shared.BackupJobStatus
	err = json.Unmarshal([]byte(rawResult), &result)
	if err != nil {
		msg := fmt.Sprintf("The database query used to retrieve the job report for backup "+
			"definition '%s' having job id '%s' returned a result which is corrupted", jobName, jobId)
		logger.Error(msg)
		return shared.BackupJobStatus{}, numRows, errors.New(msg)
	}

	return result, numRows, nil
}

func (srvSrc SrvData) handlerPostReportBackupFileList(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	globals.Stats.IncrementRoutines("httpd_handlers")
	defer globals.Stats.DecrementRoutines("httpd_handlers")

	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson ReportBackupFileList
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if err != nil {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
		return
	}
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name"+
			" is needed in order to know what backup definition you're requesting a file listing for"))
		return
	}
	jobName := decodedJson.Name
	jobId := decodedJson.JobId
	descend := decodedJson.Descend
	path := decodedJson.Path
	var limit uint64 = LimitReportFileResultsDefault
	var offset uint64 = 0

	if decodedJson.Next != "" {
		// overwrite decodedJson.MaxResults with the $limit value from the Next token and then validate that
		// The API documentation makes it clear that decodedJson.MaxResults will be ignored if a $Next token is present
		decodedJson.MaxResults, offset, jobId, path, descend, err = decodeNextTokenOfReportBackupFileList(decodedJson.Next)
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
			return
		}
	}

	if decodedJson.MaxResults > LimitReportFileResults {
		logger.Debugf("Requested max results of '%d' is larger then hardcoded limit of '%d'. Will return up"+
			" to '%d' results", decodedJson.MaxResults, LimitReportFileResults, LimitReportFileResults)
		limit = LimitReportFileResults
	} else {
		if decodedJson.MaxResults > 0 {
			limit = decodedJson.MaxResults
		}
	}

	dbData, backupJobsState, err := getDbAccess(srvSrc, jobName, w, loggingContext+".handlerPostReportBackupFileList")
	if err == nil {
		// TODO - there is a potentially big problem here if a client disconnects before we setup the below defer() as
		// the structure which records how many DB connected clients we have would end up with extra marked clients
		// (but the actual clients have long disconnected). This leads to any attempt to close the database to hang as
		// the closing function waits for all clients to disconnect
		defer dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	} else {
		// getDbAccess() takes care of passing the error back to the client via $w so there isn't anything else to do
		return
	}

	if jobId == "" {
		// if we don't have specified job id then the only way to reliably produce a file listing is if a backup is
		//  not running for the given backup name. Even so, between calls to this handler, if using a Next token it is
		// still possible to have a backup run in between and then the result would be incomplete and unreliable
		if backupJobsState.IsRunning(decodedJson.Name, "", loggingContext+".handlerPostReportBackupFileList") {
			JSONError(w, http.StatusBadRequest, HttpErrIncorrectClientData, fmt.Sprintf("Backup for job having "+
				"name '%s' is running and you have requested a file listing without supplying the jobid. In this case a "+
				"running backup may lead to incomplete and inconsistent results during file listing. Please retry the "+
				"operation once the running backup is completed or specify a job id", decodedJson.Name))
			return
		}
	} else {
		err := checkIfBackupJobIdIsUsable(dbData, jobName, jobId)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
			return
		}
	}
	// TODO - remove next line
	logger.Infof("descend %t, path: %s limit: %d offset: %d", descend, path, limit, offset)
	dbResults, err := getRowsForHandlerPostReportBackupFileListWithJobId(dbData, jobId, jobName, path, limit, offset)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	// TODO - enable "next" generator
	next := ""
	//// if returned results == $limit  then sent back a "next" value to client so it knows it doesn't have the full result set
	//if uint64(len(dbResults)) == limit {
	//	next = buildNextTokenOfReportBackupList(limit, offset+limit, FromStartTime, UntilStartTime)
	//}

	JSONSuccessWithResultPaginated(w, "success", "success", next, dbResults)

}

// parses srvSrc in order to figure out various details and then proceeds to get database access. On failure it will
// write back to the http client a specific error message and return control to the calling function but the caller
// should no longer attempt to write to the http client. Returns: shared.DbData struct with required details for DB
// operations followed by an *shared.BackupJobsState pointer and error object if the error object is populated then
// the shared.DbData struct and *shared.BackupJobsState should be discarded
func getDbAccess(srvSrc SrvData, jobName string, w http.ResponseWriter, logContext string) (shared.DbData, *shared.BackupJobsState, error) {
	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(logContext)
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(logContext)
	found := false
	// while "runtimeCfg" is a copy, some of the data is pointers so locking is still needed as it may be
	// shared with other functions (running in other routines)
	configCopy.Mutex.RLock()
	for _, backup := range configCopy.Backup {
		if backup.Name == jobName {
			found = true
		}
	}
	configCopy.Mutex.RUnlock()

	if !found {
		msg := fmt.Sprintf("No backup job definition was found matching name: %s", jobName)
		JSONError(w, http.StatusNotFound, HttpErrNotFound, msg)
		return shared.DbData{}, nil, errors.New(msg)
	}

	// start - all the plumbing needed in order to get DB access
	backupJobsState := srvSrc.GetJobState(logContext)
	db, err := database.OpenDb(configCopy.DataDir, jobName, true, backupJobsState, 15*time.Second)
	if err != nil {
		if err.Error() == database.ErrTimedOut {
			logger.Debugf("Timed out while trying to get database access from '%s' being ran for job definition '%s'", logContext, jobName)
			msg := fmt.Sprint("Timed out while trying to get database access. Please try again later.")
			JSONError(w, http.StatusServiceUnavailable, HttpErrServiceUnavailable, msg)
			return shared.DbData{}, nil, errors.New(msg)
		} else {
			msg := fmt.Sprintf("While trying to get database access for backup definition '%s', encountered error: %s", jobName, err)
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
			return shared.DbData{}, nil, errors.New(msg)
		}
	}
	backupConfig, err := shared.MakeCopyOfBackupJobDefinition(jobName, configCopy)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState, db)
		msg := fmt.Sprintf("While trying to get a copy of the backup definition for bakup name '%s', encountered error: %s", jobName, err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		return shared.DbData{}, backupJobsState, errors.New(msg)
	}

	dbData, err := dbops.PrepareDb(jobName, "", configCopy, backupJobsState, backupConfig, false, db)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState, db)
		msg := fmt.Sprintf("While trying to setup prepared SQL statements for bakup name '%s', encountered error: %s", jobName, err)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, msg)
		return shared.DbData{}, backupJobsState, errors.New(msg)
	}
	return dbData, backupJobsState, nil
	// end - all the plumbing needed in order to get DB access
}

// for a given job id and backup definition name, checks if there are previusly ran jobs
func checkIfBackupJobIdIsUsable(dbData shared.DbData, jobName string, jobId string) error {
	rows, err := dbData.Db.Query(dbData.PreparedStatements.ReportBackupJobsFileListFindJobQuery, jobName, jobId)
	if err != nil {
		return fmt.Errorf("While querying the database in order validate that job id '%s' belonging to  backup "+
			"definition '%s' is usable, the following error was encountered: %s", jobId, jobName, err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Error("While trying to Close() a the database query used to validate that job id '%s' belonging"+
				" to backup definition '%s' is usable, the following error was encountered: %s", jobId, jobName, err)
		}
	}()

	var matches int
	for rows.Next() {
		err := rows.Scan(&matches)
		if err != nil {
			return fmt.Errorf("while retrieving the database record in with the details for backup definition "+
				"'%s' having job id '%s', the following error was encountered: '%s'", jobName, jobId, err)
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("while enumerating the results of querying the database details of job id '%s' belonging to "+
			"backup definition '%s', the following error was encountered: '%s'", jobId, jobName, err)
	}

	if matches != 1 {
		return fmt.Errorf("no previously ran backup jobs were found for given job id '%s' belonging to backup "+
			"definition '%s'. Expected to find 1 match but instead found '%d'", jobId, jobName, matches)
	}
	return nil
}

// decodes a $nextToken produced by buildNextTokenOfReportBackupFileList(); if returned $err is not nil then all other returned values should be ignored
func decodeNextTokenOfReportBackupFileList(nextToken string) (limit uint64, offset uint64, jobId string, path string, descend bool, err error) {
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return limit, offset, jobId, path, descend, fmt.Errorf("could not base64 decode 'Next' token '%s' due to error: %s", nextToken, err)
	}
	parts := strings.Split(string(decoded), ":")
	// expected format is limit:offset:jobId:path:descend
	if len(parts) != 5 {
		return limit, offset, jobId, path, descend, fmt.Errorf("base64 decoded token '%s' to '%+v' is "+
			"not made up of five parts separated by ':'", nextToken, parts)
	}
	limit, err = strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return limit, offset, jobId, path, descend, fmt.Errorf("base64 decoded token '%s' to '%+v'. First "+
			"part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[0], err)
	}

	offset, err = strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return limit, offset, jobId, path, descend, fmt.Errorf("base64 decoded token '%s' to '%+v'. Second"+
			" part which is '%s' could not be converted to an integer due to error: %s", nextToken, parts, parts[1], err)
	}

	jobId = parts[2]
	path = parts[3]

	descend, err = strconv.ParseBool(parts[4])
	if err != nil {
		return limit, offset, jobId, path, descend, fmt.Errorf("base64 decoded token '%s' to '%+v'. Fifth "+
			"part which is '%s' could not be converted to a boolean due to error: %s", nextToken, parts, parts[4], err)
	}

	return limit, offset, jobId, path, descend, nil
}

// retrieves results from the database and reports any errors. If an error is returned then the
// []ReportBackupFileListDbResults should be discarded/disregarded
// $limit represents how many records to retrieve in one go; $offset is the offset to start request records from
// (this is related to $limit); $jobId represents the job id to look for and if its an empty string then all jobs will be searched
// (for the current backup definition which the $dbData represents); $parentDir is the parent directory for the items to search.
func getRowsForHandlerPostReportBackupFileListWithJobId(dbData shared.DbData, jobId string, jobName string, parentDir string, limit uint64, offset uint64) ([]ReportBackupFileListDbResults, error) {
	results := make([]ReportBackupFileListDbResults, 0)
	rows, err := dbData.Db.Query(dbData.PreparedStatements.ReportBackupJobsFileListWithJobId, jobId, parentDir, limit, offset)
	if err != nil {
		logger.Errorf("While querying the database in order to get the list of backed up files, the "+
			"following error was encountered: %s", err)
		return results, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a the database query used to get the list of backed up files for "+
				"backup job id '%s', the following error was encountered: %s", jobId, err)
		}
	}()

	for rows.Next() {
		// "SELECT local_path, upload_date, rf.target, type, size, delete_marker FROM remote_files rf
		// INNER JOIN backup_collections bc ON bc.file_uuid=rf.uuid WHERE bc.job_id=? AND rf.parent=? ORDER BY local_path ASC LIMIT ? OFFSET ?"
		rowResult := ReportBackupFileListInstanceDbResults{}
		var path string
		var tmpUploadDate int64
		rowResult.JobId = jobId
		rowResult.JobName = jobName
		err := rows.Scan(&path, &tmpUploadDate, &rowResult.JobTarget, &rowResult.Type, &rowResult.Size, &rowResult.DeleteMarker)
		if err != nil {
			logger.Errorf("While retrieving the database records in order to build report list for backup "+
				"job id '%s', the following error was encountered: '%s'", jobId, err)
			return results, err
		}
		rowResult.UploadDate = time.Unix(0, tmpUploadDate).Format(time.RFC3339Nano)
		if len(results) > 0 {
			lastItem := results[len(results)-1]
			// this is a new "instance" of an already examined path
			if lastItem.Path == path {
				results[len(results)-1].Instances = append(results[len(results)-1].Instances, rowResult)
			} else {
				results = append(results, ReportBackupFileListDbResults{
					Path:      path,
					Instances: []ReportBackupFileListInstanceDbResults{rowResult}})
			}
		} else {
			results = append(results, ReportBackupFileListDbResults{
				Path:      path,
				Instances: []ReportBackupFileListInstanceDbResults{rowResult}})
		}
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to build report list for "+
			"backup job id '%s', the following error was encountered: '%s'", jobId, err)
		return results, err
	}
	err = addJobStartTimeToReportBackupFileListDbResults(dbData, jobId, jobName, "backup", results)
	if err != nil {
		logger.Error("While adding the backup job start time to the list of results, the following error was "+
			"encountered: '%s'", jobId, err)
		return results, err
	}

	return results, nil
}

// Gets the start time for a give job name + id + type and returns it as RFC3339Nano formatted string
func GetJobStartTime(dbData shared.DbData, jobId string, jobName string, jobType string) (string, error) {
	rows, err := dbData.Db.Query(dbData.PreparedStatements.JobStartTime, jobId, jobName, jobType)
	if err != nil {
		logger.Errorf("While trying to get from the database any the start time for job '%s' having id '%s' the "+
			"following error was encountered: '%s'", jobName, jobId, err)
		return "", err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			logger.Warnf("While trying to Close() a db.Query for retrieving job start time of a previously ran "+
				"job, the following error was encountered: '%s'", err)
		}
	}()
	var tmpResult int64
	for rows.Next() {
		err := rows.Scan(&tmpResult)
		if err != nil {
			logger.Errorf("While enumerating from the database the start time for job '%s' having id '%s' , the "+
				"following error was encountered: '%s'", jobName, jobId, err)
			return "", err
		}
		// any result row means we had a match
		return time.Unix(0, tmpResult).Format(time.RFC3339Nano), nil // nolint:staticcheck
	}
	err = rows.Err()
	if err != nil {
		logger.Errorf("Could not enumerate the list of all targets from the database due to the following "+
			"error: '%s'", err)
		return "", err
	}
	// if we got here there there wasn't any match and no error was encountered
	return "", fmt.Errorf("did not find in the database a record of job '%s' having id '%s'", jobName, jobId)
}

// traverses a []ReportBackupFileListDbResults and adds job start time. Returns error if any encountered. Given that
//   []ReportBackupFileListDbResults is basically a pointer then we don't really need to return a changed data set as we adjust it in place
func addJobStartTimeToReportBackupFileListDbResults(dbData shared.DbData, jobId string, jobName string, jobType string, results []ReportBackupFileListDbResults) error {
	var err error
	// key is job id + job name + job type   and value is the start time
	startTimeCache := make(map[string]string)
	for k, v := range results {
		for instanceId, instanceValues := range v.Instances {
			startTime, ok := startTimeCache[instanceValues.JobId+jobName+jobType]
			if ok {
				// using cached value
				results[k].Instances[instanceId].JobStartTime = startTime
			} else {
				// db lookup
				startTime, err = GetJobStartTime(dbData, jobId, jobName, jobType)
				if err != nil {
					return err
				}
				results[k].Instances[instanceId].JobStartTime = startTime
				startTimeCache[instanceValues.JobId+jobName+jobType] = startTime
			}
		}
	}
	return nil
}
