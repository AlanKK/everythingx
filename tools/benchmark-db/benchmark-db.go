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
	searchPrefixes = []string{"a", "al", "ala", "alan", "alank", "alanke", "alankeister"}
	numSearches    int
	noDB           bool
	baseline       bool
	baselineStmt   *sql.Stmt
)

func main() {
	flag.IntVar(&numSearches, "n", 100, "number of searches to perform")
	flag.BoolVar(&noDB, "no-db", false, "skip database setup")
	flag.BoolVar(&baseline, "baseline", false, "use old LIKE %%term%% query instead of FTS5")
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

	if baseline {
		var err error
		baselineStmt, err = targetDB.Prepare("SELECT fullpath, object_type FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?")
		if err != nil {
			log.Fatalf("Failed to prepare baseline statement: %v", err)
		}
		defer baselineStmt.Close()
		fmt.Println("Mode: BASELINE (LIKE '%term%')")
	} else {
		fmt.Println("Mode: FTS5 trigram (>= 3 chars) + prefix LIKE (< 3 chars)")
	}

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

	// Rebuild FTS5 index from the bulk-loaded data
	fmt.Println("Rebuilding FTS5 trigram index...")
	_, err = targetDB.Exec("INSERT INTO files_fts(files_fts) VALUES('rebuild')")
	if err != nil {
		log.Fatalf("Failed to rebuild FTS index: %v", err)
	}
	fmt.Println("FTS5 index rebuilt.")
}

func benchmarkPrefixSearch() {
	totalStart := time.Now()

	logFile, err := os.OpenFile("benchmark_results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	// Collect times per prefix
	perPrefix := make(map[string][]time.Duration)
	for _, p := range searchPrefixes {
		perPrefix[p] = make([]time.Duration, 0, numSearches)
	}

	for i := 0; i < numSearches; i++ {
		if i%10 == 0 {
			fmt.Print(i, " ")
		}
		for _, prefix := range searchPrefixes {
			searchStart := time.Now()
			var results []*shared.SearchResult
			var err error
			if baseline {
				results, err = baselineLikeSearch(prefix, maxSearchResults)
			} else {
				results, err = ffdb.PrefixSearch(prefix, maxSearchResults)
			}
			if err != nil {
				log.Fatalf("Search failed for prefix '%s': %v", prefix, err)
			}
			duration := time.Since(searchStart)
			perPrefix[prefix] = append(perPrefix[prefix], duration)
			_ = results
		}
	}
	fmt.Println()

	// Print per-prefix statistics
	fmt.Printf("\n%-15s %8s %8s %8s %8s %8s %8s\n", "Prefix", "Results", "Min", "Median", "Avg", "Max", "StdDev")
	fmt.Println("----------------------------------------------------------------------")

	for _, prefix := range searchPrefixes {
		times := perPrefix[prefix]
		if len(times) == 0 {
			continue
		}

		// Get result count for this prefix
		var results []*shared.SearchResult
		if baseline {
			results, _ = baselineLikeSearch(prefix, maxSearchResults)
		} else {
			results, _ = ffdb.PrefixSearch(prefix, maxSearchResults)
		}

		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

		min := times[0]
		max := times[len(times)-1]
		median := times[len(times)/2]

		var total time.Duration
		for _, t := range times {
			total += t
		}
		avg := total / time.Duration(len(times))

		var sumSq float64
		for _, t := range times {
			diff := float64(t - avg)
			sumSq += diff * diff
		}
		stdDev := time.Duration(math.Sqrt(sumSq / float64(len(times))))

		fmt.Printf("%-15s %8d %7.1fms %7.1fms %7.1fms %7.1fms %7.1fms\n",
			"\""+prefix+"\"", len(results),
			float64(min.Microseconds())/1000,
			float64(median.Microseconds())/1000,
			float64(avg.Microseconds())/1000,
			float64(max.Microseconds())/1000,
			float64(stdDev.Microseconds())/1000)

		logger.Printf(",%s,%d,%d,%d,%d,%d,%d iterations\n", prefix, min.Microseconds(), max.Microseconds(), avg.Microseconds(), median.Microseconds(), stdDev.Microseconds(), numSearches)
	}

	totalElapsed := time.Since(totalStart)
	fmt.Printf("\nTotal time: %.1fs (%d searches x %d prefixes = %d queries)\n",
		totalElapsed.Seconds(), numSearches, len(searchPrefixes), numSearches*len(searchPrefixes))
}

func baselineLikeSearch(prefix string, limit int) ([]*shared.SearchResult, error) {
	rows, err := baselineStmt.Query("%"+prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*shared.SearchResult
	for rows.Next() {
		result := &shared.SearchResult{}
		if err := rows.Scan(&result.Fullpath, &result.ObjectType); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}
