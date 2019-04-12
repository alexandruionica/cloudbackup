package fileproperties

import (
	"errors"
	"github.com/djherbis/times"
	log "github.com/sirupsen/logrus"
	"time"
)

const loggingContext = "backup.fileproperties"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

var (
	ErrCouldNotGetSecurityInfo    = errors.New("unable to get security information (file access control details)")
	ErrCouldNotGetOwner           = errors.New("could not establish owner")
	ErrCouldNotGetGroup           = errors.New("could not establish owning group")
	ErrCouldNotGetAccountDetails  = errors.New("could not get account details")
	ErrCouldNotGetSidString       = errors.New("could not obtain a string representation of the account SID")
	ErrUnsupportedAceType         = errors.New("unsupported type of access control entry")
	ErrCouldNotJsonEncode         = errors.New("could not json encode the object's permissions")
	ErrPlatformDoesntSupportCtime = errors.New("the platform doesn't support reporting file or directory change time property")
	ErrWhileRetrievingCtime       = errors.New("encountered an error while trying to get file or directory change time property")
)

// get a file or directory's Ctime; if $dereference == true then follow symlinks
func GetCtime(path string, dereference bool) (time.Time, error) {
	var t times.Timespec
	var err error
	if dereference {
		t, err = times.Stat(path)
	} else {
		t, err = times.Lstat(path)
	}
	if err != nil {
		logger.Warnf("while trying to get change time for '%s' the following error was encountered: %s", path, err)
		return time.Time{}, ErrWhileRetrievingCtime
	}

	if t.HasChangeTime() {
		return t.ChangeTime(), nil
	}
	return time.Time{}, ErrPlatformDoesntSupportCtime
}
