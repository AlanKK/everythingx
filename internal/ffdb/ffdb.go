package ffdb

import (
	"database/sql"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/AlanKK/everythingx/internal/shared"
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
var deleteDirStmt *sql.Stmt
var renameDirStmt *sql.Stmt
var fullPathLikeStmt *sql.Stmt

// Creates and opens a new database at the given pathname.
func CreateDB(pathname string) (*sql.DB, error) {
	if shared.FileExists(pathname) {
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
	if !shared.FileExists(pathname) {
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

// Open read-only for the UI.  Can't use PRAGMAS in read-only mode.
func OpenDBReadOnly(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if !shared.FileExists(pathname) {
		return nil, error(os.ErrNotExist)
	}

	db, err := sql.Open("sqlite3", "file:"+pathname+"?mode=ro")
	if err != nil {
		return nil, err
	}

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
}

// Configures the database with necessary settings.
func configureDB(db *sql.DB) {

	// For the UI to use case sensitive search.
	// _, err := db.Exec("PRAGMA case_sensitive_like = ON")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Enable WAL mode - multiple readers and writer
	_, err := db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Fatal(err)
	}

	// No performane impact for this
	_, err = db.Exec("PRAGMA synchronous=NORMAL;")
	if err != nil {
		log.Fatal(err)
	}
}

// Prepares the SQL statements for later use.
func prepareStatements(db *sql.DB) {
	var err error

	prefixSearchStmt, err = db.Prepare("SELECT fullpath, object_type FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?")
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

	deleteDirStmt, err = db.Prepare("DELETE FROM files WHERE fullpath = ? OR (fullpath > ? AND fullpath < ?)")
	if err != nil {
		log.Fatal(err)
	}

	renameDirStmt, err = db.Prepare("UPDATE files SET fullpath = ? || SUBSTR(fullpath, LENGTH(?) + 1), filename = CASE WHEN fullpath = ? THEN ? ELSE filename END WHERE fullpath = ? OR (fullpath > ? AND fullpath < ?)")
	if err != nil {
		log.Fatal(err)
	}

	fullPathLikeStmt, err = db.Prepare("SELECT fullpath FROM files where fullpath like ?")
	if err != nil {
		log.Fatal(err)
	}
}

// Performs a search for filenames starting with the given prefix and returns a limited number of results.
func PrefixSearch(prefix string, limit int) ([]*shared.SearchResult, error) {
	var results []*shared.SearchResult

	rows, err := prefixSearchStmt.Query("%"+prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		result := &shared.SearchResult{
			Fullpath:   "",
			ObjectType: 0,
		}
		err := rows.Scan(&result.Fullpath, &result.ObjectType)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
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
func InsertRecord(db *sql.DB, filename string, path string, eventID uint64, objectType shared.ObjectType) error {
	_, err := db.Exec("INSERT OR IGNORE INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)", filename, path, eventID, objectType)
	return err
}

func FullPathLikeQuery(path string) (*[]string, error) {
	var results []string

	rows, err := fullPathLikeStmt.Query(path + "%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var result string
		err = rows.Scan(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return &results, nil
}

// FullPathLikeQueryEach streams matching paths one at a time via the callback,
// avoiding materializing all results into a slice.
func FullPathLikeQueryEach(path string, fn func(string)) error {
	rows, err := fullPathLikeStmt.Query(path + "%")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		fn(result)
	}
	return rows.Err()
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
func BulkStoreEvents(db *sql.DB, eventRecordQueue *[]shared.EventRecord) error {
	if len(*eventRecordQueue) == 0 {
		log.Println("No events to store")
		return nil
	}
	bulkTime := time.Now()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	var num_committed, num_missing, num_duplicate, num_deleted, num_renamed int

	// Pre-process: detect directory rename pairs.
	// FSEvents/fanotify fire two events for a rename: one for the old path
	// (no longer exists) and one for the new path (now exists).
	handled := make(map[int]bool)
	for i, e := range *eventRecordQueue {
		if !e.IsRename || e.ObjectType != shared.ItemIsDir || e.FoundOnScan || shared.FileExists(e.Path) {
			continue
		}
		// Old path of a dir rename. Find the new-path partner.
		for j := i + 1; j < len(*eventRecordQueue); j++ {
			ej := (*eventRecordQueue)[j]
			if ej.IsRename && ej.ObjectType == shared.ItemIsDir && !handled[j] && shared.FileExists(ej.Path) {
				_, err = renameDirStmt.Exec(ej.Path, e.Path, e.Path, ej.Filename, e.Path, e.Path+"/", e.Path+"0")
				if err != nil {
					return err
				}
				handled[i] = true
				handled[j] = true
				num_renamed++
				break
			}
		}
	}

	for i, e := range *eventRecordQueue {
		if handled[i] {
			continue
		}
		if e.FoundOnScan || shared.FileExists(e.Path) {
			_, err = insertStmt.Exec(e.Filename, e.Path, e.EventID, e.ObjectType)
			if err != nil {
				if isDuplicate(err) {
					num_duplicate++
				} else {
					return err
				}
			}
			num_committed++
		} else {
			if e.ObjectType == shared.ItemIsDir {
				if e.IsRename {
					// Unpaired rename — just remove the dir entry, don't cascade.
					_, err = deleteStmt.Exec(e.Path)
				} else {
					_, err = deleteDirStmt.Exec(e.Path, e.Path+"/", e.Path+"0")
				}
			} else {
				_, err = deleteStmt.Exec(e.Path)
			}
			if err != nil {
				return err
			}
			num_deleted++
		}
	}

	commitTime := time.Now()

	err = tx.Commit()
	if err != nil {
		return err
	}

	commitTimeEnd := time.Since(commitTime).Round(time.Microsecond).String()
	bulkTimeEnd := time.Since(bulkTime).Round(time.Microsecond).String()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf(
		"Events: %-6d Del: %-6d New: %-6d Ren: %-6d Missing: %-6d Dups: %-6d Time fn/commit: %-9s / %-8s Queue: %p Capacity: %-7d Mem: %.1f heap %.1f MB",
		len(*eventRecordQueue),
		num_deleted,
		num_committed-num_duplicate,
		num_renamed,
		num_missing,
		num_duplicate,
		bulkTimeEnd,
		commitTimeEnd,
		eventRecordQueue,
		cap(*eventRecordQueue),
		float64(shared.BToMb(m.Sys)),
		float64(shared.BToMb(m.Alloc)),
	)

	return nil
}

// Checks if the given error is a SQLite constraint violation error.
func isDuplicate(err error) bool {
	if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.Code == sqlite3.ErrConstraint {
		return true
	}
	return false
}
