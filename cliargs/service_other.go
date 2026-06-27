//go:build !windows
// +build !windows

package cliargs

// maybeRunAsService is a no-op on non-Windows platforms. The Windows Service
// Control Manager integration lives in service_windows.go; everywhere else the
// daemon only ever runs in the foreground (managed by systemd, etc.).
func maybeRunAsService(_ string, _ bool) bool {
	return false
}
