// +build darwin freebsd netbsd openbsd solaris linux

package config

import (
	"errors"
	"os"
)

func isExecutable(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}
	mode := info.Mode()
	// This test checks that the file has an executable bit set but it doesn't guarantee that the user can execute it.
	// For example it might have +x on the group but the user running it is not a member of the group.
	if mode&0111 == 0 {
		return errors.New("not executable")
	}
	return nil
}
