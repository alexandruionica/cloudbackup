package scheduler

import (
	"cloudbackup/shared"
	"context"
	"sync"
	"testing"
	"time"
)

// makeRuntimeConfigWithSchedule builds a minimal *shared.RuntimeConfig containing one backup
// with the supplied name and schedule entries. Tests use this to drive the cron manager without
// going through the full YAML config loader.
func makeRuntimeConfigWithSchedule(name string, schedule []string) *shared.RuntimeConfig {
	cfg := shared.CfgTemplate{
		Backup: []shared.ConfigBackup{
			{
				Name:     name,
				Schedule: append([]string{}, schedule...),
			},
		},
		Mutex: &sync.RWMutex{},
	}
	return &shared.RuntimeConfig{
		Mutex:  &sync.RWMutex{},
		Config: cfg,
	}
}

// stubResponder simulates the eventProcessor: it reads commands from ReceivedCommand and writes
// responses on SendResponse. The reply is built by the caller-supplied function so individual
// tests can simulate success, "already running", etc.
//
// Stops when ctx is cancelled. Returns a channel that receives every command read so the test
// can observe and assert on them.
func stubResponder(ctx context.Context, comm *shared.CommWithSchedulerForBackup,
	reply func(shared.ReceiveBackupCommand) shared.ResponseBackupCommand) chan shared.ReceiveBackupCommand {
	observed := make(chan shared.ReceiveBackupCommand, 16)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case cmd := <-comm.ReceivedCommand:
				observed <- cmd
				resp := reply(cmd)
				select {
				case comm.SendResponse <- resp:
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}
	}()
	return observed
}

// TestCronFiresScheduledBackup registers an "@every 1s" entry and asserts that within 3 seconds
// at least one start command has been delivered to the scheduler channel for the right job name.
func TestCronFiresScheduledBackup(t *testing.T) {
	cfg := makeRuntimeConfigWithSchedule("nightly", []string{"@every 1s"})
	comm := &shared.CommWithSchedulerForBackup{}
	comm.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	observed := stubResponder(ctx, comm, func(cmd shared.ReceiveBackupCommand) shared.ResponseBackupCommand {
		return shared.ResponseBackupCommand{Name: cmd.Name, Id: cmd.Id, BackupJobId: "stub-uuid"}
	})

	cm := newCronManager(make(chan bool, 1), comm, cfg, shared.NewJobsState())
	go cm.run(ctx)

	select {
	case got := <-observed:
		if got.Name != "nightly" {
			t.Fatalf("expected scheduled command for 'nightly', got '%s'", got.Name)
		}
		if got.Command != "start" {
			t.Fatalf("expected Command='start', got '%s'", got.Command)
		}
		if got.Id == "" {
			t.Fatal("expected a non-empty request id")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cron did not fire scheduled backup within 3 seconds")
	}
}

// TestCronReloadSwapsSchedule starts with a schedule for one backup, then mutates the
// configuration to a different backup name, sends a reload, and asserts new firings target the
// new name. The old name must stop firing.
func TestCronReloadSwapsSchedule(t *testing.T) {
	cfg := makeRuntimeConfigWithSchedule("alpha", []string{"@every 1s"})
	comm := &shared.CommWithSchedulerForBackup{}
	comm.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	observed := stubResponder(ctx, comm, func(cmd shared.ReceiveBackupCommand) shared.ResponseBackupCommand {
		return shared.ResponseBackupCommand{Name: cmd.Name, Id: cmd.Id, BackupJobId: "stub-uuid"}
	})

	reload := make(chan bool, 1)
	cm := newCronManager(reload, comm, cfg, shared.NewJobsState())
	go cm.run(ctx)

	// wait for at least one "alpha" firing
	deadline := time.After(3 * time.Second)
waitAlpha:
	for {
		select {
		case got := <-observed:
			if got.Name == "alpha" {
				break waitAlpha
			}
		case <-deadline:
			t.Fatal("did not see an 'alpha' firing within 3s")
		}
	}

	// swap the schedule to target a different backup name
	cfg.Mutex.Lock()
	cfg.Config.Backup[0].Name = "beta"
	cfg.Mutex.Unlock()
	reload <- true

	// wait for a "beta" firing AND assert no further "alpha" firings appear once we've seen one
	sawBeta := false
	endAt := time.Now().Add(4 * time.Second)
	for time.Now().Before(endAt) {
		select {
		case got := <-observed:
			if got.Name == "beta" {
				sawBeta = true
			}
			if got.Name == "alpha" && sawBeta {
				t.Fatalf("received 'alpha' firing after reload swapped schedule to 'beta'")
			}
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !sawBeta {
		t.Fatal("did not see a 'beta' firing within 4s of reload")
	}
}

// TestCronSkipsAlreadyRunning asserts that when the scheduler responds with the
// "job already running" error the cron manager logs and continues without crashing — i.e. the
// next firing still reaches the responder.
func TestCronSkipsAlreadyRunning(t *testing.T) {
	cfg := makeRuntimeConfigWithSchedule("hourly", []string{"@every 1s"})
	comm := &shared.CommWithSchedulerForBackup{}
	comm.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	observed := stubResponder(ctx, comm, func(cmd shared.ReceiveBackupCommand) shared.ResponseBackupCommand {
		return shared.ResponseBackupCommand{
			Name:    cmd.Name,
			Id:      cmd.Id,
			Err:     true,
			Message: shared.ErrJobAlreadyRunning,
		}
	})

	cm := newCronManager(make(chan bool, 1), comm, cfg, shared.NewJobsState())
	go cm.run(ctx)

	// expect at least two firings even though every one is rejected — proves the cron loop is
	// not derailed by the "already running" reply.
	count := 0
	deadline := time.After(4 * time.Second)
	for count < 2 {
		select {
		case <-observed:
			count++
		case <-deadline:
			t.Fatalf("expected at least 2 firings, got %d", count)
		}
	}
}

// TestCronShutdownIsClean verifies that cancelling the context exits cm.run promptly and that an
// in-flight firing does not deadlock.
func TestCronShutdownIsClean(t *testing.T) {
	cfg := makeRuntimeConfigWithSchedule("any", []string{"@every 1s"})
	comm := &shared.CommWithSchedulerForBackup{}
	comm.Init()

	ctx, cancel := context.WithCancel(context.Background())

	// no responder — sends to ReceivedCommand will block forever, simulating a stalled
	// eventProcessor at shutdown time. The cron firing closure must abandon via ctx.Done().

	done := make(chan struct{})
	cm := newCronManager(make(chan bool, 1), comm, cfg, shared.NewJobsState())
	go func() {
		cm.run(ctx)
		close(done)
	}()

	// give cron a moment to register and fire once into the empty channel
	time.Sleep(1500 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cronManager.run did not exit within 5 seconds of ctx cancel")
	}
}

// TestCronNextRunFor asserts that NextRunFor returns a future time once a schedule entry is
// registered, and a zero time for unknown backup names.
func TestCronNextRunFor(t *testing.T) {
	cfg := makeRuntimeConfigWithSchedule("daily", []string{"@every 5s"})
	comm := &shared.CommWithSchedulerForBackup{}
	comm.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cm := newCronManager(make(chan bool, 1), comm, cfg, shared.NewJobsState())
	// rebuild populates nextRunByName synchronously so we can read it without timing-dependent waits
	cm.rebuild(ctx)

	got := cm.NextRunFor("daily")
	if got.IsZero() {
		t.Fatal("expected non-zero next-run time for 'daily'")
	}
	if !got.After(time.Now()) {
		t.Fatalf("expected next-run in the future, got %s", got)
	}
	if !cm.NextRunFor("does-not-exist").IsZero() {
		t.Fatal("expected zero time for unknown job name")
	}
}
