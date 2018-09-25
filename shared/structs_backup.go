package shared

import (
	"time"
)

// this needs to have a 1 to 1 mapping with the SQL table definition mentioned in cloudbackup/backup/backup.go
type BackedUpFileProperties struct {
	Path string
	// one of: file / dir / symlink
	Type string
	// valid only for "symlink" type; otherwise it will be empty string
	LinkTarget string
	Size int64
	// time object modified
	Mtime time.Time
	// time inode got changed, basically file properties got changed (but not file content). Exception is that ctime
	// will also get updated if the file contents got changed. Ctime is platform and file system dependent (probably
	// MS Windows doesn't have it) ; to check out this https://github.com/djherbis/times/issues/1 and the library it provides
	Ctime time.Time
	// user id on *nix , Username on Windows (hence this is a string)
	// TODO - validate that on Windows this is better than using a SID and also what to do in the Username or SID doesn't exist (on Windows only)
	Uid string
	// group id on *nix, Group name on Windows
	// TODO - validate that on Windows this is better than using a SID and also what to do in the Groupname or SID doesn't exist (on Windows only)
	Gid string
	// on *nix this is the file mode (ex: 0755) ; on Windows some kind of basic permissions
	// TODO - figure out file permissions on Windows
	PermMode string
	// if checksuming is enabled then this will be non empty
	Checksum string
	// if checksuming is enabled then this will hold whatever algorithm was used for checksumming
	ChecksumType string
	Encrypted bool
	// references the "name" of one or more entries in "targets" table ; multiple entries will be comma separated
	Targets string
}
