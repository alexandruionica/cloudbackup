//go:build !windows
// +build !windows

package misc

// setupPlatformLogging is a no-op on non-Windows platforms: when no --logfile
// is supplied, logging stays on stdout (the default set in SetupLogging). The
// Windows Event Log integration lives in logging_windows.go.
func setupPlatformLogging() {}
