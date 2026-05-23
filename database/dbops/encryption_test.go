package dbops

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openEphemeralFilesTable opens an in-memory SQLite DB and creates the
// files table with just the columns these tests need. Keeps the test
// independent of database.Start which requires a config + temp dir.
func openEphemeralFilesTable(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE files (path TEXT PRIMARY KEY, encrypted INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	return db
}

func TestHasAnyEncryptedFiles_Empty(t *testing.T) {
	db := openEphemeralFilesTable(t)
	defer db.Close()
	got, err := HasAnyEncryptedFiles(db)
	if err != nil {
		t.Fatalf("HasAnyEncryptedFiles: %v", err)
	}
	if got {
		t.Errorf("expected false for empty table, got true")
	}
}

func TestHasAnyEncryptedFiles_NoneEncrypted(t *testing.T) {
	db := openEphemeralFilesTable(t)
	defer db.Close()
	for _, p := range []string{"/a", "/b", "/c"} {
		if _, err := db.Exec("INSERT INTO files (path, encrypted) VALUES (?, 0)", p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := HasAnyEncryptedFiles(db)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Errorf("expected false when all rows have encrypted=0, got true")
	}
}

func TestHasAnyEncryptedFiles_OneEncrypted(t *testing.T) {
	db := openEphemeralFilesTable(t)
	defer db.Close()
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/a', 0)")
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/b', 1)")
	got, err := HasAnyEncryptedFiles(db)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected true when at least one row has encrypted=1")
	}
}

func TestResetEncryptedFlags_OnlyClearsEncryptedRows(t *testing.T) {
	db := openEphemeralFilesTable(t)
	defer db.Close()
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/a', 0)")
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/b', 1)")
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/c', 1)")
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/d', 0)")

	n, err := ResetEncryptedFlags(db)
	if err != nil {
		t.Fatalf("ResetEncryptedFlags: %v", err)
	}
	if n != 2 {
		t.Errorf("affected rows = %d, want 2", n)
	}

	// All rows now have encrypted = 0.
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM files WHERE encrypted = 1").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("expected zero encrypted=1 rows after reset, got %d", remaining)
	}

	// Total row count is preserved (no rows were deleted).
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM files").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("row count changed: got %d, want 4", total)
	}
}

func TestResetEncryptedFlags_NoOpWhenNoneEncrypted(t *testing.T) {
	db := openEphemeralFilesTable(t)
	defer db.Close()
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/a', 0)")
	_, _ = db.Exec("INSERT INTO files (path, encrypted) VALUES ('/b', 0)")
	n, err := ResetEncryptedFlags(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("affected rows = %d, want 0", n)
	}
}
