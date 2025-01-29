package shared

import (
	"context"
	"errors"
	"github.com/paulbellamy/ratecounter"
	log "github.com/sirupsen/logrus"
	"runtime"
	"sync"
	"time"
)

const ErrJobAlreadyRunning = "job already running"
const ErrJobAlreadyStopped = "job already stopped"
const ErrJobAlreadyStopping = "job already stopping"
const ErrJobNotFoundInRunningState = "no running job with given name and uuid was found"
const ErrJobNotFoundInEvaluatingState = "no evaluating job with given name and uuid was found"
const ErrCouldNotGenerateJobId = "could not generate a unique id for the job"
const ErrUnknownJobType = "unknown job type"

const loggingContext = "shared"

// 1000 seems to be a good value to allow for fluctuations
const watcherChanSize = 1000

//var logger = log.WithFields(log.Fields{
//	"context": loggingContext,
//})

type CommWithSchedulerForBackup struct {
	// this needs to be locked before acquiring the channel to send messages to the scheduler goroutine or read messages
	// sent by the scheduler goroutine
	Mutex *sync.Mutex
	// on this channel the scheduler receives commands
	ReceivedCommand chan ReceiveBackupCommand
	// on this channel the scheduler sends the response to commands it received
	SendResponse chan ResponseBackupCommand
	// on this channel we signal the Scheduler to stop all running backups and then shutdown itself. Scheduler will
	// reply back on the same channel when before it exits
	Shutdown chan bool
}

type ReceiveBackupCommand struct {
	// uuid of the command
	Id string
	// one of "start" / "stop" / "state"
	Command string
	// uuid of the backup job referenced. Makes sense only for "stop" command
	BackupJobId string
	// name of the backup job as it is defined in the configuration file
	Name string
	// state of running jobs ; this will be a copy of the actual data
	//BackupJobsState []BackupJobStatus
}

type ResponseBackupCommand struct {
	// uuid of the command
	Id string
	// what command was requested one of "start" or "stop"
	Command string
	// uuid of the backup job referenced. This will be an existing uuid for responses to "stop" commands and a new
	// uuid when this is a response of a successful "start" command.
	BackupJobId string
	// name of the backup job as it is defined in the configuration file
	Name string
	// true if the command did not succeed
	Err bool
	// message to send back to the user. Will matter only when err == true
	Message string
}

// init the CommWithSchedulerForBackup structure
func (comm *CommWithSchedulerForBackup) Init() {
	comm.Mutex = &sync.Mutex{}
	// channel used for synchronization; do NOT change it to a buffered channel
	comm.ReceivedCommand = make(chan ReceiveBackupCommand)
	// channel used for synchronization; do NOT change it to a buffered channel
	comm.SendResponse = make(chan ResponseBackupCommand)
	// on this channel we signal the Scheduler to stop all running backups and then shutdown itself; Scheduler will
	// reply back on the same channel when before it exits;  do NOT change it to a buffered channel
	comm.Shutdown = make(chan bool)
}

// ANY CHANGES TO THIS STRUCT MAY NEED ALSO AN UPDATE in *BackupJobsState.Get() method as this does a deep copy like
// operation (without reflection so manual care needs to be taken in order to update the method)
type BackupJobStatus struct {
	// name of the backup job as it was defined in the configuration file at job start (things may have changed after)
	Name string `json:"name"`
	// one of "running" or "stopped", "stopping" for Backup jobs and when this is used for reporting purposes (of Backup
	// jobs by reading from the "jobs" DB table) then the possible values are: started, finished, failed, cancelled and
	// crashed. "started" means the job is running, "finished" that it completed its run, "failed" means some critical
	// enough error was encountered that all progress was aborted, "cancelled" means that the job was signaled to stop
	// while it was running and "crashed" means that it did not reach the "finished" state (this is equivalent to "stopped"
	// state when querying list of backup definitions and their status) and that somewhere before that the whole program crashed
	State string `json:"state"`
	// uuid of the backup job - makes sense only for $State == "running"
	BackupJobId string `json:"job_id,omitempty"`
	// - makes sense only for $State == "running"
	StartTime time.Time `json:"start_time,omitempty"`
	// time when job finished (or got cancelled, or failed)
	EndTime  time.Time `json:"end_time,omitempty"`
	Platform string    `json:"platform"`
	// bandwidth/second used during last 1/5/15 minute(s) - makes sense only for $State == "running" . This
	// value is the lower of disk read bandwidth and the upload speed to the backend object store
	Rate1Min             int64 `json:"rate_1min"`
	_rate1Min            *ratecounter.RateCounter
	Rate5Min             int64 `json:"rate_5min"`
	_rate5Min            *ratecounter.RateCounter
	Rate15Min            int64 `json:"rate_15min"`
	_rate15Min           *ratecounter.RateCounter
	FileContentBytesRead uint64            `json:"file_content_bytes_read"`
	ObjectStoreRates     []ObjectStoreRate `json:"object_store_rates,omitempty"`
	StatsCounters        map[string]uint64 `json:"stats_counters,omitempty"`
	StatsText            map[string]string `json:"stats_text,omitempty"`
	// used for keeping track of messages when sending real time upload status to connected clients
	Sequence uint64 `json:"-"`
	// TODO - to implement this . Lists the UTC time when the next run is scheduled
	NextRun time.Time `json:"next_run"`
	// using this context we signal a Backup job task that it should proceed to shutdown now
	Ctx context.Context `json:"-"`
	// cancel function produced when above context is created. This is needed in order to actually issue the cancel
	Cancel context.CancelFunc `json:"-"`
}

type BackupJobsState struct {
	Running []BackupJobStatus
	// used for locking during reads or writes as this struct will be shared all over the place
	Lock *sync.RWMutex
	// TODO - when implementing the "restoring" field also adjust the MarkRunning (and probably the MarkRestoring to
	// be created) in order to check also that a restore isn't running for the same backup name (to implement when
	// restores are implemented)

	// Clients should obtain a lock whenever they want to setup a connection to a DB. This is mainly because of
	// issues with SQLite3 implementation and it's interaction with GOLang's DB module
	// The map keys are matching the name of the $BackupJobName for backup jobs, restore jobs will have a db which
	// is prefixed by ___restore_$name_of_job
	DbOpenAllowed map[string]*DbAccess

	// The watch multiplexer listens on this chan and for each received message it sends it to any connected clients
	// which have requested to watch backup/restore progress for that given job name and job id
	WatchMsgReceiver chan WatchMessage
	// The struct of the multiplexer. This is the component which takes care of forwarding messages about files
	// being backedup/restored to connected clients
	Watcher *WatchMultiplexer
}

type ObjectStoreRate struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// this one is used only for the current file and reset to 0 whenever a new one starts being uploaded.
	_currentFileRate *ratecounter.RateCounter
	Rate1Min         int64 `json:"rate_1min"`
	_rate1Min        *ratecounter.RateCounter
	Rate5Min         int64 `json:"rate_5min"`
	_rate5Min        *ratecounter.RateCounter
	Rate15Min        int64 `json:"rate_15min"`
	_rate15Min       *ratecounter.RateCounter
}

// this interface is used only for cloudbackup/backup/scan/Scan() in order to be able to pass a different object when doing a
//
//	dry run report
type BackupJobsStateInterface interface {
	AddBytesRead(BackupJobName string, bytesRead uint64)
	IncrementCounter(BackupJobName string, counterName string, Path string, fileType string, OperationType string, Error string)
	IncrementRateCounter(BackupJobName string, ObjectStoreName string, ObjectStoreType string, IncrementValue int64, Path string, PercentDone uint, NewItem bool)
	// The Sequence is used when sending messages to Watch clients about objects being uploaded, up to date or marked as deleted
	IncrementSequence(BackupJobName string)
	UpdateStatsText(BackupJobName string, statName string, statValue string, exclusionExpr string, fileError string)
}

// returns a slice with the state of both running and stopped jobs. $cfgCopy MUST be a copy and not a dereference of
// the actual pointer to the main config (as slices are passed by reference and bad things will happen)
func (jobs *BackupJobsState) Get(cfgCopy CfgTemplate, logContext string) []BackupJobStatus {
	result := make([]BackupJobStatus, 0)
	runningList := map[string]string{}
	//log.WithFields(log.Fields{"context": logContext + ".Get"}).Debug("Acquiring read lock before reading running " +
	//	"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		//log.WithFields(log.Fields{"context": logContext + ".Get"}).Debug("Read lock released after reading running " +
		//	"backup jobs struct")
	}()
	// add state of running jobs
	for _, job := range jobs.Running {
		jobCopy := job
		// init empty maps
		jobCopy.StatsCounters = make(map[string]uint64)
		jobCopy.StatsText = make(map[string]string)
		// copy maps
		for k, v := range job.StatsCounters {
			jobCopy.StatsCounters[k] = v
		}
		for k, v := range job.StatsText {
			jobCopy.StatsText[k] = v
		}
		// copy ObjectStore rate slice
		jobCopy.ObjectStoreRates = make([]ObjectStoreRate, len(job.ObjectStoreRates))
		copy(jobCopy.ObjectStoreRates, job.ObjectStoreRates)
		//
		result = append(result, jobCopy)
		runningList[job.Name] = job.Name
		// copy ObjectStoreRates slice - unexported struct members should not cause issues with uses of the copy
		copy(jobCopy.ObjectStoreRates, job.ObjectStoreRates)
	}

	// while "cfgCopy" is a copy, some of the data is pointers so locking is still needed as it may be shared with
	// other functions (running in other routines)
	cfgCopy.Mutex.RLock()
	// add state of stopped jobs (what is not part of running must be stopped)
	for _, backupJob := range cfgCopy.Backup {
		if _, foundMatch := runningList[backupJob.Name]; !foundMatch {
			result = append(result, BackupJobStatus{
				Name:  backupJob.Name,
				State: "stopped",
				// TODO - add NextRun (see struct definition)
			})
		}
	}
	cfgCopy.Mutex.RUnlock()
	return result
}

// checks if a given job is running. Returns true if running, false otherwise
// ("stopping" state is considered running too)
func (jobs *BackupJobsState) IsRunning(name string, JobId string, logContext string) bool {
	//log.WithFields(log.Fields{"context": logContext + ".IsRunning"}).Debug("Acquiring read lock before reading running " +
	//	"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		//log.WithFields(log.Fields{"context": logContext + ".IsRunning"}).Debug("Read lock released after reading running " +
		//	"backup jobs struct")
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if JobId == "" {
				return true
			} else {
				if JobId != "" && job.BackupJobId == JobId {
					return true
				}
			}
		}
	}
	return false
}

// checks if a given job is stopping. Returns true if stopping, false otherwise
func (jobs *BackupJobsState) IsStopping(name string, JobId string, logContext string) bool {
	//log.WithFields(log.Fields{"context": logContext + ".IsStopping"}).Debug("Acquiring read lock before reading running " +
	//	"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		//log.WithFields(log.Fields{"context": logContext + ".IsStopping"}).Debug("Read lock released after reading running " +
		//	"backup jobs struct")
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if JobId == "" && job.State == "stopping" {
				return true
			} else {
				if JobId != "" && job.BackupJobId == JobId && job.State == "stopping" {
					return true
				}
			}
		}
	}
	return false
}

func (jobs *BackupJobsState) MarkRunning(name string, logContext string, BackupJobId string) error {
	log.WithFields(log.Fields{"context": logContext}).Debugf("Marking job '%s' as 'running'", name)
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read/write lock before updating running " +
		"backup jobs struct")
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("read/write lock released after updating running " +
			"backup jobs struct")
	}()
	// TODO - check also that a restore isn't running for the same backup name (to implement when restores are implemented)
	for _, job := range jobs.Running {
		if name == job.Name {
			return errors.New(ErrJobAlreadyRunning)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	jobs.Running = append(jobs.Running, BackupJobStatus{
		Name:        name,
		State:       "running",
		BackupJobId: BackupJobId,
		StartTime:   time.Now(),
		Platform:    runtime.GOOS,
		// init statistics related fields ; IF ANY NEW ENTRY IS ADDED BELOW THEN REVISIT AT LEAST METHOD
		// IncrementCounter() AND SEE IF SAID ADDITION NEEDS TO BE EXCLUDED FROM BEING SEND TO WATCHERS
		StatsCounters: map[string]uint64{
			"examined_files":       0,
			"examined_directories": 0,
			"examined_symlinks":    0,
			"examined_unknown":     0,
			"failed_to_examine":    0,
			"failed_to_enumerate":  0,
			// excluded files or directories due to matching some exclusion rule provided by the user (in the config)
			//  excluded don't count against examined_files or examined_directories
			"excluded": 0,
			// files, directories and symlinks for which an up to date copy is already in a backup
			"up_to_date_files":             0,
			"up_to_date_directories":       0,
			"up_to_date_symlinks":          0,
			"uploaded_files":               0,
			"uploaded_directories":         0,
			"uploaded_symlinks":            0,
			"failed_to_upload_files":       0,
			"failed_to_upload_directories": 0,
			"failed_to_upload_symlinks":    0,
			// this counter will always increment whenever we encounter an object different from "file", "dir", "symlink" types
			"failed_to_upload_unknown":         0,
			"updated_metadata_for_files":       0,
			"updated_metadata_for_directories": 0,
			"updated_metadata_for_symlinks":    0,
			// items discovered to no longer exist on the local disk (but we've previously backed them up)
			"marked_deleted_files":       0,
			"marked_deleted_directories": 0,
			"marked_deleted_symlinks":    0,
			// during the "mark_deleted_*" operation an error was encountered and it could not be fullfilled.
			"failed_to_mark_deleted_files":       0,
			"failed_to_mark_deleted_directories": 0,
			"failed_to_mark_deleted_symlinks":    0,
			// some kind of database related error was encountered when trying to find deleted items
			"failed_to_find_deleted":                    0,
			"failed_to_update_metadata_for_files":       0,
			"failed_to_update_metadata_for_directories": 0,
			"failed_to_update_metadata_for_symlinks":    0,
			// errors encountered while creating and uploading a copy of the database
			"database_copy_errors": 0,
			// pre_run / post_run scripts which have failed will each increment once this counter
			// (excludes notification scripts)
			"scripts_failed": 0,
			// pre_run / post_run scripts which have started will each increment once this counter
			// (excludes notification scripts)
			"scripts_ran": 0,
			// how many user supplied scripts are defined (excludes notification scripts)
			"scripts_num": 0,
		},
		StatsText: map[string]string{
			"current_directory": "",
			"current_file":      "",
			"current_operation": "",
		},
		Ctx:      ctx,
		Cancel:   cancel,
		Sequence: 0,
		// TODO - init metadata for Bandwidth usage (also several new fields are needed in order to note when the last update was
		// TODO - add NextRun
	})
	return nil
}

// If $stopped == false then mark job as "stopping"; if $stopped == true then remove job from Running Jobs list
// the $stopped bool parameter signifies when having value "false" the job state should be changed to "stopping" while
// when the parameter is "true" then the job has been stopped and it should be removed from the list of running jobs
func (jobs *BackupJobsState) MarkStopped(name string, logContext string, BackupJobId string, stopped bool) error {
	var state string
	if stopped {
		state = "stopped"
	} else {
		state = "stopping"
	}
	log.WithFields(log.Fields{"context": logContext}).Debugf("Marking job '%s' having job id '%s' as '%s'", name,
		BackupJobId, state)
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read/write lock before updating running " +
		"backup jobs struct")
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("read/write lock released after updating running " +
			"backup jobs struct")
	}()
	found := false
	updatedJobsRunning := make([]BackupJobStatus, 0)
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if BackupJobId == "" {
				found = true
				if !stopped {
					job.State = "stopping"
					updatedJobsRunning = append(updatedJobsRunning, job)
				}
				continue
			} else {
				if BackupJobId != "" && job.BackupJobId == BackupJobId {
					found = true
					if !stopped {
						job.State = "stopping"
						updatedJobsRunning = append(updatedJobsRunning, job)
					}
					continue
				}
			}
		} else {
			updatedJobsRunning = append(updatedJobsRunning, job)
		}
	}
	if found {
		jobs.Running = updatedJobsRunning
		return nil
	} else {
		return errors.New(ErrJobAlreadyStopped)
	}
}

// increment a statistics counter; this will not error if a job having the same name does not exist;
// CRITICAL assumption is that we never have more than one jobs having the same name but different UUIDs in a non
// stopped state
func (jobs *BackupJobsState) IncrementCounter(BackupJobName string, counterName string, Path string, fileType string, OperationType string, Error string) {
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()

MainLoop:
	for _, job := range jobs.Running {
		if BackupJobName == job.Name {
			job.StatsCounters[counterName] += 1

			// don't send a message to the multiplexer for the below $counterName
			switch counterName {
			case
				"examined_files", "examined_directories", "examined_symlinks", "examined_unknown", "scripts_failed",
				"failed_to_find_deleted":
				break MainLoop
			}
			// if this is a file, and no errors were encountered and this was a content upload then don't send a
			// message to the multiplexer (because IncrementRateCounter() does it).
			if Error == "" && fileType == "file" && OperationType == "upload" {
				break
			}

			var PercentDone uint = 0
			// if no error then the operation was successful (metadata operations are either 0% done ore 100% done)
			if Error == "" {
				PercentDone = 100
			}
			msg := WatchMessage{
				Sequence:        job.Sequence,
				JobType:         "backup",
				JobName:         BackupJobName,
				JobId:           job.BackupJobId,
				Path:            Path,
				PercentDone:     PercentDone,
				Rate:            0,
				ObjectType:      fileType,
				ObjectStoreName: "",
				ObjectStoreType: "",
				OperationType:   OperationType,
				Error:           Error,
				JobCompleted:    false,
			}
			SendMsgToWatcher(msg, jobs.WatchMsgReceiver)

			break
		}
	}
}

// update StatsText map; this will not error if a job having the same name does not exist;
// CRITICAL assumption is that we never have more than one jobs having the same name but different UUIDs in a non
// stopped state; $exclusionExpr and $fileError are not used but are needed in the signature in order to match
// interface expectations
func (jobs *BackupJobsState) UpdateStatsText(BackupJobName string, statName string, statValue string,
	exclusionExpr string, fileError string) {
	// we use the "unknown" marker when reporting errors for getting file stat() of files/folder being excluded. This
	// 	maker is useful only in the other implementation of this interface method so here we just skip over it
	// 	all together
	if statName == "unknown" {
		return
	}
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()
	for _, job := range jobs.Running {
		if BackupJobName == job.Name {
			// if an exclusion has matched or we got an error then we don't want the file/directory to appear any more as
			// currently being processed
			if exclusionExpr != "" || fileError != "" {
				job.StatsText[statName] = ""
			} else {
				job.StatsText[statName] = statValue
			}
			break
		}
	}
}

// increment a rate counter; this will not error if a job having the same name does not exist;
// CRITICAL assumption is that we never have more than one jobs having the same name but different UUIDs in a non
// stopped state
// TODO - write unit tests for this function - depends on having the object store "test_null" implemented
func (jobs *BackupJobsState) IncrementRateCounter(BackupJobName string, ObjectStoreName string, ObjectStoreType string, IncrementValue int64, Path string, PercentDone uint, NewItem bool) {
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()
	for k, job := range jobs.Running {
		if BackupJobName == job.Name {

			// if the job rate counters(pointers) are not initialised then init them
			if job._rate1Min == nil || job._rate5Min == nil || job._rate15Min == nil {
				jobs.Running[k]._rate1Min = ratecounter.NewRateCounter(time.Minute * 1)
				jobs.Running[k]._rate5Min = ratecounter.NewRateCounter(time.Minute * 5)
				jobs.Running[k]._rate15Min = ratecounter.NewRateCounter(time.Minute * 15)
			}

			// increment job rate counters
			jobs.Running[k]._rate1Min.Incr(IncrementValue)
			jobs.Running[k]._rate5Min.Incr(IncrementValue)
			jobs.Running[k]._rate15Min.Incr(IncrementValue)
			// update job rate counters which are retrievable
			jobs.Running[k].Rate1Min = jobs.Running[k]._rate1Min.Rate() / 60
			jobs.Running[k].Rate5Min = jobs.Running[k]._rate5Min.Rate() / 300
			jobs.Running[k].Rate15Min = jobs.Running[k]._rate15Min.Rate() / 900

			// increment backend rate counters - initialise slice if nil
			if job.ObjectStoreRates == nil {
				jobs.Running[k].ObjectStoreRates = make([]ObjectStoreRate, 0)
			}

			foundObjectStoreEntry := false
			for _, objectStore := range jobs.Running[k].ObjectStoreRates {
				if ObjectStoreName == objectStore.Name {
					foundObjectStoreEntry = true
					break
				}
			}
			// add entry and init counters
			if !foundObjectStoreEntry {
				jobs.Running[k].ObjectStoreRates = append(jobs.Running[k].ObjectStoreRates, ObjectStoreRate{
					Name:             ObjectStoreName,
					Type:             ObjectStoreType,
					_currentFileRate: ratecounter.NewRateCounter(time.Second * 10),
					_rate1Min:        ratecounter.NewRateCounter(time.Minute * 1),
					_rate5Min:        ratecounter.NewRateCounter(time.Minute * 5),
					_rate15Min:       ratecounter.NewRateCounter(time.Minute * 15),
				})
			}

			for k2, objectStore := range jobs.Running[k].ObjectStoreRates {
				var increment int64 = 0
				if ObjectStoreName == objectStore.Name {
					increment = IncrementValue
				}
				// whenever a new file starts being uploaded, reset this Rate as this is file specific only
				if NewItem {
					jobs.Running[k].ObjectStoreRates[k2]._currentFileRate = ratecounter.NewRateCounter(time.Second * 10)
				}
				jobs.Running[k].ObjectStoreRates[k2]._currentFileRate.Incr(increment)
				jobs.Running[k].ObjectStoreRates[k2]._rate1Min.Incr(increment)
				jobs.Running[k].ObjectStoreRates[k2]._rate5Min.Incr(increment)
				jobs.Running[k].ObjectStoreRates[k2]._rate15Min.Incr(increment)
				// update job rate counters which are retrievable
				jobs.Running[k].ObjectStoreRates[k2].Rate1Min = jobs.Running[k].ObjectStoreRates[k2]._rate1Min.Rate() / 60
				jobs.Running[k].ObjectStoreRates[k2].Rate5Min = jobs.Running[k].ObjectStoreRates[k2]._rate5Min.Rate() / 300
				jobs.Running[k].ObjectStoreRates[k2].Rate15Min = jobs.Running[k].ObjectStoreRates[k2]._rate15Min.Rate() / 900
				// send message to multiplexer so it can forwarder to connected clients
				msg := WatchMessage{
					Sequence:        job.Sequence,
					JobType:         "backup",
					JobName:         BackupJobName,
					JobId:           job.BackupJobId,
					Path:            Path,
					PercentDone:     PercentDone,
					Rate:            jobs.Running[k].ObjectStoreRates[k2]._currentFileRate.Rate() / 10,
					ObjectType:      "file",
					ObjectStoreName: ObjectStoreName,
					ObjectStoreType: ObjectStoreType,
					OperationType:   "upload",
					JobCompleted:    false,
				}
				SendMsgToWatcher(msg, jobs.WatchMsgReceiver)
			}
			break
		}
	}
}

// add to *BackupJobsState.FileContentBytesRead of a given backup job a number of bytes which were read(represents file contents)
func (jobs *BackupJobsState) AddBytesRead(BackupJobName string, bytesRead uint64) {
	if bytesRead == 0 {
		return
	}
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()
	for k, job := range jobs.Running {
		if BackupJobName == job.Name {
			jobs.Running[k].FileContentBytesRead += bytesRead
			break
		}
	}
}

// increments *BackupJobsState.Sequence of a given backup job. The Sequence is used when sending messages to clients
// about objects being uploaded
func (jobs *BackupJobsState) IncrementSequence(BackupJobName string) {
	jobs.Lock.Lock()
	defer jobs.Lock.Unlock()
	for k, job := range jobs.Running {
		if BackupJobName == job.Name {
			jobs.Running[k].Sequence += 1
			break
		}
	}
}

// return the cancel function for a particular Running job with a particular uuid (or if uuid="" then match on
//
//	name only)
func (jobs *BackupJobsState) GetCancelFunctionForJob(BackupJobName string, BackupJobId string) (context.CancelFunc, error) {
	//log.WithFields(log.Fields{"context": loggingContext + ".GetCancelFunctionForJob"}).Debug("Acquiring read lock " +
	//	"before reading the backup jobs struct")
	jobs.Lock.RLock()

	defer func() {
		jobs.Lock.RUnlock()
		//log.WithFields(log.Fields{"context": loggingContext + ".GetCancelFunctionForJob"}).Debug("Read lock " +
		//	"released after reading the backup jobs struct")
	}()

	var CancelFunction context.CancelFunc
	found := false

	for _, job := range jobs.Running {
		if BackupJobName == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if BackupJobId == "" {
				found = true
				CancelFunction = job.Cancel
				break
			} else {
				if BackupJobId != "" && job.BackupJobId == BackupJobId {
					found = true
					CancelFunction = job.Cancel
					break
				}
			}
		}
	}

	if found {
		return CancelFunction, nil
	}
	return nil, errors.New(ErrJobNotFoundInRunningState)
}

// return the context for a particular Running job with a particular uuid (or if uuid="" then match on
//
//	name only)
func (jobs *BackupJobsState) GetContextForJob(BackupJobName string, BackupJobId string) (context.Context, error) {
	//log.WithFields(log.Fields{"context": loggingContext + ".GetContextForJob"}).Debug("Acquiring read lock " +
	//	"before reading the backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		//log.WithFields(log.Fields{"context": loggingContext + ".GetContextForJob"}).Debug("Read lock " +
		//	"released after reading the backup jobs struct")
	}()

	var ctx context.Context
	found := false

	for _, job := range jobs.Running {
		if BackupJobName == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if BackupJobId == "" {
				found = true
				ctx = job.Ctx
				break
			} else {
				if BackupJobId != "" && job.BackupJobId == BackupJobId {
					found = true
					ctx = job.Ctx
					break
				}
			}
		}
	}

	if found {
		return ctx, nil
	}
	return nil, errors.New(ErrJobNotFoundInRunningState)
}

// gets the start time of a backup job
// returns: time of start ; error if encountered and error
func (jobs *BackupJobsState) GetStartTime(name string, JobId string, logContext string) (time.Time, error) {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if JobId == "" {
				return job.StartTime, nil
			} else {
				if JobId != "" && job.BackupJobId == JobId {
					return job.StartTime, nil
				}
			}
		}
	}
	return time.Time{}, errors.New(ErrJobNotFoundInRunningState)
}

// gets the jobid of a running job
// returns: job id ; error if no running job has the same job name
func (jobs *BackupJobsState) GetRunningBackupJobId(name string, logContext string) (string, error) {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			return job.BackupJobId, nil
		}
	}
	return "", errors.New("no running job was found matching supplied job name")
}

// checks if a given job running/stopping (but not stopped) job is cancelled . Returns true if cancelled, false otherwise
func (jobs *BackupJobsState) IsCancelled(name string, JobId string, logContext string) bool {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if JobId == "" {
				if job.Ctx.Err() == context.Canceled {
					return true
				} else {
					return false
				}
			} else {
				if JobId != "" && job.BackupJobId == JobId {
					if job.Ctx.Err() == context.Canceled {
						return true
					} else {
						return false
					}
				}
			}
		}
	}
	return false
}

// initialise struct which holds jobs state
func NewJobsState() *BackupJobsState {
	msgChan := make(chan WatchMessage, watcherChanSize)
	return &BackupJobsState{
		Lock:             &sync.RWMutex{},
		DbOpenAllowed:    make(map[string]*DbAccess),
		WatchMsgReceiver: msgChan,
		Watcher:          NewWatcherState(msgChan),
	}
}

// reports true if the Backup job has been cancelled which means its context is cancelled
func (job *BackupJobStatus) IsCancelled() bool {
	return job.Ctx.Err() == context.Canceled
}
