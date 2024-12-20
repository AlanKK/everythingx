package ffdb

import (
	"database/sql"
	"log"
	"os"

	"github.com/AlanKK/findfiles/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

// TODO:  check out driver benchmarks for sqlite3 at https://github.com/cvilsmeier/go-sqlite-bench

var records []struct {
	filename string
	path     string
}

var prefixSearchStmt *sql.Stmt
var insertStmt *sql.Stmt
var deleteStmt *sql.Stmt

func CreateAndOpenNewDatabase(pathname string) (*sql.DB, error) {
	os.Remove(pathname)

	err := CreateDBAndTable(pathname)
	if err != nil {
		log.Fatalf("Expected no error, got %v", err)
	}

	if !fileExists(pathname) {
		log.Fatalf("Expected database file to be created, but it does not exist")
	}

	db, err := OpenDB(pathname)
	if err != nil {
		log.Fatalf("Expected no error, got %v", err)
	}
	return db, err
}

func OpenDB(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if _, err := os.Stat(pathname); os.IsNotExist(err) {
		return nil, err
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA case_sensitive_like = ON")
	if err != nil {
		log.Fatal(err)
	}
	// Enable WAL mode - multiple readers and writer
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Fatal(err)
	}

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

	return db, err
}

func CreateDBAndTable(pathname string) error {
	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS files(filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER)")
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

	return err
}

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

func DeleteRecord(db *sql.DB, fullpath string) error {
	_, err := db.Exec("DELETE FROM files WHERE fullpath = ?", fullpath)
	return err
}

func InsertRecord(db *sql.DB, filename string, path string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO files (filename, fullpath) VALUES (?, ?)", filename, path)
	return err
}

func fileExists(pathname string) bool {
	_, err := os.Stat(pathname)
	return !os.IsNotExist(err)
}

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

func BulkStoreEvents(db *sql.DB, events *[]models.EventRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	num_committed := 0
	num_missing := 0
	for _, e := range *events {
		if e.EventAction == models.ItemCreated {
			if fileExists(e.Path) {
				num_committed++
				log.Println("Creating ", e.Path)
				_, err = insertStmt.Exec(e.Filename, e.Path, e.EventID, e.ObjectType)
				if err != nil {
					return err
				}
			} else {
				num_missing++
			}
		} else {
			log.Println("Deleting ", e.Path)
			_, err = deleteStmt.Exec(e.Path)
			if err != nil {
				return err
			}
		}

	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf(
		"Committed %d events, %d missing files, %d total events. ----------------------------",
		num_committed,
		num_missing,
		len(*events),
	)

	return nil
}
