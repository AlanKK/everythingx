package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var records []struct {
	filename string
	path     string
}

var PrefixSearchStmt *sql.Stmt

func preparePrefixSearchStmt(db *sql.DB) (*sql.Stmt, error) {
	// Prepare the statement for prefix search - performance optimization
	stmt, err := db.Prepare("SELECT filename, fullpath FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?")
	if err != nil {
		return nil, err
	}

	return stmt, err
}

func openDB(pathname string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("PRAGMA case_sensitive_like = ON")
	if err != nil {
		log.Fatal(err)
	}

	return db, err
}

func createDBAndTable(pathname string) (*sql.DB, error) {
	db, err := openDB(pathname)
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

	return db, err
}

func prefixSearch(db *sql.DB, prefix string, limit ...int) ([]string, error) {
	var results []string

	resultLimit := 20
	if len(limit) > 0 {
		resultLimit = limit[0]
	}

	if PrefixSearchStmt == nil {
		var err error
		PrefixSearchStmt, err = preparePrefixSearchStmt(db)
		if err != nil {
			log.Fatal(err)
		}
	}

	rows, err := PrefixSearchStmt.Query(prefix+"%", resultLimit)
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
		// fmt.Printf("Filename: %s, Fullpath: %s\n", filename, fullpath)

		results = append(results, fullpath)
	}

	return results, nil
}

func deleteRecord(db *sql.DB, key string) error {
	_, err := db.Exec("DELETE FROM files WHERE filename = ?", key)
	return err
}

func insertRecord(db *sql.DB, filename string, path string) error {
	_, err := db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", filename, path)
	return err
}

func bulkInsertRecords(db *sql.DB, filename string, path string) error {
	records = append(records, struct {
		filename string
		path     string
	}{filename, path})

	if len(records) >= 10000 {
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
				tx.Rollback()
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

func commitRecords(db *sql.DB) error {
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
			tx.Rollback()
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
