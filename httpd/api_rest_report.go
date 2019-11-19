package httpd

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/database"
	"cloudbackup/database/dbops"
	"cloudbackup/notifications"
	"cloudbackup/shared"
	"encoding/base64"
	"encoding/json"
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
}

type ReportBackupListDbResults struct {
	JobId     string `json:"job_id"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	State     string `json:"state"`
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
			" to '%d' results", decodedJson.MaxResults, LimitReportResults, decodedJson.MaxResults)
	} else {
		if decodedJson.MaxResults > 0 {
			limit = decodedJson.MaxResults
		}
	}

	var offset uint64 = 0
	if decodedJson.Next != "" {
		limit, offset, err = decodeNextToken(decodedJson.Next)
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

	dbResults, err := getRowsForHandlerPostReportBackupList(dbData, jobName, limit, offset)
	dbops.CloseStatementsAndDisconnectFromDb(dbData, backupJobsState)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, HttpErrInternalServerError, err.Error())
		return
	}

	next := ""
	// if returned results == $limit  then sent back a "next" value to client so it knows it doesn't have the full result set
	if uint64(len(dbResults)) == limit {
		next = buildNextToken(limit, offset+limit)
	}

	JSONSuccessWithResultPaginated(w, "success", "success", next, dbResults)
}

// retrieves results from the database and reports any errors. If an error is returned then the []ReportBackupListDbResults should be discarded/disregarded
func getRowsForHandlerPostReportBackupList(dbData shared.DbData, jobName string, limit uint64, offset uint64) ([]ReportBackupListDbResults, error) {
	results := make([]ReportBackupListDbResults, 0)
	rows, err := dbData.Db.Query(dbData.PreparedStatements.ReportBackupJobsListQuery, jobName, time.Now().UnixNano(), limit, offset)
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

	for rows.Next() {
		rowResult := ReportBackupListDbResults{}
		// "SELECT id, start_time, end_time, state FROM jobs WHERE name = ? AND state != 'started' AND end_time < ? ORDER BY start_time LIMIT ? OFFSET ?"
		/*
			type ReportBackupListDbResults struct {
				JobId     string
				StartTime int64
				EndTime   int64
				State     string
			}
		*/
		err := rows.Scan(&rowResult.JobId, &rowResult.StartTime, &rowResult.EndTime, &rowResult.State)
		if err != nil {
			logger.Errorf("While retrieving the database records in order to build report list for backup "+
				"definition '%s', the following error was encountered: '%s'", jobName, err)
			return results, err
		}
		results = append(results, rowResult)
	}
	if err = rows.Err(); err != nil {
		logger.Error("While enumerating the results of querying the database in order to build report list for "+
			"backup definition '%s', the following error was encountered: '%s'", jobName, err)
		return results, err
	}

	return results, nil
}

// builds a next token by concatenating $limit + ":" + $offset and then base64 encoding the result
func buildNextToken(limit uint64, offset uint64) string {
	// format is offset:max_results
	plain := strconv.FormatUint(limit, 10) + ":" + strconv.FormatUint(offset, 10)
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

// decodes a $nextToken produced by buildNextToken()
func decodeNextToken(nextToken string) (limit uint64, offset uint64, err error) {
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return limit, offset, fmt.Errorf("could not base64 decode 'Next' token '%s' due to error: %s", nextToken, err)
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return limit, offset, fmt.Errorf("base64 decoded token '%s' to '%+v' is not made up of two parts "+
			"separated by ':'", nextToken, parts)
	}
	limit, err = strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return limit, offset, fmt.Errorf("base64 decoded token '%s' to '%+v'. First part which is '%s' could "+
			"not be converted to an integer due to error: %s", nextToken, parts, parts[0], err)
	}

	offset, err = strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return limit, offset, fmt.Errorf("base64 decoded token '%s' to '%+v'. Second part which is '%s' could "+
			"not be converted to an integer due to error: %s", nextToken, parts, parts[1], err)
	}
	return limit, offset, nil
}
