package main

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

const (
	sourceDBPath     = "./files.db.gz"
	targetDBPath     = "benchmark.db"
	numSearches      = 1000
	maxSearchResults = 5000
)

var searchPrefixes = []string{"a", "al", "ala", "alan"}

func main() {
	var targetDB *sql.DB
	var err error

	noDB := false
	if len(os.Args) > 1 && os.Args[1] == "--no-db" {
		noDB = true
	}

	if !noDB {
		os.Remove(targetDBPath)

		// Unzip the source database file
		unzippedSourceDBPath := "/tmp/unzipped_files.db"
		err := ungzipFile(sourceDBPath, unzippedSourceDBPath)
		if err != nil {
			log.Fatalf("Failed to unzip source database: %v", err)
		}

		// Open the unzipped source database
		sourceDB, err := ffdb.OpenDB(unzippedSourceDBPath)
		if err != nil {
			log.Fatalf("Failed to open source database: %v", err)
		}
		defer sourceDB.Close()

		// Create the target database
		targetDB, err := ffdb.CreateDB(targetDBPath)
		if err != nil {
			if err == os.ErrExist {
				targetDB, err = ffdb.OpenDB(targetDBPath)
				if err != nil {
					log.Fatalf("Failed to open existing target database: %v", err)
				}
			} else {
				log.Fatalf("Failed to create target database: %v", err)
			}
		}
		// Copy data from the source database to the target database
		copyData(sourceDB, targetDB)

		sourceDB.Close()
	} else {
		targetDB, err = ffdb.OpenDB(targetDBPath)
		if err != nil {
			log.Fatalf("Failed to open target database: %v", err)
		}
	}
	defer targetDB.Close()

	benchmarkPrefixSearch()
}

func ungzipFile(sourcePath, targetPath string) error {
	// Open the gzip file
	gzipFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open gzip file: %w", err)
	}
	defer gzipFile.Close()

	// Create a gzip reader
	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create the target file
	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer targetFile.Close()

	// Copy the data from the gzip reader to the target file
	_, err = io.Copy(targetFile, gzipReader)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

func copyData(sourceDB, targetDB *sql.DB) {
	fmt.Println("Copying data from source database to target database...")

	rows, err := sourceDB.Query("SELECT filename, fullpath, event_id, object_type FROM files")
	if err != nil {
		log.Fatalf("Failed to query source database: %v", err)
	}
	defer rows.Close()

	tx, err := targetDB.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}

	stmt, err := tx.Prepare("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Failed to prepare insert statement: %v", err)
	}
	defer stmt.Close()

	rowCount := 0
	for rows.Next() {
		var filename, fullpath string
		var eventID int
		var objectType models.ObjectType

		err := rows.Scan(&filename, &fullpath, &eventID, &objectType)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}

		_, err = stmt.Exec(filename, fullpath, eventID, objectType)
		if err != nil {
			log.Fatalf("Failed to insert row into target database: %v", err)
		}
		rowCount++
	}

	fmt.Printf("Imported %d rows.\n", rowCount)

	err = tx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	fmt.Println("Data copied successfully.")
}

func benchmarkPrefixSearch() {
	totalStart := time.Now()
	totalSearches := 0

	// Open the log file
	logFile, err := os.OpenFile("benchmark_results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	var times = make(map[string][]time.Duration)
	for _, prefix := range searchPrefixes {
		times[prefix] = []time.Duration{}
	}

	for i := 0; i < numSearches; i++ {
		if i%10 == 0 {
			fmt.Print(i, " ")
		}
		for _, prefix := range searchPrefixes {
			searchStart := time.Now()
			results, err := ffdb.PrefixSearch(prefix, maxSearchResults)
			if err != nil {
				log.Fatalf("Prefix search failed for prefix '%s': %v", prefix, err)
			}
			if len(results) == 0 {
				log.Printf("No results found for prefix '%s'", prefix)
			}
			times[prefix] = append(times[prefix], time.Since(searchStart))
		}
		totalSearches += len(searchPrefixes)
	}
	fmt.Println()

	for _, prefix := range searchPrefixes {
		// Calculate statistics
		var total, min, max time.Duration
		min = times[prefix][0]
		for _, t := range times[prefix] {
			total += t
			if t < min {
				min = t
			}
			if t > max {
				max = t
			}
		}
		average := total / time.Duration(numSearches)
		var sumSquares time.Duration
		for _, t := range times[prefix] {
			diff := t - average
			sumSquares += diff * diff
		}
		stdDev := time.Duration(math.Sqrt(float64(sumSquares / time.Duration(numSearches))))

		// Log results in CSV format
		logger.Printf(",%s,%d,%d,%d,%d\n", prefix, min.Milliseconds(), max.Milliseconds(), average.Milliseconds(), stdDev.Milliseconds())
		fmt.Printf("%s,%d,%d,%d,%d\n", prefix, min.Milliseconds(), max.Milliseconds(), average.Milliseconds(), stdDev.Milliseconds())
	}

	totalElapsed := time.Since(totalStart)
	fmt.Printf("Completed %d total searches in %fs\n", totalSearches, totalElapsed.Seconds())
	fmt.Printf("Overall average time per search: %dms\n", totalElapsed.Milliseconds()/int64(totalSearches))
}
