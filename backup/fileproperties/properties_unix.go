// +build darwin freebsd netbsd openbsd solaris linux

package fileproperties

import (
	"encoding/json"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

type Account struct {
	Id uint32
	// this is the name to which the above numerical id resolves - we'll try to use this when restoring if the numerical
	// id doesn't exist
	Name string
}

type FilePermissions struct {
	Owner Account
	Group Account
	Mode os.FileMode
}

// gets in a platform dependent way the properties of a file or directory. The code here works probably only on
// parameters: $path is the path to the file/directory ; $stat - not used in the Windows implementation
// ext3/ext4/xfs/ufs/zfs file systems while probably also other posix compliant ones will work
// returns: owner name (string) ; FilePermissions object which was JSON Marshalled (string); error if != nil then the first
// two strings will be empty
//
// Example usage:
// 	_, jsonPermissions, err := getObjectPermissions(`/home/testuser/Desktop/test`, os.stat(`/home/testuser/Desktop/test`))
//	if err != nil {
//		fmt.Printf("Got error: %s\n", err)
//	} else {
//		fmt.Printf("%+v\n", jsonPermissions)
//	}
//
func getObjectPermissions(path string, stat os.FileInfo) (string, string, error) {
	filePerm := FilePermissions{
		Mode: stat.Mode(),
		Owner: Account{
			Id: stat.Sys().(*syscall.Stat_t).Uid,
		},
		Group: Account{
			Id: stat.Sys().(*syscall.Stat_t).Gid,
		},
	}

	/* !!! WARNING - !!!! os/user package uses CGO and it has potential implications
	https://golang.org/pkg/os/user/#User
	For most Unix systems, this package has two internal implementations of resolving user and group ids to names. One
	is written in pure Go and parses /etc/passwd and /etc/group. The other is cgo-based and relies on the standard C
	library (libc) routines such as getpwuid_r and getgrnam_r.
	When cgo is available, cgo-based (libc-backed) code is used by default.
	 */

	// lookup username from user id
	username, err := user.LookupId(strconv.FormatUint(uint64(filePerm.Owner.Id), 10))
	if err != nil {
		logger.Warnf("While trying to get the username for user id '%d' the following error was encountered: %s",
			filePerm.Owner.Id, err)
		return "", "", ErrCouldNotGetAccountDetails
	}
	filePerm.Owner.Name = username.Username

	// lookup group name from group id
	groupname, err := user.LookupGroupId(strconv.FormatUint(uint64(filePerm.Group.Id), 10))
	if err != nil {
		logger.Warnf("While trying to get the group name for group id '%d' the following error was encountered: %s",
			filePerm.Group.Id, err)
		return "", "", ErrCouldNotGetAccountDetails
	}
	filePerm.Group.Name = groupname.Name

	logger.Debugf("permissions of '%s' are: %+v\n", path, filePerm)

	jsonPayload, err := json.Marshal(filePerm)
	if err !=nil {
		logger.Warnf("Could not JSON encode the permissions of '%s' due to error: '%s'", path, err)
		return "", "", ErrCouldNotJsonEncode
	}
	return filePerm.Owner.Name, string(jsonPayload), nil
}