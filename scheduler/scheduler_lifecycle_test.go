package scheduler

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestStartRestoreAndShutdown drives the scheduler the way the daemon does: Start, a restore
// command over the channels (which fails fast because the source job id does not exist, still
// exercising runRestore and cleanupAfterRestore), NextRunFor for the scheduled job, then the
// shutdown handshake.
func TestStartRestoreAndShutdown(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_scheduler_lifecycle_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)
	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatal(err)
	}
	// the mock config carries cloud target types; the restore below must not reach a real provider
	configuration.Mutex.Lock()
	for i := range configuration.Config.Backup {
		for j := range configuration.Config.Backup[i].Target {
			configuration.Config.Backup[i].Target[j].Type = "test_null"
		}
	}
	configuration.Mutex.Unlock()
	state := shared.NewJobsState()
	if err := config.ValidateAndCreateDB(configuration.GetCopyWithLock("test"), state); err != nil {
		t.Fatal(err)
	}
	stopDrain := drainWatchChan(t, state)
	defer stopDrain()

	backupComm := &shared.CommWithSchedulerForBackup{}
	backupComm.Init()
	restoreComm := &shared.CommWithSchedulerForRestore{}
	restoreComm.Init()
	cfgChange := make(chan bool, 4)

	Start(cfgChange, backupComm, restoreComm, state, configuration)

	// the cron manager knows the scheduled job ("05 01 * * *" in the mock config)
	deadline := time.Now().Add(5 * time.Second)
	for NextRunFor("first_backup").IsZero() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if NextRunFor("first_backup").IsZero() {
		t.Fatal("NextRunFor returned the zero time for a scheduled job")
	}
	if !NextRunFor("no-such-job").IsZero() {
		t.Fatal("NextRunFor must return the zero time for an unknown job")
	}

	// a config reload notification is forwarded to the cron manager without blocking
	cfgChange <- true

	// restore start: accepted, runs, fails on the unknown source job, cleaned up
	restoreComm.ReceivedCommand <- shared.ReceiveRestoreCommand{Id: "cmd-1", Command: "start", Name: "first_backup",
		SourceBackupJobId: "00000000-0000-0000-0000-000000000000", AllFiles: true,
		RestoreDirOverride: filepath.Join(t.TempDir(), "restore")}
	var resp shared.ResponseRestoreCommand
	select {
	case resp = <-restoreComm.SendResponse:
	case <-time.After(10 * time.Second):
		t.Fatal("no response to the restore start command")
	}
	if resp.Err || resp.RestoreJobId == "" {
		t.Fatalf("restore start was refused: %+v", resp)
	}
	deadline = time.Now().Add(20 * time.Second)
	for len(state.GetRestoresRunning("test")) > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(state.GetRestoresRunning("test")); n != 0 {
		t.Fatalf("%d restore(s) still marked running after the job failed", n)
	}
	// once cleaned up, a stop for that id is refused as already stopped
	restoreComm.ReceivedCommand <- shared.ReceiveRestoreCommand{Id: "cmd-2", Command: "stop", Name: "first_backup", RestoreJobId: resp.RestoreJobId}
	select {
	case resp = <-restoreComm.SendResponse:
	case <-time.After(10 * time.Second):
		t.Fatal("no response to the restore stop command")
	}
	if !resp.Err {
		t.Fatalf("stopping a finished restore must be refused: %+v", resp)
	}

	// shutdown handshake: the scheduler acknowledges on the same channel
	backupComm.Shutdown <- true
	select {
	case <-backupComm.Shutdown:
	case <-time.After(10 * time.Second):
		t.Fatal("scheduler did not acknowledge shutdown")
	}
}
