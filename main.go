package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func main() {
	var err error
	var db *sql.DB = nil
	dbFile := "data/files.db"

	if !fileExists(dbFile) {
		fmt.Println("Database does not exist, creating it...")
		err = createDBAndTable("data/files.db")
		if err != nil {
			return
		}
		fmt.Println("Loading database from file...")
		db, err = initializeDB(dbFile)
		if err != nil {
			return
		}
		err = loadDBFromFile(db, "data/all-files.txt")
		if err != nil {
			return
		}
	}

	db, err = initializeDB(dbFile)
	if err != nil {
		return
	}
	fmt.Println("Opened database ", dbFile)
	defer db.Close()

	loadUI()
}

func loadDBFromFile(db *sql.DB, filename string) error {
	var records int = 0

	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "/" {
			continue
		}
		base := filepath.Base(line)
		err = bulkInsertRecords(db, base, line)
		if err != nil {
			fmt.Println("Error inserting record:", err)
			return err
		}

		records++
	}

	err = commitRecords(db)
	if err != nil {
		fmt.Println("Error committing records:", err)
		return err
	}

	fmt.Println("Loaded", records, "records from %s.", filename)
	return nil
}
