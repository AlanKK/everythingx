package ffdb

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// TODO:  check out driver benchmarks for sqlite3 at https://github.com/cvilsmeier/go-sqlite-bench

var records []struct {
	filename string
	path     string
}

var prefixSearchStmt *sql.Stmt
var countStmt *sql.Stmt

func preparePrefixSearchStmt(db *sql.DB) (*sql.Stmt, error) {
	// Prepare the statement for prefix search - performance optimization
	stmt, err := db.Prepare("SELECT filename, fullpath FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?")
	if err != nil {
		return nil, err
	}

	return stmt, err
}

func prepareCountStmt(db *sql.DB) (*sql.Stmt, error) {
	// Prepare the statement for prefix search - performance optimization
	stmt, err := db.Prepare("SELECT count(*) FROM files WHERE filename LIKE ?")
	if err != nil {
		return nil, err
	}

	return stmt, err
}

func InitializeDB(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if _, err := os.Stat(pathname); os.IsNotExist(err) {
		log.Fatal("DB file missing: ", pathname, err)
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		log.Fatal(err)
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

	prefixSearchStmt, err = preparePrefixSearchStmt(db)
	if err != nil {
		log.Fatal(err)
	}

	countStmt, err = prepareCountStmt(db)
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

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS files (filename TEXT, fullpath TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE INDEX idx_filename ON files(filename COLLATE BINARY)")
	if err != nil {
		log.Fatal(err)
	}

	return err
}

func PrefixSearch(prefix string, limit ...int) ([]string, int, error) {
	var results []string

	resultLimit := 200
	if len(limit) > 0 {
		resultLimit = limit[0]
	}

	rows, err := prefixSearchStmt.Query(prefix+"%", resultLimit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var filename, fullpath string
		err := rows.Scan(&filename, &fullpath)
		if err != nil {
			return nil, 0, err
		}

		results = append(results, fullpath)
	}

	// Get the total number of rows

	// TODO: this is super slow.  Need a paginated solution for results and count results instead

	// rows, err = CountStmt.Query("%" + prefix + "%")
	// if err != nil {
	// 	return nil, 0, err
	// }
	// defer rows.Close()

	// var count int
	// if rows.Next() {
	// 	err := rows.Scan(&count)
	// 	if err != nil {
	// 		return nil, 0, err
	// 	}
	// 	return results, count, nil
	// }
	// defer rows.Close()
	// fmt.Println("Total rows: ", count)
	count := 0

	return results, count, nil
}

func DeleteRecord(db *sql.DB, key string) error {
	_, err := db.Exec("DELETE FROM files WHERE filename = ?", key)
	return err
}

func InsertRecord(db *sql.DB, filename string, path string) error {
	_, err := db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", filename, path)
	return err
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
