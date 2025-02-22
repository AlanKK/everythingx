package main

import (
	"compress/gzip"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	_ "github.com/mattn/go-sqlite3"
)

const (
	sourceDBPath     = "./files.db.gz"
	targetDBPath     = "benchmark.db"
	maxSearchResults = 1000
)

var (
	searchPrefixes = []string{"a", "al", "ala", "alan"}
	numSearches    int
	noDB           bool
)

func main() {
	flag.IntVar(&numSearches, "n", 100, "number of searches to perform")
	flag.BoolVar(&noDB, "no-db", false, "skip database setup")
	flag.Parse()

	var sourceDB, targetDB *sql.DB
	var err error

	if !noDB {
		os.Remove(targetDBPath)

		// Unzip the source database file
		unzippedSourceDBPath := "/tmp/unzipped_files.db"
		err = shared.GUnZipFile(sourceDBPath, unzippedSourceDBPath)
		if err != nil {
			log.Fatalf("Failed to unzip source database: %v", err)
		}

		// Open the unzipped source database
		sourceDB, err = ffdb.OpenDB(unzippedSourceDBPath)
		if err != nil {
			log.Fatalf("Failed to open source database: %v", err)
		}
		defer sourceDB.Close()

		// Create the target database
		targetDB, err = ffdb.CreateDB(targetDBPath)
		if err != nil {
			log.Fatalf("Failed to create target database: %v", err)
		}

		// Copy data from source database to target database
		copyData(sourceDB, targetDB)
	} else {
		if _, err := os.Stat(targetDBPath); os.IsNotExist(err) {
			log.Fatalf("Target database does not exist: %v", err)
		}
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

	targetTx, err := targetDB.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}

	insertStmt, err := targetTx.Prepare("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Failed to prepare insert statement: %v", err)
	}
	defer insertStmt.Close()

	rowCount := 0
	for rows.Next() {
		var filename, fullpath string
		var eventID int
		var objectType shared.ObjectType

		err := rows.Scan(&filename, &fullpath, &eventID, &objectType)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}

		_, err = insertStmt.Exec(filename, fullpath, eventID, objectType)
		if err != nil {
			log.Fatalf("Failed to insert row into target database: %v", err)
		}
		rowCount++
	}

	fmt.Printf("Imported %d rows.\n", rowCount)

	err = targetTx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	fmt.Println("Data copied successfully.")
}

func benchmarkPrefixSearch() {
	totalStart := time.Now()
	totalSearches := 0

	logFile, err := os.OpenFile("benchmark_results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	var allTimes []time.Duration

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
				continue
			}
			duration := time.Since(searchStart)
			allTimes = append(allTimes, duration)
		}
		totalSearches += len(searchPrefixes)
	}
	fmt.Println()

	// Calculate overall statistics
	var total, min, max, average, stdDev time.Duration
	min = time.Duration(math.MaxInt64)
	max = time.Duration(math.MinInt64)

	for _, t := range allTimes {
		total += t
		if t < min {
			min = t
		}
		if t > max {
			max = t
		}
	}
	average = total / time.Duration(totalSearches)
	var sumSquares time.Duration
	for _, t := range allTimes {
		diff := t - average
		sumSquares += diff * diff
	}
	stdDev = time.Duration(math.Sqrt(float64(sumSquares) / float64(totalSearches)))

	// Calculate overall median
	sort.Slice(allTimes, func(i, j int) bool { return allTimes[i] < allTimes[j] })
	median := allTimes[totalSearches/2]
	if totalSearches%2 == 0 {
		median = (allTimes[totalSearches/2-1] + allTimes[totalSearches/2]) / 2
	}

	// Log overall results in CSV format
	// Log overall results in CSV format: min, max, average, median, standard deviation, total searches
	logger.Printf(",Overall,%d,%d,%d,%d,%d,%d iterations\n", min.Milliseconds(), max.Milliseconds(), average.Milliseconds(), median.Milliseconds(), stdDev.Milliseconds(), totalSearches)

	totalElapsed := time.Since(totalStart)
	//fmt.Printf("Completed %d total searches in %fs\n", totalSearches, totalElapsed.Seconds())
	fmt.Printf("Median: %dms, Standard deviation: %dms, Average: %dms\n", median.Milliseconds(), stdDev.Milliseconds(), totalElapsed.Milliseconds()/int64(totalSearches))
}
