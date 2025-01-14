package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlanKK/findfiles/internal/ffdb"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func main() {
	dbFile := getCommandLineArgs()

	if !fileExists(dbFile) {
		fmt.Println("Database does not exist ", dbFile)
		os.Exit(1)
	}

	fmt.Println("Opening database ", dbFile)
	_, err := ffdb.OpenDB(dbFile)
	if err != nil {
		fmt.Println("Error opening database: ", dbFile, err)
		os.Exit(1)
	}

	loadUI()
}

func getCommandLineArgs() string {
	dbFile := flag.String("db_path", "/var/lib/findfiles/files.db", "Path to the database file")
	flag.Parse()

	if *dbFile == "" {
		fmt.Println("Database path is required")
		os.Exit(1)
	}
	return *dbFile
}

// func loadDBFromFile(db *sql.DB, filename string) error {
// 	var records int = 0

// 	file, err := os.Open(filename)
// 	if err != nil {
// 		fmt.Println("Error opening file:", err)
// 		return err
// 	}
// 	defer file.Close()

// 	scanner := bufio.NewScanner(file)
// 	for scanner.Scan() {
// 		line := strings.TrimSpace(scanner.Text())
// 		if line == "" || line == "/" {
// 			continue
// 		}
// 		base := filepath.Base(line)
// 		err = ffdb.BulkInsertRecords(db, base, line)
// 		if err != nil {
// 			fmt.Println("Error inserting record:", err)
// 			return err
// 		}

// 		records++
// 	}

// 	err = ffdb.CommitRecords(db)
// 	if err != nil {
// 		fmt.Println("Error committing records:", err)
// 		return err
// 	}

// 	fmt.Println("Loaded", records, "records from %s.", filename)
// 	return nil
// }
