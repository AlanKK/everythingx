package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// Get files from the database and check if they exist
// Print counts.
func main() {
	filename := "/Users/alan/Documents/everythingx/files.db"
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	fmt.Println("Checking ", filename)

	rows, err := db.Query("SELECT fullpath FROM files")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var totalFiles, filesExist, filesNotExist int

	for rows.Next() {
		var fullpath string
		if err := rows.Scan(&fullpath); err != nil {
			log.Fatal(err)
		}

		if fileExists(fullpath) {
			//fmt.Printf("File exists: %s\n", fullpath)
			filesExist++
		} else {
			fmt.Printf("File does not exist: %s\n", fullpath)
			filesNotExist++
		}
		totalFiles++
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nTotal files: %d\n", totalFiles)
	fmt.Printf("Files that exist: %d\n", filesExist)
	fmt.Printf("Files that do not exist: %d\n", filesNotExist)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
