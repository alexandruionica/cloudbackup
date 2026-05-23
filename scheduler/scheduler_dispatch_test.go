package scheduler

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/uuid"
)

// loadMockConfig is the standard mock-config setup used throughout the scheduler tests.
// It returns a populated CfgTemplate plus a cleanup function that removes any temporary
// files the loader created on disk.
func loadMockConfig(t *testing.T, prefix string) (shared.CfgTemplate, func()) {
	t.Helper()
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, prefix)
	cleanup := func() { testutils.DeleteTestFilesAndDirs(pathsToDelete) }

	result, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		cleanup()
		t.Fatalf("could not load mock config: %v", err)
	}
	return result.GetCopyWithLock("test"), cleanup
}

// drainWatchChan is the same helper as in shared package tests — duplicated here so the
// package's tests stay self-contained.
func drainWatchChan(t *testing.T, s *shared.BackupJobsState) func() {
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
// processBackupCommand — error-path coverage
// -----------------------------------------------------------------------------

func TestProcessBackupCommand_RejectsUnknownCommand(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_unknown_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processBackupCommand(shared.ReceiveBackupCommand{
		Name:    cfg.Backup[0].Name,
		Command: "wibble",
		Id:      "req-1",
	}, state, cfg)

	if !resp.Err {
		t.Fatal("expected Err=true for unknown command")
	}
	if !strings.Contains(resp.Message, "not one of 'start' or 'stop'") {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestProcessBackupCommand_StopWhenNotRunning(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_stop_notrun_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processBackupCommand(shared.ReceiveBackupCommand{
		Name:    cfg.Backup[0].Name,
		Command: "stop",
		Id:      "req-1",
	}, state, cfg)

	if !resp.Err {
		t.Fatal("expected Err=true when stopping a job that isn't running")
	}
	if resp.Message != shared.ErrJobAlreadyStopped {
		t.Errorf("got Message=%q, want %q", resp.Message, shared.ErrJobAlreadyStopped)
	}
}

func TestProcessBackupCommand_StopWhenAlreadyStopping(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_stop_stopping_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	jobUuid := u.String()
	if err := state.MarkRunning(cfg.Backup[0].Name, "test", jobUuid); err != nil {
		t.Fatal(err)
	}
	// Transition to "stopping" via the normal MarkStopped(name, ctx, id, false) flow.
	if err := state.MarkStopped(cfg.Backup[0].Name, "test", jobUuid, false); err != nil {
		t.Fatalf("MarkStopped(stopping): %v", err)
	}

	resp := processBackupCommand(shared.ReceiveBackupCommand{
		Name:        cfg.Backup[0].Name,
		Command:     "stop",
		Id:          "req-1",
		BackupJobId: jobUuid,
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true when stopping an already-stopping job")
	}
	if resp.Message != shared.ErrJobAlreadyStopping {
		t.Errorf("got Message=%q, want %q", resp.Message, shared.ErrJobAlreadyStopping)
	}
}

// Asking to start a job whose name is already running surfaces the ErrJobAlreadyRunning
// error returned by MarkRunning. The actual error appears in resp.Message.
func TestProcessBackupCommand_StartWhenAlreadyRunning(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_start_alreadyrun_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.MarkRunning(cfg.Backup[0].Name, "test", u.String()); err != nil {
		t.Fatal(err)
	}
	resp := processBackupCommand(shared.ReceiveBackupCommand{
		Name:    cfg.Backup[0].Name,
		Command: "start",
		Id:      "req-1",
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true when starting a job that's already running")
	}
	if !strings.Contains(resp.Message, shared.ErrJobAlreadyRunning) {
		t.Errorf("expected message to contain %q, got: %q", shared.ErrJobAlreadyRunning, resp.Message)
	}
}

// -----------------------------------------------------------------------------
// processRestoreCommand — synchronous error paths
// -----------------------------------------------------------------------------

func TestProcessRestoreCommand_RejectsUnknownCommand(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_restore_unknown_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:    cfg.Backup[0].Name,
		Command: "wibble",
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true for unknown restore command")
	}
	if !strings.Contains(resp.Message, "not one of 'start', 'stop' or 'resume'") {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestProcessRestoreCommand_ResumeRequiresJobId(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_resume_no_jobid_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:       cfg.Backup[0].Name,
		Command:    "resume",
		TargetName: "t1",
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true for resume without restore_job_id")
	}
	if !strings.Contains(resp.Message, "restore_job_id is required") {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestProcessRestoreCommand_ResumeRequiresTargetName(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_resume_no_target_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:         cfg.Backup[0].Name,
		Command:      "resume",
		RestoreJobId: "r-uuid",
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true for resume without target_name")
	}
	if !strings.Contains(resp.Message, "target_name is required") {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestProcessRestoreCommand_ResumeRejectsWhileBackupRunning(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_resume_busy_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	u, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.MarkRunning(cfg.Backup[0].Name, "test", u.String()); err != nil {
		t.Fatal(err)
	}
	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:         cfg.Backup[0].Name,
		Command:      "resume",
		RestoreJobId: "r-uuid",
		TargetName:   cfg.Backup[0].Target[0].Name,
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true for resume while a backup is running on the same name")
	}
	if resp.Message != shared.ErrJobAlreadyRunning {
		t.Errorf("got Message=%q, want %q", resp.Message, shared.ErrJobAlreadyRunning)
	}
}

func TestProcessRestoreCommand_StopWhenNotRunning(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_restore_stop_notrun_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:         cfg.Backup[0].Name,
		Command:      "stop",
		RestoreJobId: "r-uuid",
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true when stopping a restore that isn't running")
	}
	if resp.Message != shared.ErrJobAlreadyStopped {
		t.Errorf("got Message=%q, want %q", resp.Message, shared.ErrJobAlreadyStopped)
	}
}

// Stopping an already-stopping restore: mark a restore as stopping, then re-issue stop.
func TestProcessRestoreCommand_StopWhenAlreadyStopping(t *testing.T) {
	cfg, cleanup := loadMockConfig(t, "unittest_scheduler_dispatch_restore_stop_stopping_")
	defer cleanup()
	state := shared.NewJobsState()
	defer drainWatchChan(t, state)()

	restoreId := "restore-uuid-stop-stopping"
	if err := state.MarkRestoreRunning(cfg.Backup[0].Name, "test", restoreId); err != nil {
		t.Fatal(err)
	}
	if err := state.MarkStopped(cfg.Backup[0].Name, "test", restoreId, false); err != nil {
		t.Fatalf("MarkStopped(stopping): %v", err)
	}
	resp := processRestoreCommand(shared.ReceiveRestoreCommand{
		Name:         cfg.Backup[0].Name,
		Command:      "stop",
		RestoreJobId: restoreId,
	}, state, cfg)
	if !resp.Err {
		t.Fatal("expected Err=true for stop of already-stopping restore")
	}
	if resp.Message != shared.ErrJobAlreadyStopping {
		t.Errorf("got Message=%q, want %q", resp.Message, shared.ErrJobAlreadyStopping)
	}
}
