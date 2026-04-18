package shared

import (
	"cloudbackup/utils"
	"database/sql"
)

type DbData struct {
	Db *sql.DB
	// it is CRITICAL that the name matches the name of the backup job as defined in the configuration file.
	// If not, it will end up with a null pointer dereference caused by another function which reads DbData.Name and
	// uses it to search a map containing pointers
	Name               string
	Connected          bool
	PreparedStatements DbPreparedStatements
}

// this struct is normally consulted by a client in order to see if there is a *sql.DB available and if not, if it can obtain a new one
type DbAccess struct {
	// if its allowed to open/start using the database then a lock can be obtained (must be immediately released after incrementing $NumClients)
	DbOpenAllowed *utils.MutexWithTimeout
	// number of connected clients to this Database. Must be read / written only via sync.Atomic package . Can be read/decremented also when $DbOpenAllowed is not held
	NumClients uint32
}

// this normally populated by dataabase/dbops/Prepare()
type DbPreparedStatements struct {
	// each "string" entry contains the sql statement to be used for preparing the statement
	FilesQuery                                  string
	FilesQueryStmt                              *sql.Stmt
	FilesInsert                                 string
	FilesInsertStmt                             *sql.Stmt
	FilesUpdate                                 string
	FilesUpdateStmt                             *sql.Stmt
	FilesDelete                                 string
	JobStartTime                                string
	JobPlatform                                 string
	RemoteFilesInsert                           string
	RemoteFilesQueryNewestVersion               string
	RemoteFilesQueryNewestVersionUuidStmt       *sql.Stmt
	RemoteFilesQueryRemoteVersion               string
	BackupCollectionsInsert                     string
	BackupCollectionsInsertStmt                 *sql.Stmt
	FindDeletedItemsStmt                        *sql.Stmt
	FailedFilesInsertStmt                       *sql.Stmt
	ReportBackupJobsListQuery                   string
	ReportBackupJobsShowQuery                   string
	ReportBackupJobsFileListFindJobQuery        string
	ReportBackupJobsFileListWithJobId           string
	ReportBackupJobsFileListWithJobIdAndDescend string
	TopItemsInsert                              string
}

// RestoreDbData is the equivalent of DbData for a per-target restore database.
type RestoreDbData struct {
	Db                 *sql.DB
	Name               string
	Connected          bool
	PreparedStatements RestoreDbPreparedStatements
}

// RestoreDbPreparedStatements holds prepared statements and SQL text used by
// the restore-tracking database. Populated by database/dbops.PrepareRestore().
type RestoreDbPreparedStatements struct {
	InsertJob            string
	UpdateJobFinal       string
	UpdateJobState       string
	JobExists            string
	JobFetch             string
	InsertManifestRow    string
	SetFileState         string
	LoadPendingManifest  string
	BumpCounter          string
	InitCounterRow       string
	ReadCounters         string
	ReportJobsListQuery  string
	ReportJobShowQuery   string
	ReportFilesListQuery string
	FindCrashedJobsQuery string
	MarkCrashedJobQuery  string
}
