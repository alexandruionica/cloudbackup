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
	// if its allowed to open/start using the database then a lock can be obtained (must be immediately released after incrementing $NumClients and reading/writing $DB)
	DbOpenAllowed *utils.MutexWithTimeout
	// number of connected clients to this Database. Must be read / written only via sync.Atomic package . Can be read/decremented also when $DbOpenAllowed is not held
	NumClients uint32
	// pointer to DB client object. Before using, must test it's not nil . Value must be read / written only while the $DbOpenAllowed lock is held
	DB *sql.DB
}

// this normally populated by dataabase/dbops/Prepare()
type DbPreparedStatements struct {
	// each "string" entry contains the sql statement to be used for preparing the statement
	FilesQuery                            string
	FilesQueryStmt                        *sql.Stmt
	FilesInsert                           string
	FilesInsertStmt                       *sql.Stmt
	FilesUpdate                           string
	FilesUpdateStmt                       *sql.Stmt
	FilesDelete                           string
	RemoteFilesInsert                     string
	RemoteFilesQueryNewestVersion         string
	RemoteFilesQueryNewestVersionUuidStmt *sql.Stmt
	RemoteFilesQueryRemoteVersion         string
	BackupCollectionsInsert               string
	BackupCollectionsInsertStmt           *sql.Stmt
	FindDeletedItemsStmt                  *sql.Stmt
	FailedFilesInsertStmt                 *sql.Stmt
	ReportBackupJobsListQuery             string
	ReportBackupJobsShowQuery             string
	TopItemsInsert                        string
}
