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

const LimitReportResults = 1000

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

	var limit uint64 = LimitReportResults
	if decodedJson.MaxResults > LimitReportResults {
		logger.Debugf("Requested max results of '%d' is larger then hardcoded limit of '%d'. Will return up"+
			" to '%d' results", decodedJson.MaxResults, LimitReportResults, LimitReportResults)
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
		limit, offset, FromStartTime, UntilStartTime, err = decodeNextToken(decodedJson.Next)
		if err != nil {
			JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, err.Error())
			return
		}
	}

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostReportBackupList")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostReportBackupList")
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
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job definition was found matching name:"+
			" %s", jobName))
		return
	}

	// start - all the plumbing needed in order to get DB access
	backupJobsState := srvSrc.GetJobState(".handlerPostReportBackupList")
	db, err := database.OpenDb(configCopy.DataDir, jobName, true, backupJobsState, 15*time.Second)
	if err != nil {
		if err.Error() == database.ErrTimedOut {
			logger.Debugf("Timed out while trying to get database access from handlerPostReportBackupList() being ran for job definition '%s'", jobName)
			JSONError(w, http.StatusServiceUnavailable, HttpErrServiceUnavailable, fmt.Sprint("Timed out while trying to get database access. Please try again later."))
			return
		} else {
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to get database access for backup definition '%s', encountered error: %s",
				jobName, err))
			return
		}
	}
	backupConfig, err := shared.MakeCopyOfBackupJobDefinition(jobName, configCopy)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to get a copy of the backup definition for bakup name '%s', encountered error: %s",
			jobName, err))
		return
	}

	dbData, err := dbops.PrepareDb(jobName, "", configCopy, backupJobsState, backupConfig, false, db)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to setup prepared SQL statements for bakup name '%s', encountered error: %s",
			jobName, err))
		return
	}
	// end - all the plumbing needed in order to get DB access
	defer dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	// TODO - there is a potentially big problem here if a client disconnects before we setup the above defer() as the structure which records how many DB connected clients we have would end up with
	// extra marked clients (but the actual clients have long disconnected). This leads to any attempt to close the database to hang as the closing function waits for all clients to disconnect

	dbResults, err := getRowsForHandlerPostReportBackupList(dbData, jobName, limit, offset, FromStartTime, UntilStartTime)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	next := ""
	// if returned results == $limit  then sent back a "next" value to client so it knows it doesn't have the full result set
	if uint64(len(dbResults)) == limit {
		next = buildNextToken(limit, offset+limit, FromStartTime, UntilStartTime)
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
func buildNextToken(limit uint64, offset uint64, earliestStart int64, latestStart int64) string {
	// format is offset:max_results:earliestStart:latestStart
	plain := strconv.FormatUint(limit, 10) + ":" + strconv.FormatUint(offset, 10) + ":" +
		strconv.FormatInt(earliestStart, 10) + ":" + strconv.FormatInt(latestStart, 10)
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

// decodes a $nextToken produced by buildNextToken(); if returned $err is not nil then all other returned values should be ignored
func decodeNextToken(nextToken string) (limit uint64, offset uint64, earliestStart int64, latestStart int64, err error) {
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

	// while a copy, some of the data is pointers so locking is still needed
	srvCopy := srvSrc.GetCopyWithLock(loggingContext + ".handlerPostReportBackupShow")
	// while a copy, some of the data is pointers so locking is still needed
	configCopy := srvCopy.globalcfg.GetCopyWithLock(loggingContext + ".handlerPostReportBackupShow")
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
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job definition was found matching name:"+
			" %s", jobName))
		return
	}

	// start - all the plumbing needed in order to get DB access
	backupJobsState := srvSrc.GetJobState(".handlerPostReportBackupShow")
	db, err := database.OpenDb(configCopy.DataDir, jobName, true, backupJobsState, 15*time.Second)
	if err != nil {
		if err.Error() == database.ErrTimedOut {
			logger.Debugf("Timed out while trying to get database access from handlerPostReportBackupShow() being ran for job definition '%s'", jobName)
			JSONError(w, http.StatusServiceUnavailable, HttpErrServiceUnavailable, fmt.Sprint("Timed out while trying to get database access. Please try again later."))
			return
		} else {
			JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to get database access for backup definition '%s', encountered error: %s",
				jobName, err))
			return
		}
	}
	backupConfig, err := shared.MakeCopyOfBackupJobDefinition(jobName, configCopy)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to get a copy of the backup definition for bakup name '%s', encountered error: %s",
			jobName, err))
		return
	}

	dbData, err := dbops.PrepareDb(jobName, "", configCopy, backupJobsState, backupConfig, false, db)
	if err != nil {
		database.DisconnectFromDb(jobName, backupJobsState)
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, fmt.Sprintf("While trying to setup prepared SQL statements for bakup name '%s', encountered error: %s",
			jobName, err))
		return
	}
	// end - all the plumbing needed in order to get DB access
	defer dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	// TODO - there is a potentially big problem here if a client disconnects before we setup the above defer() as the structure which records how many DB connected clients we have would end up with
	// extra marked clients (but the actual clients have long disconnected). This leads to any attempt to close the database to hang as the closing function waits for all clients to disconnect

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
	var rawResult string
	for rows.Next() {
		numRows += 1
		err := rows.Scan(&rawResult)
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
