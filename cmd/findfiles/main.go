package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlanKK/findfiles/internal/ffdb"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

var dataFolder = "../../data"

func main() {
	dbFile := dataFolder + "/files.db"

	if !fileExists(dbFile) {
		fmt.Println("Database does not exist. Creating it.")
		db, err := ffdb.CreateDB(dbFile)
		if err != nil {
			return
		}
		defer db.Close()

		err = loadDBFromFile(db, dataFolder+"/all-files.txt")
		if err != nil {
			return
		}
	}

	fmt.Println("Opened database ", dbFile)

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
		err = ffdb.BulkInsertRecords(db, base, line)
		if err != nil {
			fmt.Println("Error inserting record:", err)
			return err
		}

		records++
	}

	err = ffdb.CommitRecords(db)
	if err != nil {
		fmt.Println("Error committing records:", err)
		return err
	}

	fmt.Println("Loaded", records, "records from %s.", filename)
	return nil
}
