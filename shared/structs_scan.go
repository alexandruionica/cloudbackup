package shared

import (
	"errors"
	"sync"
	"time"

	"context"
	log "github.com/sirupsen/logrus"
)

type DryRunBackupJobsState struct {
	DryRunning []BackupJobStatus
	// used for locking during reads or writes as this struct will be shared all over the place
	Lock *sync.RWMutex
	// used for reporting what file or folder is being evaluated. This in turn is to be read by a web go routine
	// reporting back, in real time, to a client
	ReportChan chan ScanEvalItemReport
}

type ScanEvalItemReport struct {
	// name of File or Directory being examine
	Name string `json:"name"`
	// one of "file", "directory" or "unknown"
	Type string `json:"type"`
	// If this file or directory is going to be excluded due to matching an exclusion expression
	Excluded bool `json:"excluded"`
	// if $Excluded==true then the below will contain what exclusion expression matched
	ExclusionExpr string `json:"exclusion_expr"`
	// if an error was reported while reading file properties then this will be passed back here
	Error string `json:"error"`
}

// $Path string, $fileType string, $OperationType string, $Error string  are defined only to satisfy interface requirements
func (jobs *DryRunBackupJobsState) IncrementCounter(BackupJobName string, counterName string, Path string,
	fileType string, OperationType string, Error string) {
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()
	for _, job := range jobs.DryRunning {
		if BackupJobName == job.Name {
			job.StatsCounters[counterName] += 1
			break
		}
	}
}

// we don't need this for a dry run but this function is needed in order to satisfy the interface constrains
func (jobs *DryRunBackupJobsState) IncrementRateCounter(BackupJobName string, ObjectStoreName string,
	ObjectStoreType string, IncrementValue int64, Path string, PercentDone uint, NewItem bool) {
}

// we don't need this for a dry run but this function is needed in order to satisfy the interface constrains
func (jobs *DryRunBackupJobsState) IncrementSequence(BackupJobName string) {
}

// we don't need this for a dry run but this function is needed in order to satisfy the interface constrains
func (jobs *DryRunBackupJobsState) AddBytesRead(BackupJobName string, bytesRead uint64) {
}

// This will not error if a job having the same name does not exist;
// CRITICAL assumption is that we never have more than one jobs having the same name but different UUIDs in a non
// stopped state
// This method is used in order to send messages (via a channel) during an eval job run. The method name was "inherited",
//
//	from the initial implementation but since switched to using an interface and adjusting behaviour as needed; the
//	method name makes sense in the BackupJobsState struct
func (jobs *DryRunBackupJobsState) UpdateStatsText(BackupJobName string, statName string, statValue string,
	exclusionExpr string, fileError string) {
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
	}()
	for _, job := range jobs.DryRunning {
		if BackupJobName == job.Name {
			job.StatsText[statName] = statValue
			if statValue != "" {
				response := ScanEvalItemReport{
					Name:          statValue,
					ExclusionExpr: exclusionExpr,
				}
				switch statName {
				case "current_file":
					response.Type = "file"
				case "current_directory":
					response.Type = "directory"
				default:
					// unknown
					response.Type = "unknown"
				}
				if exclusionExpr != "" {
					response.Excluded = true
				} else {
					response.Excluded = false
				}
				if fileError != "" {
					response.Error = fileError
				}
				// this blocks until something else reads from the channel
				jobs.ReportChan <- response
			}
			break
		}
	}
}

func (jobs *DryRunBackupJobsState) MarkEvaluating(name string, logContext string, BackupJobId string) error {
	log.WithFields(log.Fields{"context": logContext}).Debugf("Marking job '%s' as 'evaluating'", name)
	log.WithFields(log.Fields{"context": logContext}).Debug("Acquiring read/write lock before updating " +
		"evaluating backup jobs struct")
	jobs.Lock.Lock()
	defer func() {
		jobs.Lock.Unlock()
		log.WithFields(log.Fields{"context": logContext}).Debug("read/write lock released after updating " +
			"evaluating backup jobs struct")
	}()
	for _, job := range jobs.DryRunning {
		if name == job.Name {
			return errors.New(ErrJobAlreadyRunning)
		}
	}

	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // cancel is stored on the job status and invoked via Cancel()/MarkStopped()
	jobs.DryRunning = append(jobs.DryRunning, BackupJobStatus{
		Name:        name,
		State:       "evaluating",
		BackupJobId: BackupJobId,
		StartTime:   time.Now(),
		// init statistics related fields
		StatsCounters: map[string]uint64{
			"examined_files":       0,
			"examined_directories": 0,
			"failed_to_examine":    0,
			"failed_to_enumerate":  0,
			// excluded files or directories due to matching some exclusion rule provided by the user (in the config)
			//  excluded don't count against examined_files or examined_directories
			"excluded":           0,
			"uploaded_files":     0,
			"uploaded_non_files": 0,
			"failed_to_upload":   0,
		},
		StatsText: map[string]string{
			"current_directory": "",
			"current_file":      "",
		},
		Ctx:    ctx,
		Cancel: cancel,
	})
	return nil
}

// return the signal channel used by a particular DryRunning job with a particular uuid (or if uuid="" then match on
//
//	name only)
func (jobs *DryRunBackupJobsState) GetCancelFunctionForJob(BackupJobName string, BackupJobId string) (context.CancelFunc, error) {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()

	var CancelFunction context.CancelFunc
	found := false

	for _, job := range jobs.DryRunning {
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
	return nil, errors.New(ErrJobNotFoundInEvaluatingState)
}

// return the context for a particular Running job with a particular uuid (or if uuid="" then match on
//
//	name only)
func (jobs *DryRunBackupJobsState) GetContextForJob(BackupJobName string, BackupJobId string) (context.Context, error) {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()

	var ctx context.Context
	found := false

	for _, job := range jobs.DryRunning {
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
	return nil, errors.New(ErrJobNotFoundInEvaluatingState)
}

// returns a copy of stats so far; the copy is of StatsCounters & StatsText
func (jobs *DryRunBackupJobsState) GetStats(BackupJobName string) (BackupJobStatus, error) {
	jobs.Lock.RLock()
	defer func() {
		jobs.Lock.RUnlock()
	}()

	result := BackupJobStatus{
		StatsCounters: make(map[string]uint64),
		StatsText:     make(map[string]string),
	}
	found := false
	for _, job := range jobs.DryRunning {
		if BackupJobName == job.Name {
			found = true
			// copy maps
			for k, v := range job.StatsCounters {
				result.StatsCounters[k] = v
			}
			for k, v := range job.StatsText {
				result.StatsText[k] = v
			}
		}
	}
	if found {
		return result, nil
	}
	return result, errors.New(ErrJobNotFoundInEvaluatingState)
}
