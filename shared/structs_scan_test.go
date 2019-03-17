package shared

import (
	"github.com/satori/go.uuid"
	"sync"
	"testing"
)

// test struct.MarkEvaluating() and struct.GetStats() and struct.GetCancelFunctionForJob()
func TestMarkEvaluatingAndGetStatsAndGetCancelAndGetContextForJob(t *testing.T) {
	backupJobsState := &DryRunBackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob1"
	logContext := "TestMarkEvaluatingAndGetStatsAndGetSignalChanForJob"

	JobUuid := uuid.NewV4().String()
	err := backupJobsState.MarkEvaluating(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}
	if backupJobsState.DryRunning[0].Name != jobName {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead '%s' is reported as "+
			"evaluating", jobName, backupJobsState.DryRunning[0].Name)
	}

	if backupJobsState.DryRunning[0].State != "evaluating" {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead is reported as having "+
			"state '%s'", jobName, backupJobsState.DryRunning[0].State)
	}

	_, err = backupJobsState.GetStats(jobName)
	if err != nil {
		t.Fatalf("backupJobsState.GetStats() returned error: %s", err)
	}

	_, err = backupJobsState.GetStats("jobWichDoesNotExist")
	if err == nil {
		t.Fatal("backupJobsState.GetStats() for a job which does not exist did not return an error while one" +
			" was expected")
	}

	_, err = backupJobsState.GetContextForJob(jobName, JobUuid)
	if err != nil {
		t.Fatalf("backupJobsState.GetContextForJob() when called using valid job name and valid job id"+
			" returned error: %s", err)
	}

	_, err = backupJobsState.GetContextForJob(jobName, "")
	if err != nil {
		t.Fatalf("backupJobsState.GetContextForJob() when called using valid job name and  empty job id"+
			" returned error: %s", err)
	}

	_, err = backupJobsState.GetContextForJob("jobWichDoesNotExist2", "")
	if err == nil {
		t.Fatal("backupJobsState.GetContextForJob() when called using INVALID job name and empty job id" +
			" did not return any error despite one being expected")
	}

	_, err = backupJobsState.GetCancelFunctionForJob(jobName, JobUuid)
	if err != nil {
		t.Fatalf("backupJobsState.GetCancelFunctionForJob() when called using valid job name and valid job id"+
			" returned error: %s", err)
	}

	_, err = backupJobsState.GetCancelFunctionForJob(jobName, "")
	if err != nil {
		t.Fatalf("backupJobsState.GetCancelFunctionForJob() when called using valid job name and  empty job id"+
			" returned error: %s", err)
	}

	_, err = backupJobsState.GetCancelFunctionForJob("jobWichDoesNotExist2", "")
	if err == nil {
		t.Fatal("backupJobsState.GetCancelFunctionForJob() when called using INVALID job name and empty job id" +
			" did not return any error despite one being expected")
	}

}

// test struct.IncrementCounter()
func TestIncrementCounter(t *testing.T) {
	backupJobsState := &DryRunBackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	jobName := "backupJob1"
	logContext := "TestMarkEvaluatingAndGetStatsAndGetSignalChanForJob"
	counterName := "examined_files"

	JobUuid := uuid.NewV4().String()
	err := backupJobsState.MarkEvaluating(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}
	if backupJobsState.DryRunning[0].Name != jobName {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead '%s' is reported as "+
			"evaluating", jobName, backupJobsState.DryRunning[0].Name)
	}

	if backupJobsState.DryRunning[0].State != "evaluating" {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead is reported as having "+
			"state '%s'", jobName, backupJobsState.DryRunning[0].State)
	}

	backupJobsState.IncrementCounter(jobName, counterName, "/a/random/path", "file", "examine", "")

	jobStats, err := backupJobsState.GetStats(jobName)
	if err != nil {
		t.Fatalf("backupJobsState.GetStats() returned error: %s", err)
	}

	if jobStats.StatsCounters[counterName] != 1 {
		t.Fatalf("%s counter was expected to have value 1 but instead it has: %d", counterName,
			jobStats.StatsCounters[counterName])
	}
}

//ReportChan chan ScanEvalItemReport

// test struct.UpdateStatsText()
func TestUpdateStatsText(t *testing.T) {
	backupJobsState := &DryRunBackupJobsState{}
	backupJobsState.Lock = &sync.RWMutex{}
	// setup a buffered channel where we will receive 1 reply
	backupJobsState.ReportChan = make(chan ScanEvalItemReport, 1)
	jobName := "backupJob1"
	logContext := "TestMarkEvaluatingAndGetStatsAndGetSignalChanForJob"
	statName := "current_file"
	statValue := "dsf9023kjldsfji2894234"

	JobUuid := uuid.NewV4().String()
	err := backupJobsState.MarkEvaluating(jobName, logContext, JobUuid)
	if err != nil {
		t.Fatal(err)
	}
	if backupJobsState.DryRunning[0].Name != jobName {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead '%s' is reported as "+
			"evaluating", jobName, backupJobsState.DryRunning[0].Name)
	}

	if backupJobsState.DryRunning[0].State != "evaluating" {
		t.Fatalf("Expected job having name '%s' to be marked as evaluating but instead is reported as having "+
			"state '%s'", jobName, backupJobsState.DryRunning[0].State)
	}

	// update stats
	backupJobsState.UpdateStatsText(jobName, statName, statValue, "", "")

	var result ScanEvalItemReport
	// read stat from chan; fail if no value is there
	select {
	case result = <-backupJobsState.ReportChan:
		{
			if result.Name != statValue {
				t.Fatalf("Was expecting on report channel to get name '%s' but got '%s'", statValue, result.Name)
			}

			if result.Type != "file" {
				t.Fatalf("Was expecting on report channel to get type '%s' but got '%s'", "file",
					result.Type)
			}

			if result.Error != "" {
				t.Fatalf("Was expecting on report channel to get error '%s' but got '%s'", "",
					result.Error)
			}
			if result.Excluded {
				t.Fatal("Was expecting on report channel to get Excluded=false but got true")
			}
			if result.ExclusionExpr != "" {
				t.Fatalf("Was expecting on report channel to get exclusion expr '%s' but got '%s'", "",
					result.ExclusionExpr)
			}
		}
	default:
		{
			t.Fatalf("Report channel was empty but we were expecting 1 value")
		}
	}
}
