package shared

import "database/sql"

type DbData struct {
	Db *sql.DB
	// it is CRITICAL that the name matches the name of the backup job as defined in the configuration file.
	// If not, it will end up with a null pointer dereference caused by another function which reads DbData.Name and
	// uses it to search a map containing pointers
	Name               string
	Connected          bool
	PreparedStatements DbPreparedStatements
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
}
