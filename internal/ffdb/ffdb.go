package ffdb

import (
	"database/sql"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/AlanKK/findfiles/internal/models"
	"github.com/mattn/go-sqlite3"
)

// TODO:  check out driver benchmarks for sqlite3 at https://github.com/cvilsmeier/go-sqlite-bench

var records []struct {
	filename string
	path     string
}

var prefixSearchStmt *sql.Stmt
var insertStmt *sql.Stmt
var deleteStmt *sql.Stmt

// Creates and opens a new database at the given pathname.
func CreateDB(pathname string) (*sql.DB, error) {
	if fileExists(pathname) {
		return nil, os.ErrExist
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		return nil, err
	}

	createTablesAndIndexes(db)
	configureDB(db)
	prepareStatements(db)

	return db, err
}

// Opens an existing database
func OpenDB(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if !fileExists(pathname) {
		return nil, error(os.ErrNotExist)
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		return nil, err
	}

	configureDB(db)
	prepareStatements(db)

	return db, err
}

// Creates the necessary tables and indexes in the database.
func createTablesAndIndexes(db *sql.DB) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS files(filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER)")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE INDEX idx_filename ON files(filename COLLATE BINARY)")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE INDEX idx_fullpath ON files(fullpath COLLATE BINARY)")
	if err != nil {
		log.Fatal(err)
	}
}

// Configures the database with necessary settings.
func configureDB(db *sql.DB) {
	_, err := db.Exec("PRAGMA case_sensitive_like = ON")
	if err != nil {
		log.Fatal(err)
	}
	// Enable WAL mode - multiple readers and writer
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("PRAGMA busy_timeout = 5000;")
	if err != nil {
		log.Fatal(err)
	}

}

// Prepares the SQL statements for later use.
func prepareStatements(db *sql.DB) {
	var err error

	prefixSearchStmt, err = db.Prepare("SELECT filename, fullpath FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?")
	if err != nil {
		log.Fatal(err)
	}

	insertStmt, err = db.Prepare("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Fatal(err)
	}

	deleteStmt, err = db.Prepare("DELETE FROM files WHERE fullpath = ?")
	if err != nil {
		log.Fatal(err)
	}
}

// Performs a search for filenames starting with the given prefix and returns a limited number of results.
func PrefixSearch(prefix string, limit ...int) ([]string, error) {
	var results []string

	resultLimit := 200
	if len(limit) > 0 {
		resultLimit = limit[0]
	}

	rows, err := prefixSearchStmt.Query(prefix+"%", resultLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var filename, fullpath string
		err := rows.Scan(&filename, &fullpath)
		if err != nil {
			return nil, err
		}

		results = append(results, fullpath)
	}

	return results, nil
}

// Returns the count of records in the files table.
func RecordCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Deletes a record from the database with the given fullpath.
func DeleteRecord(db *sql.DB, fullpath string) error {
	_, err := db.Exec("DELETE FROM files WHERE fullpath = ?", fullpath)
	return err
}

// Inserts a new record into the database.
func InsertRecord(db *sql.DB, filename string, path string, eventID uint64, objectType models.ObjectType) error {
	_, err := db.Exec("INSERT OR IGNORE INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)", filename, path, eventID, objectType)
	return err
}

// Collects records and commits them to the database when enough records are collected.
func BulkInsertRecords(db *sql.DB, filename string, path string) error {

	// Collect records here
	records = append(records, struct {
		filename string
		path     string
	}{filename, path})

	// Commit records when we have enough
	if len(records) >= 1000 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		for _, record := range records {
			_, err = insertStmt.Exec(record.filename, record.path, nil, 0)
			if err != nil {
				return err
			}
		}

		err = tx.Commit()
		if err != nil {
			return err
		}

		records = records[:0] // Clear the slice
	}

	return nil
}

// Commits any collected records to the database.
func CommitRecords(db *sql.DB) error {

	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO files (filename, fullpath) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, record := range records {
		_, err = stmt.Exec(record.filename, record.path)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	records = records[:0] // Clear the slice
	return nil
}

// Stores a batch of events in the database.
func BulkStoreEvents(db *sql.DB, events *[]models.EventRecord) error {
	bulkTime := time.Now()
	//log.Printf("BulkStoreEvents() - &eventRecordQueue: %p", events)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	var num_committed int
	var num_missing int
	var num_duplicate int
	var num_deleted int

	for _, e := range *events {
		if e.EventAction == models.ItemCreated {
			if fileExists(e.Path) {
				num_committed++
				_, err = insertStmt.Exec(e.Filename, e.Path, e.EventID, e.ObjectType)
				if err != nil {
					if isDuplicate(err) {
						num_duplicate++
					} else {
						return err
					}
				}
			} else {
				num_missing++
			}
		} else if e.EventAction == models.ItemDeleted {
			_, err = deleteStmt.Exec(e.Path)
			if err != nil {
				return err
			}
			num_deleted++
		} else {
			log.Fatal("Unknown event action: ", e.EventAction)
		}
	}

	commitTime := time.Now()

	err = tx.Commit()
	if err != nil {
		return err
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf(
		"Events: %-6d Del: %-6d New: %-6d Missing: %-6d Dups: %-6d Time fn/commit: %-9s / %-8s Queue: %p Capacity: %-7d Mem: %.1f heap %.1f MB",
		len(*events),
		num_deleted,
		num_committed-num_duplicate,
		num_missing,
		num_duplicate,
		time.Since(bulkTime).Round(time.Microsecond).String(),
		time.Since(commitTime).Round(time.Microsecond).String(),
		events,
		cap(*events),
		bToMb(m.Sys),
		bToMb(m.Alloc),
	)

	return nil
}

// Converts bytes to megabytes.
func bToMb(b uint64) float64 {
	return float64(b) / 1024 / 1024
}

// Checks if the given error is a SQLite constraint violation error.
func isDuplicate(err error) bool {
	if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.Code == sqlite3.ErrConstraint {
		return true
	}
	return false
}

func fileExists(pathname string) bool {
	_, err := os.Stat(pathname)
	return !os.IsNotExist(err)
}
