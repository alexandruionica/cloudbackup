//go:build windows
// +build windows

package misc

import (
	"io"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// eventLogSource is the source name events are reported under in the Windows
// Event Log (Event Viewer -> Windows Logs -> Application). It matches the
// service name registered by the MSI (packaging/windows/cloudbackup.wxs).
const eventLogSource = "cloudbackup"

// Event IDs reported to the Windows Event Log, grouped loosely by severity.
const (
	eidInfo    = 1
	eidWarning = 2
	eidError   = 3
)

// eventLogHook is a logrus hook that mirrors every log entry to the Windows
// Event Log, mapping logrus levels onto the three Event Log severities.
type eventLogHook struct {
	elog *eventlog.Log
}

func (h *eventLogHook) Levels() []log.Level { return log.AllLevels }

func (h *eventLogHook) Fire(entry *log.Entry) error {
	// entry.String() renders the entry with the formatter configured in
	// SetupLogging (JSON by default, plaintext with --textlog), so the Event
	// Log body matches what a --logfile would contain.
	msg, err := entry.String()
	if err != nil {
		msg = entry.Message
	}

	switch entry.Level {
	case log.PanicLevel, log.FatalLevel, log.ErrorLevel:
		return h.elog.Error(eidError, msg)
	case log.WarnLevel:
		return h.elog.Warning(eidWarning, msg)
	default:
		return h.elog.Info(eidInfo, msg)
	}
}

// setupPlatformLogging routes logging to the Windows Event Log. It is called
// from SetupLogging only when no --logfile was supplied. When --logfile is
// given the Event Log is intentionally not used and this is never called.
func setupPlatformLogging() {
	// Best-effort registration of the event source so the Event Viewer renders
	// messages with a proper description rather than a "description not found"
	// note. This writes under HKLM and needs admin/LocalSystem privileges; the
	// service runs as LocalSystem so it normally succeeds. If it fails (already
	// registered, or running interactively without admin) events still appear,
	// so the error is deliberately ignored.
	_ = eventlog.InstallAsEventCreate(eventLogSource, eventlog.Info|eventlog.Warning|eventlog.Error)

	elog, err := eventlog.Open(eventLogSource)
	if err != nil {
		// Could not reach the Event Log; keep the stdout output set by
		// SetupLogging so messages are not lost.
		log.Errorf("could not open Windows event log source %q: %s; logging to stdout instead", eventLogSource, err)
		return
	}

	log.AddHook(&eventLogHook{elog: elog})

	// When launched by the Service Control Manager there is no usable console,
	// so silence the stdout writer (the hook carries the messages to the Event
	// Log). When running interactively keep stdout so foreground/console
	// debugging still shows output in addition to the Event Log.
	if isService, svcErr := svc.IsWindowsService(); svcErr == nil && isService {
		log.SetOutput(io.Discard)
	}
}
