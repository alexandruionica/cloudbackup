package shared

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// drainWatchChan asynchronously drains the WatchMsgReceiver so MarkRunning et al. don't
// block when they emit watcher messages. The returned cancel function stops draining.
func drainWatchChan(t *testing.T, s *BackupJobsState) (cancel func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-s.WatchMsgReceiver:
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// -----------------------------------------------------------------------------
// MarkRunning / MarkStopped lifecycle
// -----------------------------------------------------------------------------

func TestMarkRunning_AddsJobAndIsRunning(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	if err := s.MarkRunning("job1", "test", "uuid-1"); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	if !s.IsRunning("job1", "", "test") {
		t.Error("expected job1 to be reported as running")
	}
	if !s.IsRunning("job1", "uuid-1", "test") {
		t.Error("expected job1+uuid-1 to be reported as running")
	}
	if s.IsRunning("job1", "wrong-uuid", "test") {
		t.Error("did not expect job1+wrong-uuid to be running")
	}
}

func TestMarkRunning_RefusesDuplicateName(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	if err := s.MarkRunning("job1", "test", "uuid-1"); err != nil {
		t.Fatal(err)
	}
	err := s.MarkRunning("job1", "test", "uuid-2")
	if err == nil || !strings.Contains(err.Error(), ErrJobAlreadyRunning) {
		t.Fatalf("expected ErrJobAlreadyRunning, got %v", err)
	}
}

func TestMarkStopped_TransitionsToStopping(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	_ = s.MarkRunning("job1", "test", "uuid-1")
	if err := s.MarkStopped("job1", "test", "uuid-1", false); err != nil {
		t.Fatalf("MarkStopped(stopped=false): %v", err)
	}
	if !s.IsStopping("job1", "uuid-1", "test") {
		t.Error("expected job1 to be stopping after MarkStopped(false)")
	}
	// transition to fully stopped — entry should disappear from Running
	if err := s.MarkStopped("job1", "test", "uuid-1", true); err != nil {
		t.Fatalf("MarkStopped(stopped=true): %v", err)
	}
	if s.IsRunning("job1", "", "test") {
		t.Error("expected job1 to no longer be running after MarkStopped(true)")
	}
}

func TestMarkStopped_ReturnsErrorForUnknownJob(t *testing.T) {
	s := NewJobsState()
	err := s.MarkStopped("ghost", "test", "u", true)
	if err == nil || !strings.Contains(err.Error(), ErrJobAlreadyStopped) {
		t.Fatalf("expected ErrJobAlreadyStopped, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// MarkRestoreRunning shares the running-name slot with backups
// -----------------------------------------------------------------------------

func TestMarkRestoreRunning_RefusesIfBackupAlreadyRunningWithSameName(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	if err := s.MarkRunning("job1", "test", "uuid-1"); err != nil {
		t.Fatal(err)
	}
	err := s.MarkRestoreRunning("job1", "test", "restore-1")
	if err == nil || !strings.Contains(err.Error(), ErrJobAlreadyRunning) {
		t.Fatalf("expected ErrJobAlreadyRunning when name slot taken, got %v", err)
	}
}

func TestMarkRestoreRunning_AddsRestoreWithCorrectJobType(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	if err := s.MarkRestoreRunning("job1", "test", "restore-1"); err != nil {
		t.Fatal(err)
	}
	restores := s.GetRestoresRunning("test")
	if len(restores) != 1 {
		t.Fatalf("expected exactly 1 running restore, got %d", len(restores))
	}
	if restores[0].JobType != "restore" {
		t.Errorf("expected JobType=restore, got %q", restores[0].JobType)
	}
	if restores[0].BackupJobId != "restore-1" {
		t.Errorf("expected BackupJobId=restore-1, got %q", restores[0].BackupJobId)
	}
}

// -----------------------------------------------------------------------------
// IncrementCounter
// -----------------------------------------------------------------------------

func TestIncrementCounter_IncrementsTheRightJob(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	_ = s.MarkRunning("job1", "test", "uuid-1")
	_ = s.MarkRunning("job2", "test", "uuid-2")

	for i := 0; i < 3; i++ {
		s.IncrementCounter("job1", "examined_files", "/p", "file", "examine", "")
	}
	s.IncrementCounter("job2", "examined_files", "/p", "file", "examine", "")

	s.Lock.RLock()
	defer s.Lock.RUnlock()
	for _, j := range s.Running {
		switch j.Name {
		case "job1":
			if got := j.StatsCounters["examined_files"]; got != 3 {
				t.Errorf("job1 examined_files = %d, want 3", got)
			}
		case "job2":
			if got := j.StatsCounters["examined_files"]; got != 1 {
				t.Errorf("job2 examined_files = %d, want 1", got)
			}
		}
	}
}

func TestIncrementCounter_NoOpForUnknownJob(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	// must not panic; just silently no-op
	s.IncrementCounter("does_not_exist", "examined_files", "/p", "file", "examine", "")
}

// -----------------------------------------------------------------------------
// IncrementRateCounter
// -----------------------------------------------------------------------------

func TestIncrementRateCounter_RegistersStoreAndAccumulates(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	_ = s.MarkRunning("job1", "test", "uuid-1")
	s.IncrementRateCounter("job1", "aws_1", "aws_s3", 1024, "/p", 50, true)
	s.IncrementRateCounter("job1", "aws_1", "aws_s3", 2048, "/p", 75, false)

	s.Lock.RLock()
	defer s.Lock.RUnlock()
	if len(s.Running) != 1 {
		t.Fatalf("expected 1 running job, got %d", len(s.Running))
	}
	if len(s.Running[0].ObjectStoreRates) != 1 {
		t.Fatalf("expected exactly one ObjectStoreRate entry, got %d", len(s.Running[0].ObjectStoreRates))
	}
	rate := s.Running[0].ObjectStoreRates[0]
	if rate.Name != "aws_1" || rate.Type != "aws_s3" {
		t.Errorf("unexpected store metadata: %+v", rate)
	}
}

func TestIncrementRateCounter_NoOpForUnknownJob(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()
	// must not panic
	s.IncrementRateCounter("nope", "aws_1", "aws_s3", 1, "/p", 1, true)
}

// -----------------------------------------------------------------------------
// GetCancelFunctionForJob / GetContextForJob / GetStartTime / GetRunningBackupJobId
// -----------------------------------------------------------------------------

func TestGetCancelFunctionForJob(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	_ = s.MarkRunning("job1", "test", "uuid-1")
	cancel, err := s.GetCancelFunctionForJob("job1", "uuid-1")
	if err != nil {
		t.Fatalf("expected cancel function, got error %v", err)
	}
	if cancel == nil {
		t.Fatal("expected non-nil cancel function")
	}
	// Calling cancel should mark the job's context as canceled.
	cancel()
	ctx, err := s.GetContextForJob("job1", "")
	if err != nil {
		t.Fatalf("GetContextForJob: %v", err)
	}
	// give the context machinery a moment to propagate the cancel (it should be instant
	// but the docs are explicit that Done() may not be ready before cancel returns)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled within 1s after Cancel()")
	}
	if ctx.Err() != context.Canceled {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
}

func TestGetCancelFunctionForJob_UnknownReturnsError(t *testing.T) {
	s := NewJobsState()
	_, err := s.GetCancelFunctionForJob("ghost", "")
	if err == nil || !strings.Contains(err.Error(), ErrJobNotFoundInRunningState) {
		t.Fatalf("expected ErrJobNotFoundInRunningState, got %v", err)
	}
}

func TestGetContextForJob_UnknownReturnsError(t *testing.T) {
	s := NewJobsState()
	_, err := s.GetContextForJob("ghost", "")
	if err == nil || !strings.Contains(err.Error(), ErrJobNotFoundInRunningState) {
		t.Fatalf("expected ErrJobNotFoundInRunningState, got %v", err)
	}
}

func TestGetStartTime_RoundtripAndError(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	beforeMark := time.Now()
	_ = s.MarkRunning("job1", "test", "uuid-1")
	got, err := s.GetStartTime("job1", "uuid-1", "test")
	if err != nil {
		t.Fatalf("GetStartTime: %v", err)
	}
	if got.Before(beforeMark) {
		t.Errorf("StartTime %v is before the MarkRunning call %v", got, beforeMark)
	}
	if _, err := s.GetStartTime("ghost", "", "test"); err == nil {
		t.Fatal("expected error for unknown job")
	}
}

func TestGetRunningBackupJobId_RoundtripAndError(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	_ = s.MarkRunning("job1", "test", "uuid-42")
	got, err := s.GetRunningBackupJobId("job1", "test")
	if err != nil {
		t.Fatalf("GetRunningBackupJobId: %v", err)
	}
	if got != "uuid-42" {
		t.Errorf("got %q, want uuid-42", got)
	}
	if _, err := s.GetRunningBackupJobId("ghost", "test"); err == nil {
		t.Fatal("expected error for unknown job")
	}
}

// -----------------------------------------------------------------------------
// Concurrent stress — exercises the locks under -race
// -----------------------------------------------------------------------------

func TestBackupJobsState_ConcurrentReadersAndCounterIncrements(t *testing.T) {
	s := NewJobsState()
	stop := drainWatchChan(t, s)
	defer stop()

	for i := 0; i < 5; i++ {
		name := "job" + string(rune('A'+i))
		if err := s.MarkRunning(name, "test", "u-"+name); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	const writers = 8
	const reads = 8
	const iter = 200

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "job" + string(rune('A'+(i%5)))
			for j := 0; j < iter; j++ {
				s.IncrementCounter(name, "examined_files", "/p", "file", "examine", "")
			}
		}(i)
	}
	for i := 0; i < reads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iter; j++ {
				_ = s.IsRunning("jobA", "", "test")
				_, _ = s.GetRunningBackupJobId("jobA", "test")
			}
		}()
	}
	wg.Wait()
}
