package cliargs

import "testing"

// TestMaybeRunAsServiceForeground verifies that when the process is not being
// run by the Windows Service Control Manager (which is always the case during
// the unit-test run, on every platform) maybeRunAsService reports that it did
// not take over, so ArgsCommandServerStart.Execute falls through to starting
// the daemon in the foreground.
func TestMaybeRunAsServiceForeground(t *testing.T) {
	if maybeRunAsService("does-not-matter.yaml", false) {
		t.Fatal("maybeRunAsService returned true outside of the Windows SCM; " +
			"the daemon would never start in the foreground")
	}
}
