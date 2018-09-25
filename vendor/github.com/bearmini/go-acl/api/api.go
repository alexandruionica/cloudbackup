// +build windows

package api

import (
	"golang.org/x/sys/windows"
)

var (
	advapi32 = windows.MustLoadDLL("advapi32.dll")
	kernel32 = windows.MustLoadDLL("kernel32.dll")
)

var (
	procLocalAlloc = kernel32.MustFindProc("LocalAlloc")
	procLocalFree  = kernel32.MustFindProc("LocalFree")
)
