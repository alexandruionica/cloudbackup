package scheduler

import (
	"cloudbackup/daemon/globals"
	"cloudbackup/shared"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/robfig/cron/v3"
)

// cronParser is the parser used both at config load (in config.ValidateBackupSchedule) and at
// runtime registration. Keeping the two in sync guarantees that anything which passed validation
// can be registered without surprise.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// cronManager owns a *cron.Cron plus the bookkeeping needed to expose the next scheduled run for
// each backup job. It is started by scheduler.Start and lives for the lifetime of the daemon.
//
// Concurrency:
//   - the *cron.Cron value is rebuilt on every config reload — old entries fire no more once
//     Stop() returns. Only the manager goroutine touches the *cron.Cron value, so no extra
//     locking is needed around it.
//   - nextRunByName is updated under nextRunMu and read by callers from any goroutine
//     (BackupJobsState.Get does this when assembling /backup/list responses).
type cronManager struct {
	cfgChange       <-chan bool
	commBackup      *shared.CommWithSchedulerForBackup
	configuration   *shared.RuntimeConfig
	backupJobsState *shared.BackupJobsState

	c *cron.Cron

	// jobNameByEntry maps cron internal entry IDs back to the backup job name they trigger.
	// Rebuilt on every reload alongside the cron instance.
	jobNameByEntry map[cron.EntryID]string

	// nextRunByName caches the soonest next-run time across all schedule entries belonging to a
	// given backup job. Reads are done from outside the goroutine, so guarded by nextRunMu.
	nextRunMu     sync.RWMutex
	nextRunByName map[string]time.Time
}

func newCronManager(cfgChange <-chan bool, commBackup *shared.CommWithSchedulerForBackup,
	configuration *shared.RuntimeConfig, backupJobsState *shared.BackupJobsState) *cronManager {
	return &cronManager{
		cfgChange:       cfgChange,
		commBackup:      commBackup,
		configuration:   configuration,
		backupJobsState: backupJobsState,
		jobNameByEntry:  map[cron.EntryID]string{},
		nextRunByName:   map[string]time.Time{},
	}
}

// run is the manager's main loop. It exits when ctx is cancelled.
func (m *cronManager) run(ctx context.Context) {
	globals.Stats.IncrementRoutines("other")
	defer globals.Stats.DecrementRoutines("other")

	logger.Debug("Starting cron scheduling component")

	m.rebuild(ctx)

	for {
		select {
		case <-ctx.Done():
			if m.c != nil {
				stopCtx := m.c.Stop()
				// Cron.Stop returns a context that is Done once all currently-running jobs
				// finish. Our jobs only do a channel send/receive against the eventProcessor
				// and abandon on ctx.Done() so this should return quickly. Bound the wait so
				// we never block daemon shutdown.
				select {
				case <-stopCtx.Done():
				case <-time.After(2 * time.Second):
					logger.Warn("cron scheduler did not stop within 2s of shutdown signal")
				}
			}
			logger.Debug("Cron scheduling component exited")
			return
		case _, ok := <-m.cfgChange:
			if !ok {
				return
			}
			logger.Info("Cron scheduler reloading schedule entries")
			m.rebuild(ctx)
		}
	}
}

// rebuild stops the current cron instance (if any) and registers fresh entries from the current
// configuration. Called on startup and on every config reload.
func (m *cronManager) rebuild(ctx context.Context) {
	if m.c != nil {
		stopCtx := m.c.Stop()
		select {
		case <-stopCtx.Done():
		case <-time.After(2 * time.Second):
			logger.Warn("cron scheduler did not stop within 2s of reload; new schedule will start anyway")
		}
	}
	m.c = cron.New(cron.WithParser(cronParser))
	m.jobNameByEntry = map[cron.EntryID]string{}

	cfgCopy := m.configuration.GetCopyWithLock(loggingContext + ".cronManager.rebuild")
	cfgCopy.Mutex.RLock()
	registered := 0
	for _, backup := range cfgCopy.Backup {
		jobName := backup.Name
		for _, expr := range backup.Schedule {
			expr := strings.TrimSpace(expr)
			if expr == "" {
				continue
			}
			id, err := m.c.AddFunc(expr, func() { m.fireBackup(ctx, jobName) })
			if err != nil {
				// ValidateBackupSchedule should have caught this at config load; log and skip.
				logger.Warnf("Could not register schedule '%s' for backup '%s': %s", expr, jobName, err)
				continue
			}
			m.jobNameByEntry[id] = jobName
			registered++
		}
	}
	cfgCopy.Mutex.RUnlock()

	m.c.Start()
	m.refreshNextRuns()

	logger.Infof("Cron scheduler registered %d schedule entries", registered)
}

// fireBackup is the closure invoked by the cron library when a schedule entry matches. It pushes
// a "start" command onto the scheduler's command channel and reads the response, exactly as the
// HTTP handler does — so all the existing serialization, UUID generation and "already running"
// detection inside processBackupCommand is reused unchanged.
//
// The select on ctx.Done() at every channel operation is what makes this safe during shutdown:
// if cron fires concurrently with daemon shutdown, the closure abandons mid-send instead of
// deadlocking against an eventProcessor that has already exited.
func (m *cronManager) fireBackup(ctx context.Context, jobName string) {
	if ctx.Err() != nil {
		return
	}
	cmdId, err := uuid.NewV4()
	if err != nil {
		logger.Warnf("Could not generate a request id for scheduled backup '%s': %s", jobName, err)
		return
	}
	cmd := shared.ReceiveBackupCommand{
		Name:    jobName,
		Command: "start",
		Id:      cmdId.String(),
	}

	logger.Infof("Schedule fired for backup job '%s' — requesting start", jobName)

	select {
	case m.commBackup.ReceivedCommand <- cmd:
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
		logger.Warnf("Scheduled trigger for backup '%s' could not be delivered to the scheduler within 5s", jobName)
		return
	}

	select {
	case resp := <-m.commBackup.SendResponse:
		if resp.Err {
			// Most common case: a previous run is still running. Log at info, not warn — this
			// is expected when a slow backup overlaps the next firing.
			if resp.Message == shared.ErrJobAlreadyRunning {
				logger.Infof("Scheduled run for backup '%s' skipped — previous run still active", jobName)
			} else {
				logger.Warnf("Scheduled run for backup '%s' could not be started: %s", jobName, resp.Message)
			}
			return
		}
		logger.Infof("Scheduled run for backup '%s' started with job id '%s'", jobName, resp.BackupJobId)
		// the next-run time has just advanced; refresh the cache so /backup/list reflects it
		m.refreshNextRuns()
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
		logger.Warnf("No response from scheduler within 20s for scheduled trigger of backup '%s'", jobName)
	}
}

// refreshNextRuns walks the live cron entries and rebuilds the name → next-run cache. The
// soonest next-run wins when a job has multiple schedule entries.
func (m *cronManager) refreshNextRuns() {
	updated := map[string]time.Time{}
	if m.c != nil {
		for _, e := range m.c.Entries() {
			name, ok := m.jobNameByEntry[e.ID]
			if !ok {
				continue
			}
			existing, seen := updated[name]
			if !seen || e.Next.Before(existing) {
				updated[name] = e.Next
			}
		}
	}
	m.nextRunMu.Lock()
	m.nextRunByName = updated
	m.nextRunMu.Unlock()
}

// NextRunFor returns the soonest scheduled next run for a given backup job name, or the zero
// time.Time if the backup has no schedule (or no schedule has been registered yet).
func (m *cronManager) NextRunFor(name string) time.Time {
	m.nextRunMu.RLock()
	defer m.nextRunMu.RUnlock()
	return m.nextRunByName[name]
}
