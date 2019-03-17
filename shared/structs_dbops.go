package shared

import "database/sql"

type DbData struct {
	Db *sql.DB
	// db name, used only for error messages
	Name               string
	Connected          bool
	PreparedStatements DbPreparedStatements
}

// this normally populated by dataabase/dbops/Prepare()
type DbPreparedStatements struct {
	// each "string" entry contains the sql statement to be used for preparing the statement
	Query      string
	QueryStmt  *sql.Stmt
	Insert     string
	InsertStmt *sql.Stmt
	Update     string
	UpdateStmt *sql.Stmt
}
