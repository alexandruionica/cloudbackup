//go:build windows
// +build windows

package cliargs

import (
	"cloudbackup/daemon"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc"
)

// serviceName is the name the MSI registers the service under (see
// packaging/windows/cloudbackup.wxs ServiceInstall) and must match.
const serviceName = "cloudbackup"

// cloudbackupService implements svc.Handler. It runs the normal daemon in a
// background goroutine and bridges Windows Service Control Manager (SCM)
// Stop/Shutdown controls to the daemon's graceful shutdown path.
type cloudbackupService struct {
	configFile string
	debug      bool
}

// Execute is called by svc.Run once the SCM has connected. It must report
// status transitions to the SCM via the changes channel and react to control
// requests on r.
func (m *cloudbackupService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// daemon.Start blocks until shutdown, so run it in the background. It
	// returns (rather than calling os.Exit) once WaitForEvent receives on
	// daemon.ServiceShutdown.
	daemonDone := make(chan struct{})
	go func() {
		daemon.Start(m.configFile, m.debug)
		close(daemonDone)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				// Ask the daemon to tear down gracefully and wait for it.
				daemon.ServiceShutdown <- struct{}{}
				<-daemonDone
				return false, 0
			default:
				log.Warnf("unexpected service control request #%d", c.Cmd)
			}
		case <-daemonDone:
			// The daemon exited on its own (e.g. a fatal config/DB error
			// during startup). Report stopped so the SCM doesn't hang.
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		}
	}
}

// maybeRunAsService returns true when the process was launched by the Windows
// Service Control Manager, in which case it hands control to svc.Run (which
// blocks until the service is stopped). When run from an interactive console it
// returns false so the caller starts the daemon in the foreground as usual.
func maybeRunAsService(configFile string, debug bool) bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Errorf("could not determine if running as a Windows service: %s; starting in foreground", err)
		return false
	}
	if !isService {
		return false
	}

	handler := &cloudbackupService{configFile: configFile, debug: debug}
	if err := svc.Run(serviceName, handler); err != nil {
		log.Errorf("Windows service %q failed: %s", serviceName, err)
	}
	return true
}
