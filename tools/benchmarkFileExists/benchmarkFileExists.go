package main

import (
	"database/sql"
	"errors"
	"math/rand"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Benchmark the two functions to check if a file exists.  Turns out they are pretty much the same.

// First function to test
func fileExists1(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// Second function to test
func fileExists2(filename string) bool {
	_, err := os.Stat(filename)
	return !errors.Is(err, os.ErrNotExist)
}

func main() {
	// Open the database
	db, err := sql.Open("sqlite3", "/var/lib/everythingx/files.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Query all file paths
	rows, err := db.Query("SELECT fullpath FROM files")
	if err != nil {
		if err == sql.ErrNoRows {
			println("No files found in the database.")
			return
		}
		panic(err)
	}
	defer rows.Close()

	var filepaths []string
	for rows.Next() {
		var filepath string
		if err := rows.Scan(&filepath); err != nil {
			panic(err)
		}
		filepaths = append(filepaths, filepath)
	}

	if err := rows.Err(); err != nil {
		panic(err)
	}

	// Shuffle the file paths
	rand.Shuffle(len(filepaths), func(i, j int) {
		filepaths[i], filepaths[j] = filepaths[j], filepaths[i]
	})

	// Split the file paths into two halves
	mid := len(filepaths) / 2
	filepaths2 := filepaths[:mid]
	filepaths1 := filepaths[mid:]

	// Track file existence
	var fileExists1Count, fileNotExists1Count, fileExists2Count, fileNotExists2Count int

	// Measure time for the first function
	start1 := time.Now()
	for _, filepath := range filepaths1 {
		if fileExists1(filepath) {
			fileExists1Count++
		} else {
			fileNotExists1Count++
		}
	}
	elapsed1 := time.Since(start1)
	println("os.IsNotExist() - Total Time:", elapsed1.String(), "Average Time:", elapsed1/time.Duration(len(filepaths1)))
	println("os.IsNotExist() - Exists:", fileExists1Count, "Not Exists:", fileNotExists1Count)

	// Measure time for the second function
	start2 := time.Now()
	for _, filepath := range filepaths2 {
		if fileExists2(filepath) {
			fileExists2Count++
		} else {
			fileNotExists2Count++
		}
	}
	elapsed2 := time.Since(start2)
	println("errors.Is() - Total Time:", elapsed2.String(), "Average Time:", elapsed2/time.Duration(len(filepaths2)))
	println("errors.Is() - Exists:", fileExists2Count, "Not Exists:", fileNotExists2Count)
}
