package shared

import "database/sql"


type DbData struct {
	Db *sql.DB
	// db name, used only for error messages
	Name string
	Connected bool
	PreparedStatements DbPreparedStatements
}
// this normally populated by dataabase/dbops/Prepare()
type DbPreparedStatements struct {
	QueryStmt *sql.Stmt
	InsertStmt *sql.Stmt
	UpdateStmt *sql.Stmt
}