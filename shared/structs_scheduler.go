package shared

import (
	"sync"
	"time"
	"cloudbackup/config"
	log "github.com/sirupsen/logrus"
	"errors"
)

const ErrJobAlreadyRunning = "job already running"
const ErrJobAlreadyStopped = "job already stopped"
const ErrJobAlreadyStopping = "job already stopping"

type CommWithSchedulerForBackup struct {
	// this needs to be locked before acquiring the channel to send messages to the scheduler goroutine or read messages
	// sent by the scheduler goroutine
	Mutex *sync.Mutex
	// on this chanel the scheduler receives commands
	ReceivedCommand chan ReceiveBackupCommand
	// on this chanel the scheduler sends the response to commands it received
	SendResponse chan ResponseBackupCommand
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
func (comm *CommWithSchedulerForBackup) Init () {
	comm.Mutex = &sync.Mutex{}
	// channel used for synchronization; do NOT change it to a buffered channel
	comm.ReceivedCommand = make(chan ReceiveBackupCommand)
	// channel used for synchronization; do NOT change it to a buffered channel
	comm.SendResponse = make(chan ResponseBackupCommand)
}

type BackupJobStatus struct {
	// name of the backup job as it was defined in the configuration file at job start (things may have changed after)
	Name string `json:"name"`
	// one of "running" or "stopped" or "stopping"
	State string `json:"state"`
	// uuid of the backup job - makes sense only for $State == "running"
	BackupJobId string `json:"job_id,omitempty"`
	// - makes sense only for $State == "running"
	StartTime time.Time `json:"start_time,omitempty"`
	// transmit bandwidth/second used during last 1 minute - makes sense only for $State == "running"
	TxBandwidth1Min int64 `json:"TxBandwidth1Min,omitempty"`
	// receive bandwidth/second used during last 1 minute - makes sense only for $State == "running"
	RxBandwidth1Min int64 `json:"rx_bandwidth_1_min,omitempty"`
	TxBandwidth5Min int64 `json:"tx_bandwidth_5_min,omitempty"`
	RxBandwidth5Min int64 `json:"rx_bandwidth_5_min,omitempty"`
	TxBandwidth15Min int64 `json:"tx_bandwidth_15_min,omitempty"`
	RxBandwidth15Min int64 `json:"rx_bandwidth_15_min,omitempty"`
	// TODO - to implement this . Lists the UTC time when the next run is scheduled
	NextRun string `json:"next_run"`
}

type BackupJobsState struct {
	Running []BackupJobStatus
	// used for locking during reads or writes as this struct will be shared all over the place
	Lock *sync.RWMutex
}


// returns a slice with the state of both running and stopped jobs. $cfgCopy MUST be a copy and not a dereference of
// the actual pointer to the main config (as slices are passed by reference and bad things will happen)
func (jobs *BackupJobsState) Get (cfgCopy config.CfgTemplate, logContext string) []BackupJobStatus {
	result := make([]BackupJobStatus, 0)
	runningList := map[string]string{}
	log.WithFields(log.Fields{"context": logContext + ".Get"}).Debug("Acquiring read lock before reading running " +
		"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		log.WithFields(log.Fields{"context": logContext + ".Get"}).Debug("Read lock released after reading running " +
			"backup jobs struct")
	}()
	// add state of running jobs
	for _, job := range jobs.Running {
		result = append(result, job)
		runningList[job.Name] = job.Name
	}
	// add state of stopped jobs (what is not part of running must be stopped)
	for _, backupJob := range cfgCopy.Backup {
		if _, foundMatch := runningList[backupJob.Name]; foundMatch == false {
			result = append(result, BackupJobStatus{
				Name: backupJob.Name,
				State: "stopped",
				// TODO - add NextRun (see struct definition)
			})
		}
	}
	return result
}

// checks if a given job is running. Returns true if running, false otherwise
// ("stopping" state is considered running too)
func (jobs *BackupJobsState) IsRunning(name string, JobId string, logContext string) bool {
	log.WithFields(log.Fields{"context": logContext + ".IsRunning"}).Debug("Acquiring read lock before reading running " +
		"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		log.WithFields(log.Fields{"context": logContext + ".IsRunning"}).Debug("Read lock released after reading running " +
			"backup jobs struct")
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
	log.WithFields(log.Fields{"context": logContext + ".IsStopping"}).Debug("Acquiring read lock before reading running " +
		"backup jobs struct")
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
		log.WithFields(log.Fields{"context": logContext + ".IsStopping"}).Debug("Read lock released after reading running " +
			"backup jobs struct")
	}()
	for _, job := range jobs.Running {
		if name == job.Name {
			// if JobId is not specified then any match is sufficient otherwise a matching name + matching jobids are required
			if JobId == "" && job.State == "stopping" {
				return true
			} else {
				if JobId != "" && job.BackupJobId == JobId && job.State == "stopping"{
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
	for _, job := range jobs.Running {
		if name == job.Name {
			return errors.New(ErrJobAlreadyRunning)
		}
	}
	jobs.Running = append(jobs.Running, BackupJobStatus{
		Name: name,
		State: "running",
		BackupJobId: BackupJobId,
		StartTime: time.Now(),
		// TODO - add NextRun
	})
	return nil
}


// If $stopped == false then mark job as "stopping"; if $stopped == true then remove job from Running Jobs list
// the $stopped bool parameter signifies when having value "false" the job state should be changed to "stopping" while
// when the parameter is "true" then the job has been stopped and it should be removed from the list of running jobs
func (jobs *BackupJobsState) MarkStopped(name string, logContext string, BackupJobId string, stopped bool) error {
	var state string
	if stopped{
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
				if stopped == false {
					job.State = "stopping"
					updatedJobsRunning = append(updatedJobsRunning, job)
				}
				continue
			} else {
				if BackupJobId != "" && job.BackupJobId == BackupJobId {
					found = true
					if stopped == false {
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
	if found{
		jobs.Running = updatedJobsRunning
		return nil
	} else {
		return errors.New(ErrJobAlreadyStopped)
	}
}