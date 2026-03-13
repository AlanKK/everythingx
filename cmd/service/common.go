package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"
)

var dbChannel chan *shared.EventRecord
var fullPathLikeQueryEach = ffdb.FullPathLikeQueryEach
var fileExists = shared.FileExists

var verbose bool

type Config struct {
	MonitorPath string
	DBPath      string
	NoCache     bool
	Verbose     bool
}

var config Config

func getCommandLineArgs() Config {
	var showVersion bool
	config = Config{}
	flag.StringVar(&config.MonitorPath, "monitor_path", "/", "Path to monitor for file system events")
	flag.StringVar(&config.DBPath, "db_path", "/var/lib/everythingx/files.db", "Path to the database file")
	flag.BoolVar(&config.NoCache, "nocache", false, "Disable caching")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&showVersion, "version", false, "Display version information and exit")
	flag.Parse()

	if config.MonitorPath == "" {
		log.Fatal("Monitor path is required")
	}
	if config.DBPath == "" {
		log.Fatal("Database path is required")
	}
	if config.NoCache {
		log.Println("--nocache is enabled.")
	}
	if config.Verbose {
		log.Println("--verbose logging is enabled.")
		verbose = true
	}
	if showVersion {
		fmt.Println("everythingxd service", version.Info())
		os.Exit(0)
	}

	return config
}

func setupDatabase(databasePath string) (*sql.DB, bool) {
	var err error
	var db *sql.DB
	var new = false

	if shared.FileExists(databasePath) {
		db, err = ffdb.OpenDB(databasePath)
		if err != nil {
			log.Println("Database does not exist: ", databasePath)
		}
	} else {
		log.Println("Database does not exist. Creating ", databasePath)
		if err = os.MkdirAll(filepath.Dir(databasePath), 0755); err != nil {
			log.Fatal("Error creating database directory: ", err)
		}
		db, err = ffdb.CreateDB(databasePath)
		if err != nil {
			log.Fatal("Error creating database: ", err)
		}
		new = true
	}

	count, err := ffdb.RecordCount(db)
	if err != nil {
		log.Fatal("Error getting record count: ", err)
	}
	log.Printf("Opened database %s. %d records", databasePath, count)

	return db, new
}

func scanHomeDirs() {
	startTime := time.Now()

	homeDir := shared.GetHomeDirPath()
	log.Printf("Scanning %s", homeDir)

	scanDisk(homeDir)
	deleteMissing(homeDir)

	elapsedTime := time.Since(startTime)
	log.Printf("Scan of %s completed in %s", homeDir, elapsedTime)
}

// Create a delay before writing to the db. FSEvents / fanotify may send
// create events for files that are quickly deleted or don't exist yet.
func addEventToQueue(db *sql.DB, lastFlushTime *time.Time, eventRecordQueue *[]shared.EventRecord, event *shared.EventRecord, noCache bool) {
	maxQueueSize := 100000 // Set a queue size limit to keep memory usage in check
	if noCache {
		maxQueueSize = 1
	}
	var delayTime time.Duration = 10 * time.Second

	*eventRecordQueue = append(*eventRecordQueue, *event)

	tt := time.Since(*lastFlushTime)
	if tt >= delayTime || len(*eventRecordQueue) >= maxQueueSize {
		err := ffdb.BulkStoreEvents(db, eventRecordQueue)
		if err != nil {
			log.Println("Error writing to db: ", err)
		}
		*eventRecordQueue = (*eventRecordQueue)[:0] // clear the queue
		// Release the backing array if it grew large, so the GC can
		// reclaim strings referenced by previously queued records.
		if cap(*eventRecordQueue) > 10000 {
			*eventRecordQueue = make([]shared.EventRecord, 0, 1000)
		}
		*lastFlushTime = time.Now()
	}
}

// Scan the database for file paths and check if they still exist on the filesystem.
// If a file is missing, create an EventRecord with a delete action.
func deleteMissing(root string) {
	startTime := time.Now()

	var totalFiles, filesExist, filesMissing int

	err := fullPathLikeQueryEach(root, func(fullpath string) {
		if !fileExists(fullpath) {
			eventRecord := &shared.EventRecord{
				Filename:   "",
				Path:       fullpath,
				ObjectType: 0,
				EventID:    0,
				EventTime:  0,
			}

			dbChannel <- eventRecord
			filesMissing++
		} else {
			filesExist++
		}
		totalFiles++
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("DB cleanup complete. Total files %d, found %d, missing %d, elapsed time %s", totalFiles, filesExist, filesMissing, time.Since(startTime).String())
}

func scanDisk(path string) {
	startTime := time.Now()

	var fileCount, dirCount, linkCount, pipeCount, socketCount, charDeviceCount, blockDeviceCount int
	var objType shared.ObjectType = shared.ItemIsFile

	filepath.WalkDir(path, func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if shouldIgnorePath(path) {
			if file.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case file.Type().IsRegular():
			fileCount++
			objType = shared.ItemIsFile
		case file.IsDir():
			dirCount++
			objType = shared.ItemIsDir
		case file.Type()&fs.ModeSymlink != 0:
			linkCount++
			objType = shared.ItemIsSymlink
		case file.Type()&fs.ModeNamedPipe != 0:
			pipeCount++
			objType = shared.ItemIsNamedPipe
		case file.Type()&fs.ModeSocket != 0:
			socketCount++
			objType = shared.ItemIsSocket
		case file.Type()&fs.ModeCharDevice != 0:
			charDeviceCount++
			objType = shared.ItemIsCharDevice
		case file.Type()&fs.ModeDevice != 0:
			blockDeviceCount++
			objType = shared.ItemIsBlockDevice
		default:
			log.Printf("Unknown file type: %s", path)
		}

		eventRecord := &shared.EventRecord{
			Filename:    file.Name(),
			Path:        path,
			ObjectType:  objType,
			EventID:     0,
			EventTime:   0,
			FoundOnScan: true,
		}

		dbChannel <- eventRecord

		return nil
	})

	log.Printf("Scan complete. Files/dirs: %d/%d. Pipes: %d, Sockets: %d, Char Devices: %d, Block Devices: %d. Elapsed time: %s", fileCount, dirCount, pipeCount, socketCount, charDeviceCount, blockDeviceCount, time.Since(startTime).String())
}

func databaseWriter(db *sql.DB, noCache bool) {
	log.Println("databaseWriter thread started.")

	var lastFlushTime time.Time = time.Now()
	var eventRecordQueue = []shared.EventRecord{}
	log.Printf("Queue %p", &eventRecordQueue)

	for event := range dbChannel {
		addEventToQueue(db, &lastFlushTime, &eventRecordQueue, event, noCache)
	}
}

// shouldIgnorePath is declared in each platform-specific file (main_darwin.go, main_linux.go).
// It returns true if the given path should be skipped during scanning and event processing.
